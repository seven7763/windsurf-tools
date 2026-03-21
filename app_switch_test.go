package main

import (
	"testing"
	"windsurf-tools-wails/backend/models"
)

func TestPickNextSwitchableAccount_RespectsFilterAndSkipsExhausted(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", Token: "tok-current", PlanName: "Trial"},
		{ID: "trial-empty", Email: "trial-empty@example.com", Token: "tok-trial-empty", PlanName: "Trial", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "pro-ok", Email: "pro-ok@example.com", Token: "tok-pro-ok", PlanName: "Pro", DailyRemaining: "32.00%"},
		{ID: "trial-ok", Email: "trial-ok@example.com", Token: "tok-trial-ok", PlanName: "Trial", DailyRemaining: "88.00%"},
	}

	got, err := pickNextSwitchableAccount(accounts, "current", "trial")
	if err != nil {
		t.Fatalf("pickNextSwitchableAccount() error = %v", err)
	}
	if got.ID != "trial-ok" {
		t.Fatalf("pickNextSwitchableAccount() picked %q, want %q", got.ID, "trial-ok")
	}
}

func TestPickNextSwitchableAccount_SkipsInvalidCandidates(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", Token: "tok-current", PlanName: "Pro"},
		{ID: "no-token", Email: "no-token@example.com", PlanName: "Pro"},
		{ID: "disabled", Email: "disabled@example.com", Token: "tok-disabled", PlanName: "Pro", Status: "disabled"},
		{ID: "expired", Email: "expired@example.com", Token: "tok-expired", PlanName: "Pro", Status: "expired"},
		{ID: "ok", Email: "ok@example.com", Token: "tok-ok", PlanName: "Pro", DailyRemaining: "12.00%"},
	}

	got, err := pickNextSwitchableAccount(accounts, "current", "all")
	if err != nil {
		t.Fatalf("pickNextSwitchableAccount() error = %v", err)
	}
	if got.ID != "ok" {
		t.Fatalf("pickNextSwitchableAccount() picked %q, want %q", got.ID, "ok")
	}
}

func TestPickNextSwitchableAccount_ReturnsErrorWhenNoCandidateMatches(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", Token: "tok-current", PlanName: "Teams"},
		{ID: "free", Email: "free@example.com", Token: "tok-free", PlanName: "Free", DailyRemaining: "99.00%"},
	}

	if _, err := pickNextSwitchableAccount(accounts, "current", "trial"); err == nil {
		t.Fatal("pickNextSwitchableAccount() expected error when nothing matches plan filter")
	}
}
