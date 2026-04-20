package services

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"windsurf-tools-wails/backend/paths"
)

// ══════════════════════════════════════════════════════════════
// Proto field tree dumper — 递归解析 protobuf wire format 并输出
// 可读的字段树，用于逆向 GetChatMessage 等未知消息结构。
// ══════════════════════════════════════════════════════════════

// DumpProtoFieldTree 递归解析 protobuf 字节流，返回缩进的字段树字符串。
func DumpProtoFieldTree(data []byte, maxDepth int) string {
	if maxDepth <= 0 {
		maxDepth = 12
	}
	var sb strings.Builder
	dumpFields(&sb, data, 0, maxDepth)
	return sb.String()
}

func dumpFields(sb *strings.Builder, data []byte, depth, maxDepth int) {
	if depth >= maxDepth {
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(sb, "%s... (max depth reached)\n", indent)
		return
	}

	indent := strings.Repeat("  ", depth)
	pos := 0
	fieldIdx := 0

	for pos < len(data) {
		tag, newPos := readVarint(data, pos)
		if newPos == pos || tag == 0 {
			break
		}
		pos = newPos
		fn := tag >> 3
		wt := tag & 7

		switch wt {
		case 0: // varint
			val, newPos := readVarint(data, pos)
			pos = newPos
			fmt.Fprintf(sb, "%sF%d (varint): %d", indent, fn, val)
			if val > 1000000000 && val < 2000000000 {
				t := time.Unix(int64(val), 0).UTC()
				fmt.Fprintf(sb, "  [maybe timestamp: %s]", t.Format(time.RFC3339))
			}
			sb.WriteByte('\n')

		case 1: // fixed64
			if pos+8 > len(data) {
				return
			}
			val := binary.LittleEndian.Uint64(data[pos : pos+8])
			pos += 8
			fmt.Fprintf(sb, "%sF%d (fixed64): %d\n", indent, fn, val)

		case 2: // length-delimited
			length, newPos := readVarint(data, pos)
			pos = newPos
			end := pos + int(length)
			if end > len(data) {
				fmt.Fprintf(sb, "%sF%d (bytes): <truncated, declared %d, avail %d>\n", indent, fn, length, len(data)-pos)
				return
			}
			fieldData := data[pos:end]
			pos = end

			// 尝试当子消息递归解析
			if len(fieldData) > 1 && looksLikeProtobuf(fieldData) {
				fmt.Fprintf(sb, "%sF%d (message, %d bytes):\n", indent, fn, len(fieldData))
				dumpFields(sb, fieldData, depth+1, maxDepth)
			} else if isLikelyUTF8(fieldData) {
				s := string(fieldData)
				if len(s) > 200 {
					s = s[:200] + "..."
				}
				// 标记特殊字符串
				tag := ""
				if strings.HasPrefix(s, "sk-ws-") {
					tag = " [API_KEY]"
				} else if strings.HasPrefix(s, "eyJ") && len(s) > 100 {
					tag = " [JWT]"
				}
				fmt.Fprintf(sb, "%sF%d (string, %d bytes): %q%s\n", indent, fn, len(fieldData), s, tag)
			} else {
				fmt.Fprintf(sb, "%sF%d (bytes, %d bytes): %s\n", indent, fn, len(fieldData), hexPreview(fieldData, 64))
			}

		case 5: // fixed32
			if pos+4 > len(data) {
				return
			}
			val := binary.LittleEndian.Uint32(data[pos : pos+4])
			pos += 4
			fmt.Fprintf(sb, "%sF%d (fixed32): %d\n", indent, fn, val)

		default:
			fmt.Fprintf(sb, "%sF%d (wire=%d): <unknown wire type>\n", indent, fn, wt)
			return
		}

		fieldIdx++
		if fieldIdx > 500 {
			fmt.Fprintf(sb, "%s... (too many fields, stopping)\n", indent)
			return
		}
	}
}

