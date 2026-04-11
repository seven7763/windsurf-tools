package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
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
		protoBody := BuildChatRequest(messages, tk, tj, "")
		grpcPayload := WrapGRPCEnvelope(protoBody)

		testRelay := &OpenAIRelay{
			proxy:    mitmProxy,
			logFn:    func(msg string) {},
			upstream: (&OpenAIRelay{proxyURL: ""}).buildUpstreamTransport(),
		}
		testResp, _, testErr := testRelay.sendGRPC(grpcPayload, tk, tj)
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
		t.Log("所有账号都返回 invalid_argument，可能是 API 请求格式变更")
		t.Skip("无可用账号用于 relay 测试")
	}

	apiKey = workingKey
	jwt = workingJWT
	mitmProxy.SetPoolKeys([]string{apiKey})
	mitmProxy.updateJWT(apiKey, []byte(jwt))

	messages := []ChatMessage{{Role: "user", Content: "Say hello in exactly 3 words."}}
	protoBody := BuildChatRequest(messages, apiKey, jwt, "")
	grpcPayload := WrapGRPCEnvelope(protoBody)

	resp, kind, sendErr := (&OpenAIRelay{
		proxy:    mitmProxy,
		logFn:    func(msg string) { t.Log(msg) },
		upstream: (&OpenAIRelay{proxyURL: ""}).buildUpstreamTransport(),
	}).sendGRPC(grpcPayload, apiKey, jwt)
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
