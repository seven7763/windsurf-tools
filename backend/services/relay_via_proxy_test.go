package services

import (
	"bytes"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// TestLiveRelayFormats 测试不同请求格式组合，找出能成功的格式
func TestLiveRelayFormats(t *testing.T) {
	apiKey := os.Getenv("WS_LIVE_API_KEY")
	if apiKey == "" {
		keys := loadTestAccountKeys(t, 1)
		if len(keys) == 0 {
			t.Skip("WS_LIVE_API_KEY not set")
		}
		apiKey = keys[0]
	}

	svc := NewWindsurfService("")
	jwt, err := svc.GetJWTByAPIKey(apiKey)
	if err != nil {
		t.Fatalf("GetJWTByAPIKey: %v", err)
	}

	fp := &KeyFingerprint{
		SessionID:  generateStableUUID(),
		DeviceHash: generateStableHexHash(),
	}
	// 简单消息
	simpleMessages := []ChatMessage{{Role: "user", Content: "Say hi"}}
	simpleBody := BuildChatRequestWithModel(simpleMessages, apiKey, jwt, "", "", fp)

	// 完整对话（模拟 IDE 请求：system + 多轮对话）
	fullMessages := []ChatMessage{
		{Role: "system", Content: "You are an intelligent assistant named Windsurf. Always provide helpful, accurate, and safe responses. You are running inside a VS Code extension that helps users with coding tasks. Be concise and direct in your responses."},
		{Role: "user", Content: "Hello, can you help me with something?"},
		{Role: "assistant", Content: "Of course! I'm here to help. What would you like assistance with?"},
		{Role: "user", Content: "Say hi"},
	}
	fullBody := BuildChatRequestWithModel(fullMessages, apiKey, jwt, "", "", fp)

	// ── Test 1: Connect + 无 envelope + 简单消息 ──
	t.Log("=== Test 1: 简单消息 (无 envelope) ===")
	testFormat(t, simpleBody, jwt, "simple-no-env", true, false, false)

	// ── Test 1b: Connect + 无 envelope + 完整对话 ──
	t.Log("=== Test 1b: 完整对话 (无 envelope) ===")
	testFormat(t, fullBody, jwt, "full-no-env", true, false, false)

	// ── Test 2: 从 proto dump 读取真实 IDE 请求，直接发送 ──
	t.Log("=== Test 2: 重放真实 IDE 请求 (从 dump 读取完整 body) ===")
	dumpBody := readLatestReqDump(t)
	if dumpBody != nil {
		// 用 ReplaceIdentityInBody 替换 key/JWT（和 MITM 代理一样）
		replacedBody, replaced := ReplaceIdentityInBody(dumpBody, []byte(apiKey), []byte(jwt), false, fp)
		if !replaced {
			t.Log("ReplaceIdentityInBody did not modify (key may already match)")
			replacedBody = dumpBody
		}
		t.Logf("Dump body: %d bytes, replaced: %d bytes, replaced=%v", len(dumpBody), len(replacedBody), replaced)
		// 检查 body 格式
		if len(replacedBody) > 5 && (replacedBody[0] == 0x00 || replacedBody[0] == 0x01) {
			isCompressed := replacedBody[0]&0x01 != 0
			t.Logf("Body is envelope format, compressed=%v", isCompressed)
			testFormat(t, replacedBody, jwt, "dump-replay", true, isCompressed, false)
		} else {
			t.Log("Body is not envelope format, sending as-is")
			testFormat(t, replacedBody, jwt, "dump-replay", true, false, false)
		}
	} else {
		t.Log("No dump found, skipping")
	}

	// ── Test 3: 复用连接测试 — 先发一个请求建立连接，再发第二个 ──
	t.Log("=== Test 3: 复用连接测试 ===")
	testReuseConnection(t, fullBody, jwt)

	// ── Test 4: 通过 MITM 代理发送请求（模拟 IDE 请求路径）──
	t.Log("=== Test 4: 通过 MITM 代理发送 (模拟 IDE 路径) ===")
	{
		// 发送到代理 (localhost:443)，代理会转发到上游
		proxyURL := "https://127.0.0.1:443/exa.api_server_pb.ApiServerService/GetChatMessage"

		// 构造 gzip envelope body（和 IDE 一样）
		gzipEnvelope := WrapGRPCEnvelopeGzip(fullBody)

		req, err := http.NewRequest(http.MethodPost, proxyURL, bytes.NewReader(gzipEnvelope))
		if err != nil {
			t.Fatalf("create request: %v", err)
		}
		req.Host = GRPCUpstreamHost
		req.Header.Set("Content-Type", "application/connect+proto")
		req.Header.Set("Connect-Protocol-Version", "1")
		req.Header.Set("Connect-Content-Encoding", "gzip")
		req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
		req.Header.Set("Accept-Encoding", "identity")

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				ServerName:         GRPCUpstreamHost,
				NextProtos:         []string{"h2"},
			},
			ForceAttemptHTTP2:     true,
			DisableCompression:    true,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
		}
		http2.ConfigureTransport(transport)

		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatalf("RoundTrip via proxy: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		ct := resp.Header.Get("Content-Type")
		t.Logf("[via-proxy] HTTP %d, ct=%s, body=%d bytes", resp.StatusCode, ct, len(body))
		envs := ExtractGRPCEnvelopes(body)
		for i, env := range envs {
			decoded, decErr := decodeStreamEnvelopePayload(env.Flags, env.Payload)
			if decErr != nil {
				t.Logf("[via-proxy] env[%d] flags=0x%02x decode-err=%v", i, env.Flags, decErr)
				continue
			}
			t.Logf("[via-proxy] env[%d] flags=0x%02x decoded=%s", i, env.Flags, truncate(string(decoded), 200))
		}
	}
}

