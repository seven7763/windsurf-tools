package main

import (
	"fmt"
	"strings"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

// ═══════════════════════════════════════
// 辅助：账号信息 enrich
// ═══════════════════════════════════════

var asiaShanghaiLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// enrichAccountQuotaOnly 热轮询额度用尽检测：只更新 JWT 解析 + 额度相关 profile，不做 RegisterUser / GetAccountInfo。
func (a *App) enrichAccountQuotaOnly(acc *models.Account) {
	a.enrichAccountQuotaOnlyWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountQuotaOnlyWithService(svc *services.WindsurfService, acc *models.Account) {
	if acc == nil {
		return
	}
	if svc == nil {
		return
	}
	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
		}
		if plan, err := svc.GetPlanStatusJSON(acc.Token); err == nil {
			applyAccountProfile(acc, plan)
		}
	}
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			applyAccountProfile(acc, profile)
		}
	}
	if acc.Nickname == "" && acc.Email != "" {
		acc.Nickname = strings.Split(acc.Email, "@")[0]
	}
	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
}

// enrichAccountInfoLite 批量导入时使用：只做本地 JWT 解析，避免 RegisterUser / GetPlan / GetUserStatus 等串行请求拖死界面。
func (a *App) enrichAccountInfoLite(acc *models.Account) {
	a.enrichAccountInfoLiteWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountInfoLiteWithService(svc *services.WindsurfService, acc *models.Account) {
	if acc == nil {
		return
	}
	if svc == nil {
		return
	}
	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
		}
		// 调用服务端获取计划与额度（JWT 或 Firebase Token 均可尝试，失败忽略）
		if plan, err := svc.GetPlanStatusJSON(acc.Token); err == nil {
			applyAccountProfile(acc, plan)
		}
	}
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			applyAccountProfile(acc, profile)
		}
	}
	if acc.Nickname == "" && acc.Email != "" {
		if at := strings.Index(acc.Email, "@"); at > 0 {
			acc.Nickname = acc.Email[:at]
		}
	}
	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
}

func (a *App) enrichAccountInfo(acc *models.Account) {
	a.enrichAccountInfoWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountInfoWithService(svc *services.WindsurfService, acc *models.Account) {
	if acc == nil {
		return
	}
	if svc == nil {
		return
	}

	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
		}
	}

	if acc.Token != "" && (acc.RefreshToken != "" || acc.Password != "") {
		if email, err := svc.GetAccountInfo(acc.Token); err == nil && email != "" {
			acc.Email = email
		}
		if reg, err := svc.RegisterUser(acc.Token); err == nil && reg != nil && reg.APIKey != "" {
			acc.WindsurfAPIKey = reg.APIKey
		}
	}

	// GetPlanStatusJSON 不依赖 RefreshToken/Password，JWT-only 账号也可调用
	if acc.Token != "" {
		if plan, err := svc.GetPlanStatusJSON(acc.Token); err == nil {
			applyAccountProfile(acc, plan)
		}
	}

	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			applyAccountProfile(acc, profile)
		}
	}

	if acc.Nickname == "" && acc.Email != "" {
		acc.Nickname = strings.Split(acc.Email, "@")[0]
	}

	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
}

func applyJWTClaims(acc *models.Account, claims *services.JWTClaims) {
	if claims == nil {
		return
	}
	if claims.Email != "" {
		acc.Email = claims.Email
	}
	if acc.Nickname == "" && claims.Name != "" {
		acc.Nickname = claims.Name
	}
	// 每次根据 JWT + 本地记录的到期时间重算套餐；到期后不再沿用缓存的 Pro/Trial（后续 GetPlanStatus 可覆盖）
	if plan := derivePlanNameFromClaims(claims, choosePreferredSubscriptionExpiry(acc, "")); plan != "" {
		acc.PlanName = plan
	}
	if claims.TrialEnd != "" {
		acc.SubscriptionExpiresAt = choosePreferredSubscriptionExpiry(acc, claims.TrialEnd)
	}
}

