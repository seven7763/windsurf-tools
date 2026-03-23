package main

import "windsurf-tools-wails/backend/services"

// StartOpenAIRelay 启动 OpenAI 兼容中转服务器
func (a *App) StartOpenAIRelay(port int, secret string) error {
	if a.openaiRelay == nil {
		return nil
	}
	return a.openaiRelay.Start(port, secret)
}

// StopOpenAIRelay 停止中转服务器
func (a *App) StopOpenAIRelay() error {
	if a.openaiRelay == nil {
		return nil
	}
	return a.openaiRelay.Stop()
}

// GetOpenAIRelayStatus 获取中转服务器状态
func (a *App) GetOpenAIRelayStatus() services.OpenAIRelayStatus {
	if a.openaiRelay == nil {
		return services.OpenAIRelayStatus{}
	}
	return a.openaiRelay.Status()
}
