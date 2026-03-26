package main

import (
	"testing"
	"time"
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
		{ID: "api-key-only", Email: "api-key-only@example.com", WindsurfAPIKey: "sk-ws-1", PlanName: "Pro", DailyRemaining: "52.00%"},
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

func TestAccountEligibleForUsage(t *testing.T) {
	cases := []struct {
		name          string
		acc           models.Account
		requireAPIKey bool
		want          bool
	}{
		{
			name: "api key candidate is allowed",
			acc:  models.Account{ID: "api", Email: "api@example.com", WindsurfAPIKey: "sk-ws-1", PlanName: "Pro", DailyRemaining: "88.00%"},
			want: true,
		},
		{
			name: "refresh token candidate is allowed",
			acc:  models.Account{ID: "refresh", Email: "refresh@example.com", RefreshToken: "rt-1", PlanName: "Pro", DailyRemaining: "33.00%"},
			want: true,
		},
		{
			name: "exhausted account is denied",
			acc:  models.Account{ID: "empty", Email: "empty@example.com", Token: "tok", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
			want: false,
		},
		{
			name:          "mitm requires api key",
			acc:           models.Account{ID: "jwt", Email: "jwt@example.com", Token: "tok", PlanName: "Pro", DailyRemaining: "50.00%"},
			requireAPIKey: true,
			want:          false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountEligibleForUsage(&tc.acc, "all", tc.requireAPIKey); got != tc.want {
				t.Fatalf("accountEligibleForUsage() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCollectEligibleMitmAPIKeysSkipsExhaustedAndDuplicates(t *testing.T) {
	accounts := []models.Account{
		{ID: "pro-ok", Email: "pro-ok@example.com", WindsurfAPIKey: "sk-ws-ok", PlanName: "Pro", DailyRemaining: "42.00%"},
		{ID: "pro-empty", Email: "pro-empty@example.com", WindsurfAPIKey: "sk-ws-empty", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "dup", Email: "dup@example.com", WindsurfAPIKey: "sk-ws-ok", PlanName: "Pro", DailyRemaining: "25.00%"},
		{ID: "trial", Email: "trial@example.com", WindsurfAPIKey: "sk-ws-trial", PlanName: "Trial", DailyRemaining: "50.00%"},
	}

	got := collectEligibleMitmAPIKeys(accounts, "pro")
	if len(got) != 1 {
		t.Fatalf("collectEligibleMitmAPIKeys() len = %d, want 1", len(got))
	}
	if got[0] != "sk-ws-ok" {
		t.Fatalf("collectEligibleMitmAPIKeys() first = %q, want %q", got[0], "sk-ws-ok")
	}
}

func TestPickNextMitmSwitchableAccount_RequiresAPIKey(t *testing.T) {
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", WindsurfAPIKey: "sk-ws-current", PlanName: "Pro", DailyRemaining: "60.00%"},
		{ID: "token-only", Email: "token-only@example.com", Token: "tok-only", PlanName: "Pro", DailyRemaining: "88.00%"},
		{ID: "empty", Email: "empty@example.com", WindsurfAPIKey: "sk-ws-empty", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{ID: "next", Email: "next@example.com", WindsurfAPIKey: "sk-ws-next", PlanName: "Pro", DailyRemaining: "42.00%"},
	}

	got, err := pickNextMitmSwitchableAccount(accounts, "current", "pro")
	if err != nil {
		t.Fatalf("pickNextMitmSwitchableAccount() error = %v", err)
	}
	if got.ID != "next" {
		t.Fatalf("pickNextMitmSwitchableAccount() picked %q, want %q", got.ID, "next")
	}
}

func TestPickNextMitmSwitchableAccount_IncludesStaleQuotaCandidateAfterReset(t *testing.T) {
	pastReset := time.Now().Add(-30 * time.Minute).Format(time.RFC3339)
	oldSync := time.Now().Add(-5 * time.Hour).Format(time.RFC3339)
	accounts := []models.Account{
		{ID: "current", Email: "current@example.com", WindsurfAPIKey: "sk-ws-current", PlanName: "Pro", DailyRemaining: "0.00%", WeeklyRemaining: "0.00%"},
		{
			ID:              "stale-reset",
			Email:           "stale-reset@example.com",
			WindsurfAPIKey:  "sk-ws-stale",
			PlanName:        "Pro",
			DailyRemaining:  "0.00%",
			WeeklyRemaining: "0.00%",
			DailyResetAt:    pastReset,
			LastQuotaUpdate: oldSync,
		},
	}

	got, err := pickNextMitmSwitchableAccount(accounts, "current", "pro")
	if err != nil {
		t.Fatalf("pickNextMitmSwitchableAccount() error = %v", err)
	}
	if got.ID != "stale-reset" {
		t.Fatalf("pickNextMitmSwitchableAccount() picked %q, want %q", got.ID, "stale-reset")
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
