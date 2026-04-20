package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// Anthropic Messages API 兼容端点 — 支持 Claude Code
// POST /v1/messages
// ═══════════════════════════════════════════════════════════════

// anthropicRequest Anthropic Messages API 请求体
type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream"`
	System    string             `json:"system,omitempty"`     // 顶层 system prompt
	SystemArr json.RawMessage    `json:"system_arr,omitempty"` // 数组形式 system（兜底）
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string 或 []content_block
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// extractAnthropicText 从 content 字段提取纯文本
func extractAnthropicText(raw json.RawMessage) string {
	// 尝试字符串
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// 尝试数组
	var blocks []anthropicContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" {
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	return string(raw)
}

// convertAnthropicToChat 将 Anthropic 消息转换为内部 ChatMessage 格式
func convertAnthropicToChat(req anthropicRequest) []ChatMessage {
	var msgs []ChatMessage
	// system prompt
	if req.System != "" {
		msgs = append(msgs, ChatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		text := extractAnthropicText(m.Content)
		role := m.Role
		if role == "assistant" {
			role = "assistant"
		} else {
			role = "user"
		}
		msgs = append(msgs, ChatMessage{Role: role, Content: text})
	}
	return msgs
}

// handleAnthropicMessages 处理 POST /v1/messages
func (r *OpenAIRelay) handleAnthropicMessages(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeAnthropicError(w, 405, "method_not_allowed", "POST only")
		return
	}
	if !r.checkAuth(w, req) {
		return
	}

	var anthReq anthropicRequest
	if err := json.NewDecoder(req.Body).Decode(&anthReq); err != nil {
		writeAnthropicError(w, 400, "invalid_request_error", err.Error())
		return
	}
	if len(anthReq.Messages) == 0 {
		writeAnthropicError(w, 400, "invalid_request_error", "messages is required")
		return
	}
	if anthReq.MaxTokens <= 0 {
		anthReq.MaxTokens = 4096
	}

	chatMessages := convertAnthropicToChat(anthReq)
	startTime := time.Now()

	// 从账号池拿 key + JWT
	var upstreamResp *http.Response
	var usedKey string
	for attempt := 0; attempt <= r.maxRetry; attempt++ {
		apiKey, jwtBytes := r.proxy.pickPoolKeyAndJWT()
		if apiKey == "" || len(jwtBytes) == 0 {
			writeAnthropicError(w, 503, "api_error", "No available accounts in pool")
			return
		}
		jwtStr := string(jwtBytes)
		usedKey = apiKey

		if attempt == 0 {
			r.log("anthropic request: model=%s messages=%d stream=%v key=%s...",
				anthReq.Model, len(anthReq.Messages), anthReq.Stream, truncKey(apiKey))
		}

		r.proxy.mu.RLock()
		anthFP := r.proxy.keyFingerprint(apiKey)
		r.proxy.mu.RUnlock()
		protoBody := BuildChatRequestWithModel(chatMessages, apiKey, jwtStr, "", anthReq.Model, anthFP)
		// Connect 协议：直接发送 protobuf body（无 envelope）
		resp, kind, err := r.sendGRPC(protoBody, apiKey, jwtStr)
		if err != nil {
			if kind == upstreamFailureQuota {
				r.proxy.markRuntimeExhaustedAndRotate(apiKey, "anthropic-quota")
				continue
			}
			if kind == upstreamFailureGlobalRateLimit {
				writeAnthropicError(w, 429, "rate_limit_error", err.Error())
				return
			}
			if kind == upstreamFailureRateLimit {
				if rotatedKey := r.proxy.markRateLimitedAndRotate(apiKey, "anthropic-rate="+err.Error()); rotatedKey != "" {
					continue
				}
				writeAnthropicError(w, 429, "rate_limit_error", err.Error())
				return
			}
			if kind == upstreamFailureAuth {
				if rotatedKey := r.proxy.rotateAfterAuthFailure(apiKey, "anthropic-auth="+err.Error()); rotatedKey != "" {
					continue
				}
				refreshed := r.proxy.refreshJWTForKey(apiKey)
				if len(refreshed) > 0 {
					continue
				}
				writeAnthropicError(w, 401, "authentication_error", err.Error())
				return
			}
			writeAnthropicError(w, 502, "api_error", err.Error())
			return
		}
		upstreamResp = resp
		break
	}
	if upstreamResp == nil {
		writeAnthropicError(w, 503, "api_error", "All accounts in pool are exhausted")
		return
	}
	defer upstreamResp.Body.Close()

	model := anthReq.Model
	if model == "" {
		model = "cascade"
	}
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())

	var finalKind upstreamFailureKind
	var finalDetail string
	var promptTokens, completionTokens int

	// 估算 prompt tokens
	for _, m := range chatMessages {
		promptTokens += estimateTokens(m.Content)
	}

	if anthReq.Stream {
		completionTokens, finalKind, finalDetail = r.streamAnthropicResponse(w, upstreamResp, msgID, model, promptTokens)
	} else {
		completionTokens, finalKind, finalDetail = r.blockingAnthropicResponse(w, upstreamResp, msgID, model, promptTokens)
	}

	r.finalizeRelayOutcome(usedKey, finalKind, finalDetail)

	// 记录用量
	status := "ok"
	if finalKind != upstreamFailureNone {
		status = "error"
	}
	r.recordUsage(UsageRecord{
		Model:            model,
		RequestModel:     anthReq.Model,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
		DurationMs:       time.Since(startTime).Milliseconds(),
		APIKeyShort:      truncKey(usedKey),
		Status:           status,
		ErrorDetail:      finalDetail,
		Format:           "anthropic",
	})
}

