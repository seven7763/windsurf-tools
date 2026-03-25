package services

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

const (
	testEmail    = "2riyduna7@yinxuan.icu"
	testPassword = "2riyduna7@yinxuan.icu"
)

// ═══ Step 1: Login ═══

func TestLogin(t *testing.T) {
	svc := NewWindsurfService("")
	resp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	t.Logf("✅ Login 成功")
	t.Logf("   IDToken长度: %d", len(resp.IDToken))
	t.Logf("   RefreshToken长度: %d", len(resp.RefreshToken))
	t.Logf("   IDToken前100字符: %s", resp.IDToken[:min(100, len(resp.IDToken))])
}

// ═══ Step 2: Login → JWT Claims ═══

func TestLoginAndDecodeJWT(t *testing.T) {
	svc := NewWindsurfService("")
	resp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	claims, err := svc.DecodeJWTClaims(resp.IDToken)
	if err != nil {
		t.Fatalf("JWT解码失败: %v", err)
	}
	t.Logf("✅ JWT Claims:")
	t.Logf("   Email: %s", claims.Email)
	t.Logf("   Pro: %v", claims.Pro)
	t.Logf("   TeamsTier: %s", claims.TeamsTier)
	t.Logf("   DisplayName: %s", claims.Name)
}

// ═══ Step 3: Login → RegisterUser (获取 APIKey) ═══

func TestLoginAndRegisterUser(t *testing.T) {
	svc := NewWindsurfService("")
	resp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	reg, err := svc.RegisterUser(resp.IDToken)
	if err != nil {
		t.Fatalf("RegisterUser 失败: %v", err)
	}
	t.Logf("✅ RegisterUser 成功")
	t.Logf("   APIKey: %s", reg.APIKey)
	t.Logf("   Name: %s", reg.Name)
}

// ═══ Step 4: Login → RegisterUser → GetJWTByAPIKey ═══

func TestGetJWTByAPIKey(t *testing.T) {
	svc := NewWindsurfService("")
	resp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	reg, err := svc.RegisterUser(resp.IDToken)
	if err != nil {
		t.Fatalf("RegisterUser 失败: %v", err)
	}
	t.Logf("APIKey: %s", reg.APIKey)

	jwt, err := svc.GetJWTByAPIKey(reg.APIKey)
	if err != nil {
		t.Fatalf("GetJWTByAPIKey 失败: %v", err)
	}
	t.Logf("✅ GetJWTByAPIKey 成功, JWT长度=%d", len(jwt))
}

// ═══ Step 5: GetPlanStatusJSON (关键：Enterprise 额度) ═══

func TestGetPlanStatusJSON(t *testing.T) {
	svc := NewWindsurfService("")
	resp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	plan, err := svc.GetPlanStatusJSON(resp.IDToken)
	if err != nil {
		t.Fatalf("GetPlanStatusJSON 失败: %v", err)
	}
	t.Logf("✅ GetPlanStatusJSON 成功:")
	t.Logf("   PlanName: %s", plan.PlanName)
	t.Logf("   BillingStrategy: %s", plan.BillingStrategy)
	t.Logf("   TotalCredits: %d", plan.TotalCredits)
	t.Logf("   UsedCredits: %d", plan.UsedCredits)
	t.Logf("   RemainingCredits: %d", plan.RemainingCredits)
	if plan.DailyQuotaRemaining != nil {
		t.Logf("   DailyQuotaRemaining: %.4f", *plan.DailyQuotaRemaining)
	} else {
		t.Logf("   DailyQuotaRemaining: <nil>")
	}
	if plan.WeeklyQuotaRemaining != nil {
		t.Logf("   WeeklyQuotaRemaining: %.4f", *plan.WeeklyQuotaRemaining)
	} else {
		t.Logf("   WeeklyQuotaRemaining: <nil>")
	}
	t.Logf("   DailyResetAt: %s", plan.DailyResetAt)
	t.Logf("   WeeklyResetAt: %s", plan.WeeklyResetAt)
	t.Logf("   SubscriptionExpiresAt: %s", plan.SubscriptionExpiresAt)

	// 打印完整 JSON 以便分析
	jsonBytes, _ := json.MarshalIndent(plan, "", "  ")
	t.Logf("   完整Profile JSON:\n%s", string(jsonBytes))
}

// ═══ Step 6: GetUserStatus (gRPC) ═══

