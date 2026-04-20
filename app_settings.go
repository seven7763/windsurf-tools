package main

import (
	"windsurf-tools-wails/backend/models"
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
	if a.mitmProxy != nil {
		a.mitmProxy.SetWindsurfService(a.windsurfSvc)
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