// streamAnthropicResponse 流式输出 Anthropic SSE 格式
func (r *OpenAIRelay) streamAnthropicResponse(w http.ResponseWriter, resp *http.Response, msgID, model string, promptTokens int) (int, upstreamFailureKind, string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAnthropicError(w, 500, "api_error", "streaming not supported")
		return 0, upstreamFailureGRPC, "streaming not supported"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)

	// 发送 message_start
	startEvent := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":      msgID,
			"type":    "message",
			"role":    "assistant",
			"model":   model,
			"content": []interface{}{},
			"usage":   map[string]int{"input_tokens": promptTokens, "output_tokens": 0},
		},
	}
	writeAnthropicSSE(w, flusher, "message_start", startEvent)

	// content_block_start
	blockStart := map[string]interface{}{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]string{"type": "text", "text": ""},
	}
	writeAnthropicSSE(w, flusher, "content_block_start", blockStart)

	body := resp.Body
	reader := bufio.NewReaderSize(body, 32768)
	buf := make([]byte, 0, 65536)
	completionTokens := 0

	for {
		tmp := make([]byte, 8192)
		n, readErr := reader.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}

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
					return completionTokens, kind, detail
				}
				continue
			}

			text, _, err := ParseChatResponseChunk(decodedPayload)
			if err != nil {
				continue
			}
			if text != "" {
				completionTokens += estimateTokens(text)
				delta := map[string]interface{}{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]string{"type": "text_delta", "text": text},
				}
				writeAnthropicSSE(w, flusher, "content_block_delta", delta)
			}
		}

		if readErr != nil {
			if readErr != io.EOF {
				return completionTokens, upstreamFailureGRPC, readErr.Error()
			}
			if len(buf) > 0 {
				return completionTokens, upstreamFailureGRPC, "stream ended with incomplete grpc frame"
			}
			if kind, detail := classifyUpstreamFailure(resp.Trailer.Get("grpc-status"), resp.Trailer.Get("grpc-message"), ""); kind != upstreamFailureNone {
				return completionTokens, kind, detail
			}

			// 正常结束
			blockStop := map[string]interface{}{"type": "content_block_stop", "index": 0}
			writeAnthropicSSE(w, flusher, "content_block_stop", blockStop)

			msgDelta := map[string]interface{}{
				"type":  "message_delta",
				"delta": map[string]interface{}{"stop_reason": "end_turn"},
				"usage": map[string]int{"output_tokens": completionTokens},
			}
			writeAnthropicSSE(w, flusher, "message_delta", msgDelta)

			msgStop := map[string]interface{}{"type": "message_stop"}
			writeAnthropicSSE(w, flusher, "message_stop", msgStop)

			return completionTokens, upstreamFailureNone, ""
		}
	}
}

// blockingAnthropicResponse 非流式 Anthropic 响应
func (r *OpenAIRelay) blockingAnthropicResponse(w http.ResponseWriter, resp *http.Response, msgID, model string, promptTokens int) (int, upstreamFailureKind, string) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		writeAnthropicError(w, 502, "api_error", err.Error())
		return 0, upstreamFailureGRPC, err.Error()
	}
	if kind, detail := classifyUpstreamFailure(resp.Trailer.Get("grpc-status"), resp.Trailer.Get("grpc-message"), string(data)); kind != upstreamFailureNone {
		writeAnthropicError(w, 502, "api_error", detail)
		return 0, kind, detail
	}

	frames := ExtractGRPCFrames(data)
	var fullText strings.Builder
	for _, frame := range frames {
		text, _, _ := ParseChatResponseChunk(frame)
		fullText.WriteString(text)
	}

	content := fullText.String()
	completionTokens := estimateTokens(content)

	reply := map[string]interface{}{
		"id":            msgID,
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"content": []map[string]string{
			{"type": "text", "text": content},
		},
		"usage": map[string]int{
			"input_tokens":  promptTokens,
			"output_tokens": completionTokens,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
	return completionTokens, upstreamFailureNone, ""
}

// ── Anthropic SSE 辅助 ──

func writeAnthropicSSE(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(b))
	flusher.Flush()
}

func writeAnthropicError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    errType,
			"message": msg,
		},
	}
	json.NewEncoder(w).Encode(resp)
}
