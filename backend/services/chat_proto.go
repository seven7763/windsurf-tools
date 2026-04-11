package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"windsurf-tools-wails/backend/utils"
)

// ══════════════════════════════════════════════════════════════
// GetChatMessage protobuf 编解码
// ══════════════════════════════════════════════════════════════

// ChatMessage OpenAI 格式的消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// parseProtoFields 从 protobuf 字节流中解析所有顶层字段（复用 windsurf.go 的 protoField）
func parseProtoFields(data []byte) protoMessage {
	return decodeProtoMessage(data)
}

// BuildChatRequest 构造 GetChatMessage 的 protobuf 请求体（不含 gRPC 5 字节信封）。
// 字段布局（基于抓包逆向）：
//
//	F1  = metadata (api_key, JWT, client info)
//	F2  = system prompt (string, 仅 system 消息)
//	F3  = chat messages (repeated, sub: F2=role(varint), F3=content, F4=index)
//	F7  = settings varint (15 = 默认)
//	F8  = generation config (max tokens / stop words)
//	F21 = model enum string (如 "MODEL_GOOGLE_GEMINI_2_5_FLASH")
//	F22 = message ID (UUID)
func BuildChatRequest(messages []ChatMessage, apiKey, jwt, conversationID string) []byte {
	return BuildChatRequestWithModel(messages, apiKey, jwt, conversationID, "")
}

// BuildChatRequestWithModel 同 BuildChatRequest，支持指定模型名。
func BuildChatRequestWithModel(messages []ChatMessage, apiKey, jwt, conversationID, model string) []byte {
	// F1: metadata
	metadata := buildChatMetadata(apiKey, jwt)
	metaField := encodeBytesField(1, metadata)

	// 分离 system 消息和 chat 消息
	var systemPrompt string
	var chatMessages []ChatMessage
	for _, m := range messages {
		if m.Role == "system" {
			if systemPrompt != "" {
				systemPrompt += "\n\n"
			}
			systemPrompt += m.Content
		} else {
			chatMessages = append(chatMessages, m)
		}
	}

	var body []byte
	body = append(body, metaField...)

	// F2: system prompt (顶层字符串)
	if systemPrompt != "" {
		body = append(body, utils.EncodeStringField(2, systemPrompt)...)
	}

	// F3: chat messages (repeated sub-message)
	// 每条消息: F2=role(varint 1=user,2=bot), F3=content(string), F4=index(varint)
	for i, m := range chatMessages {
		var msg []byte
		role := uint64(1) // user
		if m.Role == "assistant" {
			role = 2
		}
		msg = append(msg, encodeVarintField(2, role)...)
		msg = append(msg, utils.EncodeStringField(3, m.Content)...)
		msg = append(msg, encodeVarintField(4, uint64(i))...)
		body = append(body, encodeBytesField(3, msg)...)
	}
	// 如果没有 chat messages（只有 system），构造一个空 user 消息
	if len(chatMessages) == 0 && systemPrompt == "" {
		// 兜底：从所有 messages 拼接
		prompt := flattenMessages(messages)
		var msg []byte
		msg = append(msg, encodeVarintField(2, 1)...) // role=user
		msg = append(msg, utils.EncodeStringField(3, prompt)...)
		msg = append(msg, encodeVarintField(4, 0)...)
		body = append(body, encodeBytesField(3, msg)...)
	}

	// F7: settings (varint 15 — 开启 streaming)
	body = append(body, encodeVarintField(7, 15)...)

	// F8: generation config
	body = append(body, encodeBytesField(8, buildGenerationConfig())...)

	// F15: conversation context（含 conversation_id）
	if conversationID != "" {
		var convCtx []byte
		convCtx = append(convCtx, utils.EncodeStringField(1, conversationID)...)
		body = append(body, encodeBytesField(15, convCtx)...)
	}

	// F21: model name (field > 15, 需要 varint tag 编码)
	modelEnum := mapModelToWindsurfEnum(model)
	if modelEnum != "" {
		body = append(body, encodeBytesField(21, []byte(modelEnum))...)
	}

	// F22: message ID (UUID, field > 15, 需要 varint tag 编码)
	body = append(body, encodeBytesField(22, []byte(generateUUID()))...)

	return body
}

// WrapGRPCEnvelope 给 protobuf 消息加上 gRPC 5 字节信封
func WrapGRPCEnvelope(payload []byte) []byte {
	envelope := make([]byte, 5+len(payload))
	envelope[0] = 0x00
	binary.BigEndian.PutUint32(envelope[1:5], uint32(len(payload)))
	copy(envelope[5:], payload)
	return envelope
}

const (
	streamEnvelopeCompressed = 0x01
	streamEnvelopeEndStream  = 0x02
)

type streamEnvelope struct {
	Flags   byte
	Payload []byte
}

// ExtractGRPCEnvelopes 从响应字节流中提取原始 envelope（保留 flag，用于处理压缩和 end-stream）。
func ExtractGRPCEnvelopes(data []byte) []streamEnvelope {
	var envelopes []streamEnvelope
	pos := 0
	for pos+5 <= len(data) {
		flags := data[pos]
		payloadLen := int(binary.BigEndian.Uint32(data[pos+1 : pos+5]))
		pos += 5
		if pos+payloadLen > len(data) {
			break
		}
		payload := append([]byte(nil), data[pos:pos+payloadLen]...)
		envelopes = append(envelopes, streamEnvelope{
			Flags:   flags,
			Payload: payload,
		})
		pos += payloadLen
	}
	return envelopes
}

