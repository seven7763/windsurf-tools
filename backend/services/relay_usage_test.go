package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUsageTrackerGetSummaryCachesAndInvalidates(t *testing.T) {
	tracker := NewUsageTracker(t.TempDir())

	tracker.Record(UsageRecord{
		ID:               "rec-1",
		At:               "2026-04-13T10:00:00Z",
		Model:            "claude-sonnet-4",
		PromptTokens:     1200,
		CompletionTokens: 800,
		TotalTokens:      2000,
		Status:           "ok",
	})

	summary := tracker.GetSummary()
	if summary.TotalRequests != 1 {
		t.Fatalf("expected 1 request, got %d", summary.TotalRequests)
	}
	if summary.ErrorCount != 0 {
		t.Fatalf("expected 0 errors, got %d", summary.ErrorCount)
	}
	if summary.ByModel["claude-sonnet-4"] != 1 {
		t.Fatalf("expected sonnet count 1, got %d", summary.ByModel["claude-sonnet-4"])
	}

	// 返回值需要与内部缓存隔离，外部修改不应污染下一次读取。
	summary.ByModel["tampered"] = 99
	again := tracker.GetSummary()
	if _, ok := again.ByModel["tampered"]; ok {
		t.Fatalf("expected cloned summary maps, found tampered entry in cached summary")
	}

	tracker.Record(UsageRecord{
		ID:               "rec-2",
		At:               "2026-04-13T10:05:00Z",
		Model:            "gpt-4o-mini",
		PromptTokens:     200,
		CompletionTokens: 100,
		TotalTokens:      300,
		Status:           "error",
	})

	updated := tracker.GetSummary()
	if updated.TotalRequests != 2 {
		t.Fatalf("expected 2 requests after invalidation, got %d", updated.TotalRequests)
	}
	if updated.ErrorCount != 1 {
		t.Fatalf("expected 1 error after invalidation, got %d", updated.ErrorCount)
	}
	if updated.ByModel["gpt-4o-mini"] != 1 {
		t.Fatalf("expected gpt-4o-mini count 1, got %d", updated.ByModel["gpt-4o-mini"])
	}
}

func TestUsageTrackerDeleteAllLoadsDiskState(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "relay_usage.json")
	records := []UsageRecord{
		{ID: "rec-1", At: "2026-04-13T10:00:00Z", Model: "claude-sonnet-4", TotalTokens: 100},
		{ID: "rec-2", At: "2026-04-13T10:01:00Z", Model: "gpt-4o-mini", TotalTokens: 50},
	}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		t.Fatalf("write records: %v", err)
	}

	tracker := NewUsageTracker(dir)
	deleted := tracker.DeleteAll()
	if deleted != 2 {
		t.Fatalf("expected DeleteAll to report 2 deletions, got %d", deleted)
	}

	reloaded := NewUsageTracker(dir)
	if count := reloaded.Count(); count != 0 {
		t.Fatalf("expected persisted records to be cleared, got %d", count)
	}

	summary := reloaded.GetSummary()
	if summary.TotalRequests != 0 || summary.TotalTokens != 0 {
		t.Fatalf("expected empty summary after DeleteAll, got %+v", summary)
	}
}
