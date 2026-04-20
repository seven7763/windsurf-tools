package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"testing"
)

func testForgeConfig() ForgeConfig {
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

func buildTestProto(fields []protoFieldRaw) []byte {
	return serializeProtobuf(fields)
}

func TestSetVarint(t *testing.T) {
	fields := []protoFieldRaw{{FieldNum: 1, WireType: 0, Varint: 42}}
	fields = setVarint(fields, 1, 99)
	if fields[0].Varint != 99 {
		t.Fatalf("expected 99, got %d", fields[0].Varint)
	}
	fields = setVarint(fields, 5, 123)
	if len(fields) != 2 || fields[1].FieldNum != 5 || fields[1].Varint != 123 {
		t.Fatalf("expected new field 5=123, got %+v", fields)
	}
}

func TestSetString(t *testing.T) {
	fields := []protoFieldRaw{{FieldNum: 2, WireType: 2, Bytes: []byte("old")}}
	fields = setString(fields, 2, "new")
	if string(fields[0].Bytes) != "new" {
		t.Fatalf("expected 'new', got %q", fields[0].Bytes)
	}
	fields = setString(fields, 7, "added")
	if len(fields) != 2 || string(fields[1].Bytes) != "added" {
		t.Fatalf("expected new field 7='added', got %+v", fields)
	}
}

func TestSetBytes(t *testing.T) {
	fields := []protoFieldRaw{{FieldNum: 3, WireType: 2, Bytes: []byte{1, 2, 3}}}
	fields = setBytes(fields, 3, []byte{4, 5, 6, 7})
	if !bytes.Equal(fields[0].Bytes, []byte{4, 5, 6, 7}) {
		t.Fatalf("expected [4,5,6,7], got %v", fields[0].Bytes)
	}
}

func TestStripFields(t *testing.T) {
	fields := []protoFieldRaw{
		{FieldNum: 1, WireType: 0, Varint: 1},
		{FieldNum: 2, WireType: 2, Bytes: []byte("keep")},
		{FieldNum: 34, WireType: 0, Varint: 999},
		{FieldNum: 5, WireType: 0, Varint: 5},
	}
	result := stripFields(fields, 34, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(result))
	}
	if result[0].FieldNum != 1 || result[1].FieldNum != 2 {
		t.Fatalf("unexpected fields: %+v", result)
	}
}

func TestModifyNested(t *testing.T) {
	inner := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 0, Varint: 10},
	})
	fields := []protoFieldRaw{{FieldNum: 1, WireType: 2, Bytes: inner}}
	fields = modifyNested(fields, 1, func(data []byte) []byte {
		parsed := parseProtobuf(data)
		parsed = setVarint(parsed, 1, 42)
		return serializeProtobuf(parsed)
	})
	parsed := parseProtobuf(fields[0].Bytes)
	if len(parsed) == 0 || parsed[0].Varint != 42 {
		t.Fatalf("expected nested varint=42, got %+v", parsed)
	}
}

func TestModifyNestedMissing(t *testing.T) {
	fields := []protoFieldRaw{{FieldNum: 5, WireType: 0, Varint: 1}}
	original := len(fields)
	fields = modifyNested(fields, 99, func(data []byte) []byte {
		return []byte("should not be called")
	})
	if len(fields) != original {
		t.Fatal("modifyNested should not add missing fields")
	}
}

func TestBuildTimestampMsg(t *testing.T) {
	ts := buildTimestampMsg(1700000000)
	fields := parseProtobuf(ts)
	if len(fields) != 1 || fields[0].FieldNum != 1 || fields[0].Varint != 1700000000 {
		t.Fatalf("unexpected timestamp proto: %+v", fields)
	}
}

func TestForgePlanInfo(t *testing.T) {
	original := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 0, Varint: 0},
		{FieldNum: 2, WireType: 2, Bytes: []byte("Free")},
		{FieldNum: 5, WireType: 0, Varint: 1},
	})
	cfg := testForgeConfig()
	forged := forgePlanInfo(original, cfg)
	fields := parseProtobuf(forged)
	fieldMap := make(map[uint64]protoFieldRaw)
	for _, f := range fields {
		fieldMap[f.FieldNum] = f
	}

	if f, ok := fieldMap[1]; !ok || f.Varint != 2 {
		t.Errorf("F1 should be 2 (PRO), got %+v", fieldMap[1])
	}
	if f, ok := fieldMap[2]; !ok || string(f.Bytes) != "Enterprise" {
		t.Errorf("F2 should be 'Enterprise', got %+v", fieldMap[2])
	}
	if _, ok := fieldMap[5]; ok {
		t.Error("F5 should be stripped")
	}
	if f, ok := fieldMap[9]; !ok || f.Varint != ^uint64(0) {
		t.Errorf("F9 should be -1 (unlimited), got %+v", fieldMap[9])
	}
	if f, ok := fieldMap[12]; !ok || f.Varint != uint64(cfg.FakeCredits) {
		t.Errorf("F12 should be %d, got %+v", cfg.FakeCredits, fieldMap[12])
	}
}

