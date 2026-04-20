//go:build windows

package main

import (
	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// startTray 在后台线程运行系统托盘（当前仅 Windows 发布包启用）。
func (a *App) startTray() {
	go func() {
		systray.Run(a.onTrayReady, func() {})
	}()
}

func traySupported() bool { return true }

func (a *App) onTrayReady() {
	systray.SetIcon(currentTrayIcon())
	systray.SetTooltip("Windsurf Tools — 号池 · MITM · 切号")

	mShow := systray.AddMenuItem("显示主窗口", "恢复完整界面")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出并恢复环境", "完全退出应用，并清理 MITM hosts / 证书 / Codeium 配置")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				a.activateExistingWindow()
			case <-mQuit.ClickedCh:
				systray.Quit()
				if a.ctx != nil {
					runtime.Quit(a.ctx)
				}
				return
			}
		}
	}()
}
