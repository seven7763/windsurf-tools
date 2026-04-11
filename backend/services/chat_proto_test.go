package services

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"testing"
)

func TestWrapGRPCEnvelope(t *testing.T) {
	payload := []byte{0x0a, 0x03, 0x66, 0x6f, 0x6f} // field 1, string "foo"
	envelope := WrapGRPCEnvelope(payload)

	if len(envelope) != 5+len(payload) {
		t.Fatalf("envelope length = %d, want %d", len(envelope), 5+len(payload))
	}
	if envelope[0] != 0x00 {
		t.Fatalf("envelope[0] = %x, want 0x00", envelope[0])
	}
	gotLen := binary.BigEndian.Uint32(envelope[1:5])
	if gotLen != uint32(len(payload)) {
		t.Fatalf("envelope payload length = %d, want %d", gotLen, len(payload))
	}
	for i, b := range payload {
		if envelope[5+i] != b {
			t.Fatalf("envelope[%d] = %x, want %x", 5+i, envelope[5+i], b)
		}
	}
}

func TestExtractGRPCFrames(t *testing.T) {
	frame1 := []byte{0x0a, 0x02, 0x68, 0x69}
	frame2 := []byte{0x0a, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}

	var stream []byte
	stream = append(stream, WrapGRPCEnvelope(frame1)...)
	stream = append(stream, WrapGRPCEnvelope(frame2)...)

	frames := ExtractGRPCFrames(stream)
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if string(frames[0]) != string(frame1) {
		t.Fatalf("frame[0] mismatch")
	}
	if string(frames[1]) != string(frame2) {
		t.Fatalf("frame[1] mismatch")
	}
}

func TestExtractGRPCFrames_DecodesCompressedConnectFrames(t *testing.T) {
	frame1 := encodeBytesField(6, encodeBytesField(3, []byte("hello ")))
	frame2 := encodeBytesField(6, encodeBytesField(3, []byte("world")))

	var stream []byte
	stream = append(stream, appendStreamEnvelope(nil, streamEnvelopeCompressed, gzipBytes(t, frame1))...)
	stream = append(stream, appendStreamEnvelope(nil, streamEnvelopeCompressed, gzipBytes(t, frame2))...)
	stream = append(stream, appendStreamEnvelope(nil, streamEnvelopeEndStream, []byte(`{}`))...)

	frames := ExtractGRPCFrames(stream)
	if len(frames) != 2 {
		t.Fatalf("got %d decoded frames, want 2", len(frames))
	}
	if string(frames[0]) != string(frame1) {
		t.Fatalf("decoded frame[0] mismatch")
	}
	if string(frames[1]) != string(frame2) {
		t.Fatalf("decoded frame[1] mismatch")
	}
}

func TestExtractGRPCFramesTruncated(t *testing.T) {
	frame := []byte{0x0a, 0x02, 0x68, 0x69}
	envelope := WrapGRPCEnvelope(frame)
	// 截断最后一个字节
	truncated := envelope[:len(envelope)-1]

	frames := ExtractGRPCFrames(truncated)
	if len(frames) != 0 {
		t.Fatalf("got %d frames from truncated data, want 0", len(frames))
	}
}

func TestParseChatResponseChunk_PrefersNestedContentField(t *testing.T) {
	payload := append(
		encodeBytesField(1, []byte("bot-123")),
		encodeBytesField(6, encodeBytesField(3, []byte("hello from nested content")))...,
	)

	text, isDone, err := ParseChatResponseChunk(payload)
	if err != nil {
		t.Fatalf("ParseChatResponseChunk error = %v", err)
	}
	if isDone {
		t.Fatal("isDone = true, want false")
	}
	if text != "hello from nested content" {
		t.Fatalf("text = %q", text)
	}
}

