package main

import (
	"log"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"windsurf-tools-wails/backend/services"
)

func (a *App) cleanupMitmEnvironment() {
	if !a.shouldCleanupMitmEnvironment() {
		return
	}
	cleanup := a.cleanupMitmOnExitFn
	if cleanup == nil {
		cleanup = a.TeardownMitm
	}
	if err := cleanup(); err != nil {
		log.Printf("[WindsurfTools] MITM cleanup: %v", err)
	}
}

func (a *App) shouldCleanupMitmEnvironment() bool {
	if a.cleanupMitmOnExitFn != nil {
		return true
	}
	if a.mitmProxy != nil && a.mitmProxy.Status().Running {
		return true
	}
	if services.IsHostsMapped(services.TargetDomain) {
		return true
	}
	return services.IsCAInstalled()
}

func (a *App) activateExistingWindow() {
	if a.activateExistingAppFn != nil {
		a.activateExistingAppFn()
		return
	}
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

func (a *App) onSecondInstanceLaunch(options.SecondInstanceData) {
	go a.activateExistingWindow()
}
