package services

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestSetPoolKeysPreservesCurrentKey(t *testing.T) {
	proxy := NewMitmProxy(nil, nil, "")
	proxy.SetPoolKeys([]string{"sk-ws-a", "sk-ws-b", "sk-ws-c"})
	if ok := proxy.SwitchToKey("sk-ws-b"); !ok {
		t.Fatal("SwitchToKey() = false, want true")
	}

	proxy.SetPoolKeys([]string{"sk-ws-x", "sk-ws-b", "sk-ws-y"})

	if got := proxy.CurrentAPIKey(); got != "sk-ws-b" {
		t.Fatalf("CurrentAPIKey() after SetPoolKeys = %q, want %q", got, "sk-ws-b")
	}
}

func TestIsQuotaExhaustedTextDetectsIncludedQuotaBanner(t *testing.T) {
	text := "your included usage quota is exhausted. purchase extra usage to continue using premium models."
	if !isQuotaExhaustedText(text) {
		t.Fatal("isQuotaExhaustedText() = false, want true")
	}
}

func TestClassifyUpstreamFailureTreatsInternalPermissionDeniedAsNonQuota(t *testing.T) {
	kind, detail := classifyUpstreamFailure("13", "", "Permission denied: internal server error: error number 13")
	if kind != upstreamFailureInternal {
		t.Fatalf("classifyUpstreamFailure() kind = %q, want %q", kind, upstreamFailureInternal)
	}
	if detail == "" {
		t.Fatal("classifyUpstreamFailure() detail empty, want log detail")
	}
}

func TestClassifyUpstreamFailureTreatsPermissionDeniedAsPermission(t *testing.T) {
	kind, _ := classifyUpstreamFailure("7", "", "Permission denied")
	if kind != upstreamFailurePermission {
		t.Fatalf("classifyUpstreamFailure() kind = %q, want %q", kind, upstreamFailurePermission)
	}
}

func TestClassifyUpstreamFailureTreatsQuotaAsQuota(t *testing.T) {
	kind, _ := classifyUpstreamFailure("", "", "Your included usage quota is exhausted. Purchase extra usage to continue.")
	if kind != upstreamFailureQuota {
		t.Fatalf("classifyUpstreamFailure() kind = %q, want %q", kind, upstreamFailureQuota)
	}
}

func TestClassifyUpstreamFailureTreatsUnauthenticatedAsAuth(t *testing.T) {
	kind, detail := classifyUpstreamFailure("16", "", "Unauthenticated: an internal error occurred")
	if kind != upstreamFailureAuth {
		t.Fatalf("classifyUpstreamFailure() kind = %q, want %q", kind, upstreamFailureAuth)
	}
	if detail == "" {
		t.Fatal("classifyUpstreamFailure() detail empty, want auth detail")
	}
}

func TestClassifyUpstreamFailureTreatsPermissionDeniedApiWireErrorAsAuth(t *testing.T) {
	kind, detail := classifyUpstreamFailure("7", "", `{"code":"permission_denied","message":"permission denied (trace ID: abc)"}`)
	if kind != upstreamFailureAuth {
		t.Fatalf("classifyUpstreamFailure() kind = %q, want %q", kind, upstreamFailureAuth)
	}
	if detail == "" {
		t.Fatal("classifyUpstreamFailure() detail empty, want auth detail")
	}
}

func TestClassifyMitmEventTone(t *testing.T) {
	cases := []struct {
		message string
		want    string
	}{
		{message: "[MITM] 代理已启动: 127.0.0.1:443", want: "success"},
		{message: "[MITM] ⚠️ JWT 预取超时，先接受请求（不替换身份）", want: "warning"},
		{message: "[MITM] 上游权限错误(不轮转): permission denied", want: "danger"},
		{message: "[MITM] 身份替换: /exa.auth_pb.AuthService/GetUserJwt", want: "info"},
	}
	for _, tc := range cases {
		if got := classifyMitmEventTone(tc.message); got != tc.want {
			t.Fatalf("classifyMitmEventTone(%q) = %q, want %q", tc.message, got, tc.want)
		}
	}
}