func testFormat(t *testing.T, payload []byte, jwt, label string, useConnect, isEnvelopeCompressed, isBodyGzip bool) {
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
		if isEnvelopeCompressed {
			req.Header.Set("Connect-Content-Encoding", "gzip")
		}
		if isBodyGzip {
			req.Header.Set("Content-Encoding", "gzip")
		}
	} else {
		req.Header.Set("Content-Type", "application/grpc")
		req.Header.Set("te", "trailers")
		req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
		req.Header.Set("Authorization", "Bearer "+jwt)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
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
	ct := resp.Header.Get("Content-Type")
	t.Logf("[%s] HTTP %d, ct=%s, body=%d bytes", label, resp.StatusCode, ct, len(body))

	envs := ExtractGRPCEnvelopes(body)
	for i, env := range envs {
		decoded, decErr := decodeStreamEnvelopePayload(env.Flags, env.Payload)
		if decErr != nil {
			t.Logf("[%s] env[%d] flags=0x%02x decode-err=%v", label, i, env.Flags, decErr)
			continue
		}
		t.Logf("[%s] env[%d] flags=0x%02x decoded=%s", label, i, env.Flags, truncate(string(decoded), 200))
	}
}

// readLatestReqDump reads the latest req_GetChatMessage dump and returns the raw binary body
func readLatestReqDump(t *testing.T) []byte {
	t.Helper()
	dumpDir := ProtoDumpDir()
	if dumpDir == "" {
		return nil
	}
	entries, err := os.ReadDir(dumpDir)
	if err != nil {
		return nil
	}
	var reqDump string
	for i := len(entries) - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Name(), "req_GetChatMessage") {
			reqDump = dumpDir + "\\" + entries[i].Name()
			break
		}
	}
	if reqDump == "" {
		return nil
	}
	dumpContent, err := os.ReadFile(reqDump)
	if err != nil {
		return nil
	}
	hexStart := bytes.Index(dumpContent, []byte("=== Raw Hex ==="))
	if hexStart < 0 {
		return nil
	}
	hexData := dumpContent[hexStart+len("=== Raw Hex ==="):]
	var hexStrBuf strings.Builder
	for _, r := range string(hexData) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			hexStrBuf.WriteRune(r)
		} else if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			// skip whitespace/newlines in hex data
			continue
		} else if hexStrBuf.Len() > 0 {
			// hit a non-hex, non-whitespace char → end of hex data
			break
		}
	}
	hexBytes, err := hex.DecodeString(hexStrBuf.String())
	if err != nil || len(hexBytes) < 5 {
		return nil
	}
	return hexBytes
}

// testReuseConnection 测试复用 HTTP/2 连接是否影响结果
func testReuseConnection(t *testing.T, protoBody []byte, jwt string) {
	t.Helper()
	upIP := ResolveUpstreamIP()
	connectURL := fmt.Sprintf("https://%s/exa.api_server_pb.ApiServerService/GetChatMessage", upIP)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	http2.ConfigureTransport(transport)
	client := &http.Client{Transport: transport}

	for i := 0; i < 3; i++ {
		req, err := http.NewRequest(http.MethodPost, connectURL, bytes.NewReader(protoBody))
		if err != nil {
			t.Errorf("[reuse-%d] create request: %v", i, err)
			return
		}
		req.Host = GRPCUpstreamHost
		req.Header.Set("Content-Type", "application/connect+proto")
		req.Header.Set("Connect-Protocol-Version", "1")
		req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("Authorization", "Bearer "+jwt)

		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("[reuse-%d] Do: %v", i, err)
			return
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		envs := ExtractGRPCEnvelopes(body)
		result := "unknown"
		if len(envs) > 0 {
			decoded, decErr := decodeStreamEnvelopePayload(envs[0].Flags, envs[0].Payload)
			if decErr == nil {
				result = truncate(string(decoded), 150)
			}
		}
		t.Logf("[reuse-%d] HTTP %d, body=%d bytes, result=%s", i, resp.StatusCode, len(body), result)

		time.Sleep(2 * time.Second)
	}
}