func TestGetUserStatus(t *testing.T) {
	svc := NewWindsurfService("")
	resp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	reg, err := svc.RegisterUser(resp.IDToken)
	if err != nil {
		t.Fatalf("RegisterUser 失败: %v", err)
	}
	t.Logf("APIKey: %s", reg.APIKey)

	profile, err := svc.GetUserStatus(reg.APIKey)
	if err != nil {
		t.Logf("⚠️ GetUserStatus 失败: %v", err)
		t.Logf("   这可能是 Enterprise 的正常情况")
		return
	}
	t.Logf("✅ GetUserStatus 成功:")
	t.Logf("   PlanName: %s", profile.PlanName)
	if profile.DailyQuotaRemaining != nil {
		t.Logf("   DailyQuotaRemaining: %.4f", *profile.DailyQuotaRemaining)
	} else {
		t.Logf("   DailyQuotaRemaining: <nil>")
	}
	if profile.WeeklyQuotaRemaining != nil {
		t.Logf("   WeeklyQuotaRemaining: %.4f", *profile.WeeklyQuotaRemaining)
	} else {
		t.Logf("   WeeklyQuotaRemaining: <nil>")
	}
	t.Logf("   TotalCredits: %d", profile.TotalCredits)
	t.Logf("   UsedCredits: %d", profile.UsedCredits)
	jsonBytes, _ := json.MarshalIndent(profile, "", "  ")
	t.Logf("   完整Profile JSON:\n%s", string(jsonBytes))
}

// ═══ Step 7: Chat gRPC 路径测试 ═══

