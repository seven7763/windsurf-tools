package services

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
	"windsurf-tools-wails/backend/utils"
)

const (
	// FirebaseAPIKey：Firebase Identity Toolkit 的 Web 客户端 Key（与 Windsurf 网页端相同，非用户个人密钥）。勿将用户密码/Refresh Token/sk-ws API Key 写入仓库，见 SECURITY.md。
	FirebaseAPIKey   = "AIzaSyDsOl-1XpT5err0Tcnx8FFod1H8gVGIycY"
	WindsurfBaseURL  = "https://web-backend.windsurf.com"
	GRPCUpstreamHost = "server.self-serve.windsurf.com"
	GRPCUpstreamIP   = "34.49.14.144"
	WindsurfAppName  = "windsurf"
	WindsurfVersion  = "1.48.2"
	WindsurfClient   = "2.0.50"
)

type WindsurfService struct {
	client *http.Client
}

func NewWindsurfService(proxyURL string) *WindsurfService {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
		ForceAttemptHTTP2: true,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &WindsurfService{
		client: &http.Client{Timeout: 30 * time.Second, Transport: transport, Jar: jar},
	}
}

// ── Firebase Auth ──

type FirebaseSignInResp struct {
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    string `json:"expiresIn"`
	Email        string `json:"email"`
	LocalID      string `json:"localId"`
}

type FirebaseRefreshResp struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"`
	UserID       string `json:"user_id"`
}

type RegisterUserResp struct {
	APIKey       string `json:"api_key"`
	Name         string `json:"name"`
	APIServerURL string `json:"api_server_url"`
}

type WindsurfPostAuthResp struct {
	Raw        []byte
	AuthToken  string
	SessionKey string
	AccountID  string
	OrgID      string
}

type JWTClaims struct {
	Email                  string
	Name                   string
	Pro                    bool
	TeamsTier              string
	TrialEnd               string
	MaxPremiumChatMessages int
	AuthUID                string
	TeamID                 string // e.g. "devin-team$account-XXX"，真实 IDE 聊天请求 F32 用此值
	UserID                 string // e.g. "user-XXX"，真实 IDE 聊天请求 F20 用此值（从 auth_uid 切出）
	APIKeyClaim            string // JWT 里的 api_key 字段（"devin-synthetic-apikey$account-XXX$user-XXX"）
}

type AccountProfile struct {
	Email                 string
	Name                  string
	PlanName              string
	Tier                  int
	TierLabel             string
	TotalCredits          int
	UsedCredits           int
	RemainingCredits      int
	DailyQuotaRemaining   *float64
	WeeklyQuotaRemaining  *float64
	DailyResetAt          string
	WeeklyResetAt         string
	SubscriptionExpiresAt string
	BillingStrategy       string
}

type planStatusEnvelope struct {
	PlanStatus planStatusPayload `json:"planStatus"`
}

type planStatusPayload struct {
	AvailablePromptCredits      int                    `json:"availablePromptCredits"`
	AvailableFlowCredits        int                    `json:"availableFlowCredits"`
	UsedPromptCredits           *int                   `json:"usedPromptCredits"`
	UsedUsageCredits            *int                   `json:"usedUsageCredits"`
	DailyQuotaRemainingPercent  *float64               `json:"dailyQuotaRemainingPercent"`
	WeeklyQuotaRemainingPercent *float64               `json:"weeklyQuotaRemainingPercent"`
	DailyQuotaResetAtUnix       json.RawMessage        `json:"dailyQuotaResetAtUnix"`
	WeeklyQuotaResetAtUnix      json.RawMessage        `json:"weeklyQuotaResetAtUnix"`
	PlanEnd                     json.RawMessage        `json:"planEnd"`
	PlanStart                   json.RawMessage        `json:"planStart"`
	SubscriptionPeriodEnd       json.RawMessage        `json:"subscriptionPeriodEnd"`
	CurrentPeriodEnd            json.RawMessage        `json:"currentPeriodEnd"`
	PlanExpiresAt               json.RawMessage        `json:"planExpiresAt"`
	SubscriptionExpiresAtJSON   json.RawMessage        `json:"subscriptionExpiresAt"`
	TopUpStatus                 map[string]interface{} `json:"topUpStatus"`
	PlanInfo                    struct {
		PlanName        string `json:"planName"`
		BillingStrategy string `json:"billingStrategy"`
	} `json:"planInfo"`
}