func TestBuildChatRequestProducesValidProtobuf(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "user", Content: "Hello world"},
	}
	body := BuildChatRequest(msgs, "sk-ws-test123", "jwt-token", "conv-abc")

	// 应该能被 decodeProtoMessage 解析
	fields := decodeProtoMessage(body)
	if len(fields) == 0 {
		t.Fatal("BuildChatRequest produced empty protobuf")
	}

	// 新布局: F1=metadata, F3=chat messages, F7=settings, F8=config, F15=conv context, F22=msg_id
	fieldNums := map[uint64]bool{}
	for _, f := range fields {
		fieldNums[f.Number] = true
	}
	// F1=metadata, F3=chat messages, F7=settings, F8=config, F15=conv context, F22=msg_id
	for _, n := range []uint64{1, 3, 7, 8, 15, 22} {
		if !fieldNums[n] {
			t.Errorf("missing field %d in BuildChatRequest output", n)
		}
	}
}

func TestBuildChatRequestOmitsConversationIDWhenEmpty(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "test"}}
	body := BuildChatRequest(msgs, "sk-ws-key", "jwt", "")

	fields := decodeProtoMessage(body)
	// conversation_id 现在在 F15 中，空 conv_id 不应有 F15
	for _, f := range fields {
		if f.Number == 15 {
			t.Fatal("field 15 (conversation context) should be absent when conv_id is empty")
		}
	}
}

func TestBuildChatRequestMetadataContainsAPIKey(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "hi"}}
	body := BuildChatRequest(msgs, "sk-ws-mykey", "myjwt", "")

	fields := decodeProtoMessage(body)
	var metaBytes []byte
	for _, f := range fields {
		if f.Number == 1 && f.Wire == 2 {
			metaBytes = f.Bytes
			break
		}
	}
	if metaBytes == nil {
		t.Fatal("no metadata field found")
	}

	// 解析 metadata 子消息
	metaFields := decodeProtoMessage(metaBytes)
	found := false
	for _, mf := range metaFields {
		if mf.Number == 3 && mf.Wire == 2 && string(mf.Bytes) == "sk-ws-mykey" {
			found = true
		}
	}
	if !found {
		t.Error("API key not found in metadata field 3")
	}
}

func TestFlattenMessagesSingle(t *testing.T) {
	msgs := []ChatMessage{{Role: "user", Content: "hello"}}
	got := flattenMessages(msgs)
	if got != "hello" {
		t.Fatalf("flattenMessages single = %q, want %q", got, "hello")
	}
}

func TestFlattenMessagesMultiple(t *testing.T) {
	msgs := []ChatMessage{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello!"},
	}
	got := flattenMessages(msgs)
	if got == "" {
		t.Fatal("flattenMessages returned empty")
	}
	// 应包含所有角色标记
	for _, tag := range []string{"[System]", "[User]", "[Assistant]"} {
		if !contains(got, tag) {
			t.Errorf("flattenMessages missing %q", tag)
		}
	}
}

func TestFlattenMessagesEmpty(t *testing.T) {
	got := flattenMessages(nil)
	if got != "" {
		t.Fatalf("flattenMessages(nil) = %q, want empty", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestEncodeBytesFieldRoundTrip(t *testing.T) {
	data := []byte("test payload")
	encoded := encodeBytesField(5, data)

	fields := decodeProtoMessage(encoded)
	if len(fields) != 1 {
		t.Fatalf("got %d fields, want 1", len(fields))
	}
	if fields[0].Number != 5 {
		t.Fatalf("field number = %d, want 5", fields[0].Number)
	}
	if string(fields[0].Bytes) != "test payload" {
		t.Fatalf("field bytes = %q, want %q", string(fields[0].Bytes), "test payload")
	}
}

func appendStreamEnvelope(dst []byte, flags byte, payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = flags
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return append(dst, frame...)
}

func gzipBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	compressed, err := io.ReadAll(&buf)
	if err != nil {
		t.Fatalf("read gzip buffer: %v", err)
	}
	return compressed
}
