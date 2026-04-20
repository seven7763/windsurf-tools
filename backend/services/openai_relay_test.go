package services

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestRelay() *OpenAIRelay {
	proxy := NewMitmProxy(&WindsurfService{client: &http.Client{}}, nil, "", nil)
	proxy.SetPoolKeys([]string{"sk-ws-test1", "sk-ws-test2"})
	return NewOpenAIRelay(proxy, func(msg string) {}, "", nil)
}

func TestNewOpenAIRelayUsesSingleReplayBudget(t *testing.T) {
	relay := newTestRelay()
	if relay.maxRetry != defaultReplayBudget {
		t.Fatalf("relay.maxRetry = %d, want %d", relay.maxRetry, defaultReplayBudget)
	}
}

func TestRelayHealthEndpoint(t *testing.T) {
	_ = newTestRelay()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("health status = %d, want 200", w.Code)
	}
}

func TestRelayModelsEndpoint(t *testing.T) {
	relay := newTestRelay()
	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	relay.handleModels(w, req)

	if w.Code != 200 {
		t.Fatalf("models status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["object"] != "list" {
		t.Fatalf("object = %v, want list", resp["object"])
	}
	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		t.Fatal("models data empty")
	}
}

func TestRelayAuthRejectsInvalidKey(t *testing.T) {
	relay := newTestRelay()
	relay.secret = "my-secret"

	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	relay.handleModels(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRelayAuthAcceptsValidKey(t *testing.T) {
	relay := newTestRelay()
	relay.secret = "my-secret"

	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer my-secret")
	w := httptest.NewRecorder()
	relay.handleModels(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRelayAuthSkipsWhenNoSecret(t *testing.T) {
	relay := newTestRelay()
	relay.secret = ""

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	relay.handleModels(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 with no secret, got %d", w.Code)
	}
}

func TestRelayChatRejectsGet(t *testing.T) {
	relay := newTestRelay()
	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	relay.handleChatCompletions(w, req)

	if w.Code != 405 {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestRelayChatRejectsEmptyMessages(t *testing.T) {
	relay := newTestRelay()
	body := `{"model":"gpt-4","messages":[]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	relay.handleChatCompletions(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRelayChatRejectsInvalidJSON(t *testing.T) {
	relay := newTestRelay()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	relay.handleChatCompletions(w, req)

	if w.Code != 400 {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRelayStartStop(t *testing.T) {
	relay := newTestRelay()
	port := reserveTestPort(t)

	status := relay.Status()
	if status.Running {
		t.Fatal("should not be running initially")
	}

	if err := relay.Start(port, ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status = relay.Status()
	if !status.Running {
		t.Fatal("should be running after Start")
	}
	if status.Port != port {
		t.Fatalf("port = %d, want %d", status.Port, port)
	}

	// double start should error
	if err := relay.Start(port, ""); err == nil {
		t.Fatal("double Start should error")
	}

	if err := relay.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	status = relay.Status()
	if status.Running {
		t.Fatal("should not be running after Stop")
	}
}

func TestRelayCORSPreflightAllowsQuickTest(t *testing.T) {
	relay := newTestRelay()
	handler := relay.withCORS(http.HandlerFunc(relay.handleChatCompletions))

	req := httptest.NewRequest(http.MethodOptions, "/v1/chat/completions", nil)
	req.Header.Set("Origin", "http://wails.localhost")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://wails.localhost" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want request origin", got)
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Content-Type") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
	}
}

func reserveTestPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test port: %v", err)
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("reserve test port: unexpected addr type %T", ln.Addr())
	}
	return addr.Port
}

func TestBuildSSEChunk(t *testing.T) {
	chunk := buildSSEChunk("id-1", "gpt-4", "hello", false)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(chunk), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["id"] != "id-1" {
		t.Fatalf("id = %v, want id-1", parsed["id"])
	}
	if parsed["object"] != "chat.completion.chunk" {
		t.Fatalf("object = %v", parsed["object"])
	}

	// stop chunk
	stopChunk := buildSSEChunk("id-2", "gpt-4", "", true)
	var stopParsed map[string]interface{}
	json.Unmarshal([]byte(stopChunk), &stopParsed)
	choices := stopParsed["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	if choice["finish_reason"] != "stop" {
		t.Fatalf("finish_reason = %v, want stop", choice["finish_reason"])
	}
}

func TestWriteOpenAIError(t *testing.T) {
	w := httptest.NewRecorder()
	writeOpenAIError(w, 429, "rate_limit", "too many requests")

	if w.Code != 429 {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	errObj := resp["error"].(map[string]interface{})
	if errObj["type"] != "rate_limit" {
		t.Fatalf("error type = %v", errObj["type"])
	}
}

func TestTruncKey(t *testing.T) {
	got := truncKey("sk-ws-abcdefghijklmnop")
	if len(got) > 15 {
		t.Fatalf("truncKey too long: %q", got)
	}
	got = truncKey("short")
	if got != "short" {
		t.Fatalf("truncKey short = %q", got)
	}
}

func TestBuildGetChatMessageRequestUsesConnectProtocol(t *testing.T) {
	payload := []byte{0x00, 0x00, 0x00, 0x00, 0x03, 0x66, 0x6f, 0x6f}
	req, err := buildGetChatMessageRequest("1.2.3.4", payload, "jwt-a")
	if err != nil {
		t.Fatalf("buildGetChatMessageRequest() error = %v", err)
	}

	if got := req.Method; got != http.MethodPost {
		t.Fatalf("Method = %q, want %q", got, http.MethodPost)
	}
	if got := req.URL.String(); got != "https://1.2.3.4/exa.api_server_pb.ApiServerService/GetChatMessage" {
		t.Fatalf("URL = %q", got)
	}
	if got := req.Host; got != GRPCUpstreamHost {
		t.Fatalf("Host = %q, want %q", got, GRPCUpstreamHost)
	}
	if got := req.Header.Get("Content-Type"); got != "application/connect+proto" {
		t.Fatalf("Content-Type = %q, want application/connect+proto", got)
	}
	if got := req.Header.Get("Connect-Protocol-Version"); got != "1" {
		t.Fatalf("Connect-Protocol-Version = %q, want 1", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer jwt-a" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer jwt-a")
	}
	if got := req.Header.Get("User-Agent"); got != "connect-go/1.18.1 (go1.26.1)" {
		t.Fatalf("User-Agent = %q, want connect-go/1.18.1 (go1.26.1)", got)
	}
	// ★ 真实 IDE 抓包里**不带** X-Client-Name / X-Client-Version（capture 2026-04-18 确认）。
	// 多余的客户端自识别 header 反而是 fingerprint 风险，保持空以对齐 IDE。
	if got := req.Header.Get("X-Client-Name"); got != "" {
		t.Fatalf("X-Client-Name = %q, want empty (IDE 不发送该头)", got)
	}
	if got := req.Header.Get("X-Client-Version"); got != "" {
		t.Fatalf("X-Client-Version = %q, want empty (IDE 不发送该头)", got)
	}
	if got := req.Header.Get("Te"); got != "" {
		t.Fatalf("Te = %q, want empty for Connect requests", got)
	}

	// ★ IDE 真实行为：Connect envelope 帧内 gzip 压缩（flag=0x01）。
	if got := req.Header.Get("Connect-Content-Encoding"); got != "gzip" {
		t.Fatalf("Connect-Content-Encoding = %q, want gzip", got)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll(req.Body) error = %v", err)
	}
	if len(body) < 5 || body[0] != 0x01 {
		t.Fatalf("body must start with compressed envelope flag 0x01, got first byte %x", body[:min(5, len(body))])
	}
	// envelope 长度字段应等于 body 剩余长度
	declared := uint32(body[1])<<24 | uint32(body[2])<<16 | uint32(body[3])<<8 | uint32(body[4])
	if int(declared) != len(body)-5 {
		t.Fatalf("envelope length mismatch: declared=%d actual=%d", declared, len(body)-5)
	}
}

func TestSendGRPCRetriesTransientEOF(t *testing.T) {
	relay := newTestRelay()
	calls := 0
	relay.upstream = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, io.EOF
		}
		payload := appendGRPCFrame(nil, encodeBytesField(1, []byte("hello")))
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len(payload)),
			Body:          io.NopCloser(strings.NewReader(string(payload))),
			Header:        make(http.Header),
			Trailer:       make(http.Header),
			Request:       req,
		}, nil
	})

	resp, kind, err := relay.sendGRPC([]byte("payload"), "sk-ws-test1", "jwt-a")
	if err != nil {
		t.Fatalf("sendGRPC() error = %v", err)
	}
	if kind != upstreamFailureNone {
		t.Fatalf("sendGRPC() kind = %q, want none", kind)
	}
	if resp == nil {
		t.Fatal("sendGRPC() response is nil")
	}
	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls)
	}
}

func TestStreamResponse_EmitsSSEChunks(t *testing.T) {
	relay := newTestRelay()
	secondPayload := append(encodeBytesField(1, []byte("world")), 0x10, 0x01)
	body := io.NopCloser(strings.NewReader(string(
		appendGRPCFrame(
			appendGRPCFrame(nil, encodeBytesField(1, []byte("hello "))),
			secondPayload,
		),
	)))
	w := httptest.NewRecorder()

	kind, detail := relay.streamResponse(w, newRelayHTTPResponse(body, nil), "chat-1", "cascade")
	if kind != upstreamFailureNone {
		t.Fatalf("streamResponse() kind = %q detail=%q, want none", kind, detail)
	}

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	out := w.Body.String()
	if !strings.Contains(out, `"content":"hello "`) {
		t.Fatalf("stream output missing first delta: %s", out)
	}
	if !strings.Contains(out, `"content":"world"`) {
		t.Fatalf("stream output missing second delta: %s", out)
	}
	if !strings.Contains(out, `data: [DONE]`) {
		t.Fatalf("stream output missing DONE marker: %s", out)
	}
}

func TestStreamResponse_HandlesSplitFrameAcrossReads(t *testing.T) {
	relay := newTestRelay()
	payload := append(encodeBytesField(1, []byte("split")), 0x10, 0x01)
	frame := appendGRPCFrame(nil, payload)
	body := io.NopCloser(&chunkedReader{
		chunks: [][]byte{
			frame[:3],
			frame[3:7],
			frame[7:],
		},
	})
	w := httptest.NewRecorder()

	kind, detail := relay.streamResponse(w, newRelayHTTPResponse(body, nil), "chat-2", "cascade")
	if kind != upstreamFailureNone {
		t.Fatalf("streamResponse() kind = %q detail=%q, want none", kind, detail)
	}

	out := w.Body.String()
	if !strings.Contains(out, `"content":"split"`) {
		t.Fatalf("stream output missing split chunk: %s", out)
	}
	if !strings.Contains(out, `data: [DONE]`) {
		t.Fatalf("stream output missing DONE marker: %s", out)
	}
}

func TestStreamResponse_HandlesCompressedConnectFrames(t *testing.T) {
	relay := newTestRelay()
	firstPayload := append(
		encodeBytesField(1, []byte("bot-ignore-me")),
		encodeBytesField(6, encodeBytesField(3, []byte("hello ")))...,
	)
	secondPayload := encodeBytesField(6, encodeBytesField(3, []byte("world")))

	var stream []byte
	stream = append(stream, appendStreamEnvelope(nil, streamEnvelopeCompressed, gzipBytes(t, firstPayload))...)
	stream = append(stream, appendStreamEnvelope(nil, streamEnvelopeCompressed, gzipBytes(t, secondPayload))...)
	stream = append(stream, appendStreamEnvelope(nil, streamEnvelopeCompressed|streamEnvelopeEndStream, gzipBytes(t, []byte(`{}`)))...)

	body := io.NopCloser(strings.NewReader(string(stream)))
	w := httptest.NewRecorder()

	kind, detail := relay.streamResponse(w, newRelayHTTPResponse(body, nil), "chat-compressed", "cascade")
	if kind != upstreamFailureNone {
		t.Fatalf("streamResponse() kind = %q detail=%q, want none", kind, detail)
	}

	out := w.Body.String()
	if !strings.Contains(out, `"content":"hello "`) {
		t.Fatalf("stream output missing compressed first delta: %s", out)
	}
	if !strings.Contains(out, `"content":"world"`) {
		t.Fatalf("stream output missing compressed second delta: %s", out)
	}
	if strings.Contains(out, "bot-ignore-me") {
		t.Fatalf("stream output leaked metadata string: %s", out)
	}
	if !strings.Contains(out, `data: [DONE]`) {
		t.Fatalf("stream output missing DONE marker: %s", out)
	}
}

func TestStreamResponse_QuotaTrailerDoesNotEmitDone(t *testing.T) {
	relay := newTestRelay()
	body := io.NopCloser(strings.NewReader(string(
		appendGRPCFrame(nil, encodeBytesField(1, []byte("partial"))),
	)))
	trailers := http.Header{
		"Grpc-Status":  []string{"9"},
		"Grpc-Message": []string{"Failed precondition: Your weekly usage quota has been exhausted."},
	}
	w := httptest.NewRecorder()

	kind, detail := relay.streamResponse(w, newRelayHTTPResponse(body, trailers), "chat-quota", "cascade")

	if kind != upstreamFailureQuota {
		t.Fatalf("streamResponse() kind = %q, want %q", kind, upstreamFailureQuota)
	}
	if detail == "" {
		t.Fatal("streamResponse() detail empty")
	}
	if strings.Contains(w.Body.String(), `data: [DONE]`) {
		t.Fatalf("quota trailer should not emit DONE: %s", w.Body.String())
	}
}

func TestBlockingResponse_QuotaTrailerReturnsOpenAIError(t *testing.T) {
	relay := newTestRelay()
	body := io.NopCloser(strings.NewReader(string(
		appendGRPCFrame(nil, encodeBytesField(1, []byte("ignored"))),
	)))
	trailers := http.Header{
		"Grpc-Status":  []string{"9"},
		"Grpc-Message": []string{"Failed precondition: Your daily usage quota has been exhausted."},
	}
	w := httptest.NewRecorder()

	kind, detail := relay.blockingResponse(w, newRelayHTTPResponse(body, trailers), "chat-blocking", "cascade")

	if kind != upstreamFailureQuota {
		t.Fatalf("blockingResponse() kind = %q, want %q", kind, upstreamFailureQuota)
	}
	if detail == "" {
		t.Fatal("blockingResponse() detail empty")
	}
	if w.Code != 429 {
		t.Fatalf("status = %d, want 429", w.Code)
	}
}

func TestFinalizeRelayOutcome_SuccessRecordsSuccess(t *testing.T) {
	relay := newTestRelay()

	relay.finalizeRelayOutcome("sk-ws-test1", upstreamFailureNone, "")

	state := relay.proxy.keyStates["sk-ws-test1"]
	if state == nil {
		t.Fatal("missing key state")
	}
	if state.SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1", state.SuccessCount)
	}
	if state.RuntimeExhausted {
		t.Fatal("RuntimeExhausted should remain false after success")
	}
}

func TestFinalizeRelayOutcome_QuotaDoesNotRecordSuccess(t *testing.T) {
	relay := newTestRelay()

	relay.finalizeRelayOutcome("sk-ws-test1", upstreamFailureQuota, "grpc-status=9 grpc-message=quota exhausted")

	state := relay.proxy.keyStates["sk-ws-test1"]
	if state == nil {
		t.Fatal("missing key state")
	}
	if state.SuccessCount != 0 {
		t.Fatalf("SuccessCount = %d, want 0", state.SuccessCount)
	}
	if !state.RuntimeExhausted {
		t.Fatal("RuntimeExhausted should be true after quota failure")
	}
}

func TestFinalizeRelayOutcome_RateLimitRotatesToNextKey(t *testing.T) {
	relay := newTestRelay()
	relay.proxy.keyStates["sk-ws-test1"].JWT = []byte("jwt-a")
	relay.proxy.keyStates["sk-ws-test2"].JWT = []byte("jwt-b")

	relay.finalizeRelayOutcome("sk-ws-test1", upstreamFailureRateLimit, "Permission denied: Rate limit exceeded")

	if got := relay.proxy.CurrentAPIKey(); got != "sk-ws-test2" {
		t.Fatalf("CurrentAPIKey() = %q, want %q", got, "sk-ws-test2")
	}
	state := relay.proxy.keyStates["sk-ws-test1"]
	if state == nil || state.Healthy || !state.CooldownUntil.After(time.Now()) || state.RuntimeExhausted {
		t.Fatalf("old key state = %#v, want rate-limited cooldown without runtime exhaustion", state)
	}
}

func TestFinalizeRelayOutcome_AuthNonPersistentRefreshesJWTInBackground(t *testing.T) {
	originalGetJWT := getJWTByAPIKeyFn
	t.Cleanup(func() {
		getJWTByAPIKeyFn = originalGetJWT
	})

	refreshCh := make(chan string, 1)
	getJWTByAPIKeyFn = func(_ *WindsurfService, apiKey string) (string, error) {
		refreshCh <- apiKey
		return "jwt-refresh-" + apiKey, nil
	}

	relay := newTestRelay()
	relay.proxy.keyStates["sk-ws-test1"].JWT = []byte("jwt-a")
	relay.proxy.keyStates["sk-ws-test2"].JWT = []byte("jwt-b")

	relay.finalizeRelayOutcome("sk-ws-test1", upstreamFailureAuth, "Unauthenticated: an internal error occurred")

	// 非永久性 auth 错误不切号，保持在 test1
	if got := relay.proxy.CurrentAPIKey(); got != "sk-ws-test1" {
		t.Fatalf("CurrentAPIKey() = %q, want %q (non-persistent auth should NOT rotate)", got, "sk-ws-test1")
	}
	// 后台异步触发 JWT 刷新
	select {
	case key := <-refreshCh:
		if key != "sk-ws-test1" {
			t.Fatalf("JWT refresh key = %q, want %q", key, "sk-ws-test1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("JWT refresh was not triggered in background")
	}
}

func TestHandleChatCompletionsAuthFailureRefreshesJWTAndRetries(t *testing.T) {
	originalGetJWT := getJWTByAPIKeyFn
	t.Cleanup(func() {
		getJWTByAPIKeyFn = originalGetJWT
	})

	refreshCalls := 0
	getJWTByAPIKeyFn = func(_ *WindsurfService, apiKey string) (string, error) {
		refreshCalls++
		return "jwt-refreshed", nil
	}

	relay := newTestRelay()
	relay.proxy.keyStates["sk-ws-test1"].JWT = []byte("jwt-a")
	relay.proxy.keyStates["sk-ws-test2"].JWT = []byte("jwt-b")

	calls := 0
	relay.upstream = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if got := req.Header.Get("Authorization"); got != "Bearer jwt-a" {
				t.Fatalf("first request auth = %q, want %q", got, "Bearer jwt-a")
			}
			return &http.Response{
				StatusCode:    200,
				ContentLength: int64(len("Unauthenticated: an internal error occurred")),
				Body:          io.NopCloser(strings.NewReader("Unauthenticated: an internal error occurred")),
				Header:        http.Header{"Grpc-Status": []string{"16"}},
				Request:       req,
			}, nil
		}
		// 第二次用刷新后的 JWT（pickPoolKeyAndJWT 会拿到新 JWT）
		payload := appendGRPCFrame(nil, encodeBytesField(1, []byte("hello")))
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len(payload)),
			Body:          io.NopCloser(strings.NewReader(string(payload))),
			Header:        make(http.Header),
			Trailer:       make(http.Header),
			Request:       req,
		}, nil
	})

	body := `{"model":"cascade","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	relay.handleChatCompletions(w, req)

	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls)
	}
	// 非永久性 auth 错误 → rotateAfterAuthFailure 返回 "" → 走 refreshJWTForKey → 重试
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"content":"hello"`) {
		t.Fatalf("response body missing assistant content: %s", w.Body.String())
	}
}

func TestHandleChatCompletionsAuthFailureWithoutSpareReturnsAuthError(t *testing.T) {
	originalGetJWT := getJWTByAPIKeyFn
	t.Cleanup(func() {
		getJWTByAPIKeyFn = originalGetJWT
	})

	refreshCalls := 0
	getJWTByAPIKeyFn = func(_ *WindsurfService, apiKey string) (string, error) {
		refreshCalls++
		return "", errors.New("refresh failed")
	}

	proxy := NewMitmProxy(&WindsurfService{}, nil, "", nil)
	proxy.SetPoolKeys([]string{"sk-ws-test1"})
	proxy.keyStates["sk-ws-test1"].JWT = []byte("jwt-a")
	relay := NewOpenAIRelay(proxy, func(string) {}, "", nil)

	calls := 0
	relay.upstream = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len("Unauthenticated: an internal error occurred")),
			Body:          io.NopCloser(strings.NewReader("Unauthenticated: an internal error occurred")),
			Header:        http.Header{"Grpc-Status": []string{"16"}},
			Request:       req,
		}, nil
	})

	body := `{"model":"cascade","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	relay.handleChatCompletions(w, req)

	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
	if refreshCalls != 1 {
		t.Fatalf("getJWTByAPIKeyFn calls = %d, want 1", refreshCalls)
	}
	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"type":"authentication_error"`) {
		t.Fatalf("response body missing authentication_error: %s", w.Body.String())
	}
}

func TestHandleChatCompletionsRateLimitRotatesToNextKeyAndRetries(t *testing.T) {
	originalGetJWT := getJWTByAPIKeyFn
	t.Cleanup(func() {
		getJWTByAPIKeyFn = originalGetJWT
	})

	refreshCalls := 0
	getJWTByAPIKeyFn = func(_ *WindsurfService, apiKey string) (string, error) {
		refreshCalls++
		return "jwt-refresh-" + apiKey, nil
	}

	relay := newTestRelay()
	relay.proxy.keyStates["sk-ws-test1"].JWT = []byte("jwt-a")
	relay.proxy.keyStates["sk-ws-test2"].JWT = []byte("jwt-b")

	calls := 0
	relay.upstream = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			body := "Permission denied: Rate limit exceeded. Your request was not processed, and no credits were used. Please upgrade to a Pro account for higher limits or try again in about an hour. Rate limit error"
			return &http.Response{
				StatusCode:    200,
				ContentLength: int64(len(body)),
				Body:          io.NopCloser(strings.NewReader(body)),
				Header:        http.Header{"Grpc-Status": []string{"7"}},
				Request:       req,
			}, nil
		}
		if got := req.Header.Get("Authorization"); got != "Bearer jwt-b" {
			t.Fatalf("retry request auth = %q, want %q", got, "Bearer jwt-b")
		}
		payload := appendGRPCFrame(nil, encodeBytesField(1, []byte("hello")))
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len(payload)),
			Body:          io.NopCloser(strings.NewReader(string(payload))),
			Header:        make(http.Header),
			Trailer:       make(http.Header),
			Request:       req,
		}, nil
	})

	body := `{"model":"cascade","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	relay.handleChatCompletions(w, req)

	if calls != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls)
	}
	if refreshCalls != 0 {
		t.Fatalf("getJWTByAPIKeyFn calls = %d, want 0", refreshCalls)
	}
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"content":"hello"`) {
		t.Fatalf("response body missing assistant content: %s", w.Body.String())
	}
	if got := relay.proxy.CurrentAPIKey(); got != "sk-ws-test2" {
		t.Fatalf("CurrentAPIKey() = %q, want %q", got, "sk-ws-test2")
	}
}

func TestHandleChatCompletionsRateLimitWithoutSpareReturnsRateLimitError(t *testing.T) {
	proxy := NewMitmProxy(&WindsurfService{}, nil, "", nil)
	proxy.SetPoolKeys([]string{"sk-ws-test1"})
	proxy.keyStates["sk-ws-test1"].JWT = []byte("jwt-a")
	relay := NewOpenAIRelay(proxy, func(string) {}, "", nil)

	calls := 0
	relay.upstream = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		body := "Permission denied: Rate limit exceeded. Your request was not processed, and no credits were used. Please upgrade to a Pro account for higher limits or try again in about an hour. Rate limit error"
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len(body)),
			Body:          io.NopCloser(strings.NewReader(body)),
			Header:        http.Header{"Grpc-Status": []string{"7"}},
			Request:       req,
		}, nil
	})

	body := `{"model":"cascade","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	relay.handleChatCompletions(w, req)

	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
	if w.Code != 429 {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"type":"rate_limit"`) {
		t.Fatalf("response body missing rate_limit: %s", w.Body.String())
	}
}

func TestHandleChatCompletionsMessageLimitWithoutSpareReturnsRateLimitError(t *testing.T) {
	proxy := NewMitmProxy(&WindsurfService{}, nil, "", nil)
	proxy.SetPoolKeys([]string{"sk-ws-test1"})
	proxy.keyStates["sk-ws-test1"].JWT = []byte("jwt-a")
	relay := NewOpenAIRelay(proxy, func(string) {}, "", nil)

	calls := 0
	relay.upstream = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		body := "You've reached your message limit for this model. Your limit will reset in 39 minutes. Upgrade to Pro for higher limits or try a different model. https://windsurf.com/redirect/windsurf/add-credits"
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len(body)),
			Body:          io.NopCloser(strings.NewReader(body)),
			Header:        http.Header{"Grpc-Status": []string{"7"}},
			Request:       req,
		}, nil
	})

	body := `{"model":"cascade","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	relay.handleChatCompletions(w, req)

	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
	if w.Code != 429 {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"type":"rate_limit"`) {
		t.Fatalf("response body missing rate_limit: %s", w.Body.String())
	}
}

type chunkedReader struct {
	chunks [][]byte
	index  int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.index])
	r.index++
	return n, nil
}

func appendGRPCFrame(dst []byte, payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return append(dst, frame...)
}

func newRelayHTTPResponse(body io.ReadCloser, trailers http.Header) *http.Response {
	if trailers == nil {
		trailers = http.Header{}
	}
	return &http.Response{
		StatusCode: 200,
		Body:       body,
		Trailer:    trailers,
	}
}
