package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"windsurf-tools-wails/backend/models"
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
	accounts := a.store.GetAllAccounts()
	currentID := findAccountIDForMITMAPIKey(accounts, a.mitmProxy.CurrentAPIKey())
	nextAcc, err := pickNextMitmSwitchableAccount(accounts, currentID, a.store.GetSettings().AutoSwitchPlanFilter)
	if err != nil {
		return "", fmt.Errorf("MITM 号池为空，或当前没有可切换的席位")
	}
	return a.switchMitmAccountAndSyncLocalSession(nextAcc)
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
	return a.switchMitmAccountAndSyncLocalSession(acc)
}

// GetMitmProxyStatus returns the current proxy status.
// 在 MitmProxy.Status() 基础上，用号池账号信息填充 PoolKeyInfo 的 Email/Nickname。
func (a *App) GetMitmProxyStatus() services.MitmProxyStatus {
	st := a.mitmProxy.Status()
	if len(st.PoolStatus) > 0 && a.store != nil {
		accounts := a.store.GetAllAccounts()
		keyToAccount := make(map[string]models.Account, len(accounts))
		for _, acc := range accounts {
			k := strings.TrimSpace(acc.WindsurfAPIKey)
			if k != "" {
				keyToAccount[k] = acc
			}
		}
		for i := range st.PoolStatus {
			short := st.PoolStatus[i].KeyShort
			// 遍历匹配（KeyShort 是截断后的前缀 + "..."）
			for fullKey, acc := range keyToAccount {
				prefix := short
				if strings.HasSuffix(prefix, "...") {
					prefix = prefix[:len(prefix)-3]
				}
				if strings.HasPrefix(fullKey, prefix) {
					st.PoolStatus[i].Email = acc.Email
					st.PoolStatus[i].Nickname = acc.Nickname
					break
				}
			}
		}
	}
	return st
}

// GetMitmSessionBindings returns all active session bindings for the frontend.
func (a *App) GetMitmSessionBindings() []services.SessionBindingInfo {
	return a.mitmProxy.GetSessionBindings()
}

// UnbindMitmSession removes a session binding by conversation ID prefix.
func (a *App) UnbindMitmSession(convIDPrefix string) bool {
	return a.mitmProxy.UnbindSession(convIDPrefix)
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
	// ★ 恢复原始登录态
	if a.switchSvc != nil {
		a.switchSvc.RestoreOriginalAuth()
	}
	if err := services.UninstallCA(); err != nil {
		errs = append(errs, fmt.Errorf("卸载 CA: %w", err))
	}
	services.InvalidateCACache()
	return errors.Join(errs...)
}

// applyMitmSystemSetup 一键应用所有系统修改 (MITM 启动时调用)
func (a *App) applyMitmSystemSetup() {
	// ★ 备份原始登录态，退出时恢复
	if a.switchSvc != nil {
		a.switchSvc.BackupOriginalAuth()
	}
	_ = services.AddHostsEntry(services.TargetDomain)
	_ = services.AddProxyOverride()
	a.injectFirstPoolKeyToCodeiumConfig()
	// 恢复持久化的 dump/抓包设置
	settings := a.store.GetSettings()
	a.mitmProxy.SetDebugDump(settings.MitmDebugDump)
	a.mitmProxy.SetFullCapture(settings.MitmFullCapture)
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

// ToggleMitmFullCapture 开启/关闭全量抓包
func (a *App) ToggleMitmFullCapture(enabled bool) {
	a.mitmProxy.SetFullCapture(enabled)
	settings := a.store.GetSettings()
	settings.MitmFullCapture = enabled
	a.store.UpdateSettings(settings)
}

// GetMitmFullCaptureEnabled 返回全量抓包是否开启
func (a *App) GetMitmFullCaptureEnabled() bool {
	return a.mitmProxy.FullCaptureEnabled()
}

// GetCaptureDir 返回全量抓包目录路径
func (a *App) GetCaptureDir() string {
	return services.CaptureDir()
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

func (a *App) handleMitmKeyAccessDenied(apiKey, detail string) {
	apiKey = strings.TrimSpace(apiKey)
	detail = strings.TrimSpace(detail)
	if a == nil || a.store == nil || apiKey == "" {
		return
	}

	accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey)
	if accID == "" {
		utils.DLog("[回调] onKeyAccessDenied: 未找到匹配 key=%s...", apiKey[:minInt(12, len(apiKey))])
		return
	}

	acc, err := a.store.GetAccount(accID)
	if err != nil {
		utils.DLog("[回调] onKeyAccessDenied: 读取账号失败 id=%s err=%v", accID[:minInt(8, len(accID))], err)
		return
	}

	before := acc
	applyAccessErrorStatus(&acc, fmt.Errorf("%s", detail))
	if acc == before {
		utils.DLog("[回调] onKeyAccessDenied: 未命中降权规则，保持原状态 id=%s", accID[:minInt(8, len(accID))])
		return
	}

	if err := a.store.UpdateAccount(acc); err != nil {
		utils.DLog("[回调] onKeyAccessDenied: 保存账号失败 id=%s err=%v", accID[:minInt(8, len(accID))], err)
		return
	}
	a.syncMitmPoolKeys()
	utils.DLog("[回调] onKeyAccessDenied: 已持久化 %s status=%s plan=%s", labelAccountResult(acc), acc.Status, acc.PlanName)
}

func shouldSyncMitmLocalSessionOnKeyChange(reason string) bool {
	// ★ MITM 按 conversation_id 路由，自动轮转不修改本地登录态（保持 Pro 身份）
	// 只有用户手动切号 (switchMitmAccountAndSyncLocalSession) 才同步本地 auth
	return false
}

func (a *App) handleMitmCurrentKeyChanged(apiKey, reason string) {
	apiKey = strings.TrimSpace(apiKey)
	reason = strings.TrimSpace(reason)
	if a == nil || a.store == nil || apiKey == "" {
		return
	}
	if !shouldSyncMitmLocalSessionOnKeyChange(reason) {
		return
	}

	accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey)
	if accID == "" {
		utils.DLog("[回调] onCurrentKeyChanged: 未找到匹配 key=%s... reason=%s", apiKey[:minInt(12, len(apiKey))], reason)
		return
	}
	acc, err := a.store.GetAccount(accID)
	if err != nil {
		utils.DLog("[回调] onCurrentKeyChanged: 读取账号失败 id=%s err=%v", accID[:minInt(8, len(accID))], err)
		return
	}
	if strings.TrimSpace(acc.Token) == "" {
		utils.DLog("[回调] onCurrentKeyChanged: %s 缺少 Token，跳过本地 auth 同步 reason=%s", labelAccountResult(acc), reason)
		return
	}
	if err := a.syncMitmLocalAuth(acc); err != nil {
		utils.DLog("[回调] onCurrentKeyChanged: 写入本地 auth 失败 %s err=%v", labelAccountResult(acc), err)
		return
	}
	utils.DLog("[回调] onCurrentKeyChanged: 已同步本地 auth -> %s reason=%s", labelAccountResult(acc), reason)
	go a.applyPostWindsurfSwitch()
}

