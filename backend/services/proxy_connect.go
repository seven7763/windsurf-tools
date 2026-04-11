package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"
)

// ── Connect 协议 EOS Trailer 帧解析 ──
//
// Windsurf 使用 Connect 协议（不是标准 gRPC）。
// 流式端点（GetChatMessage/GetCompletions）的错误通过 HTTP 200 + EOS trailer 帧返回：
//
//   帧格式: [1字节flag][4字节bigendian长度][payload]
//   flag & 0x02 = end-of-stream (EOS trailer 帧)
//   flag & 0x01 = payload 已 gzip 压缩
//   payload 解压后是 JSON: {"error":{"code":"xxx","message":"yyy"}}
//
// 非流式端点的错误通过标准 HTTP 4xx + JSON body 返回：
//   {"code":"xxx","message":"yyy"}

// ConnectError represents a parsed Connect protocol error.
type ConnectError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ConnectEOSPayload represents the JSON payload inside an EOS trailer frame.
type ConnectEOSPayload struct {
	Error *ConnectError `json:"error"`
}

// ConnectErrorResult holds the parsed error info from either format.
type ConnectErrorResult struct {
	IsError bool
	IsEOS   bool   // true if error came from EOS trailer frame
	Code    string // Connect error code: resource_exhausted, permission_denied, etc.
	Message string // Human-readable error message
	RawJSON string // Raw JSON for logging
}

// ParseConnectEOS parses a Connect protocol response body to detect EOS trailer errors.
// It handles both:
//   - Format A: standard HTTP error body {"code":"xxx","message":"yyy"}
//   - Format B: EOS trailer frame [flag][len][json] where flag & 0x02
//
// The body should be the complete response body bytes.
func ParseConnectEOS(body []byte) ConnectErrorResult {
	if len(body) == 0 {
		return ConnectErrorResult{}
	}

	// Try Format B first: Connect EOS trailer frame
	if result := parseConnectFrame(body); result.IsError {
		return result
	}

	// Try to find EOS frame at end of body (after data frames)
	if result := findEOSInBody(body); result.IsError {
		return result
	}

	// Try Format A: plain JSON error body
	if result := parseConnectJSONError(body); result.IsError {
		return result
	}

	return ConnectErrorResult{}
}

// parseConnectFrame tries to parse a single Connect frame starting at body[0].
func parseConnectFrame(body []byte) ConnectErrorResult {
	if len(body) < 5 {
		return ConnectErrorResult{}
	}

	flag := body[0]
	isEOS := flag&0x02 != 0
	isGzipped := flag&0x01 != 0
	payloadLen := binary.BigEndian.Uint32(body[1:5])

	if !isEOS {
		return ConnectErrorResult{}
	}

	if int(payloadLen) > len(body)-5 {
		return ConnectErrorResult{}
	}

	payload := body[5 : 5+payloadLen]

	// Decompress if gzipped
	if isGzipped {
		gr, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return ConnectErrorResult{}
		}
		decompressed, err := io.ReadAll(gr)
		gr.Close()
		if err != nil {
			return ConnectErrorResult{}
		}
		payload = decompressed
	}

	// Parse JSON
	var eos ConnectEOSPayload
	if err := json.Unmarshal(payload, &eos); err != nil {
		return ConnectErrorResult{}
	}
	if eos.Error == nil || eos.Error.Code == "" {
		return ConnectErrorResult{}
	}

	return ConnectErrorResult{
		IsError: true,
		IsEOS:   true,
		Code:    eos.Error.Code,
		Message: eos.Error.Message,
		RawJSON: string(payload),
	}
}

