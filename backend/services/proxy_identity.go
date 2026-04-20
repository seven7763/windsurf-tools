package services

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	mathr "math/rand"
	"regexp"
	"strings"
	"windsurf-tools-wails/backend/utils"
)

// ── Session-aware extraction ──

// uuidPattern matches a UUID v4-like string: 8-4-4-4-12 hex chars.
var uuidPattern = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// ExtractConversationIDFromBody extracts the conversation_id (UUID format)
// from a chat request body using regex scan. Returns (convID, debugInfo).
// Protocol-agnostic: works regardless of protobuf field numbering or envelope format.
func ExtractConversationIDFromBody(body []byte) (string, string) {
	raw, envType := decompressBody(body)

	// Scan for UUID in decompressed payload
	matches := uuidPattern.FindAll(raw, 5)
	var dbg strings.Builder
	fmt.Fprintf(&dbg, "bodyLen=%d rawLen=%d env=%d uuids=%d", len(body), len(raw), envType, len(matches))

	// Filter: skip API key prefixed UUIDs, pick the first non-key UUID
	var picked string
	for _, m := range matches {
		s := string(m)
		idx := bytes.Index(raw, m)
		// 调试：打印每个 UUID 的前缀上下文
		_ = idx - 20 // ctxStart (debug only)
		prefix6 := ""
		if idx >= 6 {
			prefix6 = fmt.Sprintf("%q", string(raw[idx-6:idx]))
		}
		fmt.Fprintf(&dbg, " [%s @%d pfx=%s]", s, idx, prefix6)

		// Skip if it looks like part of an API key (sk-ws-01-...)
		// API keys contain UUID but are prefixed with sk-
		if idx >= 3 && string(raw[idx-3:idx]) == "sk-" {
			continue
		}
		if idx >= 6 {
			prefix := string(raw[idx-6 : idx])
			if strings.Contains(prefix, "sk-") {
				continue
			}
		}
		if picked == "" {
			picked = s
		}
	}
	if picked != "" {
		return picked, dbg.String()
	}

	// Fallback: also try on original body (in case decompressBody failed)
	if envType == 0 {
		matches = uuidPattern.FindAll(body, 5)
		for _, m := range matches {
			s := string(m)
			idx := bytes.Index(body, m)
			if idx >= 6 && strings.Contains(string(body[idx-6:idx]), "sk-") {
				continue
			}
			fmt.Fprintf(&dbg, " fallback[%s]", s)
			return s, dbg.String()
		}
	}

	return "", dbg.String()
}

// ExtractOriginalAPIKeyFromBody extracts the original api_key (sk-ws-*) from the
// F1 metadata sub-message in a protobuf request body. Returns "" if not found.
func ExtractOriginalAPIKeyFromBody(body []byte) string {
	raw, _ := decompressBody(body)
	if !bytes.Contains(raw, apiKeyPrefix) {
		return ""
	}
	fields := parseProtobuf(raw)
	for _, f := range fields {
		if f.FieldNum == 1 && f.WireType == 2 && len(f.Bytes) > 20 {
			sub := parseProtobuf(f.Bytes)
			for _, sf := range sub {
				if sf.WireType == 2 && bytes.HasPrefix(sf.Bytes, apiKeyPrefix) {
					return string(sf.Bytes)
				}
			}
		}
	}
	return ""
}

// ExtractSessionTitleHint extracts a short title-like snippet from the request body.
// It looks for the last user message content in the protobuf chat request (F3 repeated → F3=content).
// Returns at most 40 chars, empty if nothing useful found.
func ExtractSessionTitleHint(body []byte) string {
	raw, _ := decompressBody(body)
	if len(raw) == 0 {
		return ""
	}
	fields := parseProtobuf(raw)
	var lastUserContent string
	for _, f := range fields {
		if f.FieldNum == 3 && f.WireType == 2 && len(f.Bytes) > 4 {
			// F3 = chat message sub-message; F3.F3 = content string
			sub := parseProtobuf(f.Bytes)
			for _, sf := range sub {
				if sf.FieldNum == 3 && sf.WireType == 2 && isLikelyUTF8(sf.Bytes) {
					s := strings.TrimSpace(string(sf.Bytes))
					if s != "" && !looksLikeChatMetadataString(s) {
						lastUserContent = s
					}
				}
			}
		}
	}
	if lastUserContent == "" {
		return ""
	}
	// Truncate to 40 chars for display
	if len([]rune(lastUserContent)) > 40 {
		lastUserContent = string([]rune(lastUserContent)[:40]) + "…"
	}
	// Collapse newlines for single-line display
	lastUserContent = strings.ReplaceAll(lastUserContent, "\n", " ")
	lastUserContent = strings.ReplaceAll(lastUserContent, "\r", "")
	return lastUserContent
}

