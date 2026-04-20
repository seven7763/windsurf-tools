package main

import (
	"fmt"
	"log"
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
// 返回 true 表示至少获取到了部分有效额度数据。
func (a *App) enrichAccountQuotaOnly(acc *models.Account) bool {
	return a.enrichAccountQuotaOnlyWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountQuotaOnlyWithService(svc *services.WindsurfService, acc *models.Account) bool {
	if acc == nil || svc == nil {
		return false
	}
	label := acc.Email
	if label == "" {
		label = acc.ID
	}
	gotData := false
	utils.DLog("[enrich] %s 开始 (hasToken=%v hasKey=%v plan=%s)", label, acc.Token != "", acc.WindsurfAPIKey != "", acc.PlanName)
	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
		}
	}
	// ── 主路径: gRPC GetUserStatus（快速、不依赖 Firebase / 代理）──
	needJSONFallback := false
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			utils.DLog("[enrich] %s GetUserStatus OK: plan=%s daily=%v weekly=%v total=%d used=%d",
				label, profile.PlanName,
				profile.DailyQuotaRemaining, profile.WeeklyQuotaRemaining,
				profile.TotalCredits, profile.UsedCredits)
			applyAccountProfile(acc, profile)
			gotData = true
			// gRPC 拿不到百分比（Pro/Teams 某些号）→ 标记需要 JSON 兜底
			if profile.DailyQuotaRemaining == nil && profile.WeeklyQuotaRemaining == nil {
				needJSONFallback = true
			}
		} else {
			utils.DLog("[enrich] %s GetUserStatus 失败: %v", label, err)
			log.Printf("[enrich] %s GetUserStatus 失败: %v", label, err)
			applyAccessErrorStatus(acc, err)
			needJSONFallback = true // gRPC 完全失败也尝试 JSON
		}
	} else {
		needJSONFallback = true // 无 API key，只能走 JSON
	}

	// ── 兜底: Firebase token → JSON API（需要代理才能访问 Firebase）──
	// GetJWTByAPIKey 返回的 Windsurf JWT 被 JSON API 拒绝(401)，必须用 Firebase ID token。
	if needJSONFallback {
		firebaseToken := ""
		if acc.RefreshToken != "" {
			if resp, err := svc.RefreshToken(acc.RefreshToken); err == nil {
				firebaseToken = resp.IDToken
				acc.RefreshToken = resp.RefreshToken // 更新 refresh token
				utils.DLog("[enrich] %s RefreshToken→Firebase OK", label)
			} else {
				utils.DLog("[enrich] %s RefreshToken 失败: %v", label, err)
			}
		}
		if firebaseToken == "" && acc.Email != "" && acc.Password != "" {
			if resp, err := svc.LoginWithEmail(acc.Email, acc.Password); err == nil {
				firebaseToken = resp.IDToken
				if resp.RefreshToken != "" {
					acc.RefreshToken = resp.RefreshToken
				}
				utils.DLog("[enrich] %s Login→Firebase OK", label)
			} else {
				utils.DLog("[enrich] %s Login 失败: %v", label, err)
			}
		}
		if firebaseToken != "" {
			if plan, err := svc.GetPlanStatusJSON(firebaseToken); err == nil {
				utils.DLog("[enrich] %s GetPlanStatusJSON OK: plan=%s daily=%v weekly=%v total=%d used=%d remaining=%d",
					label, plan.PlanName,
					plan.DailyQuotaRemaining, plan.WeeklyQuotaRemaining,
					plan.TotalCredits, plan.UsedCredits, plan.RemainingCredits)
				applyAccountProfile(acc, plan)
				gotData = true
			} else {
				utils.DLog("[enrich] %s GetPlanStatusJSON 失败: %v", label, err)
			}
		}
	}
	utils.DLog("[enrich] %s 结果: gotData=%v plan=%s daily=%s weekly=%s totalQ=%d usedQ=%d",
		label, gotData, acc.PlanName, acc.DailyRemaining, acc.WeeklyRemaining, acc.TotalQuota, acc.UsedQuota)
	if acc.Nickname == "" && acc.Email != "" {
		acc.Nickname = strings.Split(acc.Email, "@")[0]
	}
	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
	return gotData
}

