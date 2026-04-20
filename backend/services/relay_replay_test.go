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

// TestLiveReplayRealIDERequest 重放真实 IDE 请求（从 proto dump 读取），
// 只替换 key/JWT，验证上游是否接受
func TestLiveReplayRealIDERequest(t *testing.T) {
	apiKey := os.Getenv("WS_LIVE_API_KEY")
	if apiKey == "" {
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

	// 读取最近的 req_GetChatMessage dump
	dumpDir := ProtoDumpDir()
	if dumpDir == "" {
		t.Skip("no proto dump dir")
	}
	entries, err := os.ReadDir(dumpDir)
	if err != nil {
		t.Skipf("read dump dir: %v", err)
	}
	var reqDump string
	for i := len(entries) - 1; i >= 0; i-- {
		if strings.Contains(entries[i].Name(), "req_GetChatMessage") {
			reqDump = dumpDir + "\\" + entries[i].Name()
			break
		}
	}
	if reqDump == "" {
		t.Skip("no req_GetChatMessage dump found")
	}
	t.Logf("Using dump: %s", reqDump)

	dumpContent, err := os.ReadFile(reqDump)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}

	// 从 dump 中提取 hex 数据
	hexStart := bytes.Index(dumpContent, []byte("=== Raw Hex ==="))
	if hexStart < 0 {
		t.Fatal("no Raw Hex section in dump")
	}
	hexData := dumpContent[hexStart+len("=== Raw Hex ==="):]
	// 读取所有 hex 字符（跳过空白字符），遇到非 hex 非空白字符停止
	var hexStrBuf strings.Builder
	for _, r := range string(hexData) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			hexStrBuf.WriteRune(r)
		} else if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			continue
		} else if hexStrBuf.Len() > 0 {
			break
		}
	}
	hexStr := hexStrBuf.String()

	hexBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}

	if len(hexBytes) < 5 {
		t.Fatalf("parsed hex too short: %d bytes", len(hexBytes))
	}

	// 去掉 5 字节 gRPC envelope
	payload := hexBytes[5:] // 去掉 5 字节 envelope，得到纯 protobuf payload
	t.Logf("Payload from dump: %d bytes", len(payload))

	// 用 ReplaceIdentity 替换 key/JWT
	newPayload, replaced := ReplaceIdentity(payload, []byte(apiKey), []byte(jwt), false, nil)
	if !replaced {
		t.Log("ReplaceIdentity did not modify payload (key may already match)")
		newPayload = payload
	}

	// 测试两种格式：无 envelope 和有 envelope
	// Connect 协议下，无 envelope 返回 resource_exhausted，有 envelope 返回 invalid_argument
	t.Logf("Payload: %d bytes, with envelope: %d bytes", len(newPayload), len(newPayload)+5)

	// 发送（先用无 envelope 格式，因为之前 resource_exhausted 比 invalid_argument 更接近成功）
	upIP := ResolveUpstreamIP()
	connectURL := fmt.Sprintf("https://%s/exa.api_server_pb.ApiServerService/GetChatMessage", upIP)

	req, err := http.NewRequest(http.MethodPost, connectURL, bytes.NewReader(newPayload))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = GRPCUpstreamHost
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
	req.Header.Set("Accept-Encoding", "identity")

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
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	grpcStatus := resp.Header.Get("grpc-status")
	grpcMsg := resp.Header.Get("grpc-message")
	ct := resp.Header.Get("Content-Type")

	t.Logf("HTTP %d, proto=%s, ct=%s, grpc-status=%s, grpc-msg=%s, body=%d bytes",
		resp.StatusCode, resp.Proto, ct, grpcStatus, grpcMsg, len(body))

	envs := ExtractGRPCEnvelopes(body)
	for i, env := range envs {
		decoded, decErr := decodeStreamEnvelopePayload(env.Flags, env.Payload)
		if decErr != nil {
			t.Logf("env[%d] flags=0x%02x len=%d decode-err=%v", i, env.Flags, len(env.Payload), decErr)
			continue
		}
		text := truncate(string(decoded), 500)
		t.Logf("env[%d] flags=0x%02x decoded=%s", i, env.Flags, text)
	}
}
