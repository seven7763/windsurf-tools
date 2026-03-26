package services

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

// OpenAIRelay 本地 OpenAI 兼容 API 中转服务器
type OpenAIRelay struct {
	mu        sync.RWMutex
	server    *http.Server
	listener  net.Listener
	running   bool
	port      int
	secret    string     // Bearer token 鉴权
	proxy     *MitmProxy // 复用账号池
	logFn     func(string)
	onSuccess func(apiKey string) // 请求成功后回调（用于触发额度刷新）
	proxyURL  string              // 出站代理
	upstream  http.RoundTripper   // 持久连接池
	maxRetry  int                 // 额度耗尽重试次数
}

// SetOnSuccess 设置请求成功回调（App 层用来触发额度刷新）
func (r *OpenAIRelay) SetOnSuccess(fn func(apiKey string)) {
	r.mu.Lock()
	r.onSuccess = fn
	r.mu.Unlock()
}

type OpenAIRelayStatus struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	URL     string `json:"url"`
}

func NewOpenAIRelay(proxy *MitmProxy, logFn func(string), proxyURL string) *OpenAIRelay {
	return &OpenAIRelay{
		port:     8787,
		proxy:    proxy,
		logFn:    logFn,
		proxyURL: proxyURL,
		maxRetry: 3,
	}
}

func (r *OpenAIRelay) log(format string, args ...interface{}) {
	if r.logFn != nil {
		r.logFn(fmt.Sprintf("[OpenAI Relay] "+format, args...))
	}
}

func (r *OpenAIRelay) Start(port int, secret string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return fmt.Errorf("relay already running")
	}

	if port <= 0 {
		port = 8787
	}
	r.port = port
	r.secret = secret

	// 构建持久 h2 transport（连接池复用）
	r.upstream = r.buildUpstreamTransport()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", r.handleChatCompletions)
	mux.HandleFunc("/v1/models", r.handleModels)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("listen :%d: %w", port, err)
	}

	r.listener = ln
	r.server = &http.Server{Handler: mux}
	r.running = true

	go func() {
		r.log("started on http://127.0.0.1:%d", port)
		if err := r.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			r.log("server error: %v", err)
		}
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()
	return nil
}

func (r *OpenAIRelay) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running || r.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := r.server.Shutdown(ctx)
	r.running = false
	r.log("stopped")
	return err
}

func (r *OpenAIRelay) Status() OpenAIRelayStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := OpenAIRelayStatus{Running: r.running, Port: r.port}
	if r.running {
		s.URL = fmt.Sprintf("http://127.0.0.1:%d", r.port)
	}
	return s
}

// ── 鉴权 ──

func (r *OpenAIRelay) checkAuth(w http.ResponseWriter, req *http.Request) bool {
	if r.secret == "" {
		return true
	}
	auth := req.Header.Get("Authorization")
	if strings.TrimPrefix(auth, "Bearer ") == r.secret {
		return true
	}
	writeOpenAIError(w, 401, "invalid_api_key", "Invalid API key")
	return false
}

// ── /v1/models ──

func (r *OpenAIRelay) handleModels(w http.ResponseWriter, req *http.Request) {
	if !r.checkAuth(w, req) {
		return
	}
	models := []string{
		// Windsurf
		"cascade",
		// OpenAI GPT
		"gpt-3.5-turbo", "gpt-3.5-turbo-16k",
		"gpt-4", "gpt-4-32k", "gpt-4-turbo",
		"gpt-4o", "gpt-4o-mini", "gpt-4o-latest",
		"gpt-4.1", "gpt-4.1-mini", "gpt-4.1-nano",
		"gpt-5", "gpt-5-nano", "gpt-5-pro",
		"gpt-5.1", "gpt-5.1-codex", "gpt-5.1-codex-mini",
		"gpt-5.2", "gpt-5.2-codex",
		"gpt-5.3-codex", "gpt-5.3-codex-spark-preview",
		"gpt-oss-120b",
		// OpenAI o-series
		"o1", "o1-mini", "o1-preview",
		"o3", "o3-mini", "o3-pro",
		// Anthropic Claude
		"claude-3-opus", "claude-3-sonnet",
		"claude-3.5-haiku", "claude-3p5", "claude-3p7",
		"claude-sonnet-4", "claude-sonnet-4.5", "claude-sonnet-4.6",
		"claude-sonnet-4-6-1m", "claude-sonnet-4-6-thinking",
		"claude-opus-4", "claude-opus-4.1", "claude-opus-4.5",
		"claude-opus-4.6", "claude-opus-4-6-1m", "claude-opus-4-6-1m-max",
		"claude-opus-4-6-thinking-1m", "claude-opus-4-6-thinking-1m-max",
		"claude-opus-4-6-fast", "claude-opus-4-6-thinking-fast",
		"claude-opus-4-5-20251101",
		// Google Gemini
		"gemini-2.0-flash", "gemini-2.5-flash-lite", "gemini-2.5-pro",
		"gemini-3.0-pro", "gemini-3.0-flash",
		"gemini-3.1-pro", "gemini-3-1-pro-high", "gemini-3-1-pro-low",
		"gemini-3-pro", "gemini-3-flash-preview",
		// Meta Llama
		"llama-3.1-70b-instruct", "llama-3.1-405b-instruct",
		"llama-3.3-70b-instruct", "llama-3.3-70b-instruct-r1",
		// DeepSeek
		"deepseek-v3", "deepseek-r1", "deepseek-r1-distill-llama-70b",
		// Qwen
		"qwen-2.5-7b-instruct", "qwen-2.5-32b-instruct",
		// Mistral
		"devstral",
		// Internal codenames
		"crispy-unicorn", "crispy-unicorn-thinking",
		"fierce-falcon", "robin-alpha-next", "skyhawk",
	}
	var data []map[string]interface{}
	for _, m := range models {
		data = append(data, map[string]interface{}{
			"id": m, "object": "model", "owned_by": "windsurf",
		})
	}
	resp := map[string]interface{}{"object": "list", "data": data}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ── /v1/chat/completions ──

type openAIChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   *bool         `json:"stream,omitempty"`
}

