package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"windsurf-tools-wails/backend/utils"
)

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