func TestMitmProxyRecentEventsSnapshotNewestFirstAndLimited(t *testing.T) {
	proxy := NewMitmProxy(nil, nil, "")
	for i := 1; i <= recentEventLimit+3; i++ {
		proxy.appendRecentEvent("event")
		proxy.recentEvents[len(proxy.recentEvents)-1].Message = "event-" + string(rune('A'+i-1))
	}

	got := proxy.recentEventsSnapshot()
	if len(got) != recentEventLimit {
		t.Fatalf("recentEventsSnapshot() len = %d, want %d", len(got), recentEventLimit)
	}
	if got[0].Message != "event-O" {
		t.Fatalf("recentEventsSnapshot()[0] = %q, want newest event", got[0].Message)
	}
	if got[len(got)-1].Message != "event-D" {
		t.Fatalf("recentEventsSnapshot()[last] = %q, want oldest retained event", got[len(got)-1].Message)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRetryTransportQuotaRotateSyncsCodeiumConfig(t *testing.T) {
	originalInject := injectCodeiumConfigFn
	t.Cleanup(func() {
		injectCodeiumConfigFn = originalInject
	})

	var injected []string
	injectCodeiumConfigFn = func(apiKey string) error {
		injected = append(injected, apiKey)
		return nil
	}

	proxy := NewMitmProxy(nil, nil, "")
	proxy.poolKeys = []string{"sk-ws-a", "sk-ws-b"}
	proxy.keyStates["sk-ws-a"] = &PoolKeyState{APIKey: "sk-ws-a", Healthy: true, JWT: []byte("jwt-a")}
	proxy.keyStates["sk-ws-b"] = &PoolKeyState{APIKey: "sk-ws-b", Healthy: true, JWT: []byte("jwt-b")}

	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode:    200,
				ContentLength: int64(len("Your included usage quota is exhausted.")),
				Body:          io.NopCloser(bytes.NewBufferString("Your included usage quota is exhausted.")),
				Header:        make(http.Header),
				Request:       req,
			}, nil
		}
		if got := req.Header.Get("X-Pool-Key-Used"); got != "sk-ws-b" {
			t.Fatalf("retry request key = %q, want %q", got, "sk-ws-b")
		}
		if got := req.Header.Get("Authorization"); got != "Bearer jwt-b" {
			t.Fatalf("retry request auth = %q, want %q", got, "Bearer jwt-b")
		}
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len("ok")),
			Body:          io.NopCloser(bytes.NewBufferString("ok")),
			Header:        make(http.Header),
			Request:       req,
		}, nil
	})

	rt := &retryTransport{base: base, proxy: proxy, maxRetry: 1}
	req, err := http.NewRequest(http.MethodPost, "https://server.self-serve.windsurf.com/test", bytes.NewBufferString("body"))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("X-Pool-Key-Used", "sk-ws-a")
	req.Header.Set("Authorization", "Bearer jwt-a")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp == nil {
		t.Fatal("RoundTrip() response is nil")
	}
	if got := proxy.CurrentAPIKey(); got != "sk-ws-b" {
		t.Fatalf("CurrentAPIKey() = %q, want %q", got, "sk-ws-b")
	}
	if len(injected) == 0 || injected[len(injected)-1] != "sk-ws-b" {
		t.Fatalf("injectCodeiumConfigFn calls = %#v, want last key sk-ws-b", injected)
	}
}

