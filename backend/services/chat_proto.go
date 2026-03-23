package services

import (
	"encoding/binary"
	"fmt"
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
func BuildChatRequest(messages []ChatMessage, apiKey, jwt, conversationID string) []byte {
	// F1: metadata
	metadata := buildChatMetadata(apiKey, jwt)
	metaField := encodeBytesField(1, metadata)

	// F2: conversation_id
	var convField []byte
	if conversationID != "" {
		convField = utils.EncodeStringField(2, conversationID)
	}

	// F3: chat content — 将 messages 拼接为单个用户提示
	prompt := flattenMessages(messages)
	contentInner := utils.EncodeStringField(1, prompt)
	contentField := encodeBytesField(3, contentInner)

	var body []byte
	body = append(body, metaField...)
	if len(convField) > 0 {
		body = append(body, convField...)
	}
	body = append(body, contentField...)
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

// ParseChatResponseChunk 从流式响应的一个 gRPC 帧中提取文本 delta。
// data 应为去掉 5 字节信封后的 protobuf payload。
func ParseChatResponseChunk(data []byte) (text string, isDone bool, err error) {
	fields := parseProtoFields(data)
	if len(fields) == 0 {
		return "", false, fmt.Errorf("empty protobuf chunk")
	}

	for _, f := range fields {
		switch {
		case f.Number == 1 && f.Wire == 2:
			if isLikelyUTF8(f.Bytes) {
				text += string(f.Bytes)
			} else {
				// 可能是子消息，递归找字符串
				sub := parseProtoFields(f.Bytes)
				for _, sf := range sub {
					if sf.Wire == 2 && isLikelyUTF8(sf.Bytes) {
						text += string(sf.Bytes)
					}
				}
			}
		case f.Number == 2 && f.Wire == 0:
			if f.Varint != 0 {
				isDone = true
			}
		case f.Number == 3 && f.Wire == 2:
			if isLikelyUTF8(f.Bytes) {
				text += string(f.Bytes)
			}
		}
	}
	return text, isDone, nil
}

// ExtractGRPCFrames 从流式响应字节流中提取多个 gRPC 帧。
func ExtractGRPCFrames(data []byte) [][]byte {
	var frames [][]byte
	pos := 0
	for pos+5 <= len(data) {
		_ = data[pos] // flag byte
		payloadLen := int(binary.BigEndian.Uint32(data[pos+1 : pos+5]))
		pos += 5
		if pos+payloadLen > len(data) {
			break
		}
		frame := make([]byte, payloadLen)
		copy(frame, data[pos:pos+payloadLen])
		frames = append(frames, frame)
		pos += payloadLen
	}
	return frames
}

// ── 内部辅助 ──

func buildChatMetadata(apiKey, jwt string) []byte {
	var meta []byte
	meta = append(meta, utils.EncodeStringField(1, WindsurfAppName)...)
	meta = append(meta, utils.EncodeStringField(2, WindsurfVersion)...)
	meta = append(meta, utils.EncodeStringField(3, apiKey)...)
	meta = append(meta, utils.EncodeStringField(4, "en")...)
	meta = append(meta, utils.EncodeStringField(5, "windows")...)
	meta = append(meta, utils.EncodeStringField(7, WindsurfClient)...)
	meta = append(meta, utils.EncodeStringField(12, WindsurfAppName)...)
	if jwt != "" {
		meta = append(meta, utils.EncodeStringField(21, jwt)...)
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
