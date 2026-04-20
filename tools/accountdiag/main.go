package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"

	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/utils"
)

type localSettings struct {
	ProxyEnabled bool   `json:"proxy_enabled"`
	ProxyURL     string `json:"proxy_url"`
}

type quotaSnapshot struct {
	PlanName              string   `json:"plan_name,omitempty"`
	DailyQuotaRemaining   *float64 `json:"daily_quota_remaining,omitempty"`
	WeeklyQuotaRemaining  *float64 `json:"weekly_quota_remaining,omitempty"`
	SubscriptionExpiresAt string   `json:"subscription_expires_at,omitempty"`
	TotalCredits          int      `json:"total_credits,omitempty"`
	UsedCredits           int      `json:"used_credits,omitempty"`
}

type authProbe struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type chatProbe struct {
	Attempted     bool   `json:"attempted"`
	OK            bool   `json:"ok"`
	Kind          string `json:"kind,omitempty"`
	Error         string `json:"error,omitempty"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	GRPCStatus    string `json:"grpc_status,omitempty"`
	GRPCMessage   string `json:"grpc_message,omitempty"`
	TextPreview   string `json:"text_preview,omitempty"`
	UsedAPIKey    string `json:"used_api_key,omitempty"`
	ResponseBytes int    `json:"response_bytes,omitempty"`
}

type probeResult struct {
	ID                        string         `json:"id"`
	Email                     string         `json:"email"`
	StoredPlanName            string         `json:"stored_plan_name,omitempty"`
	StoredStatus              string         `json:"stored_status,omitempty"`
	StoredDailyRemaining      string         `json:"stored_daily_remaining,omitempty"`
	StoredWeeklyRemaining     string         `json:"stored_weekly_remaining,omitempty"`
	StoredSubscriptionExpires string         `json:"stored_subscription_expires,omitempty"`
	WeeklyMissingBlocked      bool           `json:"weekly_missing_blocked"`
	StoredExpiryPast          bool           `json:"stored_expiry_past"`
	JSONAttempted             bool           `json:"json_attempted"`
	GRPC                      *quotaSnapshot `json:"grpc,omitempty"`
	JSON                      *quotaSnapshot `json:"json,omitempty"`
	JWT                       authProbe      `json:"jwt"`
	Chat                      chatProbe      `json:"chat"`
	Errors                    []string       `json:"errors,omitempty"`
	rawAPIKey                 string         `json:"-"`
}

type reportSummary struct {
	GeneratedAt               string `json:"generated_at"`
	TotalAccounts             int    `json:"total_accounts"`
	GRPCOK                    int    `json:"grpc_ok"`
	JWTOK                     int    `json:"jwt_ok"`
	JSONChecked               int    `json:"json_checked"`
	JSONOK                    int    `json:"json_ok"`
	WeeklyMissingBlocked      int    `json:"weekly_missing_blocked"`
	StoredExpiryPast          int    `json:"stored_expiry_past"`
	ChatAttempted             int    `json:"chat_attempted"`
	ChatOK                    int    `json:"chat_ok"`
	ChatQuotaFailures         int    `json:"chat_quota_failures"`
	ChatAuthFailures          int    `json:"chat_auth_failures"`
	ChatOtherFailures         int    `json:"chat_other_failures"`
	ChatHealthySampleSelected int    `json:"chat_healthy_sample_selected"`
}

type reportFile struct {
	Summary reportSummary `json:"summary"`
	Results []probeResult `json:"results"`
}

func main() {
	var (
		concurrency       = flag.Int("concurrency", 12, "parallelism for grpc/jwt/json probes")
		chatHealthySample = flag.Int("chat-healthy-sample", 10, "healthy accounts to include in live chat probes")
		outPath           = flag.String("out", filepath.Join("tools", "accountdiag", "last_report.json"), "output report path")
	)
	flag.Parse()

	configDir, err := os.UserConfigDir()
	if err != nil {
		fatalf("读取用户配置目录失败: %v", err)
	}
	dataDir := filepath.Join(configDir, "WindsurfTools")
	accountsPath := filepath.Join(dataDir, "accounts.json")
	settingsPath := filepath.Join(dataDir, "settings.json")

	accounts, err := loadAccounts(accountsPath)
	if err != nil {
		fatalf("读取账号失败: %v", err)
	}
	settings, err := loadSettings(settingsPath)
	if err != nil {
		fatalf("读取设置失败: %v", err)
	}
	proxyURL := ""
	if settings.ProxyEnabled {
		proxyURL = strings.TrimSpace(settings.ProxyURL)
	}

	fmt.Printf("accounts=%d proxy=%q concurrency=%d\n", len(accounts), proxyURL, *concurrency)

	results := probeAll(accounts, proxyURL, *concurrency)
	selectChatCandidates(results, *chatHealthySample)
	runChatProbes(results, proxyURL)

	report := reportFile{
		Summary: buildSummary(results, *chatHealthySample),
		Results: results,
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0755); err != nil {
		fatalf("创建输出目录失败: %v", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatalf("编码报告失败: %v", err)
	}
	if err := os.WriteFile(*outPath, data, 0644); err != nil {
		fatalf("写报告失败: %v", err)
	}

	fmt.Printf("report=%s\n", *outPath)
	fmt.Printf("summary: grpc_ok=%d jwt_ok=%d json_ok=%d/%d weekly_missing_blocked=%d chat_ok=%d/%d quota_failures=%d auth_failures=%d other_failures=%d\n",
		report.Summary.GRPCOK,
		report.Summary.JWTOK,
		report.Summary.JSONOK,
		report.Summary.JSONChecked,
		report.Summary.WeeklyMissingBlocked,
		report.Summary.ChatOK,
		report.Summary.ChatAttempted,
		report.Summary.ChatQuotaFailures,
		report.Summary.ChatAuthFailures,
		report.Summary.ChatOtherFailures,
	)
}

func loadAccounts(path string) ([]models.Account, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var accounts []models.Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func loadSettings(path string) (*localSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s localSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func probeAll(accounts []models.Account, proxyURL string, concurrency int) []probeResult {
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]probeResult, len(accounts))
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for i := range accounts {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = probeAccount(accounts[i], proxyURL)
		}()
	}

	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Email) < strings.ToLower(results[j].Email)
	})
	return results
}

func probeAccount(acc models.Account, proxyURL string) probeResult {
	result := probeResult{
		ID:                        acc.ID,
		Email:                     acc.Email,
		StoredPlanName:            acc.PlanName,
		StoredStatus:              acc.Status,
		StoredDailyRemaining:      acc.DailyRemaining,
		StoredWeeklyRemaining:     acc.WeeklyRemaining,
		StoredSubscriptionExpires: acc.SubscriptionExpiresAt,
		WeeklyMissingBlocked:      weeklyMissingBlocked(acc.DailyRemaining, acc.WeeklyRemaining, acc.WeeklyResetAt),
		StoredExpiryPast:          expiryPast(acc.SubscriptionExpiresAt),
		rawAPIKey:                 strings.TrimSpace(acc.WindsurfAPIKey),
	}

	svc := services.NewWindsurfService(proxyURL)

	if strings.TrimSpace(acc.WindsurfAPIKey) != "" {
		profile, err := svc.GetUserStatus(acc.WindsurfAPIKey)
		if err != nil {
			result.Errors = append(result.Errors, "grpc:"+err.Error())
		} else {
			result.GRPC = snapshotFromProfile(profile)
			result.WeeklyMissingBlocked = weeklyMissingBlocked(
				formatPercentPtr(profile.DailyQuotaRemaining),
				formatPercentPtr(profile.WeeklyQuotaRemaining),
				profile.WeeklyResetAt,
			)
		}

		jwt, err := svc.GetJWTByAPIKey(acc.WindsurfAPIKey)
		if err != nil {
			result.JWT = authProbe{OK: false, Error: err.Error()}
			result.Errors = append(result.Errors, "jwt:"+err.Error())
		} else if strings.TrimSpace(jwt) != "" {
			result.JWT = authProbe{OK: true}
		}
	}

	if shouldProbeJSON(acc, result) {
		result.JSONAttempted = true
		token, err := getFirebaseToken(svc, acc)
		if err != nil {
			result.Errors = append(result.Errors, "json-auth:"+err.Error())
		} else {
			profile, err := svc.GetPlanStatusJSON(token)
			if err != nil {
				result.Errors = append(result.Errors, "json:"+err.Error())
			} else {
				result.JSON = snapshotFromProfile(profile)
			}
		}
	}

	return result
}

func shouldProbeJSON(acc models.Account, result probeResult) bool {
	tone := strings.ToLower(strings.TrimSpace(acc.PlanName))
	if tone == "pro" || tone == "enterprise" || tone == "max" || tone == "team" {
		return true
	}
	if result.StoredExpiryPast || result.WeeklyMissingBlocked {
		return true
	}
	if result.GRPC == nil {
		return true
	}
	return result.GRPC.SubscriptionExpiresAt == "" && acc.SubscriptionExpiresAt != ""
}

func getFirebaseToken(svc *services.WindsurfService, acc models.Account) (string, error) {
	if strings.TrimSpace(acc.RefreshToken) != "" {
		resp, err := svc.RefreshToken(acc.RefreshToken)
		if err == nil && strings.TrimSpace(resp.IDToken) != "" {
			return resp.IDToken, nil
		}
	}
	if strings.TrimSpace(acc.Email) != "" && strings.TrimSpace(acc.Password) != "" {
		resp, err := svc.LoginWithEmail(acc.Email, acc.Password)
		if err == nil && strings.TrimSpace(resp.IDToken) != "" {
			return resp.IDToken, nil
		}
	}
	return "", fmt.Errorf("no usable refresh/password auth path")
}

func snapshotFromProfile(profile *services.AccountProfile) *quotaSnapshot {
	if profile == nil {
		return nil
	}
	return &quotaSnapshot{
		PlanName:              profile.PlanName,
		DailyQuotaRemaining:   profile.DailyQuotaRemaining,
		WeeklyQuotaRemaining:  profile.WeeklyQuotaRemaining,
		SubscriptionExpiresAt: profile.SubscriptionExpiresAt,
		TotalCredits:          profile.TotalCredits,
		UsedCredits:           profile.UsedCredits,
	}
}

func selectChatCandidates(results []probeResult, healthySample int) {
	healthyLeft := healthySample
	for i := range results {
		r := &results[i]
		if !r.JWT.OK {
			continue
		}
		if isClearlyBrokenQuota(*r) || isPaidPlan(r.StoredPlanName, r.GRPC, r.JSON) {
			r.Chat.Attempted = true
			continue
		}
		if healthyLeft > 0 && isHealthyChatCandidate(*r) {
			r.Chat.Attempted = true
			healthyLeft--
		}
	}
}

func runChatProbes(results []probeResult, proxyURL string) {
	for i := range results {
		if !results[i].Chat.Attempted || !results[i].JWT.OK {
			continue
		}
		results[i].Chat = probeChat(results[i], proxyURL)
	}
}

func probeChat(result probeResult, proxyURL string) chatProbe {
	probe := chatProbe{Attempted: true}
	apiKey := findUsableAPIKey(result)
	if apiKey == "" {
		probe.Error = "missing api key"
		return probe
	}

	svc := services.NewWindsurfService(proxyURL)
	jwt, err := svc.GetJWTByAPIKey(apiKey)
	if err != nil {
		probe.Kind = "auth"
		probe.Error = err.Error()
		return probe
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         services.GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	_ = http2.ConfigureTransport(transport)
	client := &http.Client{Timeout: 45 * time.Second, Transport: transport}

	payload := services.BuildChatRequest([]services.ChatMessage{{
		Role:    "user",
		Content: "Reply with OK only.",
	}}, apiKey, jwt, "", nil)

	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://%s/exa.api_server_pb.ApiServerService/GetChatMessage", services.ResolveUpstreamIP()),
		bytes.NewReader(payload),
	)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	req.Host = services.GRPCUpstreamHost
	req.Header.Set("content-type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("authorization", "Bearer "+jwt)
	req.Header.Set("user-agent", "connect-go/1.18.1 (go1.26.1)")

	resp, err := client.Do(req)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	probe.HTTPStatus = resp.StatusCode
	probe.GRPCStatus = strings.TrimSpace(resp.Header.Get("grpc-status"))
	if probe.GRPCStatus == "" {
		probe.GRPCStatus = strings.TrimSpace(resp.Trailer.Get("grpc-status"))
	}
	probe.GRPCMessage = decodeGRPCMessage(resp.Header.Get("grpc-message"))
	if probe.GRPCMessage == "" {
		probe.GRPCMessage = decodeGRPCMessage(resp.Trailer.Get("grpc-message"))
	}
	probe.ResponseBytes = len(body)
	probe.UsedAPIKey = trimAPIKey(apiKey)

	textLower := strings.ToLower(string(body))
	if probe.GRPCStatus == "9" || strings.Contains(textLower, "quota") || strings.Contains(textLower, "failed precondition") || strings.Contains(textLower, "failed_precondition") {
		probe.Kind = "quota"
		probe.Error = trimText(firstNonEmpty(probe.GRPCMessage, string(body)), 220)
		return probe
	}
	if resp.StatusCode >= 400 || probe.GRPCStatus == "16" || strings.Contains(textLower, "permission_denied") || strings.Contains(textLower, "unauthenticated") {
		probe.Kind = "auth"
		probe.Error = trimText(firstNonEmpty(probe.GRPCMessage, string(body)), 220)
		return probe
	}
	if probe.GRPCStatus != "" && probe.GRPCStatus != "0" {
		probe.Kind = "other"
		probe.Error = trimText(firstNonEmpty(probe.GRPCMessage, string(body)), 220)
		return probe
	}

	var textParts []string
	for _, frame := range services.ExtractGRPCFrames(body) {
		text, _, err := services.ParseChatResponseChunk(frame)
		if err == nil && strings.TrimSpace(text) != "" {
			textParts = append(textParts, strings.TrimSpace(text))
		}
	}
	probe.TextPreview = trimText(strings.Join(textParts, " "), 120)
	probe.OK = resp.StatusCode == 200 && (probe.GRPCStatus == "" || probe.GRPCStatus == "0") && probe.TextPreview != ""
	if !probe.OK {
		probe.Kind = "other"
		probe.Error = trimText(firstNonEmpty(probe.GRPCMessage, string(body)), 220)
	}
	return probe
}

func isClearlyBrokenQuota(r probeResult) bool {
	if r.WeeklyMissingBlocked {
		return true
	}
	if percentLeqZero(r.StoredDailyRemaining) || percentLeqZero(r.StoredWeeklyRemaining) {
		return true
	}
	if r.GRPC != nil {
		if ptrPercentLeqZero(r.GRPC.DailyQuotaRemaining) || ptrPercentLeqZero(r.GRPC.WeeklyQuotaRemaining) {
			return true
		}
	}
	if r.JSON != nil {
		if ptrPercentLeqZero(r.JSON.DailyQuotaRemaining) || ptrPercentLeqZero(r.JSON.WeeklyQuotaRemaining) {
			return true
		}
	}
	return false
}

func isHealthyChatCandidate(r probeResult) bool {
	if r.StoredExpiryPast || r.WeeklyMissingBlocked {
		return false
	}
	if percentLeqZero(r.StoredDailyRemaining) || percentLeqZero(r.StoredWeeklyRemaining) {
		return false
	}
	return true
}

func isPaidPlan(stored string, grpc *quotaSnapshot, jsonSnap *quotaSnapshot) bool {
	for _, plan := range []string{stored, snapshotPlan(grpc), snapshotPlan(jsonSnap)} {
		switch strings.ToLower(strings.TrimSpace(plan)) {
		case "pro", "max", "enterprise", "team":
			return true
		}
	}
	return false
}

func buildSummary(results []probeResult, healthySample int) reportSummary {
	summary := reportSummary{
		GeneratedAt:               time.Now().Format(time.RFC3339),
		TotalAccounts:             len(results),
		ChatHealthySampleSelected: healthySample,
	}
	for _, r := range results {
		if r.GRPC != nil {
			summary.GRPCOK++
		}
		if r.JWT.OK {
			summary.JWTOK++
		}
		if r.JSONAttempted {
			summary.JSONChecked++
		}
		if r.JSON != nil {
			summary.JSONOK++
		}
		if r.WeeklyMissingBlocked {
			summary.WeeklyMissingBlocked++
		}
		if r.StoredExpiryPast {
			summary.StoredExpiryPast++
		}
		if r.Chat.Attempted {
			summary.ChatAttempted++
		}
		if r.Chat.OK {
			summary.ChatOK++
		}
		switch r.Chat.Kind {
		case "quota":
			summary.ChatQuotaFailures++
		case "auth":
			summary.ChatAuthFailures++
		case "other":
			summary.ChatOtherFailures++
		}
	}
	return summary
}

func weeklyMissingBlocked(daily, weekly, weeklyResetAt string) bool {
	if strings.TrimSpace(weekly) != "" || strings.TrimSpace(weeklyResetAt) == "" {
		return false
	}
	if value, ok := utils.ParseQuotaPercentString(daily); ok && value <= 0.0001 {
		return false
	}
	return true
}

func percentLeqZero(raw string) bool {
	value, ok := utils.ParseQuotaPercentString(raw)
	return ok && value <= 0.0001
}

func ptrPercentLeqZero(v *float64) bool {
	return v != nil && *v <= 0.0001
}

func formatPercentPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.2f%%", *v)
}

func snapshotPlan(s *quotaSnapshot) string {
	if s == nil {
		return ""
	}
	return s.PlanName
}

func findUsableAPIKey(result probeResult) string {
	return strings.TrimSpace(result.rawAPIKey)
}

func expiryPast(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return false
	}
	return !t.After(time.Now())
}

func trimText(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func trimAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 15 {
		return key
	}
	return key[:12] + "..."
}

func decodeGRPCMessage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if decoded, err := url.QueryUnescape(raw); err == nil && decoded != "" {
		return decoded
	}
	return raw
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