func applyAccountProfile(acc *models.Account, profile *services.AccountProfile) {
	if profile == nil {
		return
	}
	if profile.Email != "" {
		acc.Email = profile.Email
	}
	if profile.Name != "" && (acc.Nickname == "" || acc.Nickname == strings.Split(acc.Email, "@")[0]) {
		acc.Nickname = profile.Name
	}
	if profile.PlanName != "" {
		acc.PlanName = profile.PlanName
	}
	if profile.TotalCredits > 0 || profile.UsedCredits > 0 {
		acc.TotalQuota = profile.TotalCredits
		acc.UsedQuota = profile.UsedCredits
	}
	if profile.DailyQuotaRemaining != nil {
		acc.DailyRemaining = formatQuotaPercent(*profile.DailyQuotaRemaining)
	}
	if profile.WeeklyQuotaRemaining != nil {
		acc.WeeklyRemaining = formatQuotaPercent(*profile.WeeklyQuotaRemaining)
	}
	// 与界面约定一致：日额度每日东八区 16:00；周期额度周末东八区 16:00（不沿用接口里易混淆的 unix）
	now := time.Now()
	acc.DailyResetAt = utils.NextDailyQuotaResetRFC3339(now)
	acc.WeeklyResetAt = utils.NextWeekendQuotaResetRFC3339(now)
	if preferred := choosePreferredSubscriptionExpiry(acc, profile.SubscriptionExpiresAt); preferred != "" {
		acc.SubscriptionExpiresAt = preferred
	} else {
		acc.SubscriptionExpiresAt = ""
	}
}

func choosePreferredSubscriptionExpiry(acc *models.Account, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if acc == nil {
		return candidate
	}

	current := strings.TrimSpace(acc.SubscriptionExpiresAt)
	hint := manualSubscriptionExpiryHint(acc)

	if candidate != "" && !subscriptionEndBeforeAccountCreated(acc, candidate) {
		return candidate
	}
	if current != "" && !subscriptionEndBeforeAccountCreated(acc, current) {
		return current
	}
	if hint != "" {
		return hint
	}
	return ""
}

