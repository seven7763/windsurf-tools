package main

import (
	"fmt"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// 设置与代理
// ═══════════════════════════════════════

func (a *App) GetSettings() models.Settings { return a.store.GetSettings() }

func (a *App) UpdateSettings(settings models.Settings) error {
	prev := a.store.GetSettings()
	if err := a.store.UpdateSettings(settings); err != nil {
		return err
	}
	proxyURL := ""
	if settings.ProxyEnabled && settings.ProxyURL != "" {
		proxyURL = settings.ProxyURL
	}
	a.windsurfSvc = services.NewWindsurfService(proxyURL)
	proxyChanged := prev.ProxyEnabled != settings.ProxyEnabled || prev.ProxyURL != settings.ProxyURL
	if a.mitmProxy != nil {
		wasRunning := a.mitmProxy.Status().Running
		if wasRunning && proxyChanged {
			if err := a.mitmProxy.Stop(); err != nil {
				return fmt.Errorf("停止 MITM 代理以应用新网络配置失败: %w", err)
			}
		}
		a.mitmProxy.SetWindsurfService(a.windsurfSvc)
		a.mitmProxy.SetOutboundProxy(proxyURL)
		if wasRunning && proxyChanged {
			a.syncMitmPoolKeys()
			if err := a.mitmProxy.Start(); err != nil {
				return fmt.Errorf("MITM 代理重新加载网络配置失败: %w", err)
			}
			a.applyMitmSystemSetup()
		}
	}
	if settings.AutoRefreshTokens {
		if a.cancelAutoRefresh == nil {
			a.startAutoRefresh()
		}
	} else {
		if a.cancelAutoRefresh != nil {
			a.cancelAutoRefresh()
			a.cancelAutoRefresh = nil
		}
	}
	if settings.AutoRefreshQuotas {
		if a.cancelAutoQuotaRefresh == nil {
			a.startAutoQuotaRefresh()
		}
	} else {
		if a.cancelAutoQuotaRefresh != nil {
			a.cancelAutoQuotaRefresh()
			a.cancelAutoQuotaRefresh = nil
		}
	}
	a.restartQuotaHotPollIfNeeded()
	a.syncMitmPoolKeys()
	a.syncForgeConfig()
	a.syncStaticCacheConfig()
	// 动态切换调试日志
	if prev.DebugLog != settings.DebugLog {
		utils.InitDebugLogger(a.store.DataDir(), settings.DebugLog)
		if settings.DebugLog {
			utils.DLog("[设置] 调试日志已开启")
		}
	}
	return nil
}
