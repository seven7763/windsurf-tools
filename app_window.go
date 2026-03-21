package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	toolbarW = 432
	// 该窗口保留系统标题栏，需预留非客户区高度，否则内容会被裁成截图里的白条。
	toolbarH = 116
)

// ApplyToolbarLayout 切换为桌面工具栏：小窗口、置顶、靠右下角（与微软桌面组件类似）。
func (a *App) ApplyToolbarLayout(show bool) error {
	if a.ctx == nil {
		return nil
	}
	if !show {
		return a.RestoreMainWindowLayout()
	}
	runtime.WindowSetMinSize(a.ctx, 360, 108)
	runtime.WindowSetMaxSize(a.ctx, 900, 180)
	runtime.WindowSetSize(a.ctx, toolbarW, toolbarH)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err == nil && len(screens) > 0 {
		var sc *runtime.Screen
		for i := range screens {
			if screens[i].IsCurrent || screens[i].IsPrimary {
				sc = &screens[i]
				break
			}
		}
		if sc == nil {
			sc = &screens[0]
		}
		sw := sc.Size.Width
		sh := sc.Size.Height
		if sw == 0 {
			sw = sc.Width
			sh = sc.Height
		}
		const margin = 12
		const taskbarReserve = 56
		x := sw - toolbarW - margin
		y := sh - toolbarH - taskbarReserve
		if x < margin {
			x = margin
		}
		if y < margin {
			y = margin
		}
		runtime.WindowSetPosition(a.ctx, x, y)
	}
	runtime.EventsEmit(a.ctx, "toolbar:set", true)
	return nil
}

// RestoreMainWindowLayout 恢复主窗口默认尺寸与居中（退出工具栏模式）。
func (a *App) RestoreMainWindowLayout() error {
	if a.ctx == nil {
		return nil
	}
	runtime.WindowSetMinSize(a.ctx, 800, 560)
	runtime.WindowSetMaxSize(a.ctx, 0, 0)
	runtime.WindowSetSize(a.ctx, 1100, 750)
	runtime.WindowSetAlwaysOnTop(a.ctx, false)
	runtime.WindowCenter(a.ctx)
	runtime.EventsEmit(a.ctx, "toolbar:set", false)
	return nil
}

// onBeforeClose 关闭窗口时可选最小化到托盘而非退出。
func (a *App) onBeforeClose(ctx context.Context) bool {
	if a.store == nil {
		return false
	}
	if a.store.GetSettings().MinimizeToTray {
		runtime.WindowHide(ctx)
		return true
	}
	return false
}
