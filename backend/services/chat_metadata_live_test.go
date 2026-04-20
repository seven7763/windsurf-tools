package services

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/http2"
)

// TestLiveChatMetadataFix 直接打真实的 GetChatMessage 端点，验证修正后的 buildChatMetadata
// 是否还会被服务端拒绝为 resource_exhausted / invalid_argument。
//
// 启动方式：
//
//	$env:WS_LIVE_INTEGRATION=1; go test -v -run TestLiveChatMetadataFix ./backend/services
//
// 会自动从 accounts.json 取第一个有 apiKey+token 的账号。
func TestLiveChatMetadataFix(t *testing.T) {
	requireIntegrationEnv(t)

	acc := loadFirstUsableAccount(t)
	if acc == nil {
		t.Skip("accounts.json 里没有可用账号")
	}
	t.Logf("测试账号: %s  (plan=%s)", acc.Email, acc.PlanName)

	fp := &KeyFingerprint{
		DeviceHash: generateStableHexHash(),
		SessionID:  "00000000-0000-0000-0000-000000000000", // 兜底，会被 team_id 覆盖
	}
	msgs := []ChatMessage{{Role: "user", Content: "hello"}}
	protoBody := BuildChatRequestWithModel(msgs, acc.APIKey, acc.Token, "", "claude-opus-4-7-medium", fp)
	t.Logf("raw proto body: %d bytes", len(protoBody))
	// ★ IDE 实际发送 gzip-compressed envelope (flag=0x01)，不是裸 body
	body := WrapGRPCEnvelopeGzip(protoBody)
	t.Logf("wrapped (gzip envelope): %d bytes", len(body))

	// 打印 metadata 字段树便于排查
	fields := decodeProtoMessage(body)
	for _, f := range fields {
		if f.Number == 1 {
			t.Logf("--- metadata 字段树 ---")
			t.Log(DumpProtoFieldTree(f.Bytes, 3))
			break
		}
	}

	upIP := ResolveUpstreamIP()
	connectURL := fmt.Sprintf("https://%s/exa.api_server_pb.ApiServerService/GetChatMessage", upIP)

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2: true,
	}
	_ = http2.ConfigureTransport(transport)
	client := &http.Client{Transport: transport}

	req, err := http.NewRequest("POST", connectURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = GRPCUpstreamHost
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Authorization", "Bearer "+acc.Token)
	req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
	req.Header.Set("Accept-Encoding", "identity")
	// ★ Connect streaming 帧级压缩：每个 envelope 内 gzip，通过此 header 协商算法
	req.Header.Set("Connect-Content-Encoding", "gzip")
	req.Header.Set("Connect-Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	t.Logf("HTTP %d, resp %d bytes, CT=%s", resp.StatusCode, len(respBody), resp.Header.Get("Content-Type"))

	// 尝试 gzip 解压
	if len(respBody) >= 2 && respBody[0] == 0x1f && respBody[1] == 0x8b {
		if r, err := gzip.NewReader(bytes.NewReader(respBody)); err == nil {
			if d, err := io.ReadAll(r); err == nil {
				respBody = d
				t.Logf("gzip 解压后: %d bytes", len(respBody))
			}
			r.Close()
		}
	}
	// 截断显示
	snippet := string(respBody)
	if len(snippet) > 500 {
		snippet = snippet[:500] + "...(truncated)"
	}
	t.Logf("响应: %s", snippet)

	if strings.Contains(string(respBody), "resource_exhausted") {
		t.Fatalf("❌ 仍然返回 resource_exhausted，metadata 还是有问题")
	}
	if strings.Contains(string(respBody), "invalid_argument") {
		t.Fatalf("❌ 返回 invalid_argument，metadata 字段结构有错")
	}
	if resp.StatusCode == 200 {
		t.Logf("✅ HTTP 200，服务端接受了 metadata")
	}
}

type liveAccount struct {
	Email    string `json:"email"`
	PlanName string `json:"plan_name"`
	APIKey   string `json:"windsurf_api_key"`
	Token    string `json:"token"`
}

func loadFirstUsableAccount(t *testing.T) *liveAccount {
	t.Helper()
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}
	accPath := filepath.Join(appData, "WindsurfTools", "accounts.json")
	data, err := os.ReadFile(accPath)
	if err != nil {
		return nil
	}
	var all []liveAccount
	if err := json.Unmarshal(data, &all); err != nil {
		return nil
	}
	for i := range all {
		a := &all[i]
		if a.APIKey != "" && a.Token != "" {
			return a
		}
	}
	return nil
}
