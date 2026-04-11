package services

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"windsurf-tools-wails/backend/utils"
)

// encodeVarintField 编码一个 varint 字段: (fieldNum << 3 | 0) + varint(value)
func encodeVarintField(fieldNum, value uint64) []byte {
	tag := writeVarint((fieldNum << 3) | 0)
	val := writeVarint(value)
	result := make([]byte, 0, len(tag)+len(val))
	result = append(result, tag...)
	result = append(result, val...)
	return result
}

// buildGenerationConfig 构造 F8 generation config 子消息。
// 基于抓包: F1=1, F2=8192(max_tokens), F3=200, F7=50, F9=stop_words(repeated)
func buildGenerationConfig() []byte {
	var cfg []byte
	cfg = append(cfg, encodeVarintField(1, 1)...)
	cfg = append(cfg, encodeVarintField(2, 8192)...)
	cfg = append(cfg, encodeVarintField(3, 200)...)
	cfg = append(cfg, encodeVarintField(7, 50)...)
	// stop words from captured traffic
	stopWords := []string{
		"\x3c|user|\x3e",
		"\x3c|bot|\x3e",
		"\x3c|context_request|\x3e",
		"\x3c|endoftext|\x3e",
		"\x3c|end_of_turn|\x3e",
	}
	for _, sw := range stopWords {
		cfg = append(cfg, utils.EncodeStringField(9, sw)...)
	}
	return cfg
}

// windsurf model enum 映射表（OpenAI 模型名 → Windsurf 内部 enum 字符串）
var modelEnumMap = map[string]string{
	// 直通
	"cascade": "",

	// GPT 系列
	"gpt-4o":      "MODEL_GPT_4O",
	"gpt-4o-mini": "MODEL_GPT_4O_MINI",
	"gpt-4.1":     "MODEL_GPT_4_1",
	"gpt-4.1-mini": "MODEL_GPT_4_1_MINI",
	"gpt-4.1-nano": "MODEL_GPT_4_1_NANO",

	// o 系列
	"o3-mini": "MODEL_O3_MINI",

	// Claude
	"claude-3.5-haiku":  "MODEL_CLAUDE_3_5_HAIKU",
	"claude-3p5":        "MODEL_CLAUDE_3_5_SONNET",
	"claude-3p7":        "MODEL_CLAUDE_3_7_SONNET",
	"claude-sonnet-4":   "MODEL_CLAUDE_SONNET_4",
	"claude-sonnet-4.5": "MODEL_CLAUDE_SONNET_4_5",
	"claude-sonnet-4.6": "MODEL_CLAUDE_SONNET_4_6",
	"claude-opus-4":     "MODEL_CLAUDE_OPUS_4",

	// Gemini
	"gemini-2.0-flash":      "MODEL_GOOGLE_GEMINI_2_0_FLASH",
	"gemini-2.5-flash-lite": "MODEL_GOOGLE_GEMINI_2_5_FLASH_LITE",
	"gemini-2.5-pro":        "MODEL_GOOGLE_GEMINI_2_5_PRO",
	"gemini-3.0-pro":        "MODEL_GOOGLE_GEMINI_3_0_PRO",
	"gemini-3.0-flash":      "MODEL_GOOGLE_GEMINI_3_0_FLASH",

	// DeepSeek
	"deepseek-v3": "MODEL_DEEPSEEK_V3",
	"deepseek-r1": "MODEL_DEEPSEEK_R1",

	// Qwen
	"qwen-2.5-32b-instruct": "MODEL_QWEN_2_5_CODER_32B_INSTRUCT",
}

// mapModelToWindsurfEnum 将 OpenAI 模型名映射为 Windsurf 内部 enum 字符串。
// 如果模型名已经是 MODEL_ 前缀则直接使用，空或 cascade 返空（使用默认）。
func mapModelToWindsurfEnum(model string) string {
	model = strings.TrimSpace(model)
	if model == "" || strings.ToLower(model) == "cascade" {
		return ""
	}
	// 已经是 Windsurf enum 格式
	if strings.HasPrefix(model, "MODEL_") {
		return model
	}
	if enum, ok := modelEnumMap[strings.ToLower(model)]; ok {
		return enum
	}
	// 未知模型：尝试自动转换（model-name → MODEL_MODEL_NAME）
	upper := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(model, "-", "_"), ".", "_"))
	return "MODEL_" + upper
}

// generateUUID 生成一个随机 UUID v4 字符串
func generateUUID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant RFC 4122
	return hex.EncodeToString(buf[0:4]) + "-" +
		hex.EncodeToString(buf[4:6]) + "-" +
		hex.EncodeToString(buf[6:8]) + "-" +
		hex.EncodeToString(buf[8:10]) + "-" +
		hex.EncodeToString(buf[10:16])
}
