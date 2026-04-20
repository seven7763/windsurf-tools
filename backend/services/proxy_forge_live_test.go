package services

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestLiveForgeGetUserStatus(t *testing.T) {
	apiKey := os.Getenv("WS_LIVE_API_KEY")
	if apiKey == "" {
		t.Skip("WS_LIVE_API_KEY is not set")
	}

	svc := NewWindsurfService("")

	grpcURL := fmt.Sprintf("https://%s/exa.seat_management_pb.SeatManagementService/GetUserStatus", ResolveUpstreamIP())
	metadata := buildAPIKeyMetadata(apiKey)
	envelope := buildGRPCEnvelope(metadata)

	req, err := http.NewRequest("POST", grpcURL, bytes.NewReader(envelope))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Authorization", apiKey)
	req.Host = GRPCUpstreamHost

	resp, err := svc.client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d, grpc-status=%s, body=%s",
			resp.StatusCode, resp.Header.Get("grpc-status"), truncate(string(rawBody), 200))
	}

	t.Logf("原始响应: %d bytes (format: %s)", len(rawBody), detectResponseFormat(rawBody))

	origPayload, err := unwrapGRPCPayload(rawBody)
	if err != nil {
		t.Fatalf("unwrap original: %v", err)
	}
	origProfile := parseUserStatusPayload(origPayload)
	if origProfile == nil {
		t.Fatal("original profile is nil")
	}
	t.Logf("原始: Email=%s Plan=%s Tier=%d(%s) Credits=%d/%d Daily=%s Weekly=%s Expires=%s",
		origProfile.Email, origProfile.PlanName, origProfile.Tier, origProfile.TierLabel,
		origProfile.RemainingCredits, origProfile.TotalCredits,
		fmtQuota(origProfile.DailyQuotaRemaining), fmtQuota(origProfile.WeeklyQuotaRemaining),
		origProfile.SubscriptionExpiresAt)

	cfg := DefaultForgeConfig()
	forgedBody := forgeUserStatusResponse(rawBody, cfg)
	t.Logf("伪造响应: %d bytes (原始 %d bytes, delta %+d)", len(forgedBody), len(rawBody), len(forgedBody)-len(rawBody))

	forgedPayload, err := unwrapGRPCPayload(forgedBody)
	if err != nil {
		t.Fatalf("unwrap forged: %v", err)
	}
	forgedProfile := parseUserStatusPayload(forgedPayload)
	if forgedProfile == nil {
		t.Fatal("forged profile is nil")
	}
	t.Logf("伪造: Email=%s Plan=%s Tier=%d(%s) Credits=%d/%d Daily=%s Weekly=%s Expires=%s",
		forgedProfile.Email, forgedProfile.PlanName, forgedProfile.Tier, forgedProfile.TierLabel,
		forgedProfile.RemainingCredits, forgedProfile.TotalCredits,
		fmtQuota(forgedProfile.DailyQuotaRemaining), fmtQuota(forgedProfile.WeeklyQuotaRemaining),
		forgedProfile.SubscriptionExpiresAt)

	// ── Verification ──
	if forgedProfile.Email == "" {
		t.Error("Email 不应为空")
	}
	if forgedProfile.Email != origProfile.Email {
		t.Errorf("Email 应保持不变: %q → %q", origProfile.Email, forgedProfile.Email)
	}
	if forgedProfile.PlanName != "Enterprise" && forgedProfile.PlanName != "enterprise" {
		t.Errorf("PlanName = %q, 期望 Enterprise", forgedProfile.PlanName)
	}
	if forgedProfile.TotalCredits < 100000 {
		t.Errorf("TotalCredits = %d, 期望 >= 100000", forgedProfile.TotalCredits)
	}
	if origProfile.PlanName == forgedProfile.PlanName && origProfile.TotalCredits == forgedProfile.TotalCredits {
		t.Log("⚠️ 伪造前后数据完全相同，forge 可能未生效")
	}
	t.Logf("✅ forge 验证通过: %s → %s, credits %d → %d",
		origProfile.PlanName, forgedProfile.PlanName,
		origProfile.TotalCredits, forgedProfile.TotalCredits)
}