func (a *App) switchMitmAccountAndSyncLocalSession(acc models.Account) (string, error) {
	prepared, err := a.prepareAccountForUsage(acc)
	if err != nil {
		return "", err
	}
	apiKey := strings.TrimSpace(prepared.WindsurfAPIKey)
	if apiKey == "" {
		return "", fmt.Errorf("该账号没有 API Key，无法用于 MITM 手动切号")
	}

	a.syncMitmPoolKeys()
	if !a.mitmProxy.SwitchToKey(apiKey) {
		return "", fmt.Errorf("该账号当前未加入 MITM 号池，请检查套餐筛选、额度状态或 API Key 是否可用")
	}
	if err := a.syncMitmLocalAuth(prepared); err != nil {
		return "", err
	}
	utils.DLog("[MITM] 手动切号后同步本地 auth 成功: %s", prepared.Email)
	go a.applyPostWindsurfSwitch()
	return a.describeMitmKey(apiKey), nil
}

func (a *App) syncMitmLocalAuth(acc models.Account) error {
	if a.switchSvc == nil {
		return fmt.Errorf("切号服务未初始化")
	}
	token := strings.TrimSpace(acc.Token)
	if token == "" {
		return fmt.Errorf("该账号没有可写入本地登录态的 Token")
	}
	if err := a.switchSvc.SwitchAccount(token, strings.TrimSpace(acc.Email)); err != nil {
		return fmt.Errorf("写入本地 windsurf_auth 失败: %w", err)
	}
	return nil
}

func (a *App) syncForgeConfig() {
	if a.mitmProxy == nil {
		return
	}
	s := a.store.GetSettings()
	a.mitmProxy.SetForgeConfig(services.ForgeConfig{
		Enabled:            s.ForgeEnabled,
		FakeCredits:        s.FakeCredits,
		FakeCreditsPremium: s.FakeCreditsPremium,
		FakeCreditsOther:   s.FakeCreditsOther,
		FakeCreditsUsed:    s.FakeCreditsUsed,
		FakeSubType:        s.FakeSubscriptionType,
		ExtendYears:        s.FakeBillingExtendYears,
	})
}

func (a *App) syncStaticCacheConfig() {
	if a.mitmProxy == nil || a.store == nil {
		return
	}
	s := a.store.GetSettings()
	a.mitmProxy.SetStaticCacheConfig(services.StaticCacheConfig{
		Enabled:  s.StaticCacheIntercept,
		CacheDir: a.staticCacheDir(),
	})
}

func (a *App) staticCacheDir() string {
	if a.store == nil {
		return ""
	}
	return a.store.DataDir() + string(os.PathSeparator) + "static"
}

// GetStaticCacheDir is exposed to the frontend.
func (a *App) GetStaticCacheDir() string {
	dir := a.staticCacheDir()
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}
	return dir
}
