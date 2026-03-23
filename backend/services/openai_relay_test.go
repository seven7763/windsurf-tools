package services

import (
	"encoding/json"
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

	status := relay.Status()
	if status.Running {
		t.Fatal("should not be running initially")
	}

	if err := relay.Start(0, ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	status = relay.Status()
	if !status.Running {
		t.Fatal("should be running after Start")
	}
	if status.Port != 8787 {
		t.Fatalf("port = %d, want 8787", status.Port)
	}

	// double start should error
	if err := relay.Start(0, ""); err == nil {
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