func TestLiveForgeRelay(t *testing.T) {
	apiKey := os.Getenv("WS_LIVE_API_KEY")
	if apiKey == "" {
		t.Skip("WS_LIVE_API_KEY is not set")
	}

	svc := NewWindsurfService("")
	jwt, err := svc.GetJWTByAPIKey(apiKey)
	if err != nil {
		t.Fatalf("GetJWTByAPIKey: %v", err)
	}
	t.Logf("JWT 获取成功: %d chars", len(jwt))

	mitmProxy := NewMitmProxy(svc, func(msg string) { t.Log(msg) }, "", nil)
	mitmProxy.SetPoolKeys([]string{apiKey})
	mitmProxy.SetForgeConfig(DefaultForgeConfig())
	mitmProxy.SetStaticCacheConfig(StaticCacheConfig{Enabled: false})
	mitmProxy.updateJWT(apiKey, []byte(jwt))
	mitmProxy.markJWTReady()

	key, jwtBytes := mitmProxy.pickPoolKeyAndJWT()
	if key == "" || len(jwtBytes) == 0 {
		t.Fatal("pickPoolKeyAndJWT 返回空")
	}
	t.Logf("号池 key=%s... JWT=%d bytes", truncKey(key), len(jwtBytes))

	// ── 诊断: 尝试多个账号直到找到可用的 ──
	t.Log("=== 诊断: 尝试多个账号 ===")

	// 尝试当前 key 和最多 5 个备选 key
	testKeys := []string{apiKey}
	allAccounts := loadTestAccountKeys(t, 5)
	for _, k := range allAccounts {
		if k != apiKey {
			testKeys = append(testKeys, k)
		}
		if len(testKeys) >= 6 {
			break
		}
	}

	var workingKey, workingJWT string
	for _, tk := range testKeys {
		tj, err := svc.GetJWTByAPIKey(tk)
		if err != nil {
			t.Logf("  key=%s... JWT失败: %v", truncKey(tk), err)
			continue
		}
		messages := []ChatMessage{{Role: "user", Content: "Say hi"}}
		protoBody := BuildChatRequest(messages, tk, tj, "", nil)
		// Connect 协议：无 envelope 格式（unary request body）
		// 有 envelope 返回 invalid_argument，无 envelope 返回 resource_exhausted

		testRelay := &OpenAIRelay{
			proxy:    mitmProxy,
			logFn:    func(msg string) {},
			upstream: (&OpenAIRelay{proxyURL: ""}).buildUpstreamTransport(),
		}
		testResp, _, testErr := testRelay.sendGRPC(protoBody, tk, tj)
		if testErr != nil {
			t.Logf("  key=%s... sendGRPC失败: %v", truncKey(tk), testErr)
			continue
		}
		testBody, _ := io.ReadAll(testResp.Body)
		testResp.Body.Close()

		envs := ExtractGRPCEnvelopes(testBody)
		hasData := false
		for _, env := range envs {
			if env.Flags&streamEnvelopeEndStream == 0 {
				hasData = true
				break
			}
			decoded, _ := decodeStreamEnvelopePayload(env.Flags, env.Payload)
			t.Logf("  key=%s... end-stream: %s", truncKey(tk), truncate(string(decoded), 150))
		}
		if hasData {
			workingKey = tk
			workingJWT = tj
			t.Logf("  ✅ key=%s... 有数据帧!", truncKey(tk))
			break
		}
		t.Logf("  key=%s... 无数据帧 (%d envelopes)", truncKey(tk), len(envs))
	}

	if workingKey == "" {
		t.Log("所有账号都返回 resource_exhausted/invalid_argument，额度可能全部耗尽")
		t.Skip("无可用账号用于 relay 测试")
	}

	apiKey = workingKey
	jwt = workingJWT
	mitmProxy.SetPoolKeys([]string{apiKey})
	mitmProxy.updateJWT(apiKey, []byte(jwt))

	messages := []ChatMessage{{Role: "user", Content: "Say hello in exactly 3 words."}}
	protoBody := BuildChatRequest(messages, apiKey, jwt, "", nil)

	resp, kind, sendErr := (&OpenAIRelay{
		proxy:    mitmProxy,
		logFn:    func(msg string) { t.Log(msg) },
		upstream: (&OpenAIRelay{proxyURL: ""}).buildUpstreamTransport(),
	}).sendGRPC(protoBody, apiKey, jwt)
	if sendErr != nil {
		t.Fatalf("sendGRPC 失败 (kind=%s): %v", kind, sendErr)
	}
	defer resp.Body.Close()

	rawResp, _ := io.ReadAll(resp.Body)
	t.Logf("gRPC 原始响应: %d bytes, proto=%s, ct=%s", len(rawResp), resp.Proto, resp.Header.Get("Content-Type"))

	envelopes := ExtractGRPCEnvelopes(rawResp)
	t.Logf("gRPC envelopes: %d 个", len(envelopes))
	for i, env := range envelopes {
		decoded, decErr := decodeStreamEnvelopePayload(env.Flags, env.Payload)
		if decErr != nil {
			t.Logf("  [%d] flags=0x%02x len=%d decode-err=%v", i, env.Flags, len(env.Payload), decErr)
			continue
		}
		text, isDone, _ := ParseChatResponseChunk(decoded)
		t.Logf("  [%d] flags=0x%02x payload=%d decoded=%d text=%q isDone=%v raw=%q",
			i, env.Flags, len(env.Payload), len(decoded), truncate(text, 100), isDone, truncate(string(decoded), 200))
		if text == "" && len(decoded) > 0 {
			fields := parseProtoFields(decoded)
			for _, f := range fields {
				if f.Wire == 2 {
					t.Logf("    F%d(bytes): len=%d utf8=%v preview=%q",
						f.Number, len(f.Bytes), isLikelyUTF8(f.Bytes), truncate(string(f.Bytes), 80))
				} else if f.Wire == 0 {
					t.Logf("    F%d(varint): %d", f.Number, f.Varint)
				}
			}
		}
	}

	frames := ExtractGRPCFrames(rawResp)
	var fullText string
	for _, frame := range frames {
		text, _, _ := ParseChatResponseChunk(frame)
		fullText += text
	}
	t.Logf("ExtractGRPCFrames: %d frames, fullText=%q", len(frames), truncate(fullText, 300))

	// ── Relay 端到端测试 (流式) ──
	t.Log("=== Relay 端到端测试 (流式) ===")
	relay := NewOpenAIRelay(mitmProxy, func(msg string) { t.Log(msg) }, "", nil)
	if err := relay.Start(0, ""); err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Stop()

	status := relay.Status()
	t.Logf("Relay 启动: %s", status.URL)

	streamReq := `{"model":"cascade","messages":[{"role":"user","content":"Reply with exactly: Hello World Today"}],"stream":true}`
	streamResp, err := http.Post(status.URL+"/v1/chat/completions", "application/json", bytes.NewReader([]byte(streamReq)))
	if err != nil {
		t.Fatalf("relay stream request: %v", err)
	}
	defer streamResp.Body.Close()

	streamBody, _ := io.ReadAll(streamResp.Body)
	t.Logf("Relay 流式响应: HTTP %d, %d bytes", streamResp.StatusCode, len(streamBody))
	lines := bytes.Split(streamBody, []byte("\n"))
	var sseContent string
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		data := string(line[6:])
		if data == "[DONE]" {
			t.Log("  SSE: [DONE]")
			continue
		}
		t.Logf("  SSE: %s", truncate(data, 200))
		// 提取 content
		if idx := bytes.Index(line, []byte(`"content":"`)); idx >= 0 {
			rest := line[idx+11:]
			if end := bytes.Index(rest, []byte(`"`)); end >= 0 {
				sseContent += string(rest[:end])
			}
		}
	}
	t.Logf("SSE 完整内容: %q", sseContent)

	if streamResp.StatusCode != 200 {
		t.Errorf("流式期望 200, 实际 %d", streamResp.StatusCode)
	}
	if sseContent == "" {
		t.Error("流式响应内容为空")
	}
}