func TestForgePlanStatus(t *testing.T) {
	original := buildTestProto([]protoFieldRaw{
		{FieldNum: 2, WireType: 2, Bytes: buildTimestampMsg(1000)},
		{FieldNum: 3, WireType: 2, Bytes: buildTimestampMsg(2000)},
		{FieldNum: 6, WireType: 0, Varint: 5},
		{FieldNum: 8, WireType: 0, Varint: 100},
	})
	cfg := testForgeConfig()
	forged := forgePlanStatus(original, cfg)
	fields := parseProtobuf(forged)
	fieldMap := make(map[uint64]protoFieldRaw)
	for _, f := range fields {
		fieldMap[f.FieldNum] = f
	}
	if f, ok := fieldMap[6]; !ok || f.Varint != 0 {
		t.Errorf("F6 should be 0, got %+v", fieldMap[6])
	}
	if f, ok := fieldMap[8]; !ok || f.Varint != uint64(cfg.FakeCredits) {
		t.Errorf("F8 should be %d, got %+v", cfg.FakeCredits, fieldMap[8])
	}
	if f, ok := fieldMap[9]; !ok || f.Varint != uint64(cfg.FakeCreditsPremium) {
		t.Errorf("F9 should be %d, got %+v", cfg.FakeCreditsPremium, fieldMap[9])
	}
	if f2, ok := fieldMap[2]; !ok {
		t.Error("F2 timestamp should exist")
	} else {
		inner := parseProtobuf(f2.Bytes)
		if len(inner) == 0 || inner[0].Varint < 1700000000 {
			t.Errorf("F2 timestamp should be recent, got %+v", inner)
		}
	}
}

func TestForgeUserStatusInner(t *testing.T) {
	planStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 6, WireType: 0, Varint: 3},
		{FieldNum: 8, WireType: 0, Varint: 50},
	})
	original := buildTestProto([]protoFieldRaw{
		{FieldNum: 4, WireType: 0, Varint: 0},
		{FieldNum: 13, WireType: 2, Bytes: planStatus},
		{FieldNum: 34, WireType: 0, Varint: 12345},
	})
	cfg := testForgeConfig()
	forged := forgeUserStatusInner(original, cfg)
	fields := parseProtobuf(forged)
	fieldMap := make(map[uint64]protoFieldRaw)
	for _, f := range fields {
		fieldMap[f.FieldNum] = f
	}
	if f, ok := fieldMap[4]; !ok || f.Varint != 1 {
		t.Errorf("F4 should be 1, got %+v", fieldMap[4])
	}
	if f, ok := fieldMap[6]; !ok || f.Varint != 2 {
		t.Errorf("F6 should be 2, got %+v", fieldMap[6])
	}
	if f, ok := fieldMap[10]; !ok || f.Varint != 2 {
		t.Errorf("F10 should be 2, got %+v", fieldMap[10])
	}
	if f, ok := fieldMap[11]; !ok || len(f.Bytes) != 31 {
		t.Errorf("F11 should be 31 bytes, got %+v", fieldMap[11])
	}
	if _, ok := fieldMap[34]; ok {
		t.Error("F34 should be stripped")
	}
	if f, ok := fieldMap[28]; !ok || f.Varint != 0 {
		t.Errorf("F28 should be 0, got %+v", fieldMap[28])
	}
}

func TestForgeUserStatus(t *testing.T) {
	planStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 6, WireType: 0, Varint: 1},
	})
	userStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 4, WireType: 0, Varint: 0},
		{FieldNum: 13, WireType: 2, Bytes: planStatus},
	})
	planInfo := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 0, Varint: 0},
		{FieldNum: 2, WireType: 2, Bytes: []byte("Free")},
	})
	original := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 2, Bytes: userStatus},
		{FieldNum: 2, WireType: 2, Bytes: planInfo},
	})
	cfg := testForgeConfig()
	forged := forgeUserStatus(original, cfg)
	if bytes.Equal(original, forged) {
		t.Fatal("forged should differ from original")
	}
	top := parseProtobuf(forged)
	for _, f := range top {
		if f.FieldNum == 2 && f.WireType == 2 {
			inner := parseProtobuf(f.Bytes)
			for _, sf := range inner {
				if sf.FieldNum == 2 && sf.WireType == 2 {
					if string(sf.Bytes) != "Enterprise" {
						t.Errorf("PlanInfo F2 should be 'Enterprise', got %q", sf.Bytes)
					}
				}
			}
		}
	}
}

