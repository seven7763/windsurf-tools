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
	"net/url"
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
	WindsurfClient   = "1.9566.11"
)

type WindsurfService struct {
	client *http.Client
}

func NewWindsurfService(proxyURL string) *WindsurfService {
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
		client: &http.Client{Timeout: 30 * time.Second, Transport: transport},
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

type JWTClaims struct {
	Email                  string
	Name                   string
	Pro                    bool
	TeamsTier              string
	TrialEnd               string
	MaxPremiumChatMessages int
	AuthUID                string
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
	UsedPromptCredits           int                    `json:"usedPromptCredits"`
	UsedUsageCredits            int                    `json:"usedUsageCredits"`
	DailyQuotaRemainingPercent  *float64               `json:"dailyQuotaRemainingPercent"`
	WeeklyQuotaRemainingPercent *float64               `json:"weeklyQuotaRemainingPercent"`
	DailyQuotaResetAtUnix       int64                  `json:"dailyQuotaResetAtUnix"`
	WeeklyQuotaResetAtUnix      int64                  `json:"weeklyQuotaResetAtUnix"`
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
	apiURL := fmt.Sprintf(
		"https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=%s",
		FirebaseAPIKey,
	)
	body := fmt.Sprintf(
		`{"returnSecureToken":true,"email":"%s","password":"%s","clientType":"CLIENT_TYPE_WEB"}`,
		email, password,
	)
	resp, err := s.client.Post(apiURL, "application/json", strings.NewReader(body))
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
	resp, err := s.client.Post(apiURL, "application/json", strings.NewReader(body))
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
	grpcURL := fmt.Sprintf("https://%s/exa.auth_pb.AuthService/GetUserJwt", GRPCUpstreamIP)
	metadata := buildAPIKeyMetadata(apiKey)
	envelope := buildGRPCEnvelope(metadata)

	req, _ := http.NewRequest("POST", grpcURL, bytes.NewReader(envelope))
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Authorization", apiKey)
	req.Host = GRPCUpstreamHost // 关键：必须用 req.Host 而不是 Header.Set

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gRPC JWT请求失败(网络): %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		grpcStatus := resp.Header.Get("grpc-status")
		grpcMsg := resp.Header.Get("grpc-message")
		return "", fmt.Errorf("gRPC JWT失败(HTTP %d, grpc-status=%s, msg=%s): %s",
			resp.StatusCode, grpcStatus, grpcMsg, truncate(string(respBody), 200))
	}

	if len(respBody) < 6 {
		return "", fmt.Errorf("gRPC响应体太短(%d bytes)", len(respBody))
	}

	payload := respBody[5:]
	jwt, found := utils.FindJWTInProtobuf(payload)
	if !found {
		return "", fmt.Errorf("gRPC响应中未找到JWT(payload %d bytes): %s",
			len(payload), truncate(string(payload), 200))
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
		payload.PlanStatus.UsedPromptCredits == 0 &&
		payload.PlanStatus.UsedUsageCredits == 0 &&
		len(payload.PlanStatus.TopUpStatus) == 0 {
		if err := json.Unmarshal(respBody, &payload.PlanStatus); err != nil {
			return nil, fmt.Errorf("解析 GetPlanStatus(JSON) 平铺响应失败: %w", err)
		}
	}
	return parsePlanStatusPayload(payload.PlanStatus), nil
}

func (s *WindsurfService) GetUserStatus(apiKey string) (*AccountProfile, error) {
	grpcURL := fmt.Sprintf("https://%s/exa.seat_management_pb.SeatManagementService/GetUserStatus", GRPCUpstreamIP)
	metadata := buildAPIKeyMetadata(apiKey)
	envelope := buildGRPCEnvelope(metadata)

	req, err := http.NewRequest("POST", grpcURL, bytes.NewReader(envelope))
	if err != nil {
		return nil, fmt.Errorf("创建 GetUserStatus 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Authorization", apiKey)
	req.Host = GRPCUpstreamHost

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetUserStatus请求失败(网络): %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GetUserStatus失败(HTTP %d, grpc-status=%s, msg=%s): %s",
			resp.StatusCode, resp.Header.Get("grpc-status"), resp.Header.Get("grpc-message"), truncate(string(respBody), 200))
	}

	payload, err := unwrapGRPCPayload(respBody)
	if err != nil {
		return nil, fmt.Errorf("解析 GetUserStatus 响应失败: %w", err)
	}

	profile := parseUserStatusPayload(payload)
	if profile == nil {
		return nil, fmt.Errorf("GetUserStatus 响应无法解析")
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
	}
	return claims, nil
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
	metadata = append(metadata, utils.EncodeStringField(5, "windows")...)
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
	profile := &AccountProfile{
		PlanName:              normalizePlanName(ps.PlanInfo.PlanName),
		BillingStrategy:       ps.PlanInfo.BillingStrategy,
		DailyQuotaRemaining:   ps.DailyQuotaRemainingPercent,
		WeeklyQuotaRemaining:  ps.WeeklyQuotaRemainingPercent,
		DailyResetAt:          unixToRFC3339(ps.DailyQuotaResetAtUnix),
		WeeklyResetAt:         unixToRFC3339(ps.WeeklyQuotaResetAtUnix),
		SubscriptionExpiresAt: pickSubscriptionExpiresFromPlanStatus(ps),
	}

	total := creditsToUnits(ps.AvailablePromptCredits) + creditsToUnits(ps.AvailableFlowCredits)
	used := creditsToUnits(ps.UsedPromptCredits) + creditsToUnits(ps.UsedUsageCredits)
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
	// API 有时不传日/周百分比，但可用额度已为 0 且已有消耗：补成 0%% 以便上层判定用尽并切号
	if profile.DailyQuotaRemaining == nil && profile.WeeklyQuotaRemaining == nil {
		if total == 0 && used > 0 {
			z := 0.0
			profile.DailyQuotaRemaining = &z
			profile.WeeklyQuotaRemaining = &z
		}
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
		if plan.hasVarint(14) {
			v := float64(plan.firstVarint(14))
			profile.DailyQuotaRemaining = &v
		}
		if plan.hasVarint(15) {
			v := float64(plan.firstVarint(15))
			profile.WeeklyQuotaRemaining = &v
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
	if profile.PlanName == "" {
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
		return "Enterprise"
	case 3:
		return "Teams"
	case 9:
		return "Trial"
	default:
		return "Unknown"
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