func TestChatGRPCPath(t *testing.T) {
	svc := NewWindsurfService("")
	loginResp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	reg, err := svc.RegisterUser(loginResp.IDToken)
	if err != nil {
		t.Fatalf("RegisterUser 失败: %v", err)
	}
	apiKey := reg.APIKey
	jwt := loginResp.IDToken
	t.Logf("APIKey: %s", apiKey)

	// 构建 chat request
	messages := []ChatMessage{{Role: "user", Content: "hello"}}
	protoBody := BuildChatRequest(messages, apiKey, jwt, "")
	grpcPayload := WrapGRPCEnvelope(protoBody)

	upIP := ResolveUpstreamIP()
	t.Logf("上游IP: %s", upIP)

	// 测试多条可能的 gRPC 路径
	paths := []string{
		"/exa.chat_pb.ChatService/GetChatMessage",
		"/exa.language_server_pb.LanguageServerService/GetChatMessage",
		"/windsurf.chat.v1.ChatService/GetChatMessage",
		"/exa.api_server_pb.ApiServerService/GetChatMessage",
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	http2.ConfigureTransport(transport)

	for _, path := range paths {
		grpcURL := fmt.Sprintf("https://%s%s", upIP, path)
		req, _ := http.NewRequest("POST", grpcURL, bytes.NewReader(grpcPayload))
		req.Host = GRPCUpstreamHost
		req.Header.Set("content-type", "application/grpc")
		req.Header.Set("te", "trailers")
		req.Header.Set("authorization", "Bearer "+jwt)
		req.Header.Set("user-agent", "connect-es/1.6.1")

		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Logf("❌ %s → 网络错误: %v", path, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		grpcStatus := resp.Header.Get("grpc-status")
		grpcMsg := resp.Header.Get("grpc-message")
		t.Logf("路径: %s → HTTP %d, proto=%s, grpc-status=%s, grpc-msg=%s, body=%d bytes",
			path, resp.StatusCode, resp.Proto, grpcStatus, grpcMsg, len(body))

		if resp.StatusCode == 200 && (grpcStatus == "" || grpcStatus == "0") {
			t.Logf("✅ 找到正确路径: %s", path)
			// 尝试解析前几个字节
			if len(body) > 5 {
				t.Logf("   响应前20字节: %x", body[:min(20, len(body))])
			}
		}
		if len(body) > 0 && len(body) < 500 {
			t.Logf("   body: %s", string(body))
		}
	}
}

// ═══ Step 8: 获取原始 PlanStatus JSON 响应 ═══

func TestRawPlanStatusResponse(t *testing.T) {
	svc := NewWindsurfService("")
	loginResp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	token := loginResp.IDToken

	// 直接调用 API 查看原始响应
	reqBody, _ := json.Marshal(map[string]string{"auth_token": token})
	apiURL := WindsurfBaseURL + "/exa.seat_management_pb.SeatManagementService/GetPlanStatus"
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Auth-Token", token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	t.Logf("HTTP %d", resp.StatusCode)

	// 美化 JSON
	var pretty bytes.Buffer
	if json.Indent(&pretty, body, "", "  ") == nil {
		t.Logf("原始响应:\n%s", pretty.String())
	} else {
		t.Logf("原始响应(raw): %s", string(body))
	}
}

// ═══ Step 9: 完整 enrichment 流程测试 ═══

func TestFullEnrichment(t *testing.T) {
	svc := NewWindsurfService("")

	// 模拟一个只有 email+password 的账号
	t.Log("=== 测试: 从 email+password 完整 enrich ===")

	// Step 1: Login
	loginResp, err := svc.LoginWithEmail(testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login 失败: %v", err)
	}
	token := loginResp.IDToken
	refreshToken := loginResp.RefreshToken
	t.Logf("Login OK: tokenLen=%d refreshLen=%d", len(token), len(refreshToken))

	// Step 2: JWT Claims
	claims, err := svc.DecodeJWTClaims(token)
	if err != nil {
		t.Logf("JWT解码失败: %v", err)
	} else {
		t.Logf("JWT Claims: email=%s pro=%v tier=%s name=%s", claims.Email, claims.Pro, claims.TeamsTier, claims.Name)
	}

	// Step 3: RegisterUser
	reg, err := svc.RegisterUser(token)
	if err != nil {
		t.Logf("RegisterUser 失败: %v", err)
	} else {
		t.Logf("RegisterUser: apiKey=%s name=%s", reg.APIKey, reg.Name)
	}

	var apiKey string
	if reg != nil {
		apiKey = reg.APIKey
	}

	// Step 4: GetPlanStatusJSON
	plan, err := svc.GetPlanStatusJSON(token)
	if err != nil {
		t.Logf("❌ GetPlanStatusJSON 失败: %v", err)
	} else {
		t.Logf("GetPlanStatusJSON: plan=%s daily=%v weekly=%v total=%d used=%d remaining=%d",
			plan.PlanName, plan.DailyQuotaRemaining, plan.WeeklyQuotaRemaining,
			plan.TotalCredits, plan.UsedCredits, plan.RemainingCredits)
	}

	// Step 5: GetUserStatus
	if apiKey != "" {
		profile, err := svc.GetUserStatus(apiKey)
		if err != nil {
			t.Logf("❌ GetUserStatus 失败: %v", err)
		} else {
			t.Logf("GetUserStatus: plan=%s daily=%v weekly=%v total=%d used=%d",
				profile.PlanName, profile.DailyQuotaRemaining, profile.WeeklyQuotaRemaining,
				profile.TotalCredits, profile.UsedCredits)
		}
	}

	// Step 6: 总结
	t.Log("=== 总结 ===")
	if plan != nil {
		t.Logf("最终 Plan: %s", plan.PlanName)
		if plan.DailyQuotaRemaining != nil {
			t.Logf("  Daily: %.2f%%", *plan.DailyQuotaRemaining*100)
		} else {
			t.Logf("  Daily: <无数据>")
		}
		if plan.WeeklyQuotaRemaining != nil {
			t.Logf("  Weekly: %.2f%%", *plan.WeeklyQuotaRemaining*100)
		} else {
			t.Logf("  Weekly: <无数据>")
		}
	}
}

// ═══ 辅助 ═══

func TestDNSResolve(t *testing.T) {
	ip := ResolveUpstreamIP()
	t.Logf("ResolveUpstreamIP: %s", ip)
	t.Logf("UpstreamHost: %s", UpstreamHost)
	t.Logf("GRPCUpstreamHost: %s", GRPCUpstreamHost)
	t.Logf("UpstreamIP(硬编码): %s", UpstreamIP)

	// 也测试 windsurf.go 里的常量
	t.Logf("WindsurfBaseURL: %s", WindsurfBaseURL)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ═══ 额外：测试 MITM proxy 用的 gRPC 路径（从 IDE 抓包的真实路径）═══

func TestGRPCPathDiscovery(t *testing.T) {
	upIP := ResolveUpstreamIP()
	t.Logf("上游IP: %s", upIP)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	http2.ConfigureTransport(transport)

	// 尝试 OPTIONS/HEAD 探测服务列表
	// 以及常见的 gRPC 反射路径
	testPaths := []string{
		"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
	}

	for _, path := range testPaths {
		grpcURL := fmt.Sprintf("https://%s%s", upIP, path)
		req, _ := http.NewRequest("POST", grpcURL, strings.NewReader(""))
		req.Host = GRPCUpstreamHost
		req.Header.Set("content-type", "application/grpc")
		req.Header.Set("te", "trailers")

		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Logf("❌ %s → %v", path, err)
			continue
		}
		resp.Body.Close()
		t.Logf("%s → HTTP %d, grpc-status=%s", path, resp.StatusCode, resp.Header.Get("grpc-status"))
	}
}
