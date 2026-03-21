package main

import (
	"testing"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
)

func TestDerivePlanNameFromClaims(t *testing.T) {
	cases := []struct {
		name   string
		claims services.JWTClaims
		want   string
	}{
		{name: "pro flag", claims: services.JWTClaims{Pro: true}, want: "Pro"},
		{name: "teams pro", claims: services.JWTClaims{TeamsTier: "TEAMS_TIER_PRO"}, want: "Pro"},
		{name: "teams max", claims: services.JWTClaims{TeamsTier: "TEAMS_TIER_PRO_MAX"}, want: "Max"},
		{name: "trial tier", claims: services.JWTClaims{TeamsTier: "TEAMS_TIER_TRIAL"}, want: "Trial"},
		{name: "enterprise tier", claims: services.JWTClaims{TeamsTier: "TEAMS_TIER_ENTERPRISE"}, want: "Enterprise"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := derivePlanNameFromClaims(&tc.claims, "")
			if got != tc.want {
				t.Fatalf("derivePlanNameFromClaims() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDerivePlanNameFromClaims_AfterExpiry(t *testing.T) {
	past := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	future := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	t.Run("jwt trial end passed -> Free even if pro", func(t *testing.T) {
		got := derivePlanNameFromClaims(&services.JWTClaims{Pro: true, TrialEnd: past}, "")
		if got != "Free" {
			t.Fatalf("got %q, want Free", got)
		}
	})
	t.Run("stored subscription end passed -> Free", func(t *testing.T) {
		got := derivePlanNameFromClaims(&services.JWTClaims{TeamsTier: "TEAMS_TIER_PRO"}, past)
		if got != "Free" {
			t.Fatalf("got %q, want Free", got)
		}
	})
	t.Run("trial tier still active by TrialEnd", func(t *testing.T) {
		got := derivePlanNameFromClaims(&services.JWTClaims{TeamsTier: "TEAMS_TIER_TRIAL", TrialEnd: future}, "")
		if got != "Trial" {
			t.Fatalf("got %q, want Trial", got)
		}
	})
}

func TestParseSubscriptionEndTime(t *testing.T) {
	cases := []struct {
		in     string
		wantOk bool
	}{
		{in: "2026-03-29T08:00:00Z", wantOk: true},
		{in: "2026-03-29T08:00:00.123456789Z", wantOk: true},
		{in: "", wantOk: false},
		{in: "not-a-date", wantOk: false},
	}
	for _, tc := range cases {
		_, ok := parseSubscriptionEndTime(tc.in)
		if ok != tc.wantOk {
			t.Fatalf("parseSubscriptionEndTime(%q) ok=%v, want %v", tc.in, ok, tc.wantOk)
		}
	}
}

func TestApplyAccountProfile(t *testing.T) {
	acc := &models.Account{
		Email:    "",
		Nickname: "",
		PlanName: "unknown",
	}
	daily := 78.0
	weekly := 88.0

	applyAccountProfile(acc, &services.AccountProfile{
		Email:                 "trial@example.com",
		Name:                  "Trial User",
		PlanName:              "Trial",
		TotalCredits:          53,
		UsedCredits:           14,
		DailyQuotaRemaining:   &daily,
		WeeklyQuotaRemaining:  &weekly,
		DailyResetAt:          "2026-03-21T08:00:00Z",
		WeeklyResetAt:         "2026-03-22T08:00:00Z",
		SubscriptionExpiresAt: "2026-03-29T08:00:00Z",
	})

	if acc.Email != "trial@example.com" {
		t.Fatalf("Email = %q", acc.Email)
	}
	if acc.Nickname != "Trial User" {
		t.Fatalf("Nickname = %q", acc.Nickname)
	}
	if acc.PlanName != "Trial" {
		t.Fatalf("PlanName = %q", acc.PlanName)
	}
	if acc.TotalQuota != 53 || acc.UsedQuota != 14 {
		t.Fatalf("quota = %d/%d", acc.UsedQuota, acc.TotalQuota)
	}
	if acc.DailyRemaining != "78.00%" || acc.WeeklyRemaining != "88.00%" {
		t.Fatalf("remaining = %q / %q", acc.DailyRemaining, acc.WeeklyRemaining)
	}
	if acc.DailyResetAt == "" || acc.WeeklyResetAt == "" {
		t.Fatal("reset timestamps should be set")
	}
	if acc.SubscriptionExpiresAt != "2026-03-29T08:00:00Z" {
		t.Fatalf("SubscriptionExpiresAt = %q", acc.SubscriptionExpiresAt)
	}
}

func TestApplyAccountProfile_PreservesLaterJWTExpiryWhenProfileLooksLikePlanStart(t *testing.T) {
	acc := &models.Account{
		Email:                 "trial@example.com",
		Nickname:              "Trial User",
		PlanName:              "Trial",
		CreatedAt:             "2026-03-21T04:17:41+08:00",
		SubscriptionExpiresAt: "2026-03-28T02:01:26Z",
	}

	applyAccountProfile(acc, &services.AccountProfile{
		PlanName:              "Trial",
		SubscriptionExpiresAt: "2026-03-14T02:01:20Z",
	})

	if acc.SubscriptionExpiresAt != "2026-03-28T02:01:26Z" {
		t.Fatalf("SubscriptionExpiresAt = %q, want JWT-derived later end", acc.SubscriptionExpiresAt)
	}
}

func TestChoosePreferredSubscriptionExpiry_UsesRemarkDateHint(t *testing.T) {
	acc := &models.Account{
		CreatedAt:             "2026-03-21T04:16:51+08:00",
		SubscriptionExpiresAt: "2026-02-25T16:41:57Z",
		Remark:                "2026/3/26",
	}

	got := choosePreferredSubscriptionExpiry(acc, "")
	want := "2026-03-26T15:59:59Z"
	if got != want {
		t.Fatalf("choosePreferredSubscriptionExpiry() = %q, want %q", got, want)
	}
}

func TestChoosePreferredSubscriptionExpiry_BlanksBrokenHistoricDateWithoutHint(t *testing.T) {
	acc := &models.Account{
		CreatedAt:             "2026-03-21T04:16:51+08:00",
		SubscriptionExpiresAt: "2026-02-25T16:41:57Z",
	}

	if got := choosePreferredSubscriptionExpiry(acc, ""); got != "" {
		t.Fatalf("choosePreferredSubscriptionExpiry() = %q, want blank for suspicious start-like date", got)
	}
}
