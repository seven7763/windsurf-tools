package services

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func intPtr(v int) *int { return &v }

func TestDecodeJWTClaims(t *testing.T) {
	payload, err := json.Marshal(map[string]interface{}{
		"email":                         "trial@example.com",
		"name":                          "Trial User",
		"pro":                           false,
		"teams_tier":                    "TEAMS_TIER_TRIAL",
		"windsurf_pro_trial_end_time":   "\"2026-03-29T08:00:00Z\"",
		"max_num_premium_chat_messages": 42,
		"auth_uid":                      "auth-uid",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	token := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
	claims, err := (&WindsurfService{}).DecodeJWTClaims(token)
	if err != nil {
		t.Fatalf("DecodeJWTClaims() error = %v", err)
	}

	if claims.Email != "trial@example.com" {
		t.Fatalf("Email = %q, want %q", claims.Email, "trial@example.com")
	}
	if claims.Name != "Trial User" {
		t.Fatalf("Name = %q, want %q", claims.Name, "Trial User")
	}
	if claims.TeamsTier != "TEAMS_TIER_TRIAL" {
		t.Fatalf("TeamsTier = %q", claims.TeamsTier)
	}
	if claims.TrialEnd != "2026-03-29T08:00:00Z" {
		t.Fatalf("TrialEnd = %q", claims.TrialEnd)
	}
	if claims.MaxPremiumChatMessages != 42 {
		t.Fatalf("MaxPremiumChatMessages = %d", claims.MaxPremiumChatMessages)
	}
}

func TestParsePlanStatusPayload(t *testing.T) {
	daily := 78.0
	weekly := 88.0
	profile := parsePlanStatusPayload(planStatusPayload{
		AvailablePromptCredits:      5000,
		AvailableFlowCredits:        300,
		UsedPromptCredits:           intPtr(1200),
		UsedUsageCredits:            intPtr(200),
		DailyQuotaRemainingPercent:  &daily,
		WeeklyQuotaRemainingPercent: &weekly,
		DailyQuotaResetAtUnix:       json.RawMessage(`"1774080000"`),
		WeeklyQuotaResetAtUnix:      json.RawMessage(`"1774166400"`),
		PlanEnd:                     json.RawMessage(`"2026-03-29T08:00:00Z"`),
		PlanInfo: struct {
			PlanName        string `json:"planName"`
			BillingStrategy string `json:"billingStrategy"`
		}{
			PlanName:        "Pro",
			BillingStrategy: "BILLING_STRATEGY_SUBSCRIPTION",
		},
	})

	if profile.SubscriptionExpiresAt != "2026-03-29T08:00:00Z" {
		t.Fatalf("SubscriptionExpiresAt = %q, want RFC3339 from planEnd string", profile.SubscriptionExpiresAt)
	}

	if profile.PlanName != "Pro" {
		t.Fatalf("PlanName = %q, want %q", profile.PlanName, "Pro")
	}
	if profile.TotalCredits != 53 {
		t.Fatalf("TotalCredits = %d, want 53", profile.TotalCredits)
	}
	if profile.UsedCredits != 14 {
		t.Fatalf("UsedCredits = %d, want 14", profile.UsedCredits)
	}
	if profile.RemainingCredits != 39 {
		t.Fatalf("RemainingCredits = %d, want 39", profile.RemainingCredits)
	}
	if profile.DailyQuotaRemaining == nil || *profile.DailyQuotaRemaining != 78 {
		t.Fatalf("DailyQuotaRemaining = %#v", profile.DailyQuotaRemaining)
	}
	if profile.WeeklyQuotaRemaining == nil || *profile.WeeklyQuotaRemaining != 88 {
		t.Fatalf("WeeklyQuotaRemaining = %#v", profile.WeeklyQuotaRemaining)
	}
}

func TestPickSubscriptionExpiresFromPlanStatus_StartVsEnd(t *testing.T) {
	ps := planStatusPayload{
		PlanStart: json.RawMessage(`"2026-03-01T08:00:00Z"`),
		PlanEnd:   json.RawMessage(`"2026-04-15T08:00:00Z"`),
	}
	got := pickSubscriptionExpiresFromPlanStatus(ps)
	want := "2026-04-15T08:00:00Z"
	if got != want {
		t.Fatalf("pickSubscriptionExpiresFromPlanStatus = %q, want %q", got, want)
	}
}

func TestPickSubscriptionExpiresFromPlanStatus_ExplicitPeriodEnd(t *testing.T) {
	ps := planStatusPayload{
		PlanStart:             json.RawMessage(`"2026-03-01T08:00:00Z"`),
		PlanEnd:               json.RawMessage(`"2026-03-10T08:00:00Z"`),
		SubscriptionPeriodEnd: json.RawMessage(`"2026-06-01T00:00:00Z"`),
	}
	got := pickSubscriptionExpiresFromPlanStatus(ps)
	if got != "2026-06-01T00:00:00Z" {
		t.Fatalf("pickSubscriptionExpiresFromPlanStatus = %q, want explicit subscriptionPeriodEnd", got)
	}
}

func TestParsePlanStatusPayload_PlanEndUnix(t *testing.T) {
	sec := int64(1773831056)
	profile := parsePlanStatusPayload(planStatusPayload{
		PlanEnd: json.RawMessage(`1773831056`),
	})
	want := time.Unix(sec, 0).UTC().Format(time.RFC3339)
	if profile.SubscriptionExpiresAt != want {
		t.Fatalf("SubscriptionExpiresAt = %q, want %q (unix planEnd)", profile.SubscriptionExpiresAt, want)
	}
}

func TestNormalizePlanName(t *testing.T) {
	cases := map[string]string{
		"trial":        "Trial",
		"pro trial":    "Trial",
		"pro":          "Pro",
		"max":          "Max",
		"pro max":      "Max",
		"pro_ultimate": "Max",
		"ultimate":     "Max",
		"teams":        "Teams",
		"enterprise":   "Enterprise",
		"basic":        "Basic",
	}

	for input, want := range cases {
		if got := normalizePlanName(input); got != want {
			t.Fatalf("normalizePlanName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseUserStatusPayload(t *testing.T) {
	planInfo := pbBytes(2, []byte("Trial"))
	plan := append([]byte{}, pbBytes(1, planInfo)...)
	plan = append(plan, pbVarint(14, 78)...)
	plan = append(plan, pbVarint(15, 88)...)
	plan = append(plan, pbVarint(17, 1774080000)...)
	plan = append(plan, pbVarint(18, 1774166400)...)

	user := append([]byte{}, pbBytes(3, []byte("Trial User"))...)
	user = append(user, pbVarint(6, 9)...)
	user = append(user, pbBytes(7, []byte("trial@example.com"))...)
	user = append(user, pbBytes(13, plan)...)
	user = append(user, pbBytes(34, pbTimestamp(1773831056))...)

	payload := pbBytes(1, user)
	profile := parseUserStatusPayload(payload)
	if profile == nil {
		t.Fatal("parseUserStatusPayload() returned nil")
	}

	if profile.Email != "trial@example.com" {
		t.Fatalf("Email = %q", profile.Email)
	}
	if profile.PlanName != "Trial" {
		t.Fatalf("PlanName = %q", profile.PlanName)
	}
	if profile.Tier != 9 {
		t.Fatalf("Tier = %d, want 9", profile.Tier)
	}
	if profile.DailyQuotaRemaining == nil || *profile.DailyQuotaRemaining != 78 {
		t.Fatalf("DailyQuotaRemaining = %#v", profile.DailyQuotaRemaining)
	}
	if profile.WeeklyQuotaRemaining == nil || *profile.WeeklyQuotaRemaining != 88 {
		t.Fatalf("WeeklyQuotaRemaining = %#v", profile.WeeklyQuotaRemaining)
	}
	if profile.SubscriptionExpiresAt == "" {
		t.Fatal("SubscriptionExpiresAt should not be empty")
	}
}

func pbVarint(field int, value uint64) []byte {
	out := encodeTestVarint(uint64(field << 3))
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	out = append(out, byte(value))
	return out
}

func pbBytes(field int, value []byte) []byte {
	out := encodeTestVarint(uint64((field << 3) | 2))
	out = append(out, encodeTestVarint(uint64(len(value)))...)
	out = append(out, value...)
	return out
}

func pbTimestamp(seconds uint64) []byte {
	out := pbVarint(1, seconds)
	out = append(out, pbVarint(2, 0)...)
	return out
}

func encodeTestVarint(value uint64) []byte {
	var out []byte
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	out = append(out, byte(value))
	return out
}