func fmtQuota(q *float64) string {
	if q == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%.1f%%", *q)
}

func loadTestAccountKeys(t *testing.T, maxN int) []string {
	t.Helper()
	acctPath := os.Getenv("APPDATA") + "\\WindsurfTools\\accounts.json"
	data, err := os.ReadFile(acctPath)
	if err != nil {
		return nil
	}
	type acct struct {
		WindsurfAPIKey string `json:"windsurf_api_key"`
	}
	var accounts []acct
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil
	}
	var keys []string
	for _, a := range accounts {
		if a.WindsurfAPIKey != "" {
			keys = append(keys, a.WindsurfAPIKey)
		}
		if len(keys) >= maxN {
			break
		}
	}
	return keys
}

func detectResponseFormat(body []byte) string {
	if len(body) == 0 {
		return "empty"
	}
	if isGRPCWebFrame(body) {
		return "gRPC-Web"
	}
	if len(body) >= 3 && body[0] == 0x1f && body[1] == 0x8b && body[2] == 0x08 {
		return "gzip"
	}
	if len(body) > 5 {
		flags := body[0]
		if flags == 0x00 || flags == 0x01 {
			return fmt.Sprintf("envelope(flag=0x%02x)", flags)
		}
	}
	return "raw-protobuf"
}