// ══════════════════════════════════════════════════════════════
// Protobuf identity replacement — ported from interceptor_poc.py
// Only handles api_key (F3) and JWT (F21) in metadata sub-message (F1).
// No billing/credits/F15/attack tampering.
// ══════════════════════════════════════════════════════════════

var apiKeyPrefix = []byte("sk-ws-")

// ── Protobuf primitives ──

func readVarint(data []byte, pos int) (uint64, int) {
	var result uint64
	var shift uint
	for pos < len(data) {
		b := data[pos]
		pos++
		result |= uint64(b&0x7F) << shift
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	return result, pos
}

func writeVarint(value uint64) []byte {
	var parts []byte
	for value > 0x7F {
		parts = append(parts, byte(value&0x7F)|0x80)
		value >>= 7
	}
	parts = append(parts, byte(value&0x7F))
	return parts
}

type protoFieldRaw struct {
	FieldNum uint64
	WireType uint64
	Varint   uint64
	Bytes    []byte
}

func parseProtobuf(data []byte) []protoFieldRaw {
	var fields []protoFieldRaw
	pos := 0
	for pos < len(data) {
		tag, newPos := readVarint(data, pos)
		if newPos == pos { // stuck
			break
		}
		pos = newPos
		if tag == 0 {
			break
		}
		fn := tag >> 3
		wt := tag & 7

		switch wt {
		case 0: // varint
			val, newPos := readVarint(data, pos)
			pos = newPos
			fields = append(fields, protoFieldRaw{FieldNum: fn, WireType: 0, Varint: val})
		case 2: // length-delimited
			length, newPos := readVarint(data, pos)
			pos = newPos
			end := pos + int(length)
			if end > len(data) {
				return fields
			}
			b := make([]byte, length)
			copy(b, data[pos:end])
			fields = append(fields, protoFieldRaw{FieldNum: fn, WireType: 2, Bytes: b})
			pos = end
		case 5: // fixed32
			if pos+4 > len(data) {
				return fields
			}
			b := make([]byte, 4)
			copy(b, data[pos:pos+4])
			fields = append(fields, protoFieldRaw{FieldNum: fn, WireType: 5, Bytes: b})
			pos += 4
		case 1: // fixed64
			if pos+8 > len(data) {
				return fields
			}
			b := make([]byte, 8)
			copy(b, data[pos:pos+8])
			fields = append(fields, protoFieldRaw{FieldNum: fn, WireType: 1, Bytes: b})
			pos += 8
		default:
			return fields
		}
	}
	return fields
}

func serializeProtobuf(fields []protoFieldRaw) []byte {
	var result []byte
	for _, f := range fields {
		tag := writeVarint((f.FieldNum << 3) | f.WireType)
		result = append(result, tag...)
		switch f.WireType {
		case 0:
			result = append(result, writeVarint(f.Varint)...)
		case 2:
			result = append(result, writeVarint(uint64(len(f.Bytes)))...)
			result = append(result, f.Bytes...)
		case 5:
			result = append(result, f.Bytes...)
		case 1:
			result = append(result, f.Bytes...)
		}
	}
	return result
}

// ── Connect envelope ──

type envelopeType int

const (
	envelopePlain       envelopeType = 0
	envelopeConnectRaw  envelopeType = 1
	envelopeConnectGzip envelopeType = 2
)

func decompressBody(body []byte) ([]byte, envelopeType) {
	if len(body) > 5 {
		flags := body[0]
		if flags == 0x00 || flags == 0x01 {
			payloadLen := binary.BigEndian.Uint32(body[1:5])
			diff := len(body) - 5 - int(payloadLen)
			if diff >= -10 && diff <= 10 {
				if flags&0x01 != 0 {
					reader, err := gzip.NewReader(bytes.NewReader(body[5:]))
					if err == nil {
						decompressed, err := io.ReadAll(reader)
						reader.Close()
						if err == nil {
							return decompressed, envelopeConnectGzip
						}
					}
				} else {
					return body[5 : 5+int(payloadLen)], envelopeConnectRaw
				}
			}
		}
	}
	return body, envelopePlain
}

func recompressBody(raw []byte, etype envelopeType) []byte {
	switch etype {
	case envelopeConnectGzip:
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, _ = w.Write(raw)
		w.Close()
		compressed := buf.Bytes()
		envelope := make([]byte, 5+len(compressed))
		envelope[0] = 0x01
		binary.BigEndian.PutUint32(envelope[1:5], uint32(len(compressed)))
		copy(envelope[5:], compressed)
		return envelope
	case envelopeConnectRaw:
		envelope := make([]byte, 5+len(raw))
		envelope[0] = 0x00
		binary.BigEndian.PutUint32(envelope[1:5], uint32(len(raw)))
		copy(envelope[5:], raw)
		return envelope
	default:
		return raw
	}
}

// ── Metadata field replacement ──

// ── Device fingerprint randomization ──
// Windsurf metadata (F1) contains device-identifying fields:
//   F5:  OS info JSON  {"Os":"windows","Arch":"amd64",...}
//   F8:  CPU info JSON {"NumSockets":1,"NumCores":8,...,"Memory":...}
//   F27: 64-char hex hash (machine fingerprint)
//   F32: UUID (installation/session ID)
// To prevent device-level rate limiting, we randomize these per-key.

// randomHexHash generates a random 64-char hex string.
func randomHexHash() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return []byte(hex.EncodeToString(b))
}