func (r *OpenAIRelay) handleChatCompletions(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeOpenAIError(w, 405, "method_not_allowed", "POST only")
		return
	}
	if !r.checkAuth(w, req) {
		return
	}

	var chatReq openAIChatRequest
	if err := json.NewDecoder(req.Body).Decode(&chatReq); err != nil {
		writeOpenAIError(w, 400, "invalid_request", err.Error())
		return
	}
	if len(chatReq.Messages) == 0 {
		writeOpenAIError(w, 400, "invalid_request", "messages is required")
		return
	}

	stream := chatReq.Stream != nil && *chatReq.Stream

	// 从账号池拿 key + JWT（支持额度耗尽 / 认证失败自动轮转重试）
	var upstreamResp *http.Response
	var usedKey string
	for attempt := 0; attempt <= r.maxRetry; attempt++ {
		apiKey, jwtBytes := r.proxy.pickPoolKeyAndJWT()
		if apiKey == "" || len(jwtBytes) == 0 {
			writeOpenAIError(w, 503, "no_accounts", "No available accounts in pool")
			return
		}
		jwtStr := string(jwtBytes)
		usedKey = apiKey

		if attempt == 0 {
			r.log("chat request: model=%s messages=%d stream=%v key=%s...", chatReq.Model, len(chatReq.Messages), stream, truncKey(apiKey))
		}

		protoBody := BuildChatRequest(chatReq.Messages, apiKey, jwtStr, "")
		grpcPayload := WrapGRPCEnvelope(protoBody)

		resp, kind, err := r.sendGRPC(grpcPayload, apiKey, jwtStr)
		if err != nil {
			if kind == upstreamFailureQuota {
				r.log("额度耗尽 key=%s... 自动轮转重试(%d/%d)", truncKey(apiKey), attempt+1, r.maxRetry)
				r.proxy.markRuntimeExhaustedAndRotate(apiKey, "relay-quota")
				continue
			}
			if kind == upstreamFailureAuth {
				r.log("认证失败 key=%s... 尝试刷新JWT(%d/%d)", truncKey(apiKey), attempt+1, r.maxRetry)
				refreshed := r.proxy.refreshJWTForKey(apiKey)
				if len(refreshed) > 0 {
					continue // 用刷新后的 JWT 重试（pickPoolKeyAndJWT 会拿到新 JWT）
				}
				r.log("JWT 刷新失败，尝试切到下一把 key")
				r.proxy.markRuntimeExhaustedAndRotate(apiKey, "relay-auth")
				continue
			}
			r.log("gRPC error (kind=%s): %v", string(kind), err)
			writeOpenAIError(w, 502, "upstream_error", err.Error())
			return
		}
		upstreamResp = resp
		break
	}
	if upstreamResp == nil {
		writeOpenAIError(w, 503, "all_exhausted", "All accounts in pool are exhausted")
		return
	}
	defer upstreamResp.Body.Close()

	chatID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	model := chatReq.Model
	if model == "" {
		model = "cascade"
	}

	var finalKind upstreamFailureKind
	var finalDetail string
	if stream {
		finalKind, finalDetail = r.streamResponse(w, upstreamResp, chatID, model)
	} else {
		finalKind, finalDetail = r.blockingResponse(w, upstreamResp, chatID, model)
	}
	r.finalizeRelayOutcome(usedKey, finalKind, finalDetail)
}

