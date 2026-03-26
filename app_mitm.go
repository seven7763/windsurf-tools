package main

import (
	"errors"
	"fmt"
	"strings"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
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

	keys := collectEligibleMitmAPIKeys(accounts, filter)
	a.mitmProxy.SetPoolKeys(keys)
}

func collectEligibleMitmAPIKeys(accounts []models.Account, planFilter string) []string {
	var keys []string
	seen := make(map[string]struct{})
	for _, acc := range accounts {
		if !accountEligibleForUsage(&acc, planFilter, true) {
			continue
		}
		key := strings.TrimSpace(acc.WindsurfAPIKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
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

// SwitchMitmToNext 手动切到 MITM 号池中的下一席位。
func (a *App) SwitchMitmToNext() (string, error) {
	a.syncMitmPoolKeys()
	nextKey := strings.TrimSpace(a.mitmProxy.SwitchToNext())
	if nextKey == "" {
		return "", fmt.Errorf("MITM 号池为空，或当前没有可切换的席位")
	}
	return a.describeMitmKey(nextKey), nil
}

// SwitchMitmToAccount 手动切到指定账号对应的 MITM API Key。
func (a *App) SwitchMitmToAccount(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("账号 ID 不能为空")
	}
	acc, err := a.store.GetAccount(id)
	if err != nil {
		return "", err
	}
	apiKey := strings.TrimSpace(acc.WindsurfAPIKey)
	if apiKey == "" {
		return "", fmt.Errorf("该账号没有 API Key，无法用于 MITM 手动切号")
	}

	a.syncMitmPoolKeys()
	if !a.mitmProxy.SwitchToKey(apiKey) {
		return "", fmt.Errorf("该账号当前未加入 MITM 号池，请检查套餐筛选、额度状态或 API Key 是否可用")
	}
	return a.describeMitmKey(apiKey), nil
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
	keys := collectEligibleMitmAPIKeys(a.store.GetAllAccounts(), a.store.GetSettings().AutoSwitchPlanFilter)
	if len(keys) == 0 {
		return
	}
	_ = services.InjectCodeiumConfig(keys[0])
}

// GetMitmCAPath returns the CA certificate file path.
func (a *App) GetMitmCAPath() string {
	return services.GetCACertPath()
}

// ToggleMitmDebugDump 开启/关闭 MITM proto dump
func (a *App) ToggleMitmDebugDump(enabled bool) {
	a.mitmProxy.SetDebugDump(enabled)
	settings := a.store.GetSettings()
	settings.MitmDebugDump = enabled
	a.store.UpdateSettings(settings)
}

// GetMitmDebugDumpEnabled 返回当前 debug dump 状态
func (a *App) GetMitmDebugDumpEnabled() bool {
	return a.mitmProxy.DebugDumpEnabled()
}

// GetProtoDumpDir 返回 proto dump 文件目录路径
func (a *App) GetProtoDumpDir() string {
	return services.ProtoDumpDir()
}

func (a *App) describeMitmKey(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	for _, acc := range a.store.GetAllAccounts() {
		if strings.TrimSpace(acc.WindsurfAPIKey) != apiKey {
			continue
		}
		if email := strings.TrimSpace(acc.Email); email != "" {
			return email
		}
		if nickname := strings.TrimSpace(acc.Nickname); nickname != "" {
			return nickname
		}
		break
	}
	if len(apiKey) > 16 {
		return apiKey[:12] + "..."
	}
	return apiKey
}
