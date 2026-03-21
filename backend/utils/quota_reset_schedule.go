package utils

import "time"

// 产品约定：日额度每日东八区 16:00 刷新；周期额度在周末（周六、周日）东八区 16:00 刷新。

// NextDailyQuotaResetRFC3339 返回「当前时刻之后」下一次东八区 16:00 的 UTC RFC3339。
func NextDailyQuotaResetRFC3339(now time.Time) string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	local := now.In(loc)
	y, m, d := local.Date()
	today1600 := time.Date(y, m, d, 16, 0, 0, 0, loc)
	if now.Before(today1600) {
		return today1600.UTC().Format(time.RFC3339)
	}
	return today1600.AddDate(0, 0, 1).UTC().Format(time.RFC3339)
}

// NextWeekendQuotaResetRFC3339 返回「当前时刻之后」下一次周末（周六或周日）东八区 16:00 的 UTC RFC3339。
func NextWeekendQuotaResetRFC3339(now time.Time) string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	local := now.In(loc)
	y, m, d := local.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	for i := 0; i < 14; i++ {
		day := start.AddDate(0, 0, i)
		wd := day.Weekday()
		if wd != time.Saturday && wd != time.Sunday {
			continue
		}
		cand := time.Date(day.Year(), day.Month(), day.Day(), 16, 0, 0, 0, loc)
		if cand.After(now) {
			return cand.UTC().Format(time.RFC3339)
		}
	}
	return start.AddDate(0, 0, 7).UTC().Format(time.RFC3339)
}