// findEOSInBody scans through Connect frames to find an EOS trailer at the end.
// Data frames (flag & 0x02 == 0) are skipped.
func findEOSInBody(body []byte) ConnectErrorResult {
	offset := 0
	for offset < len(body) {
		if offset+5 > len(body) {
			break
		}
		flag := body[offset]
		payloadLen := int(binary.BigEndian.Uint32(body[offset+1 : offset+5]))
		frameEnd := offset + 5 + payloadLen

		if frameEnd > len(body) {
			break
		}

		if flag&0x02 != 0 {
			// Found EOS frame
			return parseConnectFrame(body[offset:frameEnd])
		}

		offset = frameEnd
	}
	return ConnectErrorResult{}
}

// parseConnectJSONError tries to parse a plain JSON error body (Format A).
func parseConnectJSONError(body []byte) ConnectErrorResult {
	// Quick check: must look like JSON
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ConnectErrorResult{}
	}

	// Try {"error":{"code":"...","message":"..."}} first
	var eos ConnectEOSPayload
	if err := json.Unmarshal(trimmed, &eos); err == nil && eos.Error != nil && eos.Error.Code != "" {
		return ConnectErrorResult{
			IsError: true,
			IsEOS:   false,
			Code:    eos.Error.Code,
			Message: eos.Error.Message,
			RawJSON: string(trimmed),
		}
	}

	// Try {"code":"...","message":"..."} directly
	var ce ConnectError
	if err := json.Unmarshal(trimmed, &ce); err == nil && ce.Code != "" {
		return ConnectErrorResult{
			IsError: true,
			IsEOS:   false,
			Code:    ce.Code,
			Message: ce.Message,
			RawJSON: string(trimmed),
		}
	}

	return ConnectErrorResult{}
}

// ClassifyConnectError maps a ConnectErrorResult to the internal upstreamFailureKind.
func ClassifyConnectError(ce ConnectErrorResult) (upstreamFailureKind, string) {
	if !ce.IsError {
		return upstreamFailureNone, ""
	}

	code := strings.ToLower(ce.Code)
	msgLower := strings.ToLower(ce.Message)
	combined := code + " " + msgLower

	// Priority 1: Rate limit
	if isRateLimitText(combined) {
		return upstreamFailureRateLimit, formatConnectDetail(ce)
	}
	// permission_denied without credit keywords → treat as rate limit (per WindsurfGate logic)
	if code == "permission_denied" && !containsCreditKeyword(combined) {
		return upstreamFailurePermission, formatConnectDetail(ce)
	}

	// Priority 2: Credits/Quota exhausted
	if code == "resource_exhausted" || containsCreditKeyword(combined) {
		return upstreamFailureQuota, formatConnectDetail(ce)
	}
	if isQuotaExhaustedText(combined) {
		return upstreamFailureQuota, formatConnectDetail(ce)
	}

	// Priority 3: Cascade session errors (failed_precondition)
	if code == "failed_precondition" {
		// Check if it's a quota error disguised as precondition
		if strings.Contains(combined, "quota") || strings.Contains(combined, "usage") || strings.Contains(combined, "credits") {
			return upstreamFailureQuota, formatConnectDetail(ce)
		}
		// Invalid Cascade session — classified as gRPC error for retry handling
		return upstreamFailureGRPC, formatConnectDetail(ce)
	}

	// Priority 4: Auth errors
	if code == "unauthenticated" {
		return upstreamFailureAuth, formatConnectDetail(ce)
	}

	// Priority 5: Internal/unavailable
	if code == "internal" || code == "unavailable" {
		return upstreamFailureInternal, formatConnectDetail(ce)
	}

	// Unknown Connect error
	return upstreamFailureGRPC, formatConnectDetail(ce)
}

// IsCascadeSessionError returns true if the Connect error is an "Invalid Cascade session" error.
func IsCascadeSessionError(ce ConnectErrorResult) bool {
	if !ce.IsError {
		return false
	}
	code := strings.ToLower(ce.Code)
	msgLower := strings.ToLower(ce.Message)
	return code == "failed_precondition" && strings.Contains(msgLower, "cascade session")
}

