package main

import (
	"fmt"
	"strings"
	"time"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// SwitchAccountLocal writes the selected account into local Windsurf auth state.
func (a *App) SwitchAccountLocal(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("账号 ID 不能为空")
	}
	if a == nil || a.store == nil {
		return "", fmt.Errorf("应用未初始化")
	}

	acc, err := a.store.GetAccount(id)
	if err != nil {
		return "", err
	}
	prepared, err := a.prepareAccountForUsage(acc)
	if err != nil {
		return "", err
	}

	token := strings.TrimSpace(prepared.Token)
	if token == "" {
		return "", fmt.Errorf("该账号没有可写入 Windsurf 的登录态")
	}

	switchSvc := services.NewSwitchService()
	if err := switchSvc.SwitchAccountForceRefresh(token, prepared.Email); err != nil {
		return "", err
	}

	if apiKey := strings.TrimSpace(prepared.WindsurfAPIKey); apiKey != "" {
		if err := services.InjectCodeiumConfig(apiKey); err != nil {
			utils.DLog("[本地登录] %s 注入 Codeium 配置失败: %v", prepared.Email, err)
		}
	}

	prepared.LastLoginAt = time.Now().Format(time.RFC3339)
	if err := a.store.UpdateAccount(prepared); err != nil {
		utils.DLog("[本地登录] %s 更新账号登录时间失败: %v", prepared.Email, err)
	}

	utils.DLog("[本地登录] 已写入 Windsurf 本地登录态: %s", prepared.Email)
	return prepared.Email, nil
}