func manualSubscriptionExpiryHint(acc *models.Account) string {
	if acc == nil {
		return ""
	}
	for _, raw := range []string{acc.Remark, acc.Nickname} {
		if ts, ok := parseManualSubscriptionExpiryHint(raw); ok {
			return ts.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func parseManualSubscriptionExpiryHint(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(strings.Trim(raw, `"`))
	if raw == "" {
		return time.Time{}, false
	}

	layouts := []struct {
		layout   string
		endOfDay bool
	}{
		{layout: "2006/1/2", endOfDay: true},
		{layout: "2006-1-2", endOfDay: true},
		{layout: "2006.1.2", endOfDay: true},
		{layout: "2006/01/02", endOfDay: true},
		{layout: "2006-01-02", endOfDay: true},
		{layout: "2006.01.02", endOfDay: true},
		{layout: "2006/1/2 15:04"},
		{layout: "2006-1-2 15:04"},
		{layout: "2006.1.2 15:04"},
		{layout: "2006/01/02 15:04"},
		{layout: "2006-01-02 15:04"},
		{layout: "2006.01.02 15:04"},
		{layout: "2006/1/2 15:04:05"},
		{layout: "2006-1-2 15:04:05"},
		{layout: "2006.1.2 15:04:05"},
		{layout: "2006/01/02 15:04:05"},
		{layout: "2006-01-02 15:04:05"},
		{layout: "2006.01.02 15:04:05"},
		{layout: "2006-1-2T15:04"},
		{layout: "2006-01-02T15:04"},
		{layout: "2006-1-2T15:04:05"},
		{layout: "2006-01-02T15:04:05"},
	}
	for _, item := range layouts {
		t, err := time.ParseInLocation(item.layout, raw, asiaShanghaiLocation)
		if err != nil {
			continue
		}
		if item.endOfDay {
			t = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, asiaShanghaiLocation)
		}
		return t, true
	}
	return time.Time{}, false
}

func subscriptionEndBeforeAccountCreated(acc *models.Account, value string) bool {
	if acc == nil {
		return false
	}
	tEnd, ok := parseSubscriptionEndTime(value)
	if !ok {
		return false
	}
	tCreated, ok := parseSubscriptionEndTime(strings.TrimSpace(acc.CreatedAt))
	if !ok {
		return false
	}
	return tEnd.Before(tCreated)
}

// subscriptionEndLooksLikeStalePlanStart：同步到的「到期」早于账号写入本工具的时间，且日/周额度显示仍有剩余时，
// 多为 GetPlanStatus.planEnd 表示周期开始而非订阅结束。
func subscriptionEndLooksLikeStalePlanStart(acc *models.Account, profileEnd string) bool {
	if acc == nil {
		return false
	}
	if !subscriptionEndBeforeAccountCreated(acc, profileEnd) {
		return false
	}
	d, dOk := utils.ParseQuotaPercentString(acc.DailyRemaining)
	w, wOk := utils.ParseQuotaPercentString(acc.WeeklyRemaining)
	hasQuota := (dOk && d > 0.0001) || (wOk && w > 0.0001)
	return hasQuota
}

// derivePlanNameFromClaims 从 JWT 推导套餐。storedSubEnd 为 accounts.json 里已有的 subscription_expires_at（JWT 无结束时间时参与判断是否已到期）。
func derivePlanNameFromClaims(claims *services.JWTClaims, storedSubEnd string) string {
	if claims == nil {
		return ""
	}
	end := strings.TrimSpace(claims.TrialEnd)
	if end == "" {
		end = strings.TrimSpace(storedSubEnd)
	}
	if end != "" {
		if t, ok := parseSubscriptionEndTime(end); ok && !t.After(time.Now()) {
			// 订阅/试用已结束：JWT 内 pro/tier 可能尚未刷新，先标为 Free，真实档位由 GetPlanStatus/GetUserStatus 覆盖
			return "Free"
		}
	}
	if claims.Pro {
		return "Pro"
	}
	teamsTier := strings.ToUpper(claims.TeamsTier)
	switch teamsTier {
	case "TEAMS_TIER_PRO":
		return "Pro"
	case "TEAMS_TIER_MAX", "TEAMS_TIER_PRO_MAX", "TEAMS_TIER_ULTIMATE":
		return "Max"
	case "TEAMS_TIER_ENTERPRISE":
		return "Enterprise"
	case "TEAMS_TIER_TEAMS":
		return "Teams"
	case "TEAMS_TIER_TRIAL":
		return "Trial"
	case "TEAMS_TIER_FREE":
		return "Free"
	}
	if strings.Contains(teamsTier, "TRIAL") {
		return "Trial"
	}
	if strings.Contains(teamsTier, "MAX") || strings.Contains(teamsTier, "ULTIMATE") {
		return "Max"
	}
	if strings.Contains(teamsTier, "ENTERPRISE") {
		return "Enterprise"
	}
	if teamsTier == "TEAMS_TIER_TEAMS" || (strings.Contains(teamsTier, "TEAMS") && !strings.Contains(teamsTier, "TIER_FREE") && !strings.Contains(teamsTier, "TIER_PRO") && !strings.Contains(teamsTier, "TIER_TRIAL")) {
		return "Teams"
	}
	if strings.Contains(teamsTier, "PRO") {
		return "Pro"
	}
	if claims.TrialEnd != "" {
		if t, ok := parseSubscriptionEndTime(claims.TrialEnd); ok && t.After(time.Now()) {
			return "Trial"
		}
	}
	return ""
}

func parseSubscriptionEndTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	s = strings.Trim(s, `"`)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, time.DateTime} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func formatQuotaPercent(value float64) string {
	return fmt.Sprintf("%.2f%%", value)
}