// containsCreditKeyword checks if text contains credit-related keywords.
func containsCreditKeyword(text string) bool {
	keywords := []string{"credits", "not enough", "resource_exhausted", "quota", "exceeded"}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func formatConnectDetail(ce ConnectErrorResult) string {
	parts := make([]string, 0, 3)
	if ce.Code != "" {
		parts = append(parts, "code="+ce.Code)
	}
	if ce.Message != "" {
		parts = append(parts, "msg="+truncate(ce.Message, 160))
	}
	if ce.IsEOS {
		parts = append(parts, "eos=true")
	}
	if len(parts) == 0 {
		return "无上游细节"
	}
	return strings.Join(parts, " ")
}

// ConnectStreamWatcher wraps a response body and watches for Connect EOS trailer frames.
// When an EOS frame with an error is detected at the end of the stream, the appropriate
// callback is invoked.
type ConnectStreamWatcher struct {
	inner     io.ReadCloser
	buf       []byte // accumulated bytes for frame scanning
	onError   func(ConnectErrorResult)
	onSuccess func(int)
	sawError  bool
	finalized bool

	grpcBuf          []byte
	completionTokens int
}

func NewConnectStreamWatcher(inner io.ReadCloser, onError func(ConnectErrorResult), onSuccess func(int)) *ConnectStreamWatcher {
	return &ConnectStreamWatcher{
		inner:     inner,
		onError:   onError,
		onSuccess: onSuccess,
	}
}

func (w *ConnectStreamWatcher) Read(p []byte) (int, error) {
	n, err := w.inner.Read(p)
	if n > 0 {
		chunk := p[:n]
		w.buf = append(w.buf, chunk...)
		// Try to detect EOS frames as they arrive
		w.scanForEOS()

		// ── 抓取 Tokens ──
		w.grpcBuf = append(w.grpcBuf, chunk...)
		for len(w.grpcBuf) >= 5 {
			flags := w.grpcBuf[0]
			payloadLen := int(binary.BigEndian.Uint32(w.grpcBuf[1:5]))
			if len(w.grpcBuf) < 5+payloadLen {
				break // partial
			}
			payload := w.grpcBuf[5 : 5+payloadLen]
			w.grpcBuf = w.grpcBuf[5+payloadLen:]

			if flags&2 != 0 {
				continue // skip EOS
			}
			decoded, err := decodeStreamEnvelopePayload(flags, payload)
			if err == nil && len(decoded) > 0 {
				chunkResponse, _, err := ParseChatResponseChunk(decoded)
				if err == nil && len(chunkResponse) > 0 {
					w.completionTokens += estimateTokens(chunkResponse)
				}
			}
		}
	}
	if err == io.EOF {
		w.finalize()
	}
	return n, err
}

func (w *ConnectStreamWatcher) Close() error {
	err := w.inner.Close()
	w.finalize()
	return err
}

func (w *ConnectStreamWatcher) scanForEOS() {
	if w.sawError {
		return
	}
	// Only keep the last 8KB for scanning (EOS frames are typically small)
	if len(w.buf) > 8192 {
		w.buf = w.buf[len(w.buf)-8192:]
	}
}

func (w *ConnectStreamWatcher) finalize() {
	if w.finalized {
		return
	}
	w.finalized = true

	// Check for EOS error in accumulated buffer
	result := findEOSInBody(w.buf)
	if result.IsError {
		w.sawError = true
		if w.onError != nil {
			w.onError(result)
		}
		return
	}

	// Also try the old keyword-based scan on raw text
	if len(w.buf) > 0 {
		textLower := strings.ToLower(string(w.buf))
		if isQuotaExhaustedText(textLower) {
			w.sawError = true
			if w.onError != nil {
				w.onError(ConnectErrorResult{
					IsError: true,
					Code:    "resource_exhausted",
					Message: "stream-body quota keyword match",
				})
			}
			return
		}
	}

	if !w.sawError && w.onSuccess != nil {
		w.onSuccess(w.completionTokens)
	}
}