// buildUpstreamTransport 构建持久化 transport（与 MITM 上游一致，http.Transport + ForceAttemptHTTP2）
func (r *OpenAIRelay) buildUpstreamTransport() http.RoundTripper {
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         GRPCUpstreamHost,
			NextProtos:         []string{"h2"},
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	}
	if r.proxyURL != "" {
		if u, err := url.Parse(r.proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
			r.log("出站代理: %s", r.proxyURL)
		}
	}
	// 显式配置 HTTP/2（gRPC 必须 h2）
	if err := http2.ConfigureTransport(t); err != nil {
		r.log("http2.ConfigureTransport 失败: %v (回退 ForceAttemptHTTP2)", err)
	}
	r.log("transport built: ServerName=%s h2=explicit proxy=%s", GRPCUpstreamHost, r.proxyURL)
	return t
}

// sendGRPC 向 Windsurf 上游发送 gRPC 请求，返回完整响应与失败分类。
// 同时检测 gRPC Trailers-Only 模式（HTTP 200 但 grpc-status 头非零）。
func (r *OpenAIRelay) sendGRPC(payload []byte, apiKey, jwt string) (*http.Response, upstreamFailureKind, error) {
	upIP := ResolveUpstreamIP()
	grpcURL := fmt.Sprintf("https://%s/exa.api_server_pb.ApiServerService/GetChatMessage", upIP)
	httpReq, err := http.NewRequest("POST", grpcURL, bytes.NewReader(payload))
	if err != nil {
		return nil, upstreamFailureNone, err
	}
	httpReq.Host = GRPCUpstreamHost
	httpReq.Header.Set("content-type", "application/grpc")
	httpReq.Header.Set("te", "trailers")
	httpReq.Header.Set("authorization", "Bearer "+jwt)
	httpReq.Header.Set("user-agent", "connect-es/1.6.1")
	httpReq.Header.Set("x-client-name", WindsurfAppName)
	httpReq.Header.Set("x-client-version", WindsurfVersion)

	transport := r.upstream
	if transport == nil {
		transport = r.buildUpstreamTransport()
	}
	r.log("sendGRPC → %s (host=%s) payload=%dB", upIP, GRPCUpstreamHost, len(payload))
	resp, err := transport.RoundTrip(httpReq)
	if err != nil {
		return nil, upstreamFailureNone, fmt.Errorf("grpc roundtrip to %s: %w", upIP, err)
	}

	grpcStatus := resp.Header.Get("grpc-status")
	grpcMsg := resp.Header.Get("grpc-message")

	// 非 200 或 Trailers-Only 错误（HTTP 200 + grpc-status 头非空非 0）
	isHTTPErr := resp.StatusCode != 200
	isTrailersOnlyErr := grpcStatus != "" && grpcStatus != "0"
	if isHTTPErr || isTrailersOnlyErr {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		kind, detail := classifyUpstreamFailure(grpcStatus, grpcMsg, string(body))
		r.log("sendGRPC error: ip=%s status=%d proto=%s grpc-status=%s kind=%s detail=%s body=%s",
			upIP, resp.StatusCode, resp.Proto, grpcStatus, string(kind), detail, truncate(string(body), 200))
		if detail == "" {
			detail = fmt.Sprintf("upstream HTTP %d (proto=%s), grpc-status=%s, grpc-message=%s", resp.StatusCode, resp.Proto, grpcStatus, grpcMsg)
		}
		return nil, kind, fmt.Errorf("%s", detail)
	}
	r.log("sendGRPC ok: proto=%s status=%d", resp.Proto, resp.StatusCode)
	return resp, upstreamFailureNone, nil
}

