package services

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"golang.org/x/net/http2"
)

// TestLiveChatResponseStructure 发一次真实聊天请求，dump 响应 envelope 的所有字段，
// 用来定位 ParseChatResponseChunk 该提取哪个字段作为 delta。
func TestLiveChatResponseStructure(t *testing.T) {
	if os.Getenv("WS_LIVE_INTEGRATION") != "1" {
		t.Skip()
	}
	acc := loadFirstUsableAccount(t)
	if acc == nil {
		t.Skip()
	}
	t.Logf("账号: %s", acc.Email)

	fp := &KeyFingerprint{
		DeviceHash: generateStableHexHash(),
		SessionID:  "00000000-0000-0000-0000-000000000000",
	}
	msgs := []ChatMessage{{Role: "user", Content: "回复仅一个字: 好"}}
	protoBody := BuildChatRequestWithModel(msgs, acc.APIKey, acc.Token, "", "claude-opus-4-7-medium", fp)
	body := WrapGRPCEnvelopeGzip(protoBody)

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

	req, _ := http.NewRequest("POST", connectURL, bytes.NewReader(body))
	req.Host = GRPCUpstreamHost
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Authorization", "Bearer "+acc.Token)
	req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.1)")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Connect-Content-Encoding", "gzip")
	req.Header.Set("Connect-Accept-Encoding", "gzip")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	t.Logf("HTTP %d, %d bytes, CT=%s", resp.StatusCode, len(raw), resp.Header.Get("Content-Type"))

	// 按 5 字节 envelope 逐帧拆
	pos := 0
	frameNum := 0
	for pos+5 <= len(raw) {
		flag := raw[pos]
		size := uint32(raw[pos+1])<<24 | uint32(raw[pos+2])<<16 | uint32(raw[pos+3])<<8 | uint32(raw[pos+4])
		pos += 5
		if pos+int(size) > len(raw) {
			t.Logf("frame %d: size %d 越界，剩余 %d", frameNum, size, len(raw)-pos)
			break
		}
		payload := raw[pos : pos+int(size)]
		pos += int(size)
		frameNum++
		t.Logf("=== Frame %d: flag=0x%02x size=%d ===", frameNum, flag, size)

		// flag 0x01 = compressed, 0x02 = end-of-stream
		if flag&0x01 != 0 {
			if r, err := gzip.NewReader(bytes.NewReader(payload)); err == nil {
				if dec, err := io.ReadAll(r); err == nil {
					t.Logf("  gzip 解压: %d → %d bytes", size, len(dec))
					payload = dec
				}
				r.Close()
			}
		}
		if flag&0x02 != 0 {
			// end-of-stream JSON trailer
			t.Logf("  [EOS trailer] %s", string(payload))
			continue
		}
		// 打印字段树
		t.Log(DumpProtoFieldTree(payload, 5))
	}
}
