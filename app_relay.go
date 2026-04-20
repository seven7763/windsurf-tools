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

// GetUsageRecords 获取全局调用记录
func (a *App) GetUsageRecords(limit int) []services.UsageRecord {
	if a.usageTracker == nil {
		return nil
	}
	return a.usageTracker.GetRecords(limit)
}

// GetUsageSummary 获取全局调用统计汇总
func (a *App) GetUsageSummary() services.UsageSummary {
	if a.usageTracker == nil {
		return services.UsageSummary{}
	}
	return a.usageTracker.GetSummary()
}

// DeleteAllUsage 清空所有调用记录
func (a *App) DeleteAllUsage() int {
	if a.usageTracker == nil {
		return 0
	}
	// Note: usageTracker now handles the DB truncation.
	return a.usageTracker.DeleteAll()
}
