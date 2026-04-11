package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
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

// replaceMetadataFields parses the F1 metadata sub-message (~2500B) and replaces
// the api_key (any field starting with sk-ws-) and JWT (F21 starting with eyJ).
func replaceMetadataFields(metaBytes []byte, newKey []byte, newJWT []byte) ([]byte, bool) {
	fields := parseProtobuf(metaBytes)
	if len(fields) == 0 {
		return metaBytes, false
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
			if f.FieldNum == 21 && bytes.HasPrefix(f.Bytes, []byte("eyJ")) {
				if len(newJWT) > 0 {
					newFields = append(newFields, protoFieldRaw{FieldNum: 21, WireType: 2, Bytes: newJWT})
					modified = true
					continue
				}
				// strip JWT if no replacement available
				modified = true
				continue
			}
		}
		newFields = append(newFields, f)
	}

	if modified {
		return serializeProtobuf(newFields), true
	}
	return metaBytes, false
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
func ReplaceIdentity(data []byte, newKey []byte, newJWT []byte) ([]byte, bool) {
	hasKey := bytes.Contains(data, apiKeyPrefix)

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
				if hasKey && (bytes.Contains(content, apiKeyPrefix) || bytes.Contains(content, []byte("eyJ"))) {
					positions = append(positions, f1pos{tagStart, contentStart, contentEnd, "replace"})
				} else if !hasKey {
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
			newContent, changed = replaceMetadataFields(oldContent, newKey, newJWT)
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
func ReplaceIdentityInBody(body []byte, newKey []byte, newJWT []byte) ([]byte, bool) {
	raw, etype := decompressBody(body)
	newRaw, replaced := ReplaceIdentity(raw, newKey, newJWT)
	if !replaced {
		return body, false
	}
	return recompressBody(newRaw, etype), true
}

// StripConversationIDFromBody removes the top-level conversation_id (field 2)
// and parent_message_id (field 3) from a chat request body while preserving the envelope format.
// It uses a safe splice approach without parsing the entire body to avoid truncation.
func StripConversationIDFromBody(body []byte) ([]byte, bool) {
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

		// Field 2: conversation_id (string)
		// Field 3: parent_message_id (string)
		if (fn == 2 || fn == 3) && wt == 2 {
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
