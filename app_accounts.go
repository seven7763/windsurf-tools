package main

import (
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/utils"
)

func (a *App) GetAllAccounts() []models.Account {
	accounts := a.store.GetAllAccounts()
	for i := range accounts {
		accounts[i].SubscriptionExpiresAt = choosePreferredSubscriptionExpiry(&accounts[i], "")
	}
	return accounts
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
	a.syncMitmPoolKeys() // 删除后同步号池
	return deleted, nil
}