func TestForgeUserStatusResponse_RawProtobuf(t *testing.T) {
	userStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 4, WireType: 0, Varint: 0},
	})
	original := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 2, Bytes: userStatus},
	})
	cfg := testForgeConfig()
	forged := forgeUserStatusResponse(original, cfg)
	if len(forged) == 0 {
		t.Fatal("forged should not be empty")
	}
	top := parseProtobuf(forged)
	if len(top) == 0 {
		t.Fatal("forged should be valid protobuf")
	}
}

func TestForgeUserStatusResponse_GRPCWebFrame(t *testing.T) {
	userStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 4, WireType: 0, Varint: 0},
	})
	msg := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 2, Bytes: userStatus},
	})
	frame := buildGRPCFrame(0x00, msg)
	trailer := buildGRPCFrame(0x80, []byte("trailer"))
	body := append(frame, trailer...)

	cfg := testForgeConfig()
	forged := forgeUserStatusResponse(body, cfg)
	if !isGRPCWebFrame(forged) {
		t.Fatal("forged should still be gRPC-Web format")
	}
	if bytes.Equal(body, forged) {
		t.Fatal("forged should differ from original")
	}
}

func TestForgeUserStatusResponse_Gzip(t *testing.T) {
	userStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 4, WireType: 0, Varint: 0},
	})
	msg := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 2, Bytes: userStatus},
	})
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(msg)
	w.Close()
	gzipped := buf.Bytes()

	cfg := testForgeConfig()
	forged := forgeUserStatusResponse(gzipped, cfg)
	if len(forged) == 0 {
		t.Fatal("forged should not be empty")
	}
	if forged[0] != 0x1f || forged[1] != 0x8b {
		t.Fatal("forged should be gzip-compressed")
	}
	decompressed, err := gzipDecompress(forged)
	if err != nil {
		t.Fatalf("failed to decompress forged gzip: %v", err)
	}
	top := parseProtobuf(decompressed)
	if len(top) == 0 {
		t.Fatal("decompressed forged should be valid protobuf")
	}
}

func TestForgeUserStatusResponse_ConnectEnvelope(t *testing.T) {
	userStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 4, WireType: 0, Varint: 0},
	})
	msg := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 2, Bytes: userStatus},
	})
	envelope := make([]byte, 5+len(msg))
	envelope[0] = 0x00
	binary.BigEndian.PutUint32(envelope[1:5], uint32(len(msg)))
	copy(envelope[5:], msg)

	cfg := testForgeConfig()
	forged := forgeUserStatusResponse(envelope, cfg)
	if len(forged) < 5 {
		t.Fatal("forged envelope too short")
	}
}

func TestForgePlanStatusResponse(t *testing.T) {
	original := buildTestProto([]protoFieldRaw{
		{FieldNum: 6, WireType: 0, Varint: 3},
		{FieldNum: 8, WireType: 0, Varint: 100},
	})
	cfg := testForgeConfig()
	forged := forgePlanStatusResponse(original, cfg)
	if bytes.Equal(original, forged) {
		t.Fatal("forged should differ from original")
	}
	fields := parseProtobuf(forged)
	for _, f := range fields {
		if f.FieldNum == 8 && f.Varint != uint64(cfg.FakeCredits) {
			t.Errorf("F8 should be %d, got %d", cfg.FakeCredits, f.Varint)
		}
	}
}

func TestForgeEmptyBody(t *testing.T) {
	cfg := testForgeConfig()
	if result := forgeUserStatusResponse(nil, cfg); result != nil {
		t.Error("nil body should return nil")
	}
	if result := forgeUserStatusResponse([]byte{}, cfg); len(result) != 0 {
		t.Error("empty body should return empty")
	}
	if result := forgePlanStatusResponse(nil, cfg); result != nil {
		t.Error("nil body should return nil")
	}
}

