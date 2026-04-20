package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"windsurf-tools-wails/backend/paths"
)

// ══════════════════════════════════════════════════════════════
// 全量抓包：记录 MITM 代理经过的 每一个 请求/响应 到 JSONL 文件
// 请求/响应 body 另存为 .bin 文件，JSONL 中只保留路径引用。
// ══════════════════════════════════════════════════════════════

// CaptureEntry 一条抓包记录（请求或响应）
type CaptureEntry struct {
	Seq        int               `json:"seq"`
	Time       string            `json:"time"`
	Type       string            `json:"type"` // "req" | "resp"
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	Host       string            `json:"host,omitempty"`
	Status     int               `json:"status,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	BodyFile   string            `json:"body_file,omitempty"`
	BodySize   int               `json:"body_size"`
	PoolKey    string            `json:"pool_key,omitempty"`
	ConvID     string            `json:"conv_id,omitempty"`
	GRPCStatus string            `json:"grpc_status,omitempty"`
	GRPCMsg    string            `json:"grpc_msg,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type captureWriter struct {
	mu      sync.Mutex
	file    *os.File
	dir     string
	seq     int
	bodyDir string
}

var (
	captureMu   sync.Mutex
	captureInst *captureWriter
)

// CaptureDir 返回全量抓包目录
func CaptureDir() string {
	appDir, err := paths.ResolveAppConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(appDir, "capture")
}

func ensureCaptureWriter() *captureWriter {
	captureMu.Lock()
	defer captureMu.Unlock()
	if captureInst != nil {
		return captureInst
	}
	dir := CaptureDir()
	if dir == "" {
		return nil
	}
	os.MkdirAll(dir, 0755)
	bodyDir := filepath.Join(dir, "bodies")
	os.MkdirAll(bodyDir, 0755)

	ts := time.Now().Format("20060102_150405")
	logPath := filepath.Join(dir, fmt.Sprintf("capture_%s.jsonl", ts))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil
	}
	captureInst = &captureWriter{file: f, dir: dir, bodyDir: bodyDir}
	return captureInst
}

func closeCaptureWriter() {
	captureMu.Lock()
	defer captureMu.Unlock()
	if captureInst != nil && captureInst.file != nil {
		captureInst.file.Close()
	}
	captureInst = nil
}

func (cw *captureWriter) writeEntry(entry CaptureEntry) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.seq++
	entry.Seq = cw.seq
	entry.Time = time.Now().Format("2006-01-02T15:04:05.000")
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	cw.file.Write(data)
	cw.file.Write([]byte("\n"))
}

func (cw *captureWriter) saveBody(seq int, tag string, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	fname := fmt.Sprintf("%06d_%s.bin", seq, tag)
	fpath := filepath.Join(cw.bodyDir, fname)
	os.WriteFile(fpath, body, 0644)
	return fname
}

// 采集有价值的 header 子集，避免过大
func captureHeaders(h http.Header, keys []string) map[string]string {
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		if v := h.Get(k); v != "" {
			if len(v) > 300 {
				v = v[:300] + "..."
			}
			m[k] = v
		}
	}
	return m
}

var reqHeaderKeys = []string{
	"Content-Type", "Content-Length", "Authorization",
	"X-Pool-Key-Used", "X-Conv-ID", "User-Agent",
	"Accept", "Accept-Encoding", "Connect-Protocol-Version",
}

var respHeaderKeys = []string{
	"Content-Type", "Content-Length",
	"Grpc-Status", "Grpc-Message",
	"Connect-Content-Type", "Trailer",
}

// ── 全量抓包采集点 ──

// captureRequest 在 handleRequest 最前面调用，记录请求并返回 body bytes（避免重复读）。
// 如果抓包未启用则返回 nil（调用方需自行读 body）。
func (p *MitmProxy) captureRequest(req *http.Request) []byte {
	if !p.FullCaptureEnabled() {
		return nil
	}
	cw := ensureCaptureWriter()
	if cw == nil {
		return nil
	}

	// 读 body
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}

	cw.mu.Lock()
	seq := cw.seq + 1
	cw.mu.Unlock()

	path := req.URL.Path
	bodyFile := ""
	if len(bodyBytes) > 0 {
		bodyFile = cw.saveBody(seq, "req", bodyBytes)
	}

	entry := CaptureEntry{
		Type:     "req",
		Method:   req.Method,
		Path:     path,
		Host:     req.Host,
		Headers:  captureHeaders(req.Header, reqHeaderKeys),
		BodyFile: bodyFile,
		BodySize: len(bodyBytes),
		PoolKey:  req.Header.Get("X-Pool-Key-Used"),
		ConvID:   req.Header.Get("X-Conv-ID"),
	}
	cw.writeEntry(entry)

	// 还原 body（必须用 bytes.NewReader 保证二进制安全）
	if len(bodyBytes) > 0 {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	return bodyBytes
}

