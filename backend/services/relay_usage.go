package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// 用量追踪 — 记录每次 relay 请求的模型、token 消耗、时间等
// 存储为本地 JSONL 文件，支持查询与删除
// ═══════════════════════════════════════════════════════════════

// UsageRecord 单次请求的用量记录
type UsageRecord struct {
	ID               string  `json:"id"`
	At               string  `json:"at"`                // RFC3339
	Model            string  `json:"model"`
	RequestModel     string  `json:"request_model"`     // 用户请求的原始模型名
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	DurationMs       int64   `json:"duration_ms"`
	APIKeyShort      string  `json:"api_key_short"`
	Status           string  `json:"status"` // "ok" / "error"
	ErrorDetail      string  `json:"error_detail,omitempty"`
	Format           string  `json:"format"` // "openai" / "anthropic"
}

// UsageSummary 用量汇总（按天/按模型）
type UsageSummary struct {
	TotalRequests    int              `json:"total_requests"`
	TotalPrompt      int              `json:"total_prompt_tokens"`
	TotalCompletion  int              `json:"total_completion_tokens"`
	TotalTokens      int              `json:"total_tokens"`
	ByModel          map[string]int   `json:"by_model"`           // model → request count
	ByModelTokens    map[string]int   `json:"by_model_tokens"`    // model → total tokens
	ByDate           map[string]int   `json:"by_date"`            // YYYY-MM-DD → request count
	ByDateTokens     map[string]int   `json:"by_date_tokens"`     // YYYY-MM-DD → total tokens
	ErrorCount       int              `json:"error_count"`
	EstimatedCostUSD float64          `json:"estimated_cost_usd"`
}

// UsageTracker 管理用量追踪
type UsageTracker struct {
	mu       sync.Mutex
	dataDir  string
	records  []UsageRecord
	loaded   bool
	maxStore int // 最大存储条数（0=无限制）
	summary  UsageSummary
	dirty    bool
}

// NewUsageTracker 创建用量追踪器
func NewUsageTracker(dataDir string) *UsageTracker {
	return &UsageTracker{
		dataDir:  dataDir,
		maxStore: 10000,
		dirty:    true,
	}
}

func (t *UsageTracker) filePath() string {
	return filepath.Join(t.dataDir, "relay_usage.json")
}

// Record 记录一次用量
func (t *UsageTracker) Record(rec UsageRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.loaded {
		t.loadLocked()
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("u-%d", time.Now().UnixNano())
	}
	if rec.At == "" {
		rec.At = time.Now().Format(time.RFC3339)
	}
	t.records = append(t.records, rec)
	// 超过上限时裁剪旧记录
	if t.maxStore > 0 && len(t.records) > t.maxStore {
		t.records = t.records[len(t.records)-t.maxStore:]
	}
	t.markSummaryDirtyLocked()
	t.saveLocked()
}

// GetRecords 返回所有记录（最近的在前）
func (t *UsageTracker) GetRecords(limit int) []UsageRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	n := len(t.records)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]UsageRecord, limit)
	for i := 0; i < limit; i++ {
		out[i] = t.records[n-1-i] // 最近的在前
	}
	return out
}

// GetSummary 返回用量汇总
func (t *UsageTracker) GetSummary() UsageSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	if t.dirty {
		t.summary = t.computeSummaryLocked()
		t.dirty = false
	}
	return cloneUsageSummary(t.summary)
}