func (s *WindsurfService) LoginWithEmail(email, password string) (*FirebaseSignInResp, error) {
	// 先尝试老 Firebase 协议
	resp, firebaseErr := s.loginFirebase(email, password)
	if firebaseErr == nil {
		return resp, nil
	}
	// Firebase 失败，尝试新 auth1 协议（Windsurf 官网 /_devin-auth/password/login）
	resp, auth1Err := s.loginAuth1(email, password)
	if auth1Err == nil {
		return resp, nil
	}
	// 两种协议都失败，返回合并的错误信息
	return nil, fmt.Errorf("登录失败 — Firebase: %v; auth1: %v", firebaseErr, auth1Err)
}

// loginFirebase 走老的 Identity Toolkit 接口。
func (s *WindsurfService) loginFirebase(email, password string) (*FirebaseSignInResp, error) {
	apiURL := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=%s",
		FirebaseAPIKey,
	)
	payload := map[string]interface{}{
		"returnSecureToken": true,
		"email":             email,
		"password":          password,
		"clientType":        "CLIENT_TYPE_WEB",
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码登录请求失败: %w", err)
	}
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://windsurf.com/")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("登录请求失败(网络): %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("登录失败(%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	var result FirebaseSignInResp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析登录响应失败: %w", err)
	}
	return &result, nil
}

// loginAuth1 走新的 auth1 协议。响应 {token, user_id, email} —— token 填到 IDToken 字段，
// RefreshToken 留空（auth1 无 refresh_token 概念）。
func (s *WindsurfService) loginAuth1(email, password string) (*FirebaseSignInResp, error) {
	payload := map[string]string{"email": email, "password": password}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码 auth1 请求失败: %w", err)
	}
	req, _ := http.NewRequest("POST", "https://windsurf.com/_devin-auth/password/login", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://windsurf.com")
	req.Header.Set("Referer", "https://windsurf.com/account/login")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/147.0.0.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth1 登录请求失败(网络): %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("auth1 登录失败(%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	var r struct {
		Token  string `json:"token"`
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return nil, fmt.Errorf("解析 auth1 响应失败: %w", err)
	}
	if r.Token == "" {
		return nil, fmt.Errorf("auth1 响应未返回 token")
	}
	return &FirebaseSignInResp{
		IDToken: r.Token,
		Email:   r.Email,
		LocalID: r.UserID,
	}, nil
}