func TestHandleResponseStreamQuotaExhaustedRotatesImmediately(t *testing.T) {
	originalInject := injectCodeiumConfigFn
	t.Cleanup(func() {
		injectCodeiumConfigFn = originalInject
	})

	var injected []string
	injectCodeiumConfigFn = func(apiKey string) error {
		injected = append(injected, apiKey)
		return nil
	}

	proxy := NewMitmProxy(nil, nil, "")
	proxy.poolKeys = []string{"sk-ws-a", "sk-ws-b"}
	proxy.keyStates["sk-ws-a"] = &PoolKeyState{APIKey: "sk-ws-a", Healthy: true, JWT: []byte("jwt-a")}
	proxy.keyStates["sk-ws-b"] = &PoolKeyState{APIKey: "sk-ws-b", Healthy: true, JWT: []byte("jwt-b")}

	req, err := http.NewRequest(http.MethodPost, "https://server.self-serve.windsurf.com/exa.chat_pb.ChatService/GetChatMessage", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("X-Pool-Key-Used", "sk-ws-a")

	resp := &http.Response{
		StatusCode:    200,
		ContentLength: -1,
		Body:          io.NopCloser(bytes.NewBufferString("stream-prefix included usage quota is exhausted stream-suffix")),
		Header:        make(http.Header),
		Request:       req,
	}

	proxy.handleResponse(resp)
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if got := proxy.CurrentAPIKey(); got != "sk-ws-b" {
		t.Fatalf("CurrentAPIKey() = %q, want %q", got, "sk-ws-b")
	}
	if state := proxy.keyStates["sk-ws-a"]; state == nil || !state.RuntimeExhausted {
		t.Fatalf("old key state = %#v, want runtime exhausted", state)
	}
	if len(injected) == 0 || injected[len(injected)-1] != "sk-ws-b" {
		t.Fatalf("injectCodeiumConfigFn calls = %#v, want last key sk-ws-b", injected)
	}
}

func TestStatusIncludesRuntimeExhaustedFlag(t *testing.T) {
	proxy := NewMitmProxy(nil, nil, "")
	proxy.poolKeys = []string{"sk-ws-a"}
	proxy.keyStates["sk-ws-a"] = &PoolKeyState{
		APIKey:           "sk-ws-a",
		Healthy:          false,
		RuntimeExhausted: true,
		CooldownUntil:    time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC),
	}

	status := proxy.Status()
	if len(status.PoolStatus) != 1 {
		t.Fatalf("PoolStatus len = %d, want 1", len(status.PoolStatus))
	}
	if !status.PoolStatus[0].RuntimeExhausted {
		t.Fatalf("RuntimeExhausted = %v, want true", status.PoolStatus[0].RuntimeExhausted)
	}
	if status.PoolStatus[0].CooldownUntil == "" {
		t.Fatal("CooldownUntil should not be empty")
	}
}

func TestPrefetchSpecificJWTsForceRefreshesExistingJWT(t *testing.T) {
	originalGetJWT := getJWTByAPIKeyFn
	t.Cleanup(func() {
		getJWTByAPIKeyFn = originalGetJWT
	})

	calls := 0
	getJWTByAPIKeyFn = func(_ *WindsurfService, apiKey string) (string, error) {
		calls++
		return "jwt-refreshed-" + apiKey, nil
	}

	proxy := NewMitmProxy(&WindsurfService{}, nil, "")
	proxy.poolKeys = []string{"sk-ws-a"}
	proxy.keyStates["sk-ws-a"] = &PoolKeyState{
		APIKey:  "sk-ws-a",
		Healthy: true,
		JWT:     []byte("jwt-old"),
	}

	proxy.prefetchSpecificJWTs([]string{"sk-ws-a"}, true)

	if calls != 1 {
		t.Fatalf("getJWTByAPIKeyFn calls = %d, want 1", calls)
	}
	if got := string(proxy.jwtBytesForKey("sk-ws-a")); got != "jwt-refreshed-sk-ws-a" {
		t.Fatalf("jwtBytesForKey() = %q, want refreshed token", got)
	}
}

