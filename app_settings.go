package main

import (
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
)

// ═══════════════════════════════════════
// 设置与代理
// ═══════════════════════════════════════

func (a *App) GetSettings() models.Settings { return a.store.GetSettings() }

func (a *App) UpdateSettings(settings models.Settings) error {
	if err := a.store.UpdateSettings(settings); err != nil {
		return err
	}
	proxyURL := ""
	if settings.ProxyEnabled && settings.ProxyURL != "" {
		proxyURL = settings.ProxyURL
	}
	a.windsurfSvc = services.NewWindsurfService(proxyURL)
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
	return nil
}
