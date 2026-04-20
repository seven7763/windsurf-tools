package main

import (
	"testing"
	"windsurf-tools-wails/backend/models"
)

func TestFindAccountIDForMITMAPIKey(t *testing.T) {
	accounts := []models.Account{
		{ID: "a", Email: "a@example.com", WindsurfAPIKey: "sk-ws-a"},
		{ID: "b", Email: "b@example.com", WindsurfAPIKey: "sk-ws-b"},
	}

	if got := findAccountIDForMITMAPIKey(accounts, "sk-ws-b"); got != "b" {
		t.Fatalf("findAccountIDForMITMAPIKey() = %q, want %q", got, "b")
	}
	if got := findAccountIDForMITMAPIKey(accounts, ""); got != "" {
		t.Fatalf("findAccountIDForMITMAPIKey() with empty key = %q, want empty", got)
	}
}

func TestClampRefreshConcurrentLimit(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{in: -1, want: 1},
		{in: 0, want: 1},
		{in: 1, want: 1},
		{in: 4, want: 4},
		{in: 20, want: 8},
	}

	for _, tc := range cases {
		if got := clampRefreshConcurrentLimit(tc.in); got != tc.want {
			t.Fatalf("clampRefreshConcurrentLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestRunAccountRefreshBatchesPreservesOrder(t *testing.T) {
	accounts := []models.Account{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
		{ID: "d"},
	}

	outcomes := runAccountRefreshBatches(accounts, 2, 0, func(acc models.Account) accountRefreshOutcome {
		return accountRefreshOutcome{label: acc.ID, status: "ok"}
	})

	if len(outcomes) != len(accounts) {
		t.Fatalf("runAccountRefreshBatches() len = %d, want %d", len(outcomes), len(accounts))
	}
	for i, outcome := range outcomes {
		if outcome.label != accounts[i].ID {
			t.Fatalf("runAccountRefreshBatches() label[%d] = %q, want %q", i, outcome.label, accounts[i].ID)
		}
	}
}
