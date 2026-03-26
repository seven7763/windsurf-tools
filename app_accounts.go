package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) GetAllAccounts() []models.Account {
	accounts := a.store.GetAllAccounts()
	for i := range accounts {
		normalizeAccountPlanAndStatus(&accounts[i])
	}
	return accounts
}

func (a *App) GetAccount(id string) (*models.Account, error) {
	acc, err := a.store.GetAccount(id)
	if err != nil {
		return nil, err
	}
	normalizeAccountPlanAndStatus(&acc)
	return &acc, nil
}

func (a *App) DeleteAccount(id string) error {
	if err := a.store.DeleteAccount(id); err != nil {
		return err
	}
	a.syncMitmPoolKeys()
	return nil
}

func (a *App) DeleteExpiredAccounts() (int, error) {
	accounts := a.store.GetAllAccounts()
	now := time.Now()
	deleted := 0
	for _, acc := range accounts {
		acc.SubscriptionExpiresAt = choosePreferredSubscriptionExpiry(&acc, "")
		if acc.SubscriptionExpiresAt == "" {
			continue
		}
		t, ok := parseSubscriptionEndTime(acc.SubscriptionExpiresAt)
		if !ok || !t.Before(now) {
			continue
		}
		if err := a.store.DeleteAccount(acc.ID); err == nil {
			deleted++
		}
	}
	a.syncMitmPoolKeys()
	return deleted, nil
}

// DeleteFreePlanAccounts 删除计划归类为 free 或 unknown 的账号
func (a *App) DeleteFreePlanAccounts() (int, error) {
	accounts := a.store.GetAllAccounts()
	deleted := 0
	for _, acc := range accounts {
		tone := utils.PlanTone(acc.PlanName)
		if tone != "free" && tone != "unknown" {
			continue
		}
		if err := a.store.DeleteAccount(acc.ID); err == nil {
			deleted++
		}
	}
	a.syncMitmPoolKeys()
	return deleted, nil
}

// DeleteAccountsByGroup 按套餐分组删除账号（planTone: pro/trial/free/team/enterprise/max/unknown）
func (a *App) DeleteAccountsByGroup(planTone string) (int, error) {
	planTone = strings.TrimSpace(strings.ToLower(planTone))
	if planTone == "" {
		return 0, fmt.Errorf("套餐分组不能为空")
	}
	accounts := a.store.GetAllAccounts()
	deleted := 0
	for _, acc := range accounts {
		if utils.PlanTone(acc.PlanName) != planTone {
			continue
		}
		if err := a.store.DeleteAccount(acc.ID); err == nil {
			deleted++
		}
	}
	a.syncMitmPoolKeys()
	return deleted, nil
}

// ExportAccountsByGroup 按套餐分组导出账号到用户选择的 JSON 文件，返回保存路径
func (a *App) ExportAccountsByGroup(planTone string) (string, error) {
	planTone = strings.TrimSpace(strings.ToLower(planTone))
	if planTone == "" {
		return "", fmt.Errorf("套餐分组不能为空")
	}
	accounts := a.store.GetAllAccounts()
	var filtered []models.Account
	for _, acc := range accounts {
		if utils.PlanTone(acc.PlanName) != planTone {
			continue
		}
		filtered = append(filtered, acc)
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("套餐「%s」中没有账号", planTone)
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return "", fmt.Errorf("导出序列化失败: %w", err)
	}
	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           fmt.Sprintf("导出「%s」账号", planTone),
		DefaultFilename: fmt.Sprintf("accounts_%s.json", planTone),
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON 文件 (*.json)", Pattern: "*.json"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("打开保存对话框失败: %w", err)
	}
	if savePath == "" {
		return "", fmt.Errorf("已取消导出")
	}
	if err := os.WriteFile(savePath, data, 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return savePath, nil
}
