package main

import (
	"fmt"
	"strings"
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
	if acc.Token == "" {
		return fmt.Errorf("该账号没有可用的Token")
	}
	if err := a.switchSvc.SwitchAccount(acc.Token, acc.Email); err != nil {
		return err
	}
	// ★ 同步 MITM 代理当前号
	if acc.WindsurfAPIKey != "" {
		a.mitmProxy.SwitchToKey(acc.WindsurfAPIKey)
	}
	a.applyPostWindsurfSwitch()
	return nil
}

// AutoSwitchToNext 切到下一可用账号。planFilter：all 不限制；否则为 PlanTone 单值或逗号分隔多选（如 trial,pro）
func (a *App) AutoSwitchToNext(currentID string, planFilter string) (string, error) {
	accounts := a.store.GetAllAccounts()
	acc, err := pickNextSwitchableAccount(accounts, currentID, planFilter)
	if err != nil {
		f := strings.TrimSpace(strings.ToLower(planFilter))
		if f != "" && f != "all" {
			return "", fmt.Errorf("在「%s」计划筛选下没有仍有剩余额度的可切换账号", planFilter)
		}
		return "", fmt.Errorf("没有仍有剩余额度的可切换账号（号池可能均已用尽或未同步额度）")
	}
	if err := a.switchSvc.SwitchAccount(acc.Token, acc.Email); err != nil {
		return "", err
	}
	// ★ 同步 MITM 代理当前号
	if acc.WindsurfAPIKey != "" {
		a.mitmProxy.SwitchToKey(acc.WindsurfAPIKey)
	}
	a.applyPostWindsurfSwitch()
	return acc.Email, nil
}

func (a *App) GetCurrentWindsurfAuth() (*services.WindsurfAuthJSON, error) {
	return a.switchSvc.GetCurrentAuth()
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

func pickNextSwitchableAccount(accounts []models.Account, currentID string, planFilter string) (models.Account, error) {
	for _, acc := range accounts {
		if acc.ID == currentID {
			continue
		}
		if strings.TrimSpace(acc.Token) == "" {
			continue
		}
		if acc.Status == "disabled" || acc.Status == "expired" {
			continue
		}
		if !utils.PlanFilterMatch(planFilter, acc.PlanName) {
			continue
		}
		// 已同步为「日/周/月配额见底」的账号不作为自动/下一席切换目标，避免刚切过去仍无额度
		if utils.AccountQuotaExhausted(&acc) {
			continue
		}
		return acc, nil
	}
	return models.Account{}, fmt.Errorf("no switchable account")
}