func (t *UsageTracker) computeSummaryLocked() UsageSummary {
	s := UsageSummary{
		ByModel:       make(map[string]int),
		ByModelTokens: make(map[string]int),
		ByDate:        make(map[string]int),
		ByDateTokens:  make(map[string]int),
	}
	for _, r := range t.records {
		s.TotalRequests++
		s.TotalPrompt += r.PromptTokens
		s.TotalCompletion += r.CompletionTokens
		s.TotalTokens += r.TotalTokens
		model := r.Model
		if model == "" {
			model = r.RequestModel
		}
		s.ByModel[model]++
		s.ByModelTokens[model] += r.TotalTokens
		// 按日期分组
		ts, err := time.Parse(time.RFC3339, r.At)
		if err == nil {
			date := ts.Format("2006-01-02")
			s.ByDate[date]++
			s.ByDateTokens[date] += r.TotalTokens
		}
		if r.Status == "error" {
			s.ErrorCount++
		}

		// 精准计算定价 (根据模型)
		var pPrice, cPrice float64
		mL := strings.ToLower(model)
		if strings.Contains(mL, "opus") {
			pPrice, cPrice = 5.00, 25.00 // Opus 4.6 / Opus API
		} else if strings.Contains(mL, "sonnet") {
			pPrice, cPrice = 3.00, 15.00 // Sonnet 4.6 / 3.5
		} else if strings.Contains(mL, "haiku") {
			pPrice, cPrice = 1.00, 5.00
		} else if strings.Contains(mL, "o1") {
			pPrice, cPrice = 15.00, 60.00
		} else if strings.Contains(mL, "gpt-4o-mini") || strings.Contains(mL, "gpt-3.5") {
			pPrice, cPrice = 0.15, 0.60
		} else if strings.Contains(mL, "gpt-4") {
			pPrice, cPrice = 2.50, 10.00
		} else {
			// 默认按 Opus 定价估算（用户常用）
			pPrice, cPrice = 5.00, 25.00
		}
		
		cost := (float64(r.PromptTokens) / 1000000.0 * pPrice) + (float64(r.CompletionTokens) / 1000000.0 * cPrice)
		s.EstimatedCostUSD += cost
	}
	return s
}

// DeleteAll 清空所有记录
func (t *UsageTracker) DeleteAll() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	n := len(t.records)
	t.records = nil
	t.markSummaryDirtyLocked()
	t.saveLocked()
	return n
}

// DeleteBefore 删除指定日期之前的记录
func (t *UsageTracker) DeleteBefore(before time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	var kept []UsageRecord
	deleted := 0
	for _, r := range t.records {
		ts, err := time.Parse(time.RFC3339, r.At)
		if err != nil || ts.Before(before) {
			deleted++
			continue
		}
		kept = append(kept, r)
	}
	t.records = kept
	if deleted > 0 {
		t.markSummaryDirtyLocked()
		t.saveLocked()
	}
	return deleted
}

// Count 返回记录总数
func (t *UsageTracker) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.loaded {
		t.loadLocked()
	}
	return len(t.records)
}

// ── 持久化 ──

func (t *UsageTracker) loadLocked() {
	t.loaded = true
	data, err := os.ReadFile(t.filePath())
	if err != nil {
		t.records = nil
		t.markSummaryDirtyLocked()
		return
	}
	var records []UsageRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.records = nil
		t.markSummaryDirtyLocked()
		return
	}
	// 按时间排序
	sort.Slice(records, func(i, j int) bool {
		return records[i].At < records[j].At
	})
	t.records = records
	t.markSummaryDirtyLocked()
}

func (t *UsageTracker) saveLocked() {
	os.MkdirAll(t.dataDir, 0755)
	data, err := json.MarshalIndent(t.records, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(t.filePath(), data, 0644)
}

// estimateTokens 粗略估算 token 数（按 4 字符 ≈ 1 token）
func estimateTokens(text string) int {
	n := len(text) / 4
	if n == 0 && len(text) > 0 {
		n = 1
	}
	return n
}

func (t *UsageTracker) markSummaryDirtyLocked() {
	t.dirty = true
	t.summary = UsageSummary{}
}

func cloneUsageSummary(in UsageSummary) UsageSummary {
	out := in
	out.ByModel = cloneStringIntMap(in.ByModel)
	out.ByModelTokens = cloneStringIntMap(in.ByModelTokens)
	out.ByDate = cloneStringIntMap(in.ByDate)
	out.ByDateTokens = cloneStringIntMap(in.ByDateTokens)
	return out
}

func cloneStringIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