// randomUUID generates a random UUID v4 string.
func randomUUID() []byte {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return []byte(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}

// generateStableUUID returns a UUID v4 string (for per-key stable session ID).
func generateStableUUID() string {
	return string(randomUUID())
}

// generateStableHexHash returns a 64-char hex hash string (for per-key stable device fingerprint).
func generateStableHexHash() string {
	return string(randomHexHash())
}

// randomizeOSJSON slightly varies the OS info JSON to create a different fingerprint.
func randomizeOSJSON(original []byte) []byte {
	s := string(original)
	// Vary the Build number slightly
	if strings.Contains(s, "Build") {
		buildNum := 19041 + mathr.Intn(8000) // Windows build range
		s = regexp.MustCompile(`"Build":"\d+"`).ReplaceAllString(s, fmt.Sprintf(`"Build":"%d"`, buildNum))
		return []byte(s)
	}
	return original
}

// randomizeCPUJSON slightly varies the CPU info JSON.
func randomizeCPUJSON(original []byte) []byte {
	s := string(original)
	// Vary memory slightly (±10%)
	if strings.Contains(s, "Memory") {
		re := regexp.MustCompile(`"Memory":(\d+)`)
		matches := re.FindStringSubmatch(s)
		if len(matches) > 1 {
			var mem int64
			fmt.Sscanf(matches[1], "%d", &mem)
			variation := mem / 20 // ±5%
			newMem := mem - variation + int64(mathr.Intn(int(variation*2+1)))
			s = re.ReplaceAllString(s, fmt.Sprintf(`"Memory":%d`, newMem))
			return []byte(s)
		}
	}
	return original
}

// isDeviceFingerprintField returns true if this metadata field is a device identifier.
func isDeviceFingerprintField(f protoFieldRaw) bool {
	if f.WireType != 2 {
		return false
	}
	switch f.FieldNum {
	case 27: // 64-char hex hash
		return len(f.Bytes) == 64 && isHexString(f.Bytes)
	case 32: // UUID
		return len(f.Bytes) == 36 && bytes.Count(f.Bytes, []byte("-")) == 4
	}
	return false
}

func isHexString(b []byte) bool {
	for _, c := range b {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// KeyFingerprint holds per-key stable device/session fingerprint data.
// Used to replace F31/F32 in protobuf metadata so each key has a unique session.
type KeyFingerprint struct {
	SessionID  string // F32: stable UUID per key
	DeviceHash string // F31/F27: stable hex hash per key
}

// replaceMetadataFields parses the F1 metadata sub-message (~2500B) and replaces
// the api_key (any field starting with sk-ws-), JWT (F21 starting with eyJ),
// and device fingerprint fields (F5/F8/F27/F31/F32) for anti-device-tracking.
// When fp is non-nil, F32 and F31 are replaced with per-key stable values.
func replaceMetadataFields(metaBytes []byte, newKey []byte, newJWT []byte, randomizeFingerprint bool, fp *KeyFingerprint) ([]byte, bool) {
	fields := parseProtobuf(metaBytes)
	if len(fields) == 0 {
		return metaBytes, false
	}

	// ★ 从号池 JWT 解出 team_id / user_id，用于替换 F32 / F20
	// IDE 原始请求 body 里的 F20=UserID、F32=TeamID 属于 **计费字段**，
	// 只换 JWT 不换这两个 → 上游认证通过但账单仍记在原用户的 team 上
	// （Opus 等 premium 模型按 team_id 扣费时尤其明显）。
	var poolTeamID, poolUserID string
	if len(newJWT) > 0 {
		if claims, err := (&WindsurfService{}).DecodeJWTClaims(string(newJWT)); err == nil && claims != nil {
			poolTeamID = claims.TeamID
			poolUserID = claims.UserID
		}
	}

	modified := false
	var newFields []protoFieldRaw
	for _, f := range fields {
		if f.WireType == 2 {
			if bytes.HasPrefix(f.Bytes, apiKeyPrefix) {
				newFields = append(newFields, protoFieldRaw{FieldNum: f.FieldNum, WireType: 2, Bytes: newKey})
				modified = true
				continue
			}
			// ★ F3 = devin-session-token$<JWT>（IDE 登录后真实用这个做 auth）
			// 必须替换为号池 sk-ws-* key，否则 F3 仍是用户自己的会话 token，
			// 上游会按 F3 里的 session → 原用户账号去计费 Opus 等 premium 模型。
			if f.FieldNum == 3 && bytes.HasPrefix(f.Bytes, []byte("devin-session-token$")) {
				newFields = append(newFields, protoFieldRaw{FieldNum: 3, WireType: 2, Bytes: newKey})
				modified = true
				continue
			}
			if f.FieldNum == 21 && bytes.HasPrefix(f.Bytes, []byte("eyJ")) {
				if len(newJWT) > 0 {
					newFields = append(newFields, protoFieldRaw{FieldNum: 21, WireType: 2, Bytes: newJWT})
					modified = true
					continue
				}
				modified = true
				continue
			}
			// ★ F20: UserID（如 "user-XXX"）— 计费主体之一
			// IDE 原始请求里的 F20 是当前登录用户的 ID，必须换成号池用户的 ID；
			// 否则上游虽然认证为号池账号，但 billing 记到原用户头上。
			if f.FieldNum == 20 && len(f.Bytes) > 0 && bytes.HasPrefix(f.Bytes, []byte("user-")) {
				if poolUserID != "" {
					newFields = append(newFields, protoFieldRaw{FieldNum: 20, WireType: 2, Bytes: []byte(poolUserID)})
					modified = true
					continue
				}
			}
			// ★ F32: TeamID（如 "devin-team$account-XXX"）— 计费主体
			// chat_proto.go 文档确认 F32 = team_id。IDE 原始请求中此字段是用户自己的 team，
			// 必须换成号池 key 对应的 team，否则用户团队被扣费。
			// 注意：F32 同时在 Cascade 会话中参与 session 粘性校验；当我们为同一 convID
			// 保持相同 pool key（见 pickPoolKeyForSession）时，F32 也会稳定，不会触发
			// "Invalid Cascade session"。
			if f.FieldNum == 32 && len(f.Bytes) > 0 {
				// 只在目标字段看起来是 team/account 标识时替换（避免误伤）
				if bytes.HasPrefix(f.Bytes, []byte("devin-team$")) || bytes.HasPrefix(f.Bytes, []byte("account-")) {
					if poolTeamID != "" {
						newFields = append(newFields, protoFieldRaw{FieldNum: 32, WireType: 2, Bytes: []byte(poolTeamID)})
						modified = true
						continue
					}
				}
			}

			// ★ F31: 设备指纹 hex hash (长 hex 字符串) — 每个 key 用稳定 hash
			if f.FieldNum == 31 && len(f.Bytes) > 60 && isHexString(f.Bytes) {
				if fp != nil && fp.DeviceHash != "" {
					// 用 per-key hash 填充到原长度
					newHash := padHexHash(fp.DeviceHash, len(f.Bytes))
					newFields = append(newFields, protoFieldRaw{FieldNum: 31, WireType: 2, Bytes: []byte(newHash)})
					modified = true
					continue
				} else if randomizeFingerprint {
					newHash := padHexHash(string(randomHexHash()), len(f.Bytes))
					newFields = append(newFields, protoFieldRaw{FieldNum: 31, WireType: 2, Bytes: []byte(newHash)})
					modified = true
					continue
				}
			}
			// ★ F27: 64-char hex hash (旧版设备指纹) — 同样 per-key 稳定
			if f.FieldNum == 27 && len(f.Bytes) == 64 && isHexString(f.Bytes) {
				if fp != nil && fp.DeviceHash != "" {
					newFields = append(newFields, protoFieldRaw{FieldNum: 27, WireType: 2, Bytes: []byte(fp.DeviceHash)})
					modified = true
					continue
				} else if randomizeFingerprint {
					newFields = append(newFields, protoFieldRaw{FieldNum: 27, WireType: 2, Bytes: randomHexHash()})
					modified = true
					continue
				}
			}
			// ★ Device fingerprint randomization (仅对 Trial/Free 号生效)
			if randomizeFingerprint {
				if f.FieldNum == 5 && len(f.Bytes) > 10 && f.Bytes[0] == '{' {
					newFields = append(newFields, protoFieldRaw{FieldNum: 5, WireType: 2, Bytes: randomizeOSJSON(f.Bytes)})
					modified = true
					continue
				}
				if f.FieldNum == 8 && len(f.Bytes) > 10 && f.Bytes[0] == '{' {
					newFields = append(newFields, protoFieldRaw{FieldNum: 8, WireType: 2, Bytes: randomizeCPUJSON(f.Bytes)})
					modified = true
					continue
				}
			}
		}
		newFields = append(newFields, f)
	}

	if modified {
		return serializeProtobuf(newFields), true
	}
	return metaBytes, false
}

// padHexHash repeats/truncates a 64-char hex hash to fill targetLen bytes.
func padHexHash(hash string, targetLen int) string {
	if len(hash) == 0 {
		hash = string(randomHexHash())
	}
	for len(hash) < targetLen {
		hash += hash
	}
	return hash[:targetLen]
}

// injectKeyIntoMetadata injects pool key when body has no api_key.
func injectKeyIntoMetadata(metaBytes []byte, newKey []byte, newJWT []byte) ([]byte, bool) {
	fields := parseProtobuf(metaBytes)
	if len(fields) == 0 {
		return metaBytes, false
	}

	// Check if already has api_key
	for _, f := range fields {
		if f.FieldNum == 3 && f.WireType == 2 && bytes.HasPrefix(f.Bytes, apiKeyPrefix) {
			return metaBytes, false
		}
	}

	// Inject api_key after F2 (ide_version)
	var newFields []protoFieldRaw
	for _, f := range fields {
		newFields = append(newFields, f)
		if f.FieldNum == 2 && f.WireType == 2 {
			newFields = append(newFields, protoFieldRaw{FieldNum: 3, WireType: 2, Bytes: newKey})
		}
	}

	// Inject JWT if missing
	if len(newJWT) > 0 {
		hasJWT := false
		for _, f := range fields {
			if f.FieldNum == 21 && f.WireType == 2 {
				hasJWT = true
				break
			}
		}
		if !hasJWT {
			newFields = append(newFields, protoFieldRaw{FieldNum: 21, WireType: 2, Bytes: newJWT})
		}
	}

	return serializeProtobuf(newFields), true
}

// ReplaceIdentity replaces api_key and JWT in protobuf body.
// Safe approach: only touches F1 metadata sub-message, leaves all other fields intact.
// Ported from interceptor_poc.py replace_identity().
// opts: [0]=randomizeFingerprint(bool), [1]=*KeyFingerprint
func ReplaceIdentity(data []byte, newKey []byte, newJWT []byte, opts ...interface{}) ([]byte, bool) {
	var randFP bool
	var fp *KeyFingerprint
	for _, o := range opts {
		switch v := o.(type) {
		case bool:
			randFP = v
		case *KeyFingerprint:
			fp = v
		}
	}
	// Scan top-level fields, locate F1 (metadata) byte ranges
	type f1pos struct {
		tagStart     int
		contentStart int
		contentEnd   int
		mode         string // "replace" or "inject"
	}
	var positions []f1pos

	pos := 0
	for pos < len(data) {
		tagStart := pos
		tag, newPos := readVarint(data, pos)
		if newPos == pos {
			break
		}
		pos = newPos
		if tag == 0 {
			break
		}
		fn := tag >> 3
		wt := tag & 7

		switch wt {
		case 0:
			_, newPos = readVarint(data, pos)
			pos = newPos
		case 2:
			length, newPos := readVarint(data, pos)
			pos = newPos
			contentStart := pos
			contentEnd := pos + int(length)
			if contentEnd > len(data) {
				return data, false
			}

			if fn == 1 && int(length) > 20 {
				content := data[contentStart:contentEnd]
				// ★ 只要 F1 metadata 内有 JWT(eyJ) 或 sk-ws 或 devin-session-token，就走 replace 路径。
				// 原先要求顶层 hasKey（sk-ws-*）才走 replace，但 IDE 正常登录发的请求只带 JWT
				// 和 devin-session-token，没有 sk-ws-*，导致走 inject 分支只补了 sk-ws，却没
				// 替换 F20(UserID)/F32(TeamID) 等计费字段 → 账单仍记在登录用户头上。
				hasAuthField := bytes.Contains(content, apiKeyPrefix) ||
					bytes.Contains(content, []byte("eyJ")) ||
					bytes.Contains(content, []byte("devin-session-token"))
				if hasAuthField {
					positions = append(positions, f1pos{tagStart, contentStart, contentEnd, "replace"})
				} else {
					// Try to detect metadata structure (F1=ide_name, F2=ide_version)
					sub := parseProtobuf(content)
					hasF1 := false
					hasF2 := false
					for _, sf := range sub {
						if sf.FieldNum == 1 && sf.WireType == 2 {
							hasF1 = true
						}
						if sf.FieldNum == 2 && sf.WireType == 2 {
							hasF2 = true
						}
					}
					if hasF1 && hasF2 {
						positions = append(positions, f1pos{tagStart, contentStart, contentEnd, "inject"})
					}
				}
			}
			pos = contentEnd
		case 5:
			pos += 4
		case 1:
			pos += 8
		default:
			return data, false
		}
	}

	if len(positions) == 0 {
		return data, false
	}

	// Process from back to front (avoid offset changes)
	result := make([]byte, len(data))
	copy(result, data)
	modified := false

	for i := len(positions) - 1; i >= 0; i-- {
		p := positions[i]
		oldContent := result[p.contentStart:p.contentEnd]

		var newContent []byte
		var changed bool
		if p.mode == "replace" {
			newContent, changed = replaceMetadataFields(oldContent, newKey, newJWT, randFP, fp)
		} else {
			newContent, changed = injectKeyIntoMetadata(oldContent, newKey, newJWT)
		}

		if changed {
			// Rebuild: [before tagStart] + tag + new_length + new_content + [after contentEnd]
			_, afterTag := readVarint(result, p.tagStart)
			tagBytes := result[p.tagStart:afterTag]
			newLenVarint := writeVarint(uint64(len(newContent)))

			rebuilt := make([]byte, 0, p.tagStart+len(tagBytes)+len(newLenVarint)+len(newContent)+len(result)-p.contentEnd)
			rebuilt = append(rebuilt, result[:p.tagStart]...)
			rebuilt = append(rebuilt, tagBytes...)
			rebuilt = append(rebuilt, newLenVarint...)
			rebuilt = append(rebuilt, newContent...)
			rebuilt = append(rebuilt, result[p.contentEnd:]...)
			result = rebuilt
			modified = true
		}
	}

	return result, modified
}

// ReplaceIdentityInBody handles the full flow: decompress → replace → recompress.
// opts: bool (randomizeFingerprint), *KeyFingerprint (per-key session/device data)
func ReplaceIdentityInBody(body []byte, newKey []byte, newJWT []byte, opts ...interface{}) ([]byte, bool) {
	raw, etype := decompressBody(body)
	newRaw, replaced := ReplaceIdentity(raw, newKey, newJWT, opts...)
	if !replaced {
		return body, false
	}
	return recompressBody(newRaw, etype), true
}

// StripFieldsFromBody removes specific top-level fields from a protobuf request body
// while preserving the envelope format.
func StripFieldsFromBody(body []byte, targetFields ...uint64) ([]byte, bool) {
	if len(targetFields) == 0 {
		return body, false
	}

	targets := make(map[uint64]bool)
	for _, f := range targetFields {
		targets[f] = true
	}

	raw, etype := decompressBody(body)

	pos := 0
	type chunk struct {
		start, end int
		keep       bool
	}
	var chunks []chunk
	lastKeepStart := 0
	removed := false

	for pos < len(raw) {
		tagStart := pos
		tag, newPos := readVarint(raw, pos)
		if newPos == pos || tag == 0 {
			break
		}

		fn := tag >> 3
		wt := tag & 7
		fieldEnd := -1

		switch wt {
		case 0:
			_, newPos = readVarint(raw, newPos)
			fieldEnd = newPos
		case 2:
			length, newPos := readVarint(raw, newPos)
			fieldEnd = newPos + int(length)
			if fieldEnd > len(raw) {
				fieldEnd = len(raw)
			}
		case 5:
			fieldEnd = newPos + 4
			if fieldEnd > len(raw) {
				fieldEnd = len(raw)
			}
		case 1:
			fieldEnd = newPos + 8
			if fieldEnd > len(raw) {
				fieldEnd = len(raw)
			}
		default:
			fieldEnd = len(raw)
		}

		if targets[fn] {
			if lastKeepStart < tagStart {
				chunks = append(chunks, chunk{lastKeepStart, tagStart, true})
			}
			removed = true
			lastKeepStart = fieldEnd
		}

		pos = fieldEnd
		if wt != 0 && wt != 1 && wt != 2 && wt != 5 {
			break
		}
	}

	if !removed {
		return body, false
	}

	if lastKeepStart < len(raw) {
		chunks = append(chunks, chunk{lastKeepStart, len(raw), true})
	}

	var newRaw []byte
	for _, c := range chunks {
		if c.keep {
			newRaw = append(newRaw, raw[c.start:c.end]...)
		}
	}

	return recompressBody(newRaw, etype), true
}

// StripConversationIDFromBody removes the top-level conversation_id (field 2)
// and parent_message_id (field 3) from a chat request body while preserving the envelope format.
// It uses a safe splice approach without parsing the entire body to avoid truncation.
func StripConversationIDFromBody(body []byte) ([]byte, bool) {
	return StripFieldsFromBody(body, 2, 3)
}

// ExtractJWTFromBody extracts a JWT string from a protobuf response body.
func ExtractJWTFromBody(body []byte) string {
	raw := body
	// Try unwrap Connect envelope
	if len(raw) > 5 {
		flags := raw[0]
		if flags == 0x00 || flags == 0x01 {
			plen := binary.BigEndian.Uint32(raw[1:5])
			end := 5 + int(plen)
			if len(raw) >= end {
				if flags&0x01 != 0 {
					reader, err := gzip.NewReader(bytes.NewReader(raw[5:]))
					if err == nil {
						decompressed, err := io.ReadAll(reader)
						reader.Close()
						if err == nil {
							raw = decompressed
						}
					}
				} else {
					raw = raw[5:end]
				}
			}
		}
	}

	jwt, found := utils.FindJWTInProtobuf(raw)
	if found {
		return jwt
	}
	return ""
}