func TestRetryTransportAuthFailureRefreshesJWTAndRetries(t *testing.T) {
	originalGetJWT := getJWTByAPIKeyFn
	t.Cleanup(func() {
		getJWTByAPIKeyFn = originalGetJWT
	})

	getJWTByAPIKeyFn = func(_ *WindsurfService, apiKey string) (string, error) {
		return "jwt-new-" + apiKey, nil
	}

	proxy := NewMitmProxy(&WindsurfService{}, nil, "")
	proxy.poolKeys = []string{"sk-ws-a"}
	proxy.keyStates["sk-ws-a"] = &PoolKeyState{
		APIKey:  "sk-ws-a",
		Healthy: true,
		JWT:     []byte("jwt-old"),
	}

	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode:    200,
				ContentLength: int64(len("Unauthenticated: an internal error occurred")),
				Body:          io.NopCloser(bytes.NewBufferString("Unauthenticated: an internal error occurred")),
				Header:        http.Header{"grpc-status": []string{"16"}},
				Request:       req,
			}, nil
		}
		if got := req.Header.Get("Authorization"); got != "Bearer jwt-new-sk-ws-a" {
			t.Fatalf("retry request auth = %q, want refreshed JWT", got)
		}
		if got := req.Header.Get("X-Pool-Key-Used"); got != "sk-ws-a" {
			t.Fatalf("retry request key = %q, want same key sk-ws-a", got)
		}
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len("ok")),
			Body:          io.NopCloser(bytes.NewBufferString("ok")),
			Header:        make(http.Header),
			Request:       req,
		}, nil
	})

	rt := &retryTransport{base: base, proxy: proxy, maxRetry: 1}
	req, err := http.NewRequest(http.MethodPost, "https://server.self-serve.windsurf.com/test", bytes.NewBufferString("body"))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("X-Pool-Key-Used", "sk-ws-a")
	req.Header.Set("Authorization", "Bearer jwt-old")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp == nil {
		t.Fatal("RoundTrip() response is nil")
	}
	if calls != 2 {
		t.Fatalf("RoundTrip() calls = %d, want 2", calls)
	}
	if got := string(proxy.jwtBytesForKey("sk-ws-a")); got != "jwt-new-sk-ws-a" {
		t.Fatalf("jwtBytesForKey() = %q, want refreshed JWT", got)
	}
}

func TestRetryTransportPermissionDeniedWireErrorRefreshesJWTAndRetries(t *testing.T) {
	originalGetJWT := getJWTByAPIKeyFn
	t.Cleanup(func() {
		getJWTByAPIKeyFn = originalGetJWT
	})

	getJWTByAPIKeyFn = func(_ *WindsurfService, apiKey string) (string, error) {
		return "jwt-wire-" + apiKey, nil
	}

	proxy := NewMitmProxy(&WindsurfService{}, nil, "")
	proxy.poolKeys = []string{"sk-ws-a"}
	proxy.keyStates["sk-ws-a"] = &PoolKeyState{
		APIKey:  "sk-ws-a",
		Healthy: true,
		JWT:     []byte("jwt-old"),
	}

	calls := 0
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode:    200,
				ContentLength: int64(len(`{"code":"permission_denied","message":"permission denied (trace ID: abc)"}`)),
				Body:          io.NopCloser(bytes.NewBufferString(`{"code":"permission_denied","message":"permission denied (trace ID: abc)"}`)),
				Header:        http.Header{"grpc-status": []string{"7"}},
				Request:       req,
			}, nil
		}
		if got := req.Header.Get("Authorization"); got != "Bearer jwt-wire-sk-ws-a" {
			t.Fatalf("retry request auth = %q, want refreshed JWT", got)
		}
		return &http.Response{
			StatusCode:    200,
			ContentLength: int64(len("ok")),
			Body:          io.NopCloser(bytes.NewBufferString("ok")),
			Header:        make(http.Header),
			Request:       req,
		}, nil
	})

	rt := &retryTransport{base: base, proxy: proxy, maxRetry: 1}
	req, err := http.NewRequest(http.MethodPost, "https://server.self-serve.windsurf.com/test", bytes.NewBufferString("body"))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("X-Pool-Key-Used", "sk-ws-a")
	req.Header.Set("Authorization", "Bearer jwt-old")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if resp == nil {
		t.Fatal("RoundTrip() response is nil")
	}
	if calls != 2 {
		t.Fatalf("RoundTrip() calls = %d, want 2", calls)
	}
	if got := string(proxy.jwtBytesForKey("sk-ws-a")); got != "jwt-wire-sk-ws-a" {
		t.Fatalf("jwtBytesForKey() = %q, want refreshed JWT", got)
	}
}
