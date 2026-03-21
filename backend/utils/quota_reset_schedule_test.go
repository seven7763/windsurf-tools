package utils

import (
	"testing"
	"time"
)

func TestNextDailyQuotaResetRFC3339_before1600(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 3, 22, 10, 0, 0, 0, loc)
	got := NextDailyQuotaResetRFC3339(now)
	want := time.Date(2026, 3, 22, 16, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNextDailyQuotaResetRFC3339_after1600(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 3, 22, 18, 0, 0, 0, loc)
	got := NextDailyQuotaResetRFC3339(now)
	want := time.Date(2026, 3, 23, 16, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNextWeekendQuotaResetRFC3339_SaturdayMorning(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 2026-03-21 是周六
	now := time.Date(2026, 3, 21, 10, 0, 0, 0, loc)
	got := NextWeekendQuotaResetRFC3339(now)
	want := time.Date(2026, 3, 21, 16, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNextWeekendQuotaResetRFC3339_SaturdayAfter1600(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 3, 21, 17, 0, 0, 0, loc)
	got := NextWeekendQuotaResetRFC3339(now)
	want := time.Date(2026, 3, 22, 16, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