// enrichAccountInfoLite 批量导入时使用：只做本地 JWT 解析，避免 RegisterUser / GetPlan / GetUserStatus 等串行请求拖死界面。
func (a *App) enrichAccountInfoLite(acc *models.Account) {
	a.enrichAccountInfoLiteWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountInfoLiteWithService(svc *services.WindsurfService, acc *models.Account) {
	if acc == nil || svc == nil {
		return
	}
	label := acc.Email
	if label == "" {
		label = acc.ID
	}
	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
		}
	}
	// lite 只走 gRPC（快速），不走 Firebase→JSON（需代理、耗时）
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			applyAccountProfile(acc, profile)
		} else {
			log.Printf("[enrich-lite] %s GetUserStatus 失败: %v", label, err)
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

func (a *App) enrichAccountInfo(acc *models.Account) bool {
	return a.enrichAccountInfoWithService(a.windsurfSvc, acc)
}

func (a *App) enrichAccountInfoWithService(svc *services.WindsurfService, acc *models.Account) bool {
	if acc == nil || svc == nil {
		return false
	}
	label := acc.Email
	if label == "" {
		label = acc.ID
	}
	gotData := false
	utils.DLog("[enrichFull] %s 开始 (hasToken=%v hasKey=%v hasRefresh=%v hasPass=%v plan=%s)",
		label, acc.Token != "", acc.WindsurfAPIKey != "", acc.RefreshToken != "", acc.Password != "", acc.PlanName)

	if acc.Token != "" {
		if claims, err := svc.DecodeJWTClaims(acc.Token); err == nil {
			applyJWTClaims(acc, claims)
			utils.DLog("[enrichFull] %s JWT解码: email=%s plan=%s pro=%v tier=%s", label, claims.Email, acc.PlanName, claims.Pro, claims.TeamsTier)
		} else {
			utils.DLog("[enrichFull] %s JWT解码失败: %v", label, err)
		}
	}

	if acc.Token != "" && (acc.RefreshToken != "" || acc.Password != "") {
		if acc.Email == "" {
			if email, err := svc.GetAccountInfo(acc.Token); err == nil && email != "" {
				acc.Email = email
				utils.DLog("[enrichFull] %s GetAccountInfo: email=%s", label, email)
			}
		}
		if strings.TrimSpace(acc.WindsurfAPIKey) == "" {
			if reg, err := svc.RegisterUser(acc.Token); err == nil && reg != nil && reg.APIKey != "" {
				acc.WindsurfAPIKey = reg.APIKey
				utils.DLog("[enrichFull] %s RegisterUser: 获得APIKey=%s...", label, reg.APIKey[:min(12, len(reg.APIKey))])
			} else if err != nil {
				utils.DLog("[enrichFull] %s RegisterUser 失败: %v", label, err)
			}
		}
	}

	// ── 主路径: gRPC GetUserStatus（快速、不依赖 Firebase / 代理）──
	needJSONFallback := false
	if acc.WindsurfAPIKey != "" {
		if profile, err := svc.GetUserStatus(acc.WindsurfAPIKey); err == nil {
			utils.DLog("[enrichFull] %s GetUserStatus OK: plan=%s daily=%v weekly=%v total=%d used=%d",
				label, profile.PlanName, profile.DailyQuotaRemaining, profile.WeeklyQuotaRemaining, profile.TotalCredits, profile.UsedCredits)
			applyAccountProfile(acc, profile)
			gotData = true
			if profile.DailyQuotaRemaining == nil && profile.WeeklyQuotaRemaining == nil {
				needJSONFallback = true
			}
		} else {
			utils.DLog("[enrichFull] %s GetUserStatus 失败: %v", label, err)
			log.Printf("[enrich] %s GetUserStatus 失败: %v", label, err)
			applyAccessErrorStatus(acc, err)
			needJSONFallback = true
		}
	} else {
		needJSONFallback = true
	}

	// ── 兜底: Firebase token → JSON API ──
	if needJSONFallback {
		firebaseToken := ""
		if acc.RefreshToken != "" {
			if resp, err := svc.RefreshToken(acc.RefreshToken); err == nil {
				firebaseToken = resp.IDToken
				acc.RefreshToken = resp.RefreshToken
				utils.DLog("[enrichFull] %s RefreshToken→Firebase OK", label)
			} else {
				utils.DLog("[enrichFull] %s RefreshToken 失败: %v", label, err)
			}
		}
		if firebaseToken == "" && acc.Email != "" && acc.Password != "" {
			if resp, err := svc.LoginWithEmail(acc.Email, acc.Password); err == nil {
				firebaseToken = resp.IDToken
				if resp.RefreshToken != "" {
					acc.RefreshToken = resp.RefreshToken
				}
				utils.DLog("[enrichFull] %s Login→Firebase OK", label)
			} else {
				utils.DLog("[enrichFull] %s Login 失败: %v", label, err)
			}
		}
		if firebaseToken != "" {
			if plan, err := svc.GetPlanStatusJSON(firebaseToken); err == nil {
				utils.DLog("[enrichFull] %s GetPlanStatusJSON OK: plan=%s daily=%v weekly=%v total=%d used=%d",
					label, plan.PlanName, plan.DailyQuotaRemaining, plan.WeeklyQuotaRemaining, plan.TotalCredits, plan.UsedCredits)
				applyAccountProfile(acc, plan)
				gotData = true
			} else {
				utils.DLog("[enrichFull] %s GetPlanStatusJSON 失败: %v", label, err)
			}
		}
	}
	utils.DLog("[enrichFull] %s 结果: gotData=%v plan=%s daily=%s weekly=%s totalQ=%d usedQ=%d",
		label, gotData, acc.PlanName, acc.DailyRemaining, acc.WeeklyRemaining, acc.TotalQuota, acc.UsedQuota)

	if acc.Nickname == "" && acc.Email != "" {
		acc.Nickname = strings.Split(acc.Email, "@")[0]
	}

	if acc.PlanName == "" {
		acc.PlanName = "unknown"
	}
	return gotData
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
	normalizeAccountPlanAndStatus(acc)
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
	// 额度字段完全以本次官方响应为准，避免沿用旧快照。
	acc.TotalQuota = profile.TotalCredits
	acc.UsedQuota = profile.UsedCredits
	if profile.DailyQuotaRemaining != nil {
		acc.DailyRemaining = formatQuotaPercent(*profile.DailyQuotaRemaining)
	} else {
		acc.DailyRemaining = ""
	}
	if profile.WeeklyQuotaRemaining != nil {
		acc.WeeklyRemaining = formatQuotaPercent(*profile.WeeklyQuotaRemaining)
	} else {
		acc.WeeklyRemaining = ""
	}
	// 优先使用官方接口返回的 resetAt；缺失时保持为空，不再伪造周额度/周重置时间。
	acc.DailyResetAt = strings.TrimSpace(profile.DailyResetAt)
	acc.WeeklyResetAt = strings.TrimSpace(profile.WeeklyResetAt)
	if preferred := choosePreferredSubscriptionExpiry(acc, profile.SubscriptionExpiresAt); preferred != "" {
		acc.SubscriptionExpiresAt = preferred
	} else {
		acc.SubscriptionExpiresAt = ""
	}
	normalizeAccountPlanAndStatus(acc)
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

func normalizeAccountPlanAndStatus(acc *models.Account) {
	if acc == nil {
		return
	}
	acc.SubscriptionExpiresAt = choosePreferredSubscriptionExpiry(acc, "")
	status := strings.TrimSpace(strings.ToLower(acc.Status))
	if status == "" {
		status = "active"
	}
	if status == "disabled" {
		acc.Status = "disabled"
		return
	}
	if acc.SubscriptionExpiresAt == "" {
		acc.Status = status
		return
	}
	t, ok := parseSubscriptionEndTime(acc.SubscriptionExpiresAt)
	if !ok {
		acc.Status = status
		return
	}
	if !t.After(time.Now()) {
		acc.Status = "expired"
		if utils.PlanTone(acc.PlanName) != "free" {
			acc.PlanName = "Free"
		}
		return
	}
	acc.Status = "active"
}

func applyAccessErrorStatus(acc *models.Account, err error) {
	if acc == nil || err == nil {
		return
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "user is disabled in windsurf team"):
		acc.Status = "disabled"
	case strings.Contains(lower, "subscription is not active"):
		acc.Status = "expired"
		if utils.PlanTone(acc.PlanName) != "free" {
			acc.PlanName = "Free"
		}
	case strings.Contains(lower, `"code":"permission_denied"`), strings.Contains(lower, "permission denied"):
		acc.Status = "disabled"
	}
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

// dateLikePrefix 快速检查：字符串必须以 4位数字+分隔符 开头才可能是日期，跳过 99% 的非日期字符串
func looksLikeDatePrefix(s string) bool {
	if len(s) < 8 { // "2026/1/2" 最短8字符
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s[4] == '/' || s[4] == '-' || s[4] == '.'
}

func parseManualSubscriptionExpiryHint(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(strings.Trim(raw, `"`))
	if raw == "" || !looksLikeDatePrefix(raw) {
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
