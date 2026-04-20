package services

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"
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
func BuildChatRequest(messages []ChatMessage, apiKey, jwt, conversationID string, fp *KeyFingerprint) []byte {
	return BuildChatRequestWithModel(messages, apiKey, jwt, conversationID, "", fp)
}

// BuildChatRequestWithModel 同 BuildChatRequest，支持指定模型名。
func BuildChatRequestWithModel(messages []ChatMessage, apiKey, jwt, conversationID, model string, fp *KeyFingerprint) []byte {
	// F1: metadata
	metadata := buildChatMetadata(apiKey, jwt, fp)
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

	// F7: settings (varint 5 — 匹配真实 IDE 请求)
	body = append(body, encodeVarintField(7, 5)...)

	// F8: generation config
	body = append(body, encodeBytesField(8, buildGenerationConfig())...)

	// F13: varint flag (IDE 固定 F1=1 子消息)——服务端观察到缺失时内部异常
	body = append(body, encodeBytesField(13, []byte{0x08, 0x01})...)

	// F15: conversation context（含 conversation_id）
	if conversationID != "" {
		var convCtx []byte
		convCtx = append(convCtx, utils.EncodeStringField(1, conversationID)...)
		body = append(body, encodeBytesField(15, convCtx)...)
	}

	// F16: outer conversation UUID——IDE 每次请求必发（区别于 F1 metadata 里的 F16 timestamp）
	convUUID := conversationID
	if convUUID == "" {
		convUUID = generateUUID()
	}
	body = append(body, encodeBytesField(16, []byte(convUUID))...)

	// F20: flag varint=1（IDE 观察到必发）
	body = append(body, encodeVarintField(20, 1)...)

	// F21: model name (field > 15, 需要 varint tag 编码)
	// 注意：IDE 实际发 API 风格名如 "claude-opus-4-7-medium"，不是 MODEL_* enum。
	modelName := strings.TrimSpace(model)
	if modelName != "" && strings.ToLower(modelName) != "cascade" {
		body = append(body, encodeBytesField(21, []byte(modelName))...)
	}

	// F22: message ID (UUID, field > 15, 需要 varint tag 编码)
	body = append(body, encodeBytesField(22, []byte(generateUUID()))...)

	return body
}

// WrapGRPCEnvelope 给 protobuf 消息加上 gRPC/Connect 5 字节信封 (未压缩)
func WrapGRPCEnvelope(payload []byte) []byte {
	envelope := make([]byte, 5+len(payload))
	envelope[0] = 0x00
	binary.BigEndian.PutUint32(envelope[1:5], uint32(len(payload)))
	copy(envelope[5:], payload)
	return envelope
}

