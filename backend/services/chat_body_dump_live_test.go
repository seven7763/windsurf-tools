package services

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestDumpIDEGetChatMessageBody 抽真实 IDE 的 GetChatMessage 请求体，打印其顶层字段结构。
func TestDumpIDEGetChatMessageBody(t *testing.T) {
	if os.Getenv("WS_LIVE_INTEGRATION") != "1" {
		t.Skip()
	}
	capDir := filepath.Join(os.Getenv("APPDATA"), "WindsurfTools", "capture", "bodies")
	// 从抓包 70 起，找 GetChatMessage 请求（142614 bytes 是第一个）
	data, err := os.ReadFile(filepath.Join(capDir, "000070_req.bin"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Logf("IDE GetChatMessage 请求体: %d bytes, head=%x", len(data), data[:16])
	// 剥 5 字节 Connect envelope
	if len(data) > 5 && data[0] <= 3 {
		declared := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
		if int(declared) == len(data)-5 {
			t.Logf("剥 envelope flag=0x%02x len=%d", data[0], declared)
			data = data[5:]
		}
	}
	// envelope flag 0x01 说明内部 gzip 压缩
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			if dec, err := io.ReadAll(r); err == nil {
				t.Logf("gzip 解压: %d → %d bytes", len(data), len(dec))
				data = dec
			}
			r.Close()
		}
	}
	// 只显示顶层字段（depth=1），不递归展开 sub-message
	root := decodeProtoMessage(data)
	t.Logf("=== 顶层字段统计 ===")
	counts := map[uint64]int{}
	for _, f := range root {
		counts[f.Number]++
		preview := ""
		if f.Wire == 2 {
			preview = fmt.Sprintf("bytes/len=%d", len(f.Bytes))
			// 如果看起来是字符串就显示前 80 字节
			if len(f.Bytes) < 200 {
				preview += " " + fmt.Sprintf("%q", string(f.Bytes))
			}
		} else if f.Wire == 0 {
			preview = fmt.Sprintf("varint=%d", f.Varint)
		}
		t.Logf("  F%d (wire=%d) %s", f.Number, f.Wire, preview)
	}
	t.Logf("=== 字段出现次数 ===")
	for num, cnt := range counts {
		t.Logf("  F%d: %d 次", num, cnt)
	}
}
