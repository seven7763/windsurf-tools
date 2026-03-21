package utils

import (
	"testing"

	"windsurf-tools-wails/backend/models"
)

func TestAccountQuotaExhausted(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		acc  models.Account
		want bool
	}{
		{"nil guard", models.Account{}, false},
		{"monthly cap", models.Account{TotalQuota: 100, UsedQuota: 100}, true},
		{"monthly not full", models.Account{TotalQuota: 100, UsedQuota: 99}, false},
		{"daily zero only", models.Account{DailyRemaining: "0.00%"}, true},
		{"daily partial", models.Account{DailyRemaining: "12.00%"}, false},
		{"both zero", models.Account{DailyRemaining: "0%", WeeklyRemaining: "0.00%"}, true},
		{"daily zero weekly ok", models.Account{DailyRemaining: "0%", WeeklyRemaining: "50%"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			acc := tc.acc
			if got := AccountQuotaExhausted(&acc); got != tc.want {
				t.Fatalf("AccountQuotaExhausted = %v, want %v", got, tc.want)
			}
		})
	}
}
