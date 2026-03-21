package main

import (
	"errors"
	"fmt"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// MITM 代理
// ═══════════════════════════════════════

// syncMitmPoolKeys syncs pool keys from store accounts that have WindsurfAPIKey.
// ★ 遵守 AutoSwitchPlanFilter 设置：只有计划匹配的账号才加入 MITM 号池
func (a *App) syncMitmPoolKeys() {
	accounts := a.store.GetAllAccounts()
	settings := a.store.GetSettings()
	filter := settings.AutoSwitchPlanFilter

	var keys []string
	for _, acc := range accounts {
		if acc.WindsurfAPIKey == "" || acc.Status == "disabled" {
			continue
		}
		// 按计划筛选
		if !utils.PlanFilterMatch(filter, acc.PlanName) {
			continue
		}
		keys = append(keys, acc.WindsurfAPIKey)
	}
	a.mitmProxy.SetPoolKeys(keys)
}

// StartMitmProxy starts the MITM reverse proxy with full system setup.
func (a *App) StartMitmProxy() error {
	a.syncMitmPoolKeys()
	if err := a.mitmProxy.Start(); err != nil {
		return err
	}
	// 自动应用系统修改: hosts劫持 + DNS刷新 + 注册表代理白名单 + Codeium config
	a.applyMitmSystemSetup()
	return nil
}

// StopMitmProxy stops the MITM reverse proxy.
func (a *App) StopMitmProxy() error {
	return a.mitmProxy.Stop()
}

// GetMitmProxyStatus returns the current proxy status.
func (a *App) GetMitmProxyStatus() services.MitmProxyStatus {
	return a.mitmProxy.Status()
}

// SetupMitmCA generates and installs the CA certificate.
func (a *App) SetupMitmCA() error {
	if _, err := services.EnsureCA(services.TargetDomain); err != nil {
		return err
	}
	err := services.InstallCA()
	services.InvalidateCACache()
	return err
}

// SetupMitmHosts adds hosts file entries for all target domains.
func (a *App) SetupMitmHosts() error {
	if err := services.AddHostsEntry(services.TargetDomain); err != nil {
		return err
	}
	_ = services.AddProxyOverride()
	a.injectFirstPoolKeyToCodeiumConfig()
	return nil
}

// TeardownMitm removes hosts entry, cleans ProxyOverride, restores Codeium config, and uninstalls CA.
func (a *App) TeardownMitm() error {
	var errs []error
	if a.mitmProxy != nil {
		if err := a.mitmProxy.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("停止 MITM 代理: %w", err))
		}
	}
	if err := services.RemoveHostsEntry(services.TargetDomain); err != nil {
		errs = append(errs, fmt.Errorf("恢复 hosts: %w", err))
	}
	if err := services.RemoveProxyOverride(); err != nil {
		errs = append(errs, fmt.Errorf("恢复 ProxyOverride: %w", err))
	}
	if err := services.RestoreCodeiumConfig(); err != nil {
		errs = append(errs, fmt.Errorf("恢复 Codeium 配置: %w", err))
	}
	if err := services.UninstallCA(); err != nil {
		errs = append(errs, fmt.Errorf("卸载 CA: %w", err))
	}
	services.InvalidateCACache()
	return errors.Join(errs...)
}

// applyMitmSystemSetup 一键应用所有系统修改 (MITM 启动时调用)
func (a *App) applyMitmSystemSetup() {
	_ = services.AddHostsEntry(services.TargetDomain)
	_ = services.AddProxyOverride()
	a.injectFirstPoolKeyToCodeiumConfig()
}

// injectFirstPoolKeyToCodeiumConfig 将号池中第一个可用 API Key 写入 Codeium config
func (a *App) injectFirstPoolKeyToCodeiumConfig() {
	accounts := a.store.GetAllAccounts()
	for _, acc := range accounts {
		if acc.WindsurfAPIKey != "" && acc.Status != "disabled" {
			_ = services.InjectCodeiumConfig(acc.WindsurfAPIKey)
			return
		}
	}
}

// GetMitmCAPath returns the CA certificate file path.
func (a *App) GetMitmCAPath() string {
	return services.GetCACertPath()
}
