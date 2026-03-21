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
	cancelAutoRefresh      context.CancelFunc
	cancelAutoQuotaRefresh context.CancelFunc
	cancelQuotaHotPoll     context.CancelFunc
	lastQuotaHotSwitch     time.Time
	lastQuotaHotSwitchMu   sync.Mutex
	mu                     sync.Mutex
	cleanupMitmOnExitFn    func() error
	activateExistingAppFn  func(showToolbar bool)
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
		return a.silentFromFlag
	}
	return a.silentFromFlag || a.store.GetSettings().SilentStart
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initBackend(); err != nil {
		log.Fatalf("%v", err)
	}
	a.startTray()
	settings := a.store.GetSettings()
	if a.shouldStartHidden() {
		if settings.ShowDesktopToolbar {
			// 静默启动但启用桌面工具栏：先隐藏避免闪全屏主界面，前端就绪后会 ApplyToolbarLayout + WindowShow 显示小窗
			runtime.WindowHide(a.ctx)
		} else {
			go func() {
				time.Sleep(280 * time.Millisecond)
				runtime.WindowHide(a.ctx)
			}()
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.cancelAutoRefresh != nil {
		a.cancelAutoRefresh()
	}
	if a.cancelAutoQuotaRefresh != nil {
		a.cancelAutoQuotaRefresh()
	}
	a.stopQuotaHotPoll()
	a.cleanupMitmEnvironment()
}