// streamResponse 将 gRPC 流式响应转为 SSE。
// 返回值用于调用方判断这次流是正常完成，还是在流尾 / trailer 处以 quota/auth/grpc 失败收尾。
func (r *OpenAIRelay) streamResponse(w http.ResponseWriter, resp *http.Response, chatID, model string) (upstreamFailureKind, string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, 500, "internal", "streaming not supported")
		return upstreamFailureGRPC, "streaming not supported"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	body := resp.Body
	reader := bufio.NewReaderSize(body, 32768)
	buf := make([]byte, 0, 65536)
	sawTerminalFrame := false

	for {
		tmp := make([]byte, 8192)
		n, readErr := reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}

		// 尝试从 buf 中提取完整的 gRPC 帧
		for len(buf) >= 5 {
			flags := buf[0]
			envelopeLen := int(buf[1])<<24 | int(buf[2])<<16 | int(buf[3])<<8 | int(buf[4])
			totalLen := 5 + envelopeLen
			if len(buf) < totalLen {
				break
			}
			framePayload := append([]byte(nil), buf[5:totalLen]...)
			buf = buf[totalLen:]

			decodedPayload, err := decodeStreamEnvelopePayload(flags, framePayload)
			if err != nil {
				continue
			}
			if flags&streamEnvelopeEndStream != 0 {
				if kind, detail := classifyUpstreamFailure("", "", string(decodedPayload)); kind != upstreamFailureNone {
					return kind, detail
				}
				sawTerminalFrame = true
				continue
			}

			text, isDone, err := ParseChatResponseChunk(decodedPayload)
			if err != nil {
				continue
			}
			if text != "" {
				chunk := buildSSEChunk(chatID, model, text, false)
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				flusher.Flush()
			}
			if isDone {
				sawTerminalFrame = true
			}
		}

		if readErr != nil {
			if readErr != io.EOF {
				return upstreamFailureGRPC, readErr.Error()
			}
			if len(buf) > 0 {
				return upstreamFailureGRPC, "stream ended with incomplete grpc frame"
			}
			if kind, detail := classifyUpstreamFailure(resp.Trailer.Get("grpc-status"), resp.Trailer.Get("grpc-message"), ""); kind != upstreamFailureNone {
				return kind, detail
			}
			// 正常结束时才向 OpenAI SSE 客户端发 stop + [DONE]。
			// 这样 quota/auth/trailer 失败不会再伪装成一次成功完成的响应。
			_ = sawTerminalFrame // EOF without trailer failure也按正常结束处理，避免客户端悬挂。
			chunk := buildSSEChunk(chatID, model, "", true)
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return upstreamFailureNone, ""
		}
	}
}

// blockingResponse 收集所有响应后一次性返回
func (r *OpenAIRelay) blockingResponse(w http.ResponseWriter, resp *http.Response, chatID, model string) (upstreamFailureKind, string) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		writeOpenAIError(w, 502, "upstream_error", err.Error())
		return upstreamFailureGRPC, err.Error()
	}
	if kind, detail := classifyUpstreamFailure(resp.Trailer.Get("grpc-status"), resp.Trailer.Get("grpc-message"), string(data)); kind != upstreamFailureNone {
		writeRelayUpstreamFailure(w, kind, detail)
		return kind, detail
	}

	frames := ExtractGRPCFrames(data)
	var fullText strings.Builder
	for _, frame := range frames {
		text, _, _ := ParseChatResponseChunk(frame)
		fullText.WriteString(text)
	}

	reply := map[string]interface{}{
		"id":      chatID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": fullText.String()},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
	return upstreamFailureNone, ""
}

// ── 辅助 ──

func buildSSEChunk(id, model, content string, isStop bool) string {
	delta := map[string]string{}
	if content != "" {
		delta["content"] = content
	}
	finishReason := interface{}(nil)
	if isStop {
		finishReason = "stop"
	}
	chunk := map[string]interface{}{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{"index": 0, "delta": delta, "finish_reason": finishReason},
		},
	}
	b, _ := json.Marshal(chunk)
	return string(b)
}

func writeOpenAIError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"type":    errType,
			"code":    errType,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func writeRelayUpstreamFailure(w http.ResponseWriter, kind upstreamFailureKind, detail string) {
	status := 502
	errType := "upstream_error"
	switch kind {
	case upstreamFailureQuota:
		status = 429
		errType = "quota_exhausted"
	case upstreamFailureAuth:
		status = 401
		errType = "authentication_error"
	case upstreamFailurePermission:
		status = 403
		errType = "permission_denied"
	}
	if strings.TrimSpace(detail) == "" {
		detail = "upstream request failed"
	}
	writeOpenAIError(w, status, errType, detail)
}

func (r *OpenAIRelay) finalizeRelayOutcome(apiKey string, kind upstreamFailureKind, detail string) {
	if strings.TrimSpace(apiKey) == "" {
		return
	}
	if kind == upstreamFailureNone {
		r.proxy.RecordKeySuccess(apiKey)
		r.mu.RLock()
		cb := r.onSuccess
		r.mu.RUnlock()
		if cb != nil {
			go cb(apiKey)
		}
		return
	}

	detail = strings.TrimSpace(detail)
	switch kind {
	case upstreamFailureQuota:
		r.log("relay 结束为额度失败: key=%s... detail=%s", truncKey(apiKey), truncate(detail, 180))
		r.proxy.markRuntimeExhaustedAndRotate(apiKey, "relay-finished="+detail)
	case upstreamFailureAuth:
		r.log("relay 结束为认证失败: key=%s... detail=%s", truncKey(apiKey), truncate(detail, 180))
		if len(r.proxy.refreshJWTForKey(apiKey)) == 0 {
			r.proxy.markRuntimeExhaustedAndRotate(apiKey, "relay-auth="+detail)
		}
	default:
		r.log("relay 结束为上游失败: key=%s... kind=%s detail=%s", truncKey(apiKey), kind, truncate(detail, 180))
	}
}

func truncKey(key string) string {
	if len(key) > 12 {
		return key[:12]
	}
	return key
}
