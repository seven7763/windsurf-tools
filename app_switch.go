package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
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
	utils.DLog("[切号] SwitchAccount: id=%s email=%s plan=%s daily=%s weekly=%s", acc.ID, acc.Email, acc.PlanName, acc.DailyRemaining, acc.WeeklyRemaining)
	prepared, err := a.prepareAccountForUsage(acc)
	if err != nil {
		utils.DLog("[切号] prepareAccountForUsage 失败: %v", err)
		return err
	}
	if err := a.switchSvc.SwitchAccount(prepared.Token, prepared.Email); err != nil {
		utils.DLog("[切号] SwitchAccount 写auth失败: %v", err)
		return err
	}
	// ★ 同步本地 codeium config.json（API Key），否则 Windsurf 重读 auth 时仍用旧 key
	if prepared.WindsurfAPIKey != "" {
		if err := services.InjectCodeiumConfig(prepared.WindsurfAPIKey); err != nil {
			utils.DLog("[切号] 同步 codeium config 失败: %v", err)
		}
		a.mitmProxy.SwitchToKey(prepared.WindsurfAPIKey)
	}
	utils.DLog("[切号] SwitchAccount 成功: %s", prepared.Email)
	go a.applyPostWindsurfSwitch()
	return nil
}

// AutoSwitchToNext 切到下一可用账号。planFilter：all 不限制；否则为 PlanTone 单值或逗号分隔多选（如 trial,pro）
func (a *App) AutoSwitchToNext(currentID string, planFilter string) (string, error) {
	accounts := a.store.GetAllAccounts()
	candidates := orderedSwitchCandidates(accounts, currentID, planFilter)
	utils.DLog("[切号] AutoSwitchToNext: currentID=%s filter=%s 号池=%d 候选=%d", currentID[:min(8, len(currentID))], planFilter, len(accounts), len(candidates))
	if len(candidates) == 0 {
		f := strings.TrimSpace(strings.ToLower(planFilter))
		if f != "" && f != "all" {
			return "", fmt.Errorf("在「%s」计划筛选下没有仍有剩余额度的可切换账号", planFilter)
		}
		return "", fmt.Errorf("没有仍有剩余额度的可切换账号（号池可能均已用尽或未同步额度）")
	}

	// ★ 预热：并行刷新 top N 候选的 JWT + 额度，确保切号目标数据新鲜
	a.prewarmCandidates(candidates, 3)

	// 预热已将最新凭证+额度写入 store，重读避免 prepareAccountForUsage 重复调用 API
	freshAccounts := a.store.GetAllAccounts()
	freshMap := make(map[string]models.Account, len(freshAccounts))
	for _, fa := range freshAccounts {
		freshMap[fa.ID] = fa
	}

	var lastErr error
	for _, acc := range candidates {
		if fresh, ok := freshMap[acc.ID]; ok {
			acc = fresh
		}
		prepared, err := a.prepareAccountForUsage(acc)
		if err != nil {
			utils.DLog("[切号] AutoSwitch 跳过 %s: %v", acc.Email, err)
			lastErr = err
			continue
		}
		if err := a.switchSvc.SwitchAccount(prepared.Token, prepared.Email); err != nil {
			utils.DLog("[切号] AutoSwitch 写入auth失败 %s: %v", prepared.Email, err)
			lastErr = err
			continue
		}
		// ★ 同步本地 codeium config + MITM 代理
		if prepared.WindsurfAPIKey != "" {
			if err := services.InjectCodeiumConfig(prepared.WindsurfAPIKey); err != nil {
				utils.DLog("[切号] 同步 codeium config 失败: %v", err)
			}
			a.mitmProxy.SwitchToKey(prepared.WindsurfAPIKey)
		}
		utils.DLog("[切号] AutoSwitch 成功切换到 %s (plan=%s daily=%s weekly=%s)", prepared.Email, prepared.PlanName, prepared.DailyRemaining, prepared.WeeklyRemaining)
		go a.applyPostWindsurfSwitch()
		return prepared.Email, nil
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("没有可切换的账号")
}

// prewarmCandidates 并行预热 top N 候选账号：刷新JWT + 实时查额度，将结果写入 store。
func (a *App) prewarmCandidates(candidates []models.Account, maxN int) {
	n := len(candidates)
	if n > maxN {
		n = maxN
	}
	if n == 0 {
		return
	}
	utils.DLog("[切号] 预热 %d 个候选账号...", n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(acc models.Account) {
			defer wg.Done()
			copy := acc
			a.syncAccountCredentials(&copy)
			if a.enrichAccountQuotaOnly(&copy) {
				copy.LastQuotaUpdate = time.Now().Format(time.RFC3339)
			}
			_ = a.store.UpdateAccount(copy)
			if utils.AccountQuotaExhausted(&copy) {
				utils.DLog("[切号] 预热: %s 额度已耗尽 (daily=%s weekly=%s)", copy.Email, copy.DailyRemaining, copy.WeeklyRemaining)
			} else {
				utils.DLog("[切号] 预热: %s 额度OK (daily=%s weekly=%s)", copy.Email, copy.DailyRemaining, copy.WeeklyRemaining)
			}
		}(candidates[i])
	}
	wg.Wait()
}

func (a *App) GetCurrentWindsurfAuth() (*services.WindsurfAuthJSON, error) {
	return a.switchSvc.GetCurrentAuth()
}

// applyPostWindsurfSwitch 写入 auth 后：尝试协议刷新；若开启设置则重启 Windsurf（运行中 IDE 会缓存 JWT，仅改文件通常不会立即换账号）。
func (a *App) applyPostWindsurfSwitch() {
	settings := a.store.GetSettings()
	utils.DLog("[切号] applyPostSwitch: TryOpenWindsurfRefreshURIs...")
	a.switchSvc.TryOpenWindsurfRefreshURIs()
	if !settings.RestartWindsurfAfterSwitch {
		utils.DLog("[切号] applyPostSwitch: RestartWindsurfAfterSwitch=false, 跳过重启")
		return
	}
	root := strings.TrimSpace(settings.WindsurfPath)
	if root == "" {
		if p, err := a.patchSvc.FindWindsurfPath(); err == nil {
			root = p
		}
	}
	utils.DLog("[切号] applyPostSwitch: 重启 Windsurf (root=%s)", root)
	if err := a.patchSvc.RestartWindsurfFromInstall(root); err != nil {
		utils.DLog("[切号] applyPostSwitch: 重启失败: %v", err)
	}
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
	var fresh, stale []models.Account
	for _, acc := range accounts {
		if acc.ID == currentID {
			continue
		}
		if accountEligibleForUsage(&acc, planFilter, false) {
			fresh = append(fresh, acc)
			continue
		}
		// 额度数据过期的账号也纳入候选（额度可能已重置），预热阶段会刷新
		if quotaDataIsStale(&acc) && hasSwitchCredentials(&acc) && utils.PlanFilterMatch(planFilter, acc.PlanName) {
			status := strings.TrimSpace(strings.ToLower(acc.Status))
			if status != "disabled" && status != "expired" {
				stale = append(stale, acc)
			}
		}
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		return switchCredentialPriority(fresh[i]) < switchCredentialPriority(fresh[j])
	})
	sort.SliceStable(stale, func(i, j int) bool {
		return switchCredentialPriority(stale[i]) < switchCredentialPriority(stale[j])
	})
	// 新鲜的优先，过期数据的排后面
	return append(fresh, stale...)
}

