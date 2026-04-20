package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// onBeforeClose 关闭窗口时：
// - MinimizeToTray=true 且托盘可用 → 隐藏窗口到托盘（保持后台运行）
// - 否则 → 直接退出进程（不留后台残余）
func (a *App) onBeforeClose(ctx context.Context) bool {
	if a.store == nil {
		return false // 允许关闭
	}
	if a.store.GetSettings().MinimizeToTray && a.supportsTray() {
		runtime.WindowHide(ctx)
		return true // 阻止关闭，隐藏到托盘
	}
	// 直接退出：先清理 MITM 环境
	a.cleanupMitmEnvironment()
	return false // 允许关闭
}
