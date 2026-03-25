package utils

import (
	"strconv"
	"strings"

	"windsurf-tools-wails/backend/models"
)

// ParseQuotaPercentString 解析账号卡片上的日/周剩余字符串（如 "0.00%"）。
func ParseQuotaPercentString(s string) (v float64, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// AccountQuotaExhausted 根据已同步的额度字段判断是否「可用配额见底」。
// 规则：月/积分型 total>0 且 used>=total；或日、周剩余百分比任一≤0 即视为用尽
// （服务端在 weekly=0 时就会拒绝请求，即使 daily 仍有余量）。
func AccountQuotaExhausted(acc *models.Account) bool {
	if acc == nil {
		return false
	}
	if acc.TotalQuota > 0 && acc.UsedQuota >= acc.TotalQuota {
		return true
	}
	d, dOk := ParseQuotaPercentString(acc.DailyRemaining)
	w, wOk := ParseQuotaPercentString(acc.WeeklyRemaining)
	if dOk && d <= 0.0001 {
		return true
	}
	if wOk && w <= 0.0001 {
		return true
	}
	return false
}
