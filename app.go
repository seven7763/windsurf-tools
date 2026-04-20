package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/store"
	"windsurf-tools-wails/backend/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx                    context.Context
	store                  *store.Store
	windsurfSvc            *services.WindsurfService
	mitmProxy              *services.MitmProxy
	openaiRelay            *services.OpenAIRelay
	usageTracker           *services.UsageTracker
	cancelAutoRefresh      context.CancelFunc
	cancelAutoQuotaRefresh context.CancelFunc
	cancelQuotaHotPoll     context.CancelFunc
	lastQuotaHotSwitch     time.Time
	lastQuotaHotSwitchMu   sync.Mutex
	tokenRefreshRunMu      sync.Mutex
	quotaRefreshRunMu      sync.Mutex
	mu                     sync.Mutex
	cleanupMitmOnExitFn    func() error
	activateExistingAppFn  func()
	traySupportedFn        func() bool
	// silentFromFlag 由 main 在解析到 --silent 时设置，与 settings.silent_start 二选一即可触发静默启动
	silentFromFlag bool
}

func NewApp() *App { return &App{} }

// SetSilentFromFlag 由 main 在 wails.Run 前设置（--silent / --silent-start）。
func (a *App) SetSilentFromFlag(v bool) { a.silentFromFlag = v }

func (a *App) initBackend() error {
	s, err := store.NewStore()
	if err != nil {
		return fmt.Errorf("存储初始化失败: %w", err)
	}
	a.store = s
	a.windsurfSvc = services.NewWindsurfService("")
	// ── 调试日志 ──
	settings := a.store.GetSettings()
	utils.InitDebugLogger(s.DataDir(), settings.DebugLog)
	// ── 创建跨服务的用量跟踪器 ──
	a.usageTracker = services.NewUsageTracker(s.DataDir())

	a.mitmProxy = services.NewMitmProxy(a.windsurfSvc, func(msg string) {
		utils.DLog("%s", msg)
	}, "", a.usageTracker)
	a.mitmProxy.SetOnKeyExhausted(func(apiKey string) {
		utils.DLog("[回调] onKeyExhausted 触发: key=%s...", apiKey[:min(12, len(apiKey))])
		accID := findAccountIDForMITMAPIKey(a.store.GetAllAccounts(), apiKey)
		if accID == "" {
			utils.DLog("[回调] onKeyExhausted: 在号池中未找到匹配 key，跳过")
			return
		}
		utils.DLog("[回调] onKeyExhausted: 匹配到账号 id=%s，刷新额度...", accID[:min(8, len(accID))])
		_ = a.RefreshAccountQuota(accID)
		// ★ 立即触发切号（之前只刷额度不切号，导致 IDE 继续用耗尽账号）
		s := a.store.GetSettings()
		if s.AutoSwitchOnQuotaExhausted {
			utils.DLog("[回调] onKeyExhausted: AutoSwitch=true → 立即MITM轮换")
			if next, err := a.rotateMitmToNextAvailable(accID, s.AutoSwitchPlanFilter); err != nil {
				utils.DLog("[回调] onKeyExhausted: MITM轮换失败: %v", err)
			} else {
				utils.DLog("[回调] onKeyExhausted: MITM轮换成功 → %s", next.Email)
			}
		} else {
			utils.DLog("[回调] onKeyExhausted: AutoSwitchOnQuotaExhausted=false，不切号")
		}
	})
	a.mitmProxy.SetOnKeyAccessDenied(func(apiKey, detail string) {
		utils.DLog("[回调] onKeyAccessDenied 触发: key=%s...", apiKey[:min(12, len(apiKey))])
		a.handleMitmKeyAccessDenied(apiKey, detail)
	})
	a.mitmProxy.SetOnCurrentKeyChanged(func(apiKey, reason string) {
		utils.DLog("[回调] onCurrentKeyChanged 触发: key=%s... reason=%s", apiKey[:min(12, len(apiKey))], reason)
		a.handleMitmCurrentKeyChanged(apiKey, reason)
	})
	a.openaiRelay = services.NewOpenAIRelay(a.mitmProxy, func(msg string) {
		utils.DLog("%s", msg)
	}, "", a.usageTracker)
	a.openaiRelay.SetOnSuccess(func(apiKey string) {
		accounts := a.store.GetAllAccounts()
		accID := findAccountIDForMITMAPIKey(accounts, apiKey)
		if accID == "" {
			return
		}
		_ = a.RefreshAccountQuota(accID)
	})
	a.syncMitmPoolKeys()
	a.syncForgeConfig()
	a.syncStaticCacheConfig()
	if settings.AutoRefreshTokens {
		a.startAutoRefresh()
	}
	if settings.AutoRefreshQuotas {
		a.startAutoQuotaRefresh()
	}
	a.restartQuotaHotPollIfNeeded()
	return nil
}

func (a *App) shouldStartHidden() bool {
	if a.store == nil {
		return a.silentFromFlag && a.supportsTray()
	}
	settings := a.store.GetSettings()
	if !a.supportsTray() {
		return false
	}
	return a.silentFromFlag || settings.SilentStart
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initBackend(); err != nil {
		log.Printf("[WindsurfTools] desktop init: %v", err)
		log.Fatalf("%v", err)
	}
	log.Printf("[WindsurfTools] desktop backend initialized")
	if a.supportsTray() {
		a.startTray()
		log.Printf("[WindsurfTools] tray initialized")
	} else {
		log.Printf("[WindsurfTools] tray unsupported on current platform")
	}
	if a.shouldStartHidden() {
		log.Printf("[WindsurfTools] desktop start hidden")
		go func() {
			time.Sleep(280 * time.Millisecond)
			runtime.WindowHide(a.ctx)
		}()
	} else {
		log.Printf("[WindsurfTools] desktop main window visible")
	}
}

func (a *App) shutdown(ctx context.Context) {
	log.Printf("[WindsurfTools] desktop shutdown requested")
	if a.cancelAutoRefresh != nil {
		a.cancelAutoRefresh()
	}
	if a.cancelAutoQuotaRefresh != nil {
		a.cancelAutoQuotaRefresh()
	}
	a.stopQuotaHotPoll()
	if a.openaiRelay != nil {
		a.openaiRelay.Stop()
	}
	a.cleanupMitmEnvironment()
}
