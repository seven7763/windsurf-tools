package services

import (
	"bufio"
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
)

// OpenAIRelay 本地 OpenAI 兼容 API 中转服务器
type OpenAIRelay struct {
	mu       sync.RWMutex
	server   *http.Server
	listener net.Listener
	running  bool
	port     int
	secret   string     // Bearer token 鉴权
	proxy    *MitmProxy // 复用账号池
	logFn    func(string)
	proxyURL string            // 出站代理
	upstream http.RoundTripper // 持久连接池
	maxRetry int               // 额度耗尽重试次数
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
	models := []string{"gpt-4", "gpt-4o", "claude-3.5-sonnet", "cascade"}
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

	// 从账号池拿 key + JWT（支持额度耗尽自动轮转重试）
	var respBody io.ReadCloser
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

		body, err := r.sendGRPC(grpcPayload, apiKey, jwtStr)
		if err != nil {
			errStr := strings.ToLower(err.Error())
			if isQuotaExhaustedText(errStr) || strings.Contains(errStr, "resource_exhausted") {
				r.log("额度耗尽 key=%s... 自动轮转重试(%d/%d)", truncKey(apiKey), attempt+1, r.maxRetry)
				r.proxy.markRuntimeExhaustedAndRotate(apiKey, "relay-quota")
				continue
			}
			r.log("gRPC error: %v", err)
			writeOpenAIError(w, 502, "upstream_error", err.Error())
			return
		}
		respBody = body
		break
	}
	if respBody == nil {
		writeOpenAIError(w, 503, "all_exhausted", "All accounts in pool are exhausted")
		return
	}
	defer respBody.Close()
	_ = usedKey

	chatID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	model := chatReq.Model
	if model == "" {
		model = "cascade"
	}

	if stream {
		r.streamResponse(w, respBody, chatID, model)
	} else {
		r.blockingResponse(w, respBody, chatID, model)
	}
}

// buildUpstreamTransport 构建持久化 transport（与 MITM 上游一致，http.Transport + ForceAttemptHTTP2）
func (r *OpenAIRelay) buildUpstreamTransport() http.RoundTripper {
	t := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         UpstreamHost,
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
	return t
}

// sendGRPC 向 Windsurf 上游发送 gRPC 请求，返回响应 body
func (r *OpenAIRelay) sendGRPC(payload []byte, apiKey, jwt string) (io.ReadCloser, error) {
	grpcURL := fmt.Sprintf("https://%s/exa.chat_pb.ChatService/GetChatMessage", UpstreamIP)
	httpReq, err := http.NewRequest("POST", grpcURL, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	httpReq.Host = UpstreamHost
	httpReq.Header.Set("content-type", "application/grpc")
	httpReq.Header.Set("te", "trailers")
	httpReq.Header.Set("authorization", "Bearer "+jwt)

	transport := r.upstream
	if transport == nil {
		transport = r.buildUpstreamTransport()
	}
	resp, err := transport.RoundTrip(httpReq)
	if err != nil {
		return nil, fmt.Errorf("grpc roundtrip: %w", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		grpcStatus := resp.Header.Get("grpc-status")
		grpcMsg := resp.Header.Get("grpc-message")
		return nil, fmt.Errorf("grpc status %d (grpc-status=%s, msg=%s): %s",
			resp.StatusCode, grpcStatus, grpcMsg, truncate(string(body), 200))
	}
	return resp.Body, nil
}

// streamResponse 将 gRPC 流式响应转为 SSE
func (r *OpenAIRelay) streamResponse(w http.ResponseWriter, body io.ReadCloser, chatID, model string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, 500, "internal", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	reader := bufio.NewReaderSize(body, 32768)
	buf := make([]byte, 0, 65536)

	for {
		tmp := make([]byte, 8192)
		n, readErr := reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}

		// 尝试从 buf 中提取完整的 gRPC 帧
		for len(buf) >= 5 {
			frameLen := int(buf[1])<<24 | int(buf[2])<<16 | int(buf[3])<<8 | int(buf[4])
			totalLen := 5 + frameLen
			if len(buf) < totalLen {
				break
			}
			framePayload := buf[5:totalLen]
			buf = buf[totalLen:]

			text, isDone, err := ParseChatResponseChunk(framePayload)
			if err != nil {
				continue
			}
			if text != "" {
				chunk := buildSSEChunk(chatID, model, text, false)
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				flusher.Flush()
			}
			if isDone {
				chunk := buildSSEChunk(chatID, model, "", true)
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
		}

		if readErr != nil {
			// 流结束
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
	}
}

// blockingResponse 收集所有响应后一次性返回
func (r *OpenAIRelay) blockingResponse(w http.ResponseWriter, body io.ReadCloser, chatID, model string) {
	data, err := io.ReadAll(body)
	if err != nil {
		writeOpenAIError(w, 502, "upstream_error", err.Error())
		return
	}

	frames := ExtractGRPCFrames(data)
	var fullText strings.Builder
	for _, frame := range frames {
		text, _, _ := ParseChatResponseChunk(frame)
		fullText.WriteString(text)
	}

	resp := map[string]interface{}{
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
	json.NewEncoder(w).Encode(resp)
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

func truncKey(key string) string {
	if len(key) > 12 {
		return key[:12]
	}
	return key
}
