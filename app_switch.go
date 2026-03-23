package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// 无感切号
// ═══════════════════════════════════════

func (a *App) SwitchAccount(id string) error {
	acc, err := a.store.GetAccount(id)
	if err != nil {
		return err
	}
	prepared, err := a.prepareAccountForUsage(acc)
	if err != nil {
		return err
	}
	if err := a.switchSvc.SwitchAccount(prepared.Token, prepared.Email); err != nil {
		return err
	}
	// ★ 同步 MITM 代理当前号
	if prepared.WindsurfAPIKey != "" {
		a.mitmProxy.SwitchToKey(prepared.WindsurfAPIKey)
	}
	a.applyPostWindsurfSwitch()
	return nil
}

// AutoSwitchToNext 切到下一可用账号。planFilter：all 不限制；否则为 PlanTone 单值或逗号分隔多选（如 trial,pro）
func (a *App) AutoSwitchToNext(currentID string, planFilter string) (string, error) {
	accounts := a.store.GetAllAccounts()
	candidates := orderedSwitchCandidates(accounts, currentID, planFilter)
	if len(candidates) == 0 {
		f := strings.TrimSpace(strings.ToLower(planFilter))
		if f != "" && f != "all" {
			return "", fmt.Errorf("在「%s」计划筛选下没有仍有剩余额度的可切换账号", planFilter)
		}
		return "", fmt.Errorf("没有仍有剩余额度的可切换账号（号池可能均已用尽或未同步额度）")
	}

	var lastErr error
	for _, acc := range candidates {
		prepared, err := a.prepareAccountForUsage(acc)
		if err != nil {
			lastErr = err
			continue
		}
		if err := a.switchSvc.SwitchAccount(prepared.Token, prepared.Email); err != nil {
			lastErr = err
			continue
		}
		// ★ 同步 MITM 代理当前号
		if prepared.WindsurfAPIKey != "" {
			a.mitmProxy.SwitchToKey(prepared.WindsurfAPIKey)
		}
		a.applyPostWindsurfSwitch()
		return prepared.Email, nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("没有可切换的账号")
}

func (a *App) GetCurrentWindsurfAuth() (*services.WindsurfAuthJSON, error) {
	return a.switchSvc.GetCurrentAuth()
}

func (a *App) OpenAccountInIsolatedWindow(id string) (string, error) {
	if err := isolatedWindowMitmConflict(a.mitmProxy != nil && a.mitmProxy.Status().Running, services.IsHostsMapped(services.TargetDomain)); err != nil {
		return "", err
	}
	acc, err := a.store.GetAccount(id)
	if err != nil {
		return "", err
	}
	prepared, err := a.prepareAccountForUsage(acc)
	if err != nil {
		return "", err
	}
	profile := services.BuildIDEProfilePaths(a.store.DataDir(), prepared.ID, prepared.Email)
	if err := services.PrepareIsolatedIDEProfile(profile); err != nil {
		return "", err
	}
	if err := services.WriteIDEProfileMetadata(profile, prepared.ID, prepared.Email); err != nil {
		return "", err
	}
	running, err := services.IsIDEProfileRunning(profile)
	if err != nil {
		return "", err
	}
	if running {
		return filepath.Clean(profile.UserDataDir), fmt.Errorf("该账号的独立窗口已经在运行，请直接切回现有窗口；若需要重新拉起，请先关闭当前独立窗口")
	}
	if err := services.WriteAuthFile(profile.AuthPath, prepared.Token, prepared.Email); err != nil {
		return "", err
	}
	if prepared.WindsurfAPIKey != "" {
		_ = services.InjectCodeiumConfigAtHome(profile.HomeDir, prepared.WindsurfAPIKey)
	}
	if err := services.LaunchWindsurfWithProfile(a.store.GetSettings().WindsurfPath, profile); err != nil {
		return "", err
	}
	return filepath.Clean(profile.UserDataDir), nil
}

func isolatedWindowMitmConflict(proxyRunning, hostsMapped bool) error {
	if !proxyRunning && !hostsMapped {
		return nil
	}
	if proxyRunning {
		return fmt.Errorf("独立开窗与当前 MITM 多号轮换冲突：代理正在接管 Windsurf 请求，无法保证“一窗一号”。请先停止 MITM 并恢复环境后再开独立窗口")
	}
	return fmt.Errorf("独立开窗与当前系统劫持环境冲突：hosts 仍在接管 Windsurf 域名，新的窗口会被拉进旧的代理链路。请先执行恢复环境后再开独立窗口")
}

// applyPostWindsurfSwitch 写入 auth 后：尝试协议刷新；若开启设置则重启 Windsurf（运行中 IDE 会缓存 JWT，仅改文件通常不会立即换账号）。
func (a *App) applyPostWindsurfSwitch() {
	settings := a.store.GetSettings()
	a.switchSvc.TryOpenWindsurfRefreshURIs()
	if !settings.RestartWindsurfAfterSwitch {
		return
	}
	root := strings.TrimSpace(settings.WindsurfPath)
	if root == "" {
		if p, err := a.patchSvc.FindWindsurfPath(); err == nil {
			root = p
		}
	}
	_ = a.patchSvc.RestartWindsurfFromInstall(root)
}

func (a *App) GetWindsurfAuthPath() (string, error) {
	return a.switchSvc.GetWindsurfAuthPath()
}

func hasSwitchCredentials(acc *models.Account) bool {
	if acc == nil {
		return false
	}
	if strings.TrimSpace(acc.Token) != "" {
		return true
	}
	if strings.TrimSpace(acc.WindsurfAPIKey) != "" {
		return true
	}
	if strings.TrimSpace(acc.RefreshToken) != "" {
		return true
	}
	return strings.TrimSpace(acc.Email) != "" && strings.TrimSpace(acc.Password) != ""
}

func accountEligibleForUsage(acc *models.Account, planFilter string, requireAPIKey bool) bool {
	if acc == nil {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(acc.Status))
	if status == "disabled" || status == "expired" {
		return false
	}
	if requireAPIKey && strings.TrimSpace(acc.WindsurfAPIKey) == "" {
		return false
	}
	if !hasSwitchCredentials(acc) {
		return false
	}
	if !utils.PlanFilterMatch(planFilter, acc.PlanName) {
		return false
	}
	return !utils.AccountQuotaExhausted(acc)
}

func orderedSwitchCandidates(accounts []models.Account, currentID string, planFilter string) []models.Account {
	out := make([]models.Account, 0, len(accounts))
	for _, acc := range accounts {
		if acc.ID == currentID {
			continue
		}
		if !accountEligibleForUsage(&acc, planFilter, false) {
			continue
		}
		out = append(out, acc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return switchCredentialPriority(out[i]) < switchCredentialPriority(out[j])
	})
	return out
}

func orderedMitmCandidates(accounts []models.Account, currentID string, planFilter string) []models.Account {
	out := make([]models.Account, 0, len(accounts))
	for _, acc := range accounts {
		if acc.ID == currentID {
			continue
		}
		if !accountEligibleForUsage(&acc, planFilter, true) {
			continue
		}
		out = append(out, acc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return switchCredentialPriority(out[i]) < switchCredentialPriority(out[j])
	})
	return out
}

func switchCredentialPriority(acc models.Account) int {
	switch {
	case strings.TrimSpace(acc.Token) != "":
		return 0
	case strings.TrimSpace(acc.WindsurfAPIKey) != "":
		return 1
	case strings.TrimSpace(acc.RefreshToken) != "":
		return 2
	case strings.TrimSpace(acc.Email) != "" && strings.TrimSpace(acc.Password) != "":
		return 3
	default:
		return 4
	}
}

func pickNextSwitchableAccount(accounts []models.Account, currentID string, planFilter string) (models.Account, error) {
	candidates := orderedSwitchCandidates(accounts, currentID, planFilter)
	if len(candidates) == 0 {
		return models.Account{}, fmt.Errorf("no switchable account")
	}
	return candidates[0], nil
}

func pickNextMitmSwitchableAccount(accounts []models.Account, currentID string, planFilter string) (models.Account, error) {
	candidates := orderedMitmCandidates(accounts, currentID, planFilter)
	if len(candidates) == 0 {
		return models.Account{}, fmt.Errorf("no mitm switchable account")
	}
	return candidates[0], nil
}

func (a *App) prepareAccountForUsage(acc models.Account) (models.Account, error) {
	if !hasSwitchCredentials(&acc) {
		return models.Account{}, fmt.Errorf("该账号没有可用凭证")
	}
	if !accountEligibleForUsage(&acc, "all", false) {
		if utils.AccountQuotaExhausted(&acc) {
			return models.Account{}, fmt.Errorf("该账号已无可用额度，已阻止继续使用")
		}
		return models.Account{}, fmt.Errorf("该账号当前不可用")
	}
	before := acc
	a.syncAccountCredentials(&acc)
	a.enrichAccountQuotaOnly(&acc)
	acc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
	if strings.TrimSpace(acc.Token) == "" {
		return models.Account{}, fmt.Errorf("该账号无法准备有效 Token")
	}
	if utils.AccountQuotaExhausted(&acc) {
		_ = a.store.UpdateAccount(acc)
		a.syncMitmPoolKeys()
		return models.Account{}, fmt.Errorf("该账号已无可用额度，已阻止继续使用")
	}
	if acc != before {
		_ = a.store.UpdateAccount(acc)
	}
	return acc, nil
}

func (a *App) rotateMitmToNextAvailable(currentID string, planFilter string) (models.Account, error) {
	acc, err := pickNextMitmSwitchableAccount(a.store.GetAllAccounts(), currentID, planFilter)
	if err != nil {
		return models.Account{}, err
	}
	if !a.mitmProxy.SwitchToKey(acc.WindsurfAPIKey) {
		return models.Account{}, fmt.Errorf("MITM 代理未找到目标 API Key")
	}
	_ = services.InjectCodeiumConfig(acc.WindsurfAPIKey)
	return acc, nil
}