func (s *WindsurfService) RefreshToken(refreshToken string) (*FirebaseRefreshResp, error) {
	apiURL := fmt.Sprintf("https://securetoken.googleapis.com/v1/token?key=%s", FirebaseAPIKey)
	body := fmt.Sprintf("grant_type=refresh_token&refresh_token=%s", refreshToken)
	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Client-Version", "Chrome/JsCore/11.0.0/FirebaseCore-web")
	req.Header.Set("Referer", "https://windsurf.com/")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("刷新Token请求失败(网络): %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("刷新Token失败(%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	var result FirebaseRefreshResp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析刷新响应失败: %w", err)
	}
	return &result, nil
}

func (s *WindsurfService) GetAccountInfo(idToken string) (string, error) {
	apiURL := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/accounts:lookup?key=%s", FirebaseAPIKey,
	)
	body := fmt.Sprintf(`{"idToken":"%s"}`, idToken)
	req, _ := http.NewRequest("POST", apiURL, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://windsurf.com/")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询账号信息失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("查询账号失败(%d)", resp.StatusCode)
	}
	var parsed struct {
		Users []struct {
			Email string `json:"email"`
		} `json:"users"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || len(parsed.Users) == 0 {
		return "", fmt.Errorf("解析账号信息失败")
	}
	return parsed.Users[0].Email, nil
}

func (s *WindsurfService) RegisterUser(firebaseIDToken string) (*RegisterUserResp, error) {
	reqBody, err := json.Marshal(map[string]string{"firebase_id_token": firebaseIDToken})
	if err != nil {
		return nil, fmt.Errorf("编码 RegisterUser 请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.codeium.com/register_user/", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建 RegisterUser 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RegisterUser 请求失败(网络): %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RegisterUser失败(%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result RegisterUserResp
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析 RegisterUser 响应失败: %w", err)
	}
	if result.APIKey == "" {
		return nil, fmt.Errorf("RegisterUser 未返回 api_key")
	}
	return &result, nil
}

// ── Connect+Proto GetCurrentUser ──

func (s *WindsurfService) GetCurrentUser(token string) (map[string]interface{}, error) {
	apiURL := WindsurfBaseURL + "/exa.seat_management_pb.SeatManagementService/GetCurrentUser"
	body := utils.EncodeStringField(1, token)
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("x-auth-token", token)
	req.Header.Set("Referer", "https://windsurf.com/")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetCurrentUser请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GetCurrentUser失败(%d)", resp.StatusCode)
	}
	result := parseCurrentUserResponse(respBody)
	return result, nil
}

// ── Connect+Proto GetPlanStatus (额度查询) ──

func (s *WindsurfService) GetPlanStatus(token string) (map[string]interface{}, error) {
	apiURL := WindsurfBaseURL + "/exa.billing_pb.BillingService/GetPlanStatus"
	body := utils.EncodeStringField(1, token)
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("x-auth-token", token)
	req.Header.Set("Referer", "https://windsurf.com/")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetPlanStatus请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GetPlanStatus失败(%d)", resp.StatusCode)
	}
	result := parsePlanStatusResponse(respBody)
	return result, nil
}

// ── gRPC 直连获取 JWT (sk-ws-* API Key) ──

func (s *WindsurfService) GetJWTByAPIKey(apiKey string) (string, error) {
	connectURL := fmt.Sprintf("https://%s/exa.auth_pb.AuthService/GetUserJwt", ResolveUpstreamIP())
	metadata := buildAPIKeyMetadata(apiKey)

	// Connect 协议：F1 嵌套 metadata 子消息，无 gRPC 5 字节帧头
	body := make([]byte, 0, 2+len(metadata))
	body = append(body, 0x0a) // field 1, wire type 2
	body = append(body, encodeLength(len(metadata))...)
	body = append(body, metadata...)

	req, _ := http.NewRequest("POST", connectURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Host = GRPCUpstreamHost // 关键：必须用 req.Host 而不是 Header.Set

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Connect JWT请求失败(网络): %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Connect JWT失败(HTTP %d): %s",
			resp.StatusCode, truncate(string(respBody), 200))
	}

	// Connect 协议响应可能是 gzip 压缩的原始 protobuf
	if len(respBody) >= 2 && respBody[0] == 0x1f && respBody[1] == 0x8b {
		reader, gzErr := gzip.NewReader(bytes.NewReader(respBody))
		if gzErr == nil {
			decompressed, readErr := io.ReadAll(reader)
			reader.Close()
			if readErr == nil {
				respBody = decompressed
			}
		}
	}

	if len(respBody) == 0 {
		return "", fmt.Errorf("Connect JWT响应体为空")
	}

	jwt, found := utils.FindJWTInProtobuf(respBody)
	if !found {
		return "", fmt.Errorf("Connect响应中未找到JWT(%d bytes): %s",
			len(respBody), truncate(string(respBody), 200))
	}
	return jwt, nil
}

func (s *WindsurfService) GetPlanStatusJSON(token string) (*AccountProfile, error) {
	reqBody, err := json.Marshal(map[string]string{"auth_token": token})
	if err != nil {
		return nil, fmt.Errorf("编码 GetPlanStatus 请求失败: %w", err)
	}

	req, err := http.NewRequest("POST", WindsurfBaseURL+"/exa.seat_management_pb.SeatManagementService/GetPlanStatus", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建 GetPlanStatus 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetPlanStatus(JSON)请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GetPlanStatus(JSON)失败(%d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	utils.DLog("[API] GetPlanStatusJSON raw(%d bytes): %s", len(respBody), truncate(string(respBody), 500))
	// 诊断：dump planStatus 下所有 JSON key（排除巨大的 planInfo）
	{
		var rawMap map[string]map[string]json.RawMessage
		if json.Unmarshal(respBody, &rawMap) == nil {
			if ps, ok := rawMap["planStatus"]; ok {
				for k, v := range ps {
					if k == "planInfo" {
						utils.DLog("[diag] planStatus.%s = <planInfo %d bytes>", k, len(v))
					} else {
						utils.DLog("[diag] planStatus.%s = %s", k, truncate(string(v), 120))
					}
				}
			}
		}
	}
	var payload planStatusEnvelope
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("解析 GetPlanStatus(JSON) 响应失败: %w", err)
	}
	if payload.PlanStatus.PlanInfo.PlanName == "" &&
		payload.PlanStatus.DailyQuotaRemainingPercent == nil &&
		payload.PlanStatus.WeeklyQuotaRemainingPercent == nil &&
		len(bytes.TrimSpace(payload.PlanStatus.PlanEnd)) == 0 &&
		payload.PlanStatus.AvailablePromptCredits == 0 &&
		payload.PlanStatus.AvailableFlowCredits == 0 &&
		payload.PlanStatus.UsedPromptCredits == nil &&
		payload.PlanStatus.UsedUsageCredits == nil &&
		len(payload.PlanStatus.TopUpStatus) == 0 {
		if err := json.Unmarshal(respBody, &payload.PlanStatus); err != nil {
			return nil, fmt.Errorf("解析 GetPlanStatus(JSON) 平铺响应失败: %w", err)
		}
	}
	return parsePlanStatusPayload(payload.PlanStatus), nil
}

func (s *WindsurfService) GetUserStatus(apiKey string) (*AccountProfile, error) {
	// Connect unary (application/proto, 无 gRPC envelope)——匹配 IDE 2.x 的实际行为。
	connectURL := fmt.Sprintf("https://%s/exa.seat_management_pb.SeatManagementService/GetUserStatus", ResolveUpstreamIP())
	metadata := buildAPIKeyMetadata(apiKey)

	// Body: F1 嵌套 metadata 子消息（与 GetJWTByAPIKey 一致）
	body := make([]byte, 0, 2+len(metadata))
	body = append(body, 0x0a)
	body = append(body, encodeLength(len(metadata))...)
	body = append(body, metadata...)

	req, err := http.NewRequest("POST", connectURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 GetUserStatus 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Host = GRPCUpstreamHost

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetUserStatus请求失败(网络): %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	utils.DLog("[API] GetUserStatus resp: HTTP %d, body=%d bytes, ce=%s",
		resp.StatusCode, len(respBody), resp.Header.Get("Content-Encoding"))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GetUserStatus失败(HTTP %d): %s",
			resp.StatusCode, truncate(string(respBody), 200))
	}

	// 响应可能 gzip 压缩（Connect 按 Accept-Encoding 协商）
	if len(respBody) >= 2 && respBody[0] == 0x1f && respBody[1] == 0x8b {
		reader, gzErr := gzip.NewReader(bytes.NewReader(respBody))
		if gzErr == nil {
			decompressed, readErr := io.ReadAll(reader)
			reader.Close()
			if readErr == nil {
				respBody = decompressed
			}
		}
	}
	utils.DLog("[API] GetUserStatus payload: %d bytes", len(respBody))

	profile := parseUserStatusPayload(respBody)
	if profile == nil {
		return nil, fmt.Errorf("GetUserStatus 响应无法解析 (payloadLen=%d)", len(respBody))
	}
	return profile, nil
}

func (s *WindsurfService) DecodeJWTClaims(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("token 不是 JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("解码 JWT payload 失败: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("解析 JWT payload 失败: %w", err)
	}

	claims := &JWTClaims{
		Email:                  stringValue(raw["email"]),
		Name:                   stringValue(raw["name"]),
		Pro:                    boolValue(raw["pro"]),
		TeamsTier:              stringValue(raw["teams_tier"]),
		TrialEnd:               jwtSubscriptionEndFromRaw(raw),
		MaxPremiumChatMessages: int(numberValue(raw["max_num_premium_chat_messages"])),
		AuthUID:                stringValue(raw["auth_uid"]),
		TeamID:                 stringValue(raw["team_id"]),
		APIKeyClaim:            stringValue(raw["api_key"]),
	}
	claims.UserID = extractUserIDFromAuthIdent(claims.AuthUID)
	if claims.UserID == "" {
		claims.UserID = extractUserIDFromAuthIdent(claims.APIKeyClaim)
	}
	return claims, nil
}

// extractUserIDFromAuthIdent 从 "devin-auth-uid$account-XXX$user-YYY" 或
// "devin-synthetic-apikey$account-XXX$user-YYY" 里切出 "user-YYY"。
func extractUserIDFromAuthIdent(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.Split(s, "$")
	for _, p := range parts {
		if strings.HasPrefix(p, "user-") {
			return p
		}
	}
	return ""
}

// jwtSubscriptionEndFromRaw 合并试用/订阅结束时间：官方 JWT 曾只用 windsurf_pro_trial_end_time，部分账号可能用其它字段。
func jwtSubscriptionEndFromRaw(raw map[string]interface{}) string {
	keys := []string{
		"windsurf_pro_trial_end_time",
		"windsurf_subscription_plan_end_time",
		"subscription_expires_at",
		"subscription_end_time",
		"plan_end_time",
	}
	for _, k := range keys {
		v, ok := raw[k]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			if s := normalizeQuotedString(x); s != "" {
				return s
			}
		case float64:
			if sec := unixLikeToRFC3339(x); sec != "" {
				return sec
			}
		case json.Number:
			f, err := x.Float64()
			if err == nil {
				if sec := unixLikeToRFC3339(f); sec != "" {
					return sec
				}
			}
		}
	}
	return ""
}

func unixLikeToRFC3339(n float64) string {
	if n <= 0 {
		return ""
	}
	sec := n
	if n > 1e12 {
		sec = n / 1000
	}
	if sec < 1e9 {
		return ""
	}
	return time.Unix(int64(sec), 0).UTC().Format(time.RFC3339)
}

// parseFlexiblePlanEnd 解析 GetPlanStatus 的 planEnd：可能是 RFC3339 字符串，也可能是 Unix 秒/毫秒数字（直接 JSON 解到 string 会失败导致整包解析报错）。
func parseFlexiblePlanEnd(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ""
		}
		return normalizeQuotedString(s)
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return ""
	}
	return unixLikeToRFC3339(n)
}

// ── 辅助函数 ──

func buildAPIKeyMetadata(apiKey string) []byte {
	var metadata []byte
	metadata = append(metadata, utils.EncodeStringField(1, WindsurfAppName)...)
	metadata = append(metadata, utils.EncodeStringField(2, WindsurfVersion)...)
	metadata = append(metadata, utils.EncodeStringField(3, apiKey)...)
	metadata = append(metadata, utils.EncodeStringField(4, "en")...)
	metadata = append(metadata, utils.EncodeStringField(5, currentWindsurfClientPlatform())...)
	metadata = append(metadata, utils.EncodeStringField(7, WindsurfClient)...)
	metadata = append(metadata, utils.EncodeStringField(12, WindsurfAppName)...)
	return metadata
}

func buildGRPCEnvelope(message []byte) []byte {
	bodyProto := make([]byte, 0, 2+len(message))
	bodyProto = append(bodyProto, 0x0a) // field 1, wire type 2
	bodyProto = append(bodyProto, encodeLength(len(message))...)
	bodyProto = append(bodyProto, message...)

	envelope := make([]byte, 0, 5+len(bodyProto))
	envelope = append(envelope, 0x00)
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(bodyProto)))
	envelope = append(envelope, lenBuf...)
	envelope = append(envelope, bodyProto...)
	return envelope
}

func encodeLength(n int) []byte {
	if n < 128 {
		return []byte{byte(n)}
	}
	return []byte{byte((n & 0x7F) | 0x80), byte(n >> 7)}
}

func parsePlanStatusPayload(ps planStatusPayload) *AccountProfile {
	utils.DLog("[parse] planStatus: name=%s billing=%s prompt=%d flow=%d usedPrompt=%v usedUsage=%v daily=%v weekly=%v hasUsageField=%v",
		ps.PlanInfo.PlanName, ps.PlanInfo.BillingStrategy,
		ps.AvailablePromptCredits, ps.AvailableFlowCredits,
		ps.UsedPromptCredits, ps.UsedUsageCredits,
		ps.DailyQuotaRemainingPercent, ps.WeeklyQuotaRemainingPercent,
		ps.UsedPromptCredits != nil || ps.UsedUsageCredits != nil)
	profile := &AccountProfile{
		PlanName:              normalizePlanName(ps.PlanInfo.PlanName),
		BillingStrategy:       ps.PlanInfo.BillingStrategy,
		DailyQuotaRemaining:   ps.DailyQuotaRemainingPercent,
		WeeklyQuotaRemaining:  ps.WeeklyQuotaRemainingPercent,
		DailyResetAt:          unixToRFC3339(rawMsgToInt64(ps.DailyQuotaResetAtUnix)),
		WeeklyResetAt:         unixToRFC3339(rawMsgToInt64(ps.WeeklyQuotaResetAtUnix)),
		SubscriptionExpiresAt: pickSubscriptionExpiresFromPlanStatus(ps),
	}

	total := creditsToUnits(ps.AvailablePromptCredits) + creditsToUnits(ps.AvailableFlowCredits)
	usedPrompt := 0
	if ps.UsedPromptCredits != nil {
		usedPrompt = *ps.UsedPromptCredits
	}
	usedUsage := 0
	if ps.UsedUsageCredits != nil {
		usedUsage = *ps.UsedUsageCredits
	}
	used := creditsToUnits(usedPrompt) + creditsToUnits(usedUsage)
	profile.TotalCredits = total
	profile.UsedCredits = used
	if total > 0 || used > 0 {
		profile.RemainingCredits = total - used
		if profile.RemainingCredits < 0 {
			profile.RemainingCredits = 0
		}
	}

	if profile.PlanName == "" {
		profile.PlanName = "Free"
	}
	return profile
}

func parseUserStatusPayload(payload []byte) *AccountProfile {
	root := decodeProtoMessage(payload)
	userBlock := root.firstBytes(1)
	if len(userBlock) == 0 {
		return nil
	}

	user := decodeProtoMessage(userBlock)
	profile := &AccountProfile{
		Name:  user.firstString(3),
		Email: user.firstString(7),
	}

	tier := int(user.firstVarint(6))
	if tier == 0 && user.hasVarint(10) {
		tier = int(user.firstVarint(10))
	}
	profile.Tier = tier
	profile.TierLabel = tierLabel(tier)

	if planBlock := user.firstBytes(13); len(planBlock) > 0 {
		plan := decodeProtoMessage(planBlock)
		if planInfo := plan.firstBytes(1); len(planInfo) > 0 {
			planInfoFields := decodeProtoMessage(planInfo)
			profile.PlanName = normalizePlanName(planInfoFields.firstString(2))
		}
		// field 8/9: credits（与 JSON 路径的 AvailablePromptCredits/FlowCredits 对应）
		if plan.hasVarint(8) || plan.hasVarint(9) {
			total := creditsToUnits(int(plan.firstVarint(8))) + creditsToUnits(int(plan.firstVarint(9)))
			profile.TotalCredits = total
			profile.RemainingCredits = total
		}
		hasF14 := plan.hasVarint(14)
		hasF15 := plan.hasVarint(15)
		f14Val := plan.firstVarint(14)
		f15Val := plan.firstVarint(15)
		f17Val := plan.firstVarint(17)
		f18Val := plan.firstVarint(18)
		utils.DLog("[parseUserStatus] %s: hasF14=%v(%d) hasF15=%v(%d) F17=%d F18=%d",
			profile.Email, hasF14, f14Val, hasF15, f15Val, f17Val, f18Val)
		if hasF14 {
			v := float64(f14Val)
			profile.DailyQuotaRemaining = &v
		}
		if hasF15 {
			v := float64(f15Val)
			profile.WeeklyQuotaRemaining = &v
		}
		// ★ 服务端在配额用尽时会省略 F14/F15 字段（不是返回0）。
		// 当前只把「缺失的日额度」视为 0%，因为日限见底需要尽快切号；
		// 周额度缺失时不再伪造，避免把“周额度未知”误判成“周额度耗尽”。
		hasResetAt := f17Val > 0 || f18Val > 0
		if hasF15 && !hasF14 {
			zero := 0.0
			profile.DailyQuotaRemaining = &zero
		}
		if !hasF14 && !hasF15 && hasResetAt {
			// 有 resetAt 说明是日/周配额制套餐（Pro/Trial），两者都省略时至少视为日额度已见底。
			// 周额度保持未知，等待后续官方刷新返回真实值。
			zero := 0.0
			profile.DailyQuotaRemaining = &zero
		}
		if planEnd := subscriptionEndFromPlanProto(plan); planEnd != "" {
			profile.SubscriptionExpiresAt = planEnd
		}
		profile.DailyResetAt = unixToRFC3339(int64(plan.firstVarint(17)))
		profile.WeeklyResetAt = unixToRFC3339(int64(plan.firstVarint(18)))
	}

	if profile.SubscriptionExpiresAt == "" {
		profile.SubscriptionExpiresAt = parseProtoTimestamp(user.firstBytes(34))
	}
	if profile.PlanName == "" && profile.TierLabel != "" {
		profile.PlanName = normalizePlanName(profile.TierLabel)
	}
	return profile
}

func unwrapGRPCPayload(raw []byte) ([]byte, error) {
	if len(raw) < 5 {
		return nil, fmt.Errorf("gRPC响应体太短(%d bytes)", len(raw))
	}
	payload := raw[5:]
	if len(payload) >= 2 && payload[0] == 0x1f && payload[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("解压 gRPC payload 失败: %w", err)
		}
		defer reader.Close()
		decoded, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("读取解压 payload 失败: %w", err)
		}
		return decoded, nil
	}
	return payload, nil
}

type protoField struct {
	Number uint64
	Wire   uint64
	Varint uint64
	Bytes  []byte
}

type protoMessage []protoField

func decodeProtoMessage(data []byte) protoMessage {
	fields := make(protoMessage, 0)
	pos := 0
	for pos < len(data) {
		tag, next, ok := utils.ReadVarintSimple(data, pos)
		if !ok {
			break
		}
		pos = next
		fieldNum := tag >> 3
		wireType := tag & 7

		field := protoField{Number: fieldNum, Wire: wireType}
		switch wireType {
		case 0:
			val, next, ok := utils.ReadVarintSimple(data, pos)
			if !ok {
				return fields
			}
			field.Varint = val
			pos = next
		case 2:
			length, next, ok := utils.ReadVarintSimple(data, pos)
			if !ok {
				return fields
			}
			pos = next
			end := pos + int(length)
			if end > len(data) {
				return fields
			}
			field.Bytes = append([]byte(nil), data[pos:end]...)
			pos = end
		case 5:
			pos += 4
		case 1:
			pos += 8
		default:
			return fields
		}
		fields = append(fields, field)
	}
	return fields
}

func (m protoMessage) firstBytes(number uint64) []byte {
	for _, field := range m {
		if field.Number == number && field.Wire == 2 {
			return field.Bytes
		}
	}
	return nil
}

func (m protoMessage) firstString(number uint64) string {
	b := m.firstBytes(number)
	if len(b) == 0 {
		return ""
	}
	return string(b)
}

func (m protoMessage) firstVarint(number uint64) uint64 {
	for _, field := range m {
		if field.Number == number && field.Wire == 0 {
			return field.Varint
		}
	}
	return 0
}

func (m protoMessage) hasVarint(number uint64) bool {
	for _, field := range m {
		if field.Number == number && field.Wire == 0 {
			return true
		}
	}
	return false
}

func parseProtoTimestamp(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	fields := decodeProtoMessage(data)
	seconds := int64(fields.firstVarint(1))
	if seconds == 0 {
		return ""
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}

// pickSubscriptionExpiresFromPlanStatus 合并 GetPlanStatus(JSON) 多种时间字段。
// 部分环境 planEnd 实为当前计费周期起点；若同时存在 planStart，取二者中较晚者作为更接近「到期」的时间。
func pickSubscriptionExpiresFromPlanStatus(ps planStatusPayload) string {
	for _, raw := range []json.RawMessage{
		ps.SubscriptionPeriodEnd,
		ps.CurrentPeriodEnd,
		ps.PlanExpiresAt,
		ps.SubscriptionExpiresAtJSON,
	} {
		if s := parseFlexiblePlanEnd(raw); s != "" {
			return s
		}
	}
	start := parseFlexiblePlanEnd(ps.PlanStart)
	end := parseFlexiblePlanEnd(ps.PlanEnd)
	if start != "" && end != "" {
		return laterRFC3339String(start, end)
	}
	if end != "" {
		return end
	}
	return start
}

func laterRFC3339String(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	ta, oka := parseRFC3339Flexible(a)
	tb, okb := parseRFC3339Flexible(b)
	if !oka || !okb {
		return b
	}
	if tb.After(ta) {
		return b
	}
	return a
}

func parseRFC3339Flexible(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func latestRFC3339Strings(values []string) string {
	var best time.Time
	var bestS string
	for _, s := range values {
		if s == "" {
			continue
		}
		t, ok := parseRFC3339Flexible(s)
		if !ok {
			continue
		}
		if bestS == "" || t.After(best) {
			best = t
			bestS = s
		}
	}
	return bestS
}

// subscriptionEndFromPlanProto 取 plan 内 field 2/3 的 google.protobuf.Timestamp 中较晚者（避免原先「先取 2」把周期起点当到期）。
func subscriptionEndFromPlanProto(plan protoMessage) string {
	var candidates []string
	for _, n := range []uint64{2, 3} {
		if ts := parseProtoTimestamp(plan.firstBytes(n)); ts != "" {
			candidates = append(candidates, ts)
		}
	}
	return latestRFC3339Strings(candidates)
}

func creditsToUnits(v int) int {
	return int(float64(v) / 100.0)
}

func tierLabel(tier int) string {
	switch tier {
	case 0:
		return "Free"
	case 1:
		return "Basic"
	case 2:
		return "" // tier=2 是通用注册用户标记，不代表具体套餐（Free/Pro/Enterprise 都可能是 2）
	case 3:
		return "Teams"
	case 9:
		return "Trial"
	default:
		return ""
	}
}

func normalizePlanName(plan string) string {
	normalized := strings.ToLower(strings.TrimSpace(plan))
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")

	switch normalized {
	case "":
		return ""
	case "free":
		return "Free"
	case "basic":
		return "Basic"
	case "trial", "pro trial":
		return "Trial"
	case "max", "pro max", "ultimate", "pro ultimate":
		return "Max"
	case "pro":
		return "Pro"
	case "teams", "teams ultimate", "team":
		return "Teams"
	case "enterprise":
		return "Enterprise"
	default:
		if strings.Contains(normalized, "trial") {
			return "Trial"
		}
		if strings.Contains(normalized, "max") || strings.Contains(normalized, "ultimate") {
			return "Max"
		}
		if strings.Contains(normalized, "enterprise") {
			return "Enterprise"
		}
		if strings.Contains(normalized, "team") {
			return "Teams"
		}
		if strings.Contains(normalized, "pro") {
			return "Pro"
		}
		if strings.Contains(normalized, "free") || strings.Contains(normalized, "basic") {
			return "Free"
		}
		return plan
	}
}

func normalizeQuotedString(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	return value
}

// rawMsgToInt64 从 json.RawMessage 提取 int64，兼容字符串 "123" 和裸数字 123 两种 JSON 格式。
func rawMsgToInt64(raw json.RawMessage) int64 {
	if len(raw) == 0 {
		return 0
	}
	s := strings.Trim(string(raw), "\" \t\n")
	if s == "" || s == "null" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func unixToRFC3339(unix int64) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

func stringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

func boolValue(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func numberValue(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func parseCurrentUserResponse(data []byte) map[string]interface{} {
	result := make(map[string]interface{})
	pos := 0
	for pos < len(data) {
		tag, newPos, ok := utils.ReadVarintSimple(data, pos)
		if !ok {
			break
		}
		pos = newPos
		wireType := tag & 7
		fieldNum := tag >> 3

		switch wireType {
		case 0:
			val, newPos, ok := utils.ReadVarintSimple(data, pos)
			if !ok {
				return result
			}
			pos = newPos
			if fieldNum >= 2 && fieldNum <= 20 && val < 100000 {
				key := fmt.Sprintf("varint_%d", fieldNum)
				result[key] = val
			}
		case 2:
			length, newPos, ok := utils.ReadVarintSimple(data, pos)
			if !ok {
				return result
			}
			pos = newPos
			ln := int(length)
			if pos+ln > len(data) {
				return result
			}
			fieldData := data[pos : pos+ln]
			s := string(fieldData)
			if strings.Contains(s, "@") && len(s) < 200 {
				result["email"] = s
			}
			if isPlanName(s) {
				result["plan_name"] = s
			}
			if strings.HasPrefix(s, "sk-ws-") {
				result["api_key"] = s
			}
			if ln > 5 {
				nested := parseCurrentUserResponse(fieldData)
				for k, v := range nested {
					if _, exists := result[k]; !exists {
						result[k] = v
					}
				}
			}
			pos += ln
		case 5:
			pos += 4
		case 1:
			pos += 8
		default:
			return result
		}
	}
	return result
}

func parsePlanStatusResponse(data []byte) map[string]interface{} {
	result := make(map[string]interface{})
	pos := 0
	varintFields := make(map[uint64]uint64)
	for pos < len(data) {
		tag, newPos, ok := utils.ReadVarintSimple(data, pos)
		if !ok {
			break
		}
		pos = newPos
		wireType := tag & 7
		fieldNum := tag >> 3

		switch wireType {
		case 0:
			val, newPos, ok := utils.ReadVarintSimple(data, pos)
			if !ok {
				return result
			}
			pos = newPos
			varintFields[fieldNum] = val
		case 2:
			length, newPos, ok := utils.ReadVarintSimple(data, pos)
			if !ok {
				return result
			}
			pos = newPos
			ln := int(length)
			if pos+ln > len(data) {
				return result
			}
			pos += ln
		case 5:
			pos += 4
		case 1:
			pos += 8
		default:
			return result
		}
	}
	// 从 varint 字段中提取常见的额度数据
	// 这些字段号可能因版本而异，做宽松匹配
	for fn, val := range varintFields {
		if val > 0 && val < 100000 {
			result[fmt.Sprintf("field_%d", fn)] = int(val)
		}
	}
	return result
}

func isPlanName(s string) bool {
	plans := []string{"free", "pro", "teams", "enterprise", "trial", "pro_ultimate", "teams_ultimate"}
	lower := strings.ToLower(s)
	for _, p := range plans {
		if lower == p {
			return true
		}
	}
	return false
}