// quotaDataIsStale 检查额度数据是否过期（超过重置周期），过期的「已耗尽」账号应参与预热重新检查。
func quotaDataIsStale(acc *models.Account) bool {
	if acc == nil {
		return false
	}
	if !utils.AccountQuotaExhausted(acc) {
		return false // 未耗尽的不算过期
	}
	raw := strings.TrimSpace(acc.LastQuotaUpdate)
	if raw == "" {
		return true // 从未同步过
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	// 额度数据超过 4 小时视为过期（日额度每天重置，4h 足以覆盖跨日场景）
	return time.Since(t) > 4*time.Hour
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
	utils.DLog("[切号] prepareAccount: %s status=%s hasKey=%v hasToken=%v hasRefresh=%v", acc.Email, acc.Status, acc.WindsurfAPIKey != "", acc.Token != "", acc.RefreshToken != "")
	if !hasSwitchCredentials(&acc) {
		return models.Account{}, fmt.Errorf("该账号没有可用凭证")
	}
	status := strings.TrimSpace(strings.ToLower(acc.Status))
	if status == "disabled" || status == "expired" {
		return models.Account{}, fmt.Errorf("该账号状态为 %s，已跳过", status)
	}

	// ★ 如果刚预热过（30 秒内），跳过重复 API 调用，直接用缓存数据校验
	recentlyWarmed := false
	if t, err := time.Parse(time.RFC3339, acc.LastQuotaUpdate); err == nil && time.Since(t) < 30*time.Second {
		recentlyWarmed = true
	}
	utils.DLog("[切号] prepareAccount: recentlyWarmed=%v", recentlyWarmed)

	before := acc
	if !recentlyWarmed {
		a.syncAccountCredentials(&acc)
		if a.enrichAccountQuotaOnly(&acc) {
			acc.LastQuotaUpdate = time.Now().Format(time.RFC3339)
		}
	}

	if strings.TrimSpace(acc.Token) == "" {
		utils.DLog("[切号] %s Token为空，凭证同步可能失败", acc.Email)
		return models.Account{}, fmt.Errorf("该账号无法准备有效 Token（JWT/登录均失败）")
	}
	if utils.AccountQuotaExhausted(&acc) {
		_ = a.store.UpdateAccount(acc)
		a.syncMitmPoolKeys()
		utils.DLog("[切号] %s 实时额度已耗尽 (daily=%s weekly=%s)", acc.Email, acc.DailyRemaining, acc.WeeklyRemaining)
		return models.Account{}, fmt.Errorf("该账号已无可用额度（日=%s 周=%s），已跳过", acc.DailyRemaining, acc.WeeklyRemaining)
	}
	utils.DLog("[切号] prepareAccount OK: %s (daily=%s weekly=%s tokenLen=%d)", acc.Email, acc.DailyRemaining, acc.WeeklyRemaining, len(acc.Token))
	if acc != before {
		_ = a.store.UpdateAccount(acc)
	}
	return acc, nil
}

func (a *App) rotateMitmToNextAvailable(currentID string, planFilter string) (models.Account, error) {
	candidates := orderedMitmCandidates(a.store.GetAllAccounts(), currentID, planFilter)
	utils.DLog("[切号] rotateMitm: currentID=%s filter=%s 候选=%d", currentID[:min(8, len(currentID))], planFilter, len(candidates))
	if len(candidates) == 0 {
		return models.Account{}, fmt.Errorf("无可用 MITM 候选账号")
	}

	// ★ 预热 top N 候选：刷新 JWT + 实时查额度，防止切到实际已耗尽的账号
	a.prewarmCandidates(candidates, 2)

	// 预热后重读 store，仅保留仍有额度的
	freshCandidates := orderedMitmCandidates(a.store.GetAllAccounts(), currentID, planFilter)
	utils.DLog("[切号] rotateMitm: 预热后候选=%d", len(freshCandidates))
	if len(freshCandidates) == 0 {
		return models.Account{}, fmt.Errorf("预热后无可用 MITM 候选账号（候选均已耗尽）")
	}

	acc := freshCandidates[0]
	utils.DLog("[切号] rotateMitm: 切换到 %s (key=%s...)", acc.Email, acc.WindsurfAPIKey[:min(12, len(acc.WindsurfAPIKey))])
	if !a.mitmProxy.SwitchToKey(acc.WindsurfAPIKey) {
		return models.Account{}, fmt.Errorf("MITM 代理未找到目标 API Key")
	}
	_ = services.InjectCodeiumConfig(acc.WindsurfAPIKey)
	return acc, nil
}
