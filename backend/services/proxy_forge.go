package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"time"
)

// ForgeConfig controls GetUserStatus / GetPlanStatus response forging.
type ForgeConfig struct {
	Enabled            bool
	FakeCredits        int
	FakeCreditsPremium int
	FakeCreditsOther   int
	FakeCreditsUsed    int
	FakeSubType        string // e.g. "Enterprise"
	ExtendYears        int
}

// DefaultForgeConfig returns the default forge configuration.
func DefaultForgeConfig() ForgeConfig {
	return ForgeConfig{
		Enabled:            true,
		FakeCredits:        10000000,
		FakeCreditsPremium: 150000,
		FakeCreditsOther:   25000,
		FakeCreditsUsed:    0,
		FakeSubType:        "Enterprise",
		ExtendYears:        10,
	}
}

// ── Protobuf field helpers (operate on parsed []protoFieldRaw from proxy_identity.go) ──

func setVarint(fields []protoFieldRaw, fieldNum uint64, value uint64) []protoFieldRaw {
	for i := range fields {
		if fields[i].FieldNum == fieldNum && fields[i].WireType == 0 {
			fields[i].Varint = value
			return fields
		}
	}
	return append(fields, protoFieldRaw{FieldNum: fieldNum, WireType: 0, Varint: value})
}

func setString(fields []protoFieldRaw, fieldNum uint64, value string) []protoFieldRaw {
	b := []byte(value)
	for i := range fields {
		if fields[i].FieldNum == fieldNum && fields[i].WireType == 2 {
			fields[i].Bytes = b
			return fields
		}
	}
	return append(fields, protoFieldRaw{FieldNum: fieldNum, WireType: 2, Bytes: b})
}

func setBytes(fields []protoFieldRaw, fieldNum uint64, value []byte) []protoFieldRaw {
	for i := range fields {
		if fields[i].FieldNum == fieldNum && fields[i].WireType == 2 {
			fields[i].Bytes = value
			return fields
		}
	}
	return append(fields, protoFieldRaw{FieldNum: fieldNum, WireType: 2, Bytes: value})
}

func stripFields(fields []protoFieldRaw, nums ...uint64) []protoFieldRaw {
	remove := make(map[uint64]bool, len(nums))
	for _, n := range nums {
		remove[n] = true
	}
	out := make([]protoFieldRaw, 0, len(fields))
	for _, f := range fields {
		if !remove[f.FieldNum] {
			out = append(out, f)
		}
	}
	return out
}

func modifyNested(fields []protoFieldRaw, fieldNum uint64, fn func([]byte) []byte) []protoFieldRaw {
	for i := range fields {
		if fields[i].FieldNum == fieldNum && fields[i].WireType == 2 {
			fields[i].Bytes = fn(fields[i].Bytes)
			return fields
		}
	}
	return fields
}

// buildTimestampMsg builds a Google Timestamp protobuf message (F1=seconds).
func buildTimestampMsg(seconds int64) []byte {
	return serializeProtobuf([]protoFieldRaw{
		{FieldNum: 1, WireType: 0, Varint: uint64(seconds)},
	})
}

// ── gRPC-Web frame helpers ──

func isGRPCWebFrame(body []byte) bool {
	if len(body) < 5 {
		return false
	}
	flag := body[0]
	if flag != 0x00 && flag != 0x80 {
		return false
	}
	payloadLen := binary.BigEndian.Uint32(body[1:5])
	return int(payloadLen)+5 <= len(body)
}