// TestLiveDiagRelayProtoFormat 诊断 Relay 请求格式问题
// 逐步排查: dump proto 字段树 → 发送请求 → 捕获完整错误 → 尝试不同版本号
func TestLiveDiagRelayProtoFormat(t *testing.T) {
	apiKey := os.Getenv("WS_LIVE_API_KEY")
	if apiKey == "" {
		// 尝试从 accounts.json 取第一个
		keys := loadTestAccountKeys(t, 1)
		if len(keys) == 0 {
			t.Skip("WS_LIVE_API_KEY not set and no accounts.json")
		}
		apiKey = keys[0]
	}

	svc := NewWindsurfService("")
	jwt, err := svc.GetJWTByAPIKey(apiKey)
	if err != nil {
		t.Fatalf("GetJWTByAPIKey: %v", err)
	}
	t.Logf("JWT 获取成功: %d chars, key=%s...", len(jwt), truncKey(apiKey))

	// 构造 per-key 指纹
	fp := &KeyFingerprint{
		SessionID:  generateStableUUID(),
		DeviceHash: generateStableHexHash(),
	}

	// ── Step 1: Dump BuildChatRequestWithModel 的字段树 ──
	t.Log("=== Step 1: Dump BuildChatRequestWithModel 字段树 (含 F5/F8/F16/F27/F30/F31/F32) ===")
	messages := []ChatMessage{{Role: "user", Content: "Say hi"}}
	protoBody := BuildChatRequestWithModel(messages, apiKey, jwt, "", "", fp)
	t.Logf("Proto body: %d bytes", len(protoBody))
	t.Log(DumpProtoFieldTree(protoBody, 8))

	// ── Step 2: Connect 协议（gzip 压缩 envelope — 匹配真实 IDE）──
	t.Log("=== Step 2: Connect 协议 (gzip 压缩 envelope) ===")
	gzipEnvelope := WrapGRPCEnvelopeGzip(protoBody)
	sendAndDiagnose(t, gzipEnvelope, jwt, "connect-gzip-envelope", true, true)

	// ── Step 3: Connect 协议（无压缩 envelope）──
	t.Log("=== Step 3: Connect 协议 (无压缩 envelope) ===")
	envelopePayload := WrapGRPCEnvelope(protoBody)
	sendAndDiagnose(t, envelopePayload, jwt, "connect-plain-envelope", true, false)
}

func sendAndDiagnose(t *testing.T, payload []byte, jwt, label string, useConnect, compressed bool) {
	t.Helper()
	upIP := ResolveUpstreamIP()
	connectURL := fmt.Sprintf("https://%s/exa.api_server_pb.ApiServerService/GetChatMessage", upIP)

	req, err := http.NewRequest(http.MethodPost, connectURL, bytes.NewReader(payload))
	if err != nil {
		t.Errorf("[%s] create request: %v", label, err)
		return
	}
	req.Host = GRPCUpstreamHost
	if useConnect {
		req.Header.Set("Content-Type", "application/connect+proto")
		req.Header.Set("Connect-Protocol-Version", "1")
		req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("Authorization", "Bearer "+jwt)
		if compressed {
			req.Header.Set("Connect-Content-Encoding", "gzip")
		}
	} else {
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("te", "trailers")
		req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
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

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Errorf("[%s] RoundTrip: %v", label, err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	grpcStatus := resp.Header.Get("grpc-status")
	grpcMsg := resp.Header.Get("grpc-message")
	ct := resp.Header.Get("Content-Type")

	t.Logf("[%s] HTTP %d, proto=%s, ct=%s, grpc-status=%s, grpc-msg=%s, body=%d bytes",
		label, resp.StatusCode, resp.Proto, ct, grpcStatus, grpcMsg, len(body))

	// 如果有 Connect EOS trailer
	if len(body) >= 5 {
		envelopes := ExtractGRPCEnvelopes(body)
		for i, env := range envelopes {
			decoded, decErr := decodeStreamEnvelopePayload(env.Flags, env.Payload)
			if decErr != nil {
				t.Logf("[%s] env[%d] flags=0x%02x len=%d decode-err=%v", label, i, env.Flags, len(env.Payload), decErr)
				continue
			}
			text := truncate(string(decoded), 500)
			t.Logf("[%s] env[%d] flags=0x%02x decoded=%s", label, i, env.Flags, text)

			// 如果是 JSON 错误
			if strings.HasPrefix(strings.TrimSpace(text), "{") {
				var errObj map[string]interface{}
				if json.Unmarshal(decoded, &errObj) == nil {
					t.Logf("[%s] Error JSON: %v", label, errObj)
				}
			}
		}
	}

	// 非 envelope 格式的 body
	if len(body) > 0 && (len(body) < 5 || (body[0] != 0x00 && body[0] != 0x01 && body[0] != 0x02)) {
		t.Logf("[%s] Raw body: %s", label, truncate(string(body), 500))
	}
	if len(body) == 0 {
		// gRPC streaming: body may be empty, check trailers
		t.Logf("[%s] Empty body (gRPC streaming), checking trailers...", label)
		resp.Body.Close()
		trailerStatus := resp.Trailer.Get("grpc-status")
		trailerMsg := resp.Trailer.Get("grpc-message")
		t.Logf("[%s] Trailer grpc-status=%s grpc-message=%s", label, trailerStatus, trailerMsg)
		if trailerStatus != "" && trailerStatus != "0" {
			kind, detail := classifyUpstreamFailure(trailerStatus, trailerMsg, "")
			t.Logf("[%s] Trailer error: kind=%s detail=%s", label, kind, detail)
		} else {
			t.Logf("[%s] gRPC stream accepted (no error in trailers)!", label)
		}
	}
}
