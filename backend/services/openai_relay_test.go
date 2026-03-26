package services

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestRelay() *OpenAIRelay {
	proxy := NewMitmProxy(nil, nil, "")
	proxy.SetPoolKeys([]string{"sk-ws-test1", "sk-ws-test2"})
	return NewOpenAIRelay(proxy, func(msg string) {}, "")
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