func TestIsGRPCWebFrame(t *testing.T) {
	tests := []struct {
		name   string
		body   []byte
		expect bool
	}{
		{"nil", nil, false},
		{"short", []byte{0, 0}, false},
		{"data frame", buildGRPCFrame(0x00, []byte("test")), true},
		{"trailer frame", buildGRPCFrame(0x80, []byte("t")), true},
		{"invalid flag", buildGRPCFrame(0x42, []byte("test")), false},
		{"truncated", []byte{0x00, 0x00, 0x00, 0x00, 0xFF}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isGRPCWebFrame(tt.body); got != tt.expect {
				t.Errorf("isGRPCWebFrame = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestGzipRoundTrip(t *testing.T) {
	original := []byte("hello world protobuf data test")
	compressed, err := gzipCompress(original)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	decompressed, err := gzipDecompress(compressed)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	if !bytes.Equal(original, decompressed) {
		t.Fatal("round-trip mismatch")
	}
}

func TestDefaultForgeConfig(t *testing.T) {
	cfg := DefaultForgeConfig()
	if !cfg.Enabled {
		t.Error("default should be enabled")
	}
	if cfg.FakeSubType != "Enterprise" {
		t.Errorf("default sub type should be Enterprise, got %s", cfg.FakeSubType)
	}
	if cfg.FakeCredits != 10000000 {
		t.Errorf("default credits should be 10000000, got %d", cfg.FakeCredits)
	}
}

func TestForgeProtobufSerializationRoundTrip(t *testing.T) {
	planStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 2, WireType: 2, Bytes: buildTimestampMsg(1600000000)},
		{FieldNum: 3, WireType: 2, Bytes: buildTimestampMsg(1700000000)},
		{FieldNum: 6, WireType: 0, Varint: 1},
		{FieldNum: 8, WireType: 0, Varint: 500},
		{FieldNum: 9, WireType: 0, Varint: 100},
	})
	userStatus := buildTestProto([]protoFieldRaw{
		{FieldNum: 4, WireType: 0, Varint: 0},
		{FieldNum: 6, WireType: 0, Varint: 0},
		{FieldNum: 10, WireType: 0, Varint: 0},
		{FieldNum: 13, WireType: 2, Bytes: planStatus},
		{FieldNum: 28, WireType: 0, Varint: 300},
		{FieldNum: 34, WireType: 0, Varint: 9999},
	})
	planInfo := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 0, Varint: 0},
		{FieldNum: 2, WireType: 2, Bytes: []byte("Trial")},
		{FieldNum: 5, WireType: 0, Varint: 1},
		{FieldNum: 9, WireType: 0, Varint: 50},
		{FieldNum: 12, WireType: 0, Varint: 500},
	})
	original := buildTestProto([]protoFieldRaw{
		{FieldNum: 1, WireType: 2, Bytes: userStatus},
		{FieldNum: 2, WireType: 2, Bytes: planInfo},
	})

	cfg := testForgeConfig()
	forged := forgeUserStatus(original, cfg)
	reparsed := parseProtobuf(forged)
	if len(reparsed) < 2 {
		t.Fatalf("expected at least 2 top-level fields, got %d", len(reparsed))
	}

	// Verify nested roundtrip: re-parse F1 (UserStatus)
	for _, f := range reparsed {
		if f.FieldNum == 1 && f.WireType == 2 {
			inner := parseProtobuf(f.Bytes)
			fieldMap := make(map[uint64]protoFieldRaw)
			for _, sf := range inner {
				fieldMap[sf.FieldNum] = sf
			}
			if fieldMap[4].Varint != 1 {
				t.Errorf("UserStatus F4 should be 1, got %d", fieldMap[4].Varint)
			}
			if fieldMap[6].Varint != 2 {
				t.Errorf("UserStatus F6 should be 2, got %d", fieldMap[6].Varint)
			}
			if _, found := fieldMap[34]; found {
				t.Error("UserStatus F34 should be stripped")
			}
			if ps, ok := fieldMap[13]; ok {
				psFields := parseProtobuf(ps.Bytes)
				for _, psf := range psFields {
					if psf.FieldNum == 8 {
						if psf.Varint != uint64(cfg.FakeCredits) {
							t.Errorf("PlanStatus F8 = %d, want %d", psf.Varint, cfg.FakeCredits)
						}
					}
				}
			}
		}
		if f.FieldNum == 2 && f.WireType == 2 {
			inner := parseProtobuf(f.Bytes)
			fieldMap := make(map[uint64]protoFieldRaw)
			for _, sf := range inner {
				fieldMap[sf.FieldNum] = sf
			}
			if string(fieldMap[2].Bytes) != "Enterprise" {
				t.Errorf("PlanInfo F2 = %q, want Enterprise", fieldMap[2].Bytes)
			}
			if _, found := fieldMap[5]; found {
				t.Error("PlanInfo F5 should be stripped")
			}
		}
	}
}