func buildGRPCFrame(flag byte, payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = flag
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

// ── gzip helpers ──

func gzipDecompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── Format-compatible forging dispatch ──

// forgeGRPCWebFramed processes gRPC-Web multi-frame response bodies.
// Only data frames (flag=0x00) are forged; trailer frames are preserved.
func forgeGRPCWebFramed(body []byte, cfg ForgeConfig, forgeFn func([]byte, ForgeConfig) []byte) []byte {
	var result []byte
	pos := 0
	modified := false
	for pos+5 <= len(body) {
		flag := body[pos]
		frameLen := int(binary.BigEndian.Uint32(body[pos+1 : pos+5]))
		frameEnd := pos + 5 + frameLen
		if frameEnd > len(body) {
			result = append(result, body[pos:]...)
			break
		}
		payload := body[pos+5 : frameEnd]
		if flag == 0x00 && len(payload) > 0 {
			forged := forgeFn(payload, cfg)
			result = append(result, buildGRPCFrame(0x00, forged)...)
			modified = true
		} else {
			result = append(result, body[pos:frameEnd]...)
		}
		pos = frameEnd
	}
	if pos < len(body) {
		result = append(result, body[pos:]...)
	}
	if !modified {
		return body
	}
	return result
}

// forgeUserStatusResponse auto-detects response format and applies GetUserStatus forging.
// Handles: gRPC-Web frames, raw gzip, Connect envelope, raw protobuf.
func forgeUserStatusResponse(body []byte, cfg ForgeConfig) []byte {
	if len(body) == 0 {
		return body
	}
	if isGRPCWebFrame(body) {
		return forgeGRPCWebFramed(body, cfg, forgeUserStatus)
	}
	if len(body) >= 3 && body[0] == 0x1f && body[1] == 0x8b && body[2] == 0x08 {
		decompressed, err := gzipDecompress(body)
		if err == nil && len(decompressed) > 0 {
			forged := forgeUserStatus(decompressed, cfg)
			if recompressed, cerr := gzipCompress(forged); cerr == nil {
				return recompressed
			}
			return forged
		}
		return body
	}
	raw, etype := decompressBody(body)
	if etype != envelopePlain {
		return recompressBody(forgeUserStatus(raw, cfg), etype)
	}
	return forgeUserStatus(body, cfg)
}

// forgePlanStatusResponse auto-detects response format and applies GetPlanStatus forging.
func forgePlanStatusResponse(body []byte, cfg ForgeConfig) []byte {
	if len(body) == 0 {
		return body
	}
	if isGRPCWebFrame(body) {
		return forgeGRPCWebFramed(body, cfg, forgePlanStatus)
	}
	if len(body) >= 3 && body[0] == 0x1f && body[1] == 0x8b && body[2] == 0x08 {
		decompressed, err := gzipDecompress(body)
		if err == nil && len(decompressed) > 0 {
			forged := forgePlanStatus(decompressed, cfg)
			if recompressed, cerr := gzipCompress(forged); cerr == nil {
				return recompressed
			}
			return forged
		}
		return body
	}
	raw, etype := decompressBody(body)
	if etype != envelopePlain {
		return recompressBody(forgePlanStatus(raw, cfg), etype)
	}
	return forgePlanStatus(body, cfg)
}

// ── Core forge logic ──

// forgeUserStatus modifies a GetUserStatusResponse protobuf:
// F1(UserStatus) + F2(PlanInfo).
func forgeUserStatus(msg []byte, cfg ForgeConfig) []byte {
	fields := parseProtobuf(msg)
	if len(fields) == 0 {
		return msg
	}
	fields = modifyNested(fields, 1, func(data []byte) []byte {
		return forgeUserStatusInner(data, cfg)
	})
	fields = modifyNested(fields, 2, func(data []byte) []byte {
		return forgePlanInfo(data, cfg)
	})
	return serializeProtobuf(fields)
}

// forgeUserStatusInner modifies UserStatus sub-message:
// F4=1 ignore_telemetry, F6=2 team_status(APPROVED), F10=2 teams_tier(PRO),
// F11=31-byte permission bitmap, F13→PlanStatus, F28=usedCredits, strip F34.
func forgeUserStatusInner(data []byte, cfg ForgeConfig) []byte {
	fields := parseProtobuf(data)
	if len(fields) == 0 {
		return data
	}
	fields = setVarint(fields, 4, 1)
	fields = setVarint(fields, 6, 2)
	fields = setVarint(fields, 10, 2)

	perms := make([]byte, 31)
	for i := range perms {
		perms[i] = 0xFF
	}
	fields = setBytes(fields, 11, perms)

	fields = setVarint(fields, 28, uint64(cfg.FakeCreditsUsed))
	fields = stripFields(fields, 34)
	fields = modifyNested(fields, 13, func(ps []byte) []byte {
		return forgePlanStatus(ps, cfg)
	})
	return serializeProtobuf(fields)
}

// forgePlanStatus modifies PlanStatus sub-message:
// F1→PlanInfo (nested), F2/F3=Timestamp (extended billing period),
// F6=0, F8=credits, F9=premiumCredits.
func forgePlanStatus(data []byte, cfg ForgeConfig) []byte {
	fields := parseProtobuf(data)
	if len(fields) == 0 {
		return data
	}
	now := time.Now()
	startTs := now.AddDate(-1, 0, 0).Unix()
	endTs := now.AddDate(cfg.ExtendYears, 0, 0).Unix()

	fields = modifyNested(fields, 1, func(piData []byte) []byte {
		return forgePlanInfo(piData, cfg)
	})
	fields = setBytes(fields, 2, buildTimestampMsg(startTs))
	fields = setBytes(fields, 3, buildTimestampMsg(endTs))
	fields = setVarint(fields, 6, 0)
	fields = setVarint(fields, 8, uint64(cfg.FakeCredits))
	fields = setVarint(fields, 9, uint64(cfg.FakeCreditsPremium))
	return serializeProtobuf(fields)
}

// forgePlanInfo modifies PlanInfo sub-message:
// F1=2(PRO), F2=subType, F3/F4=1, strip F5, F7=16384, F8=600,
// F9/F10=-1(unlimited), F12-14=credits, F15/F18/F19/F20=1.
func forgePlanInfo(data []byte, cfg ForgeConfig) []byte {
	fields := parseProtobuf(data)
	if len(fields) == 0 {
		return data
	}
	fields = setVarint(fields, 1, 2)
	fields = setString(fields, 2, cfg.FakeSubType)
	fields = setVarint(fields, 3, 1)
	fields = setVarint(fields, 4, 1)
	fields = stripFields(fields, 5)
	fields = setVarint(fields, 7, 16384)
	fields = setVarint(fields, 8, 600)
	fields = setVarint(fields, 9, ^uint64(0))
	fields = setVarint(fields, 10, ^uint64(0))
	fields = setVarint(fields, 12, uint64(cfg.FakeCredits))
	fields = setVarint(fields, 13, uint64(cfg.FakeCreditsPremium))
	fields = setVarint(fields, 14, uint64(cfg.FakeCreditsOther))
	fields = setVarint(fields, 15, 1)
	fields = setVarint(fields, 18, 1)
	fields = setVarint(fields, 19, 1)
	fields = setVarint(fields, 20, 1)
	return serializeProtobuf(fields)
}

// ── MitmProxy forge configuration ──

// SetForgeConfig updates the forge configuration (thread-safe).
func (p *MitmProxy) SetForgeConfig(cfg ForgeConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forgeConfig = cfg
}

// GetForgeConfig returns the current forge configuration (thread-safe).
func (p *MitmProxy) GetForgeConfig() ForgeConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.forgeConfig
}
