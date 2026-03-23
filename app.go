package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/store"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx                    context.Context
	store                  *store.Store
	windsurfSvc            *services.WindsurfService
	switchSvc              *services.SwitchService
	patchSvc               *services.PatchService
	mitmProxy              *services.MitmProxy
	openaiRelay            *services.OpenAIRelay
	cancelAutoRefresh      context.CancelFunc
	cancelAutoQuotaRefresh context.CancelFunc
	cancelQuotaHotPoll     context.CancelFunc
	lastQuotaHotSwitch     time.Time
	lastQuotaHotSwitchMu   sync.Mutex
	tokenRefreshRunMu      sync.Mutex
	quotaRefreshRunMu      sync.Mutex
	mu                     sync.Mutex
	cleanupMitmOnExitFn    func() error
	closeDesktopLogFn      func()
	activateExistingAppFn  func(showToolbar bool)
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
	settings := a.store.GetSettings()
	proxyURL := ""
	if settings.ProxyEnabled && settings.ProxyURL != "" {
		proxyURL = settings.ProxyURL
	}
	a.windsurfSvc = services.NewWindsurfService(proxyURL)
	a.switchSvc = services.NewSwitchService()
	a.patchSvc = services.NewPatchService()
	a.mitmProxy = services.NewMitmProxy(a.windsurfSvc, func(msg string) {
		fmt.Println(msg)
	}, proxyURL)
	a.openaiRelay = services.NewOpenAIRelay(a.mitmProxy, func(msg string) {
		fmt.Println(msg)
	}, proxyURL)
	a.syncMitmPoolKeys()
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
	if settings.ShowDesktopToolbar {
		return a.silentFromFlag || settings.SilentStart
	}
	if !a.supportsTray() {
		return false
	}
	return a.silentFromFlag || settings.SilentStart
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logPath, closeLog, logErr := setupDesktopRuntimeLogging()
	if closeLog != nil {
		a.closeDesktopLogFn = closeLog
	}
	if logErr != nil {
		log.Printf("[WindsurfTools] desktop log setup: %v", logErr)
	} else {
		log.Printf("[WindsurfTools] desktop session start: %s", logPath)
	}
	if err := a.initBackend(); err != nil {
		log.Printf("[WindsurfTools] desktop init: %v", err)
		if a.closeDesktopLogFn != nil {
			a.closeDesktopLogFn()
			a.closeDesktopLogFn = nil
		}
		log.Fatalf("%v", err)
	}
	log.Printf("[WindsurfTools] desktop backend initialized")
	if a.supportsTray() {
		a.startTray()
		log.Printf("[WindsurfTools] tray initialized")
	} else {
		log.Printf("[WindsurfTools] tray unsupported on current platform")
	}
	settings := a.store.GetSettings()
	if a.shouldStartHidden() {
		log.Printf("[WindsurfTools] desktop start hidden")
		if settings.ShowDesktopToolbar {
			// 静默启动但启用桌面工具栏：先隐藏避免闪全屏主界面，前端就绪后会 ApplyToolbarLayout + WindowShow 显示小窗
			runtime.WindowHide(a.ctx)
		} else {
			go func() {
				time.Sleep(280 * time.Millisecond)
				runtime.WindowHide(a.ctx)
			}()
		}
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
	if a.closeDesktopLogFn != nil {
		a.closeDesktopLogFn()
		a.closeDesktopLogFn = nil
	}
}