func decodeStreamEnvelopePayload(flags byte, payload []byte) ([]byte, error) {
	if flags&streamEnvelopeCompressed == 0 {
		return append([]byte(nil), payload...), nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}
	return decoded, nil
}

// ParseChatResponseChunk 从流式响应的一个 gRPC 帧中提取文本 delta。
// data 应为去掉 5 字节信封后的 protobuf payload。
func ParseChatResponseChunk(data []byte) (text string, isDone bool, err error) {
	fields := parseProtoFields(data)
	if len(fields) == 0 {
		return "", false, fmt.Errorf("empty protobuf chunk")
	}

	// 真实 GetChatMessage 响应的文本增量位于 F6.F3；先走这个路径，
	// 避免把 bot id / request id 之类的 metadata 当成回答正文。
	if preferred := extractProtoTextAtPath(fields, 6, 3); preferred != "" {
		text = preferred
	} else {
		text = extractLegacyChatText(fields)
	}

	for _, f := range fields {
		if f.Number == 2 && f.Wire == 0 && f.Varint != 0 {
			isDone = true
		}
	}
	return text, isDone, nil
}

// ExtractGRPCFrames 从流式响应字节流中提取多个 gRPC 帧。
func ExtractGRPCFrames(data []byte) [][]byte {
	var frames [][]byte
	for _, envelope := range ExtractGRPCEnvelopes(data) {
		if envelope.Flags&streamEnvelopeEndStream != 0 {
			continue
		}
		frame, err := decodeStreamEnvelopePayload(envelope.Flags, envelope.Payload)
		if err != nil {
			continue
		}
		frames = append(frames, frame)
	}
	return frames
}

func extractProtoTextAtPath(message protoMessage, path ...uint64) string {
	if len(path) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, field := range message {
		if field.Number != path[0] || field.Wire != 2 {
			continue
		}
		if len(path) == 1 {
			if isLikelyUTF8(field.Bytes) {
				sb.WriteString(string(field.Bytes))
			}
			continue
		}
		sub := parseProtoFields(field.Bytes)
		if len(sub) == 0 {
			continue
		}
		sb.WriteString(extractProtoTextAtPath(sub, path[1:]...))
	}
	return sb.String()
}

func extractLegacyChatText(fields protoMessage) string {
	var sb strings.Builder
	for _, f := range fields {
		switch {
		case f.Number == 1 && f.Wire == 2:
			if isLikelyUTF8(f.Bytes) {
				s := string(f.Bytes)
				if !looksLikeChatMetadataString(s) {
					sb.WriteString(s)
				}
			} else {
				sub := parseProtoFields(f.Bytes)
				for _, sf := range sub {
					if sf.Wire == 2 && isLikelyUTF8(sf.Bytes) {
						s := string(sf.Bytes)
						if !looksLikeChatMetadataString(s) {
							sb.WriteString(s)
						}
					}
				}
			}
		case f.Number == 3 && f.Wire == 2:
			if isLikelyUTF8(f.Bytes) {
				sb.WriteString(string(f.Bytes))
			}
		}
	}
	return sb.String()
}

func looksLikeChatMetadataString(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "bot-") || strings.HasPrefix(s, "msg_") || strings.HasPrefix(s, "req_") {
		return true
	}
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	return false
}

// ── 内部辅助 ──

func buildChatMetadata(apiKey, jwt string) []byte {
	var meta []byte
	meta = append(meta, utils.EncodeStringField(1, WindsurfAppName)...)
	meta = append(meta, utils.EncodeStringField(2, WindsurfVersion)...)
	meta = append(meta, utils.EncodeStringField(3, apiKey)...)
	meta = append(meta, utils.EncodeStringField(4, "en")...)
	meta = append(meta, utils.EncodeStringField(5, currentWindsurfClientPlatform())...)
	meta = append(meta, utils.EncodeStringField(7, WindsurfClient)...)
	meta = append(meta, utils.EncodeStringField(12, WindsurfAppName)...)
	if jwt != "" {
		// F21 > 15, 需要 varint tag 编码（EncodeStringField 只支持 1-15）
		meta = append(meta, encodeBytesField(21, []byte(jwt))...)
	}
	return meta
}

func flattenMessages(messages []ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	if len(messages) == 1 {
		return messages[0].Content
	}
	var sb strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "system":
			sb.WriteString("[System]\n")
		case "assistant":
			sb.WriteString("[Assistant]\n")
		default:
			sb.WriteString("[User]\n")
		}
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

func encodeBytesField(fieldNum uint64, data []byte) []byte {
	tag := writeVarint((fieldNum << 3) | 2)
	length := writeVarint(uint64(len(data)))
	result := make([]byte, 0, len(tag)+len(length)+len(data))
	result = append(result, tag...)
	result = append(result, length...)
	result = append(result, data...)
	return result
}
