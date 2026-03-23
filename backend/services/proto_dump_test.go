package services

import (
	"strings"
	"testing"
)

func TestDumpProtoFieldTreeVarint(t *testing.T) {
	// field 1, varint 42: tag=0x08, value=0x2a
	data := []byte{0x08, 0x2a}
	tree := DumpProtoFieldTree(data, 5)
	if !strings.Contains(tree, "F1") || !strings.Contains(tree, "42") {
		t.Fatalf("DumpProtoFieldTree varint: got %q", tree)
	}
}

func TestDumpProtoFieldTreeString(t *testing.T) {
	// field 2, string "hello": tag=0x12, len=5, "hello"
	data := []byte{0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}
	tree := DumpProtoFieldTree(data, 5)
	if !strings.Contains(tree, "F2") || !strings.Contains(tree, "hello") {
		t.Fatalf("DumpProtoFieldTree string: got %q", tree)
	}
}

func TestDumpProtoFieldTreeNested(t *testing.T) {
	// inner: field 1, varint 99 => 0x08 0x63
	inner := []byte{0x08, 0x63}
	// outer: field 3, bytes(inner) => tag=0x1a, len=2, inner
	data := []byte{0x1a, 0x02}
	data = append(data, inner...)

	tree := DumpProtoFieldTree(data, 5)
	if !strings.Contains(tree, "F3") {
		t.Fatalf("missing outer field F3: %q", tree)
	}
	// 内层应该被递归解析
	if !strings.Contains(tree, "F1") || !strings.Contains(tree, "99") {
		t.Fatalf("missing inner field F1/99: %q", tree)
	}
}

func TestDumpProtoFieldTreeMaxDepth(t *testing.T) {
	// 深度嵌套：每层 field 1, bytes(next)
	data := []byte{0x08, 0x01} // leaf: field 1, varint 1
	for i := 0; i < 5; i++ {
		wrapper := []byte{0x0a, byte(len(data))}
		wrapper = append(wrapper, data...)
		data = wrapper
	}

	tree := DumpProtoFieldTree(data, 3)
	if !strings.Contains(tree, "max depth") {
		t.Fatalf("expected max depth warning, got: %q", tree)
	}
}

func TestLooksLikeProtobuf(t *testing.T) {
	// valid: field 1, varint 1
	if !looksLikeProtobuf([]byte{0x08, 0x01}) {
		t.Error("should detect valid protobuf")
	}
	// invalid: random bytes
	if looksLikeProtobuf([]byte{0xff, 0xff, 0xff}) {
		t.Error("should reject random bytes")
	}
	// too short
	if looksLikeProtobuf([]byte{0x08}) {
		t.Error("should reject single byte")
	}
}

func TestIsLikelyUTF8(t *testing.T) {
	if !isLikelyUTF8([]byte("hello 你好")) {
		t.Error("valid UTF-8 not detected")
	}
	if isLikelyUTF8([]byte{0xff, 0xfe, 0x80}) {
		t.Error("invalid UTF-8 not rejected")
	}
	if !isLikelyUTF8(nil) {
		t.Error("empty should be valid UTF-8")
	}
}

func TestHexPreview(t *testing.T) {
	data := []byte{0xde, 0xad, 0xbe, 0xef}
	got := hexPreview(data, 10)
	if got != "deadbeef" {
		t.Fatalf("hexPreview = %q, want %q", got, "deadbeef")
	}

	got = hexPreview(data, 2)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("hexPreview truncated should end with ..., got %q", got)
	}
}

func TestSanitizeFilename(t *testing.T) {
	got := sanitizeFilename("foo/bar\\baz:qux test")
	if strings.ContainsAny(got, "/\\: ") {
		t.Fatalf("sanitizeFilename still has special chars: %q", got)
	}

	long := strings.Repeat("a", 100)
	got = sanitizeFilename(long)
	if len(got) > 60 {
		t.Fatalf("sanitizeFilename too long: %d", len(got))
	}
}
