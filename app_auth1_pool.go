package main

import (
	"strings"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

func maybeBackfillAuth1SessionKey(svc *services.WindsurfService, acc *models.Account, label string) {
	if svc == nil || acc == nil {
		return
	}
	if strings.TrimSpace(acc.WindsurfAPIKey) != "" {
		return
	}
	token := strings.TrimSpace(acc.Token)
	if !strings.HasPrefix(token, "auth1_") {
		return
	}
	resp, err := svc.WindsurfPostAuth(token)
	if err != nil {
		utils.DLog("[auth1Pool] %s WindsurfPostAuth失败: %v", label, err)
		return
	}
	acc.WindsurfAPIKey = strings.TrimSpace(resp.SessionKey)
	if acc.WindsurfAPIKey != "" {
		utils.DLog("[auth1Pool] %s 获得SessionKey=%s...", label, acc.WindsurfAPIKey[:min(24, len(acc.WindsurfAPIKey))])
	}
}