// captureResponse 在 handleResponse 最前面调用。
// 小包（<1MB 且已知长度）：一次性读取并保存 body。
// 流式包（ContentLength == -1，如 SSE/gRPC streaming）：用 TeeReader 包装，
// 边转发给 IDE 边把 body 写入 .bin 文件，关闭时写 JSONL 记录。
func (p *MitmProxy) captureResponse(resp *http.Response) {
	if !p.FullCaptureEnabled() {
		return
	}
	cw := ensureCaptureWriter()
	if cw == nil {
		return
	}

	path := resp.Request.URL.Path

	cw.mu.Lock()
	seq := cw.seq + 1
	cw.mu.Unlock()

	entry := CaptureEntry{
		Type:       "resp",
		Method:     resp.Request.Method,
		Path:       path,
		Status:     resp.StatusCode,
		Headers:    captureHeaders(resp.Header, respHeaderKeys),
		PoolKey:    resp.Request.Header.Get("X-Pool-Key-Used"),
		ConvID:     resp.Request.Header.Get("X-Conv-ID"),
		GRPCStatus: resp.Header.Get("Grpc-Status"),
		GRPCMsg:    resp.Header.Get("Grpc-Message"),
	}

	if resp.Body == nil {
		entry.BodySize = 0
		cw.writeEntry(entry)
		return
	}

	// 小包：一次性读取
	const maxCapBody = 1 << 20 // 1MB
	if resp.ContentLength >= 0 && resp.ContentLength < maxCapBody {
		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err == nil && len(bodyBytes) > 0 {
			entry.BodyFile = cw.saveBody(seq, "resp", bodyBytes)
			entry.BodySize = len(bodyBytes)
		}
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		cw.writeEntry(entry)
		return
	}

	// 流式包 / 大包：用 TeeReader 边转发边落盘
	fname := fmt.Sprintf("%06d_resp_stream.bin", seq)
	fpath := filepath.Join(cw.bodyDir, fname)
	sf, err := os.Create(fpath)
	if err != nil {
		// 无法创建文件，只记录元信息
		entry.BodySize = -1
		cw.writeEntry(entry)
		return
	}

	entry.BodyFile = fname
	entry.BodySize = -1 // 流式，最终大小在关闭后更新

	// 先写一条 JSONL（BodySize=-1 表示流式进行中）
	cw.writeEntry(entry)

	origBody := resp.Body
	tee := io.TeeReader(origBody, sf)
	resp.Body = &streamCaptureReadCloser{
		Reader: tee,
		close: func() error {
			err := origBody.Close()
			sf.Close()
			// 写一条流式完成记录（带最终大小）
			if info, statErr := os.Stat(fpath); statErr == nil {
				finalEntry := entry
				finalEntry.Type = "resp_end"
				finalEntry.BodySize = int(info.Size())
				finalEntry.Time = time.Now().Format("2006-01-02T15:04:05.000")
				cw.writeEntry(finalEntry)
			}
			return err
		},
	}
}

// streamCaptureReadCloser 包装 TeeReader，在 Close 时执行回调
type streamCaptureReadCloser struct {
	io.Reader
	close func() error
}

func (s *streamCaptureReadCloser) Close() error {
	return s.close()
}

// ── MitmProxy 开关 ──

// SetFullCapture 开启/关闭全量抓包
func (p *MitmProxy) SetFullCapture(enabled bool) {
	p.mu.Lock()
	p.fullCapture = enabled
	p.mu.Unlock()
	if enabled {
		ensureCaptureWriter()
		p.log("★ 全量抓包已开启，目录: %s", CaptureDir())
	} else {
		closeCaptureWriter()
		p.log("★ 全量抓包已关闭")
	}
}

// FullCaptureEnabled 返回全量抓包是否开启
func (p *MitmProxy) FullCaptureEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.fullCapture
}