// WrapGRPCEnvelopeGzip 给 protobuf 消息加上 gzip 压缩的 Connect 5 字节信封
// 匹配真实 IDE 行为: flag=0x01 (compressed) + 4 bytes length + gzip(payload)
func WrapGRPCEnvelopeGzip(payload []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(payload)
	gw.Close()
	compressed := buf.Bytes()

	envelope := make([]byte, 5+len(compressed))
	envelope[0] = 0x01 // compressed flag
	binary.BigEndian.PutUint32(envelope[1:5], uint32(len(compressed)))
	copy(envelope[5:], compressed)
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
//
// 真实 IDE 2026-04 抓包显示响应每一帧的结构是：
//
//	F1  = bot id "bot-XXXX"
//	F2  = {F1=unix_sec, F2=nanos} 时间戳子消息
//	F3  = string delta                   ← 文本增量在这里（顶层 F3）
//	F4  = varint 序号
//	F5  = varint end-flag（出现即本轮结束）
//	F7  = 上游元信息（请求头、模型名等）
//	F12 = fixed64 某个 metric
//	F17 = 对话 UUID
//	F28 = token usage 详情（末帧）
//
// 旧版逆向错写成 F6.F3 ——因此加兜底；isDone 从 F5 varint 判定（F5 出现且非 0）。
func ParseChatResponseChunk(data []byte) (text string, isDone bool, err error) {
	fields := parseProtoFields(data)
	if len(fields) == 0 {
		return "", false, fmt.Errorf("empty protobuf chunk")
	}

	// 顶层 F3 字符串为文本增量（Windsurf 2026-04 抓包确认）。
	for _, f := range fields {
		if f.Number == 3 && f.Wire == 2 && isLikelyUTF8(f.Bytes) {
			text = string(f.Bytes)
			break
		}
	}
	// 兼容旧协议：若顶层 F3 不是文本，回退到 F6.F3 / legacy 扫描。
	if text == "" {
		if preferred := extractProtoTextAtPath(fields, 6, 3); preferred != "" {
			text = preferred
		} else {
			text = extractLegacyChatText(fields)
		}
	}

	for _, f := range fields {
		// F5 = end-of-turn 标志（varint 非 0 即结束，末帧典型值 4）。
		if f.Number == 5 && f.Wire == 0 && f.Varint != 0 {
			isDone = true
		}
		// 旧协议 F2 varint 结束标志兼容。
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

// buildChatMetadata 构建 GetChatMessage / CheckUserMessageRateLimit 等接口
// 外层 F1 (metadata) 子消息。字段顺序、类型与真实 IDE 2026-04 抓包严格对齐：
//
//	F1  string  AppName         "windsurf"
//	F2  string  Version         "1.48.2"
//	F3  string  API key (raw) —— IDE 实际用 "devin-session-token$..." 的会话 JWT，
//	          我们无此会话，用 sk-ws-* 兜底（服务端接受）
//	F4  string  Locale          "en" （IDE 用 "zh-cn" 等用户区域，对 API 无影响）
//	F5  string  OS JSON         {"Os":..,"Arch":..,"Version":..,"ProductName":..,...}
//	F7  string  Client version  "2.0.50"  ← ★ 过去误写为 sub-message，导致 resource_exhausted
//	F8  string  CPU JSON        {"NumSockets":..,"NumCores":..,"ModelName":..,"Memory":..}
//	F12 string  AppName (dup)   "windsurf"
//	F16 msg     Timestamp       {F1=unix_sec, F2=nanos}  ← IDE GetChatMessage 必发
//	F20 string  User ID         "user-XXX"（从 JWT auth_uid / api_key 切出）
//	F21 bytes   JWT             长 JWT（auth_uid 带 api_key 的那份）
//	F27 string  Device hash     64 字符 hex（per-key 稳定） ← IDE GetChatMessage 必发
//	F30 bytes   Flag            \x00\x01\x03
//	F31 string  Long device fp  hex，~764 字符（IDE 实测）
//	F32 string  Team ID         "devin-team$account-XXX"（从 JWT team_id 读；缺则用 SessionID 兜底）
func buildChatMetadata(apiKey, jwt string, fp *KeyFingerprint) []byte {
	// 从 JWT 提取 team_id / user_id，缺失时走兜底。
	var teamID, userID string
	if jwt != "" {
		if claims, err := (&WindsurfService{}).DecodeJWTClaims(jwt); err == nil && claims != nil {
			teamID = claims.TeamID
			userID = claims.UserID
		}
	}

	var meta []byte
	meta = append(meta, utils.EncodeStringField(1, WindsurfAppName)...)
	meta = append(meta, utils.EncodeStringField(2, WindsurfVersion)...)
	// F3: IDE 实际用 "devin-session-token$<session_JWT>" (~189B 短会话 JWT)。
	// 我们没有会话 token，改用完整 JWT（若可用）。旧代码用 sk-ws-* 会触发服务端 internal error。
	f3Value := jwt
	if f3Value == "" {
		f3Value = apiKey
	}
	meta = append(meta, utils.EncodeStringField(3, f3Value)...)
	meta = append(meta, utils.EncodeStringField(4, "en")...)
	meta = append(meta, utils.EncodeStringField(5, buildOSPlatformJSON())...)
	// F7 必须是字符串 WindsurfClient ("2.0.50")，**不是 sub-message**
	meta = append(meta, utils.EncodeStringField(7, WindsurfClient)...)
	meta = append(meta, utils.EncodeStringField(8, buildCPUInfoJSON())...)
	meta = append(meta, utils.EncodeStringField(12, WindsurfAppName)...)
	// F16 timestamp sub-message {F1=unix_sec, F2=nanos}——IDE GetChatMessage 必发
	{
		var f16 []byte
		now := timeNow()
		f16 = append(f16, encodeVarintField(1, uint64(now.Unix()))...)
		f16 = append(f16, encodeVarintField(2, uint64(now.Nanosecond()))...)
		meta = append(meta, encodeBytesField(16, f16)...)
	}
	// F20 user-XXX（来自 JWT）——IDE 实际抓包里必发此字段
	if userID != "" {
		meta = append(meta, encodeBytesField(20, []byte(userID))...)
	}
	if jwt != "" {
		// F21 > 15，需要 varint tag 编码
		meta = append(meta, encodeBytesField(21, []byte(jwt))...)
	}
	// F27 device hash 64 字符 hex (per-key 稳定)——IDE GetChatMessage 必发
	if fp != nil && fp.DeviceHash != "" {
		meta = append(meta, encodeBytesField(27, []byte(fp.DeviceHash))...)
	}
	// F30 三字节 flag，IDE 固定 0x00 0x01 0x03
	meta = append(meta, encodeBytesField(30, []byte{0x00, 0x01, 0x03})...)
	// F31 长指纹（per-key 稳定，padded hex，IDE 实测 764 字符）
	if fp != nil && fp.DeviceHash != "" {
		f31 := padHexHash(fp.DeviceHash, 764)
		meta = append(meta, encodeBytesField(31, []byte(f31))...)
	}
	// F32 team_id —— IDE 用 "devin-team$account-XXX"；兜底使用稳定 session id
	f32 := teamID
	if f32 == "" && fp != nil {
		f32 = fp.SessionID
	}
	if f32 != "" {
		meta = append(meta, encodeBytesField(32, []byte(f32))...)
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

// encodeFixed64Field encodes a fixed64 field: (fieldNum << 3 | 1) + 8 bytes little-endian.
func encodeFixed64Field(fieldNum uint64, value uint64) []byte {
	tag := writeVarint((fieldNum << 3) | 1)
	result := make([]byte, 0, len(tag)+8)
	result = append(result, tag...)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	result = append(result, buf[:]...)
	return result
}

func randomFixed64() uint64 {
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	return binary.LittleEndian.Uint64(buf[:])
}

// timeNow returns current time (extracted for testability).
var timeNow = time.Now

// buildOSPlatformJSON constructs the F5 OS platform JSON like the real IDE sends.
// 真实值优先：macOS 从 sw_vers 读，Linux/Windows 用合理默认（参见 system_info.go）。
func buildOSPlatformJSON() string {
	osName := "linux"
	switch runtime.GOOS {
	case "windows":
		osName = "windows"
	case "darwin":
		osName = "macos"
	}
	arch := runtime.GOARCH // "amd64" / "arm64" / "386" 等按实际输出
	info := getSystemOSInfo()
	return fmt.Sprintf(`{"Os":"%s","Arch":"%s","Version":"%s","ProductName":"%s","MajorVersionNumber":%d,"MinorVersionNumber":%d,"Build":"%s"}`,
		osName, arch, info.Version, info.ProductName, info.MajorVer, info.MinorVer, info.Build)
}

// buildCPUInfoJSON constructs the F8 CPU info JSON like the real IDE sends.
// macOS 上通过 sysctl 读取真实 CPU brand/核心数/内存，支持 M1~M5 Pro/Max/Ultra。
func buildCPUInfoJSON() string {
	info := getSystemCPUInfo()
	return fmt.Sprintf(`{"NumSockets":%d,"NumCores":%d,"NumThreads":%d,"VendorID":"%s","Family":"%s","Model":"","ModelName":"%s","Memory":%d}`,
		info.NumSockets, info.NumCores, info.NumThreads, info.VendorID, info.Family, info.ModelName, info.Memory)
}