// looksLikeProtobuf 启发式判断字节流是否像有效的 protobuf 消息
func looksLikeProtobuf(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	pos := 0
	validFields := 0
	for pos < len(data) && validFields < 5 {
		tag, newPos := readVarint(data, pos)
		if newPos == pos || tag == 0 {
			break
		}
		pos = newPos
		fn := tag >> 3
		wt := tag & 7

		// 合理的 field number 范围
		if fn == 0 || fn > 10000 {
			return false
		}

		switch wt {
		case 0:
			_, newPos = readVarint(data, pos)
			if newPos == pos {
				return false
			}
			pos = newPos
		case 2:
			length, newPos := readVarint(data, pos)
			pos = newPos
			end := pos + int(length)
			if end > len(data) || length > uint64(len(data)) {
				return false
			}
			pos = end
		case 5:
			pos += 4
		case 1:
			pos += 8
		default:
			return false
		}
		validFields++
	}
	return validFields >= 1 && pos <= len(data)
}

func isLikelyUTF8(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return utf8.Valid(data)
}

func hexPreview(data []byte, maxBytes int) string {
	if len(data) <= maxBytes {
		return fmt.Sprintf("%x", data)
	}
	return fmt.Sprintf("%x...", data[:maxBytes])
}

// ── Dump 文件写入 ──

// ProtoDumpDir 返回 dump 文件目录
func ProtoDumpDir() string {
	appDir, err := paths.ResolveAppConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(appDir, "proto_dumps")
}

// WriteProtoDump 将 dump 内容写入文件
func WriteProtoDump(label string, data []byte) (string, error) {
	dir := ProtoDumpDir()
	if dir == "" {
		return "", fmt.Errorf("无法确定 dump 目录")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	ts := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.txt", ts, sanitizeFilename(label))
	path := filepath.Join(dir, filename)

	// 写入原始 hex + 解析树
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== %s ===\n", label)
	fmt.Fprintf(&sb, "Time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&sb, "Size: %d bytes\n\n", len(data))

	// 先去掉 gRPC 5 字节信封
	payload, envType := decompressBody(data)
	fmt.Fprintf(&sb, "Envelope: %v\n", envType)
	fmt.Fprintf(&sb, "Payload size: %d bytes\n\n", len(payload))

	fmt.Fprintf(&sb, "=== Field Tree ===\n")
	sb.WriteString(DumpProtoFieldTree(payload, 12))

	fmt.Fprintf(&sb, "\n=== Raw Hex ===\n")
	sb.WriteString(hexPreview(data, len(data)*2+1))
	sb.WriteByte('\n')

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// ── 流式响应 dump tee ──

// dumpTeeBody 包装 ReadCloser，捕获前 maxCapture 字节用于 dump，同时透传给下游。
type dumpTeeBody struct {
	inner     io.ReadCloser
	captured  []byte
	maxCap    int
	label     string
	logger    interface{ log(string, ...interface{}) }
	finalized bool
}

const defaultStreamDumpCap = 32768 // 32KB

func newDumpTeeBody(inner io.ReadCloser, label string, logger interface{ log(string, ...interface{}) }) *dumpTeeBody {
	return &dumpTeeBody{
		inner:  inner,
		maxCap: defaultStreamDumpCap,
		label:  label,
		logger: logger,
	}
}

func (d *dumpTeeBody) Read(p []byte) (int, error) {
	n, err := d.inner.Read(p)
	if n > 0 && len(d.captured) < d.maxCap {
		remain := d.maxCap - len(d.captured)
		take := n
		if take > remain {
			take = remain
		}
		d.captured = append(d.captured, p[:take]...)
	}
	if err == io.EOF {
		d.flush()
	}
	return n, err
}

func (d *dumpTeeBody) Close() error {
	d.flush()
	return d.inner.Close()
}

func (d *dumpTeeBody) flush() {
	if d.finalized || len(d.captured) == 0 {
		return
	}
	d.finalized = true
	if path, err := WriteProtoDump(d.label, d.captured); err == nil && d.logger != nil {
		d.logger.log("📝 dump 响应(stream, %dB): %s", len(d.captured), path)
	}
}
