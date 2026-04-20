package services

import (
	"sync"
	"testing"
	"time"
)

// newTestProxy creates a minimal MitmProxy for session binding tests.
func newTestProxy(keys []string) *MitmProxy {
	p := &MitmProxy{
		poolKeys:   keys,
		keyStates:  make(map[string]*PoolKeyState),
		sessionMap: make(map[string]*SessionBinding),
		currentIdx: 0,
		jwtFetches: make(map[string]*jwtFetchCall),
		jwtReady:   make(chan struct{}),
		stopCh:     make(chan struct{}),
	}
	for _, k := range keys {
		state := newPoolKeyState(k)
		state.JWT = []byte("test-jwt-for-" + k)
		p.keyStates[k] = state
	}
	return p
}

func TestSessionBindingCount(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	p.sessionMap["conv-1"] = &SessionBinding{PoolKey: "key-a", LastSeenAt: time.Now()}
	p.sessionMap["conv-2"] = &SessionBinding{PoolKey: "key-a", LastSeenAt: time.Now()}
	p.sessionMap["conv-3"] = &SessionBinding{PoolKey: "key-b", LastSeenAt: time.Now()}

	if got := p.sessionBindingCount("key-a"); got != 2 {
		t.Errorf("sessionBindingCount(key-a) = %d, want 2", got)
	}
	if got := p.sessionBindingCount("key-b"); got != 1 {
		t.Errorf("sessionBindingCount(key-b) = %d, want 1", got)
	}
	if got := p.sessionBindingCount("key-c"); got != 0 {
		t.Errorf("sessionBindingCount(key-c) = %d, want 0", got)
	}
}

func TestLeastConnectionsKey(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b", "key-c"})

	p.sessionsMu.Lock()
	p.sessionMap["conv-1"] = &SessionBinding{PoolKey: "key-a", LastSeenAt: time.Now()}
	p.sessionMap["conv-2"] = &SessionBinding{PoolKey: "key-a", LastSeenAt: time.Now()}
	p.sessionMap["conv-3"] = &SessionBinding{PoolKey: "key-b", LastSeenAt: time.Now()}
	p.sessionsMu.Unlock()

	// key-c has 0 sessions, should win
	p.sessionsMu.RLock()
	got := p.leastConnectionsKey("")
	p.sessionsMu.RUnlock()
	if got != "key-c" {
		t.Errorf("leastConnectionsKey('') = %q, want 'key-c'", got)
	}

	// Exclude key-c → key-b (1 session) wins
	p.sessionsMu.RLock()
	got = p.leastConnectionsKey("key-c")
	p.sessionsMu.RUnlock()
	if got != "key-b" {
		t.Errorf("leastConnectionsKey('key-c') = %q, want 'key-b'", got)
	}
}

func TestPickPoolKeyForSession_Sticky(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	// First call should bind conv-1
	key1, jwt1 := p.pickPoolKeyForSession("conv-1")
	if key1 == "" || len(jwt1) == 0 {
		t.Fatal("pickPoolKeyForSession returned empty on first call")
	}

	// Second call with same conv should return same key (sticky)
	key2, jwt2 := p.pickPoolKeyForSession("conv-1")
	if key2 != key1 {
		t.Errorf("expected sticky key %q, got %q", key1, key2)
	}
	if len(jwt2) == 0 {
		t.Error("expected non-empty jwt on sticky call")
	}

	// Request count should have incremented
	p.sessionsMu.RLock()
	binding := p.sessionMap["conv-1"]
	p.sessionsMu.RUnlock()
	if binding == nil {
		t.Fatal("session binding not found for conv-1")
	}
	if binding.RequestCount != 2 {
		t.Errorf("RequestCount = %d, want 2", binding.RequestCount)
	}
}

func TestPickPoolKeyForSession_LeastConnections(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b", "key-c"})

	// Bind 3 conversations — should distribute across keys
	keys := make(map[string]int)
	for i := 0; i < 6; i++ {
		convID := "conv-" + string(rune('A'+i))
		key, _ := p.pickPoolKeyForSession(convID)
		keys[key]++
	}

	// Each key should have some sessions (least-connections distributes evenly)
	for _, k := range []string{"key-a", "key-b", "key-c"} {
		if keys[k] == 0 {
			t.Errorf("key %q got 0 sessions, expected some distribution", k)
		}
	}
}

func TestPickPoolKeyForSession_ExhaustedMigrates(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	// Bind conv-1 to key-a
	key1, _ := p.pickPoolKeyForSession("conv-1")

	// Mark key-a as exhausted
	p.mu.Lock()
	p.keyStates[key1].markExhausted()
	p.mu.Unlock()

	// 额度耗尽时已有 conversation 应该迁移到新 key（不保持粘性）
	// RuntimeExhausted 的 key 不会自动恢复，继续粘性只会让对话永远卡死
	key2, jwt2 := p.pickPoolKeyForSession("conv-1")
	if key2 == key1 {
		t.Errorf("expected migration away from exhausted %q, got same key", key1)
	}
	if len(jwt2) == 0 {
		t.Error("expected non-empty jwt after migration from exhausted key")
	}
}

func TestPickPoolKeyForSession_RateLimitedKeepSticky(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	// Bind conv-1 to key-a
	key1, _ := p.pickPoolKeyForSession("conv-1")

	// Mark key-a as rate-limited (cooldown) → should keep sticky
	p.mu.Lock()
	p.keyStates[key1].markRateLimited()
	p.mu.Unlock()

	key2, jwt2 := p.pickPoolKeyForSession("conv-1")
	if key2 != key1 {
		t.Errorf("expected sticky to %q (rate-limited), got migrated to %q", key1, key2)
	}
	if len(jwt2) == 0 {
		t.Error("expected non-empty jwt with sticky override for rate-limited key")
	}
}

func TestPickPoolKeyForSession_DisabledMigrates(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	// Bind conv-1 to key-a
	key1, _ := p.pickPoolKeyForSession("conv-1")

	// Mark key-a as disabled (永久禁用) → 应该迁移
	p.mu.Lock()
	p.keyStates[key1].Disabled = true
	p.keyStates[key1].Healthy = false
	p.mu.Unlock()

	key2, jwt2 := p.pickPoolKeyForSession("conv-1")
	if key2 == key1 {
		t.Errorf("expected migration away from disabled %q, got same key", key1)
	}
	if len(jwt2) == 0 {
		t.Error("expected non-empty jwt after migration from disabled key")
	}
}

func TestMigrateSessionsFromKey(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	p.sessionsMu.Lock()
	p.sessionMap["conv-1"] = &SessionBinding{PoolKey: "key-a", LastSeenAt: time.Now()}
	p.sessionMap["conv-2"] = &SessionBinding{PoolKey: "key-a", LastSeenAt: time.Now()}
	p.sessionMap["conv-3"] = &SessionBinding{PoolKey: "key-b", LastSeenAt: time.Now()}
	p.sessionsMu.Unlock()

	p.migrateSessionsFromKey("key-a")

	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()
	for convID, b := range p.sessionMap {
		if b.PoolKey == "key-a" {
			t.Errorf("conv %q still bound to key-a after migration", convID)
		}
	}
}

func TestCleanExpiredSessions(t *testing.T) {
	p := newTestProxy([]string{"key-a"})

	expired := time.Now().Add(-time.Duration(sessionExpireMinutes+5) * time.Minute)
	p.sessionsMu.Lock()
	p.sessionMap["old-conv"] = &SessionBinding{
		PoolKey:    "key-a",
		BoundAt:    expired,
		LastSeenAt: expired,
	}
	p.sessionMap["new-conv"] = &SessionBinding{
		PoolKey:    "key-a",
		BoundAt:    time.Now(),
		LastSeenAt: time.Now(),
	}
	p.sessionsMu.Unlock()

	p.cleanExpiredSessions()

	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()
	if _, ok := p.sessionMap["old-conv"]; ok {
		t.Error("expected old-conv to be cleaned")
	}
	if _, ok := p.sessionMap["new-conv"]; !ok {
		t.Error("expected new-conv to remain")
	}
}

func TestGetSessionBindings(t *testing.T) {
	p := newTestProxy([]string{"key-a"})

	p.sessionsMu.Lock()
	p.sessionMap["conv-abc-def-123"] = &SessionBinding{
		ConversationID: "conv-abc-def-123",
		PoolKey:        "key-a",
		BoundAt:        time.Now(),
		LastSeenAt:     time.Now(),
		RequestCount:   5,
	}
	p.sessionsMu.Unlock()

	bindings := p.GetSessionBindings()
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].RequestCount != 5 {
		t.Errorf("RequestCount = %d, want 5", bindings[0].RequestCount)
	}
}

func TestUnbindSession(t *testing.T) {
	p := newTestProxy([]string{"key-a"})

	p.sessionsMu.Lock()
	p.sessionMap["conv-abc-def-123"] = &SessionBinding{
		ConversationID: "conv-abc-def-123",
		PoolKey:        "key-a",
	}
	p.sessionsMu.Unlock()

	ok := p.UnbindSession("conv-abc")
	if !ok {
		t.Error("UnbindSession returned false, expected true")
	}

	p.sessionsMu.RLock()
	if len(p.sessionMap) != 0 {
		t.Errorf("expected 0 sessions after unbind, got %d", len(p.sessionMap))
	}
	p.sessionsMu.RUnlock()

	// Unbinding non-existent should return false
	ok = p.UnbindSession("non-existent")
	if ok {
		t.Error("UnbindSession returned true for non-existent prefix")
	}
}

func TestSessionCount(t *testing.T) {
	p := newTestProxy([]string{"key-a"})

	if got := p.SessionCount(); got != 0 {
		t.Errorf("SessionCount = %d, want 0", got)
	}

	p.sessionsMu.Lock()
	p.sessionMap["conv-1"] = &SessionBinding{PoolKey: "key-a"}
	p.sessionMap["conv-2"] = &SessionBinding{PoolKey: "key-a"}
	p.sessionsMu.Unlock()

	if got := p.SessionCount(); got != 2 {
		t.Errorf("SessionCount = %d, want 2", got)
	}
}

func TestConcurrentSessionAccess(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b", "key-c"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			convID := "conv-" + string(rune('A'+(n%26)))
			p.pickPoolKeyForSession(convID)
		}(i)
	}
	wg.Wait()

	p.sessionsMu.RLock()
	count := len(p.sessionMap)
	p.sessionsMu.RUnlock()
	if count == 0 {
		t.Error("expected some sessions after concurrent access")
	}
}

// ── Pending new-conversation key tests ──

// TestPickKeyForNewConversation_DistributesAcrossKeys verifies that
// pickKeyForNewConversation distributes first-messages across pool keys
// using pending virtual session counts.
func TestPickKeyForNewConversation_DistributesAcrossKeys(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b", "key-c"})

	key1, jwt1 := p.pickKeyForNewConversation()
	if key1 == "" || len(jwt1) == 0 {
		t.Fatal("pickKeyForNewConversation returned empty")
	}
	key2, _ := p.pickKeyForNewConversation()
	key3, _ := p.pickKeyForNewConversation()

	// All three keys should be different (round-robin via pending counts)
	if key1 == key2 || key2 == key3 || key1 == key3 {
		t.Errorf("expected 3 different keys, got %s, %s, %s", key1, key2, key3)
	}
}

// TestPendingKeyMatchesFirstAndSecondMessage verifies that when a first
// message (no convID) uses pickKeyForNewConversation, the subsequent
// pickPoolKeyForSession (with a new convID) binds to the same key.
func TestPendingKeyMatchesFirstAndSecondMessage(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b", "key-c"})

	// Simulate 3 new conversations' first messages
	firstKeys := make([]string, 3)
	for i := 0; i < 3; i++ {
		k, _ := p.pickKeyForNewConversation()
		firstKeys[i] = k
	}

	// Now simulate the second messages arriving in order
	convIDs := []string{"conv-aaa-bbb-ccc-111", "conv-aaa-bbb-ccc-222", "conv-aaa-bbb-ccc-333"}
	for i, convID := range convIDs {
		secondKey, jwt := p.pickPoolKeyForSession(convID)
		if secondKey != firstKeys[i] {
			t.Errorf("conv %d: first=%s second=%s, want same key", i, firstKeys[i], secondKey)
		}
		if len(jwt) == 0 {
			t.Errorf("conv %d: expected non-empty JWT", i)
		}
	}
}

// TestPendingKeyExpiry verifies that expired pending entries are discarded.
func TestPendingKeyExpiry(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	// Manually push an expired entry
	p.pendingNewConvMu.Lock()
	p.pendingNewConvKeys = append(p.pendingNewConvKeys, pendingKeyEntry{
		PoolKey: "key-a",
		At:      time.Now().Add(-2 * pendingNewConvMaxAge), // well past expiry
	})
	p.pendingNewConvMu.Unlock()

	// popPendingNewConvKey should skip expired and return ""
	got := p.popPendingNewConvKey()
	if got != "" {
		t.Errorf("expected empty from expired pending, got %s", got)
	}

	// New convID should fall through to leastConnectionsKey instead
	key, jwt := p.pickPoolKeyForSession("conv-new-fresh-uuid-1234")
	if key == "" || len(jwt) == 0 {
		t.Fatal("pickPoolKeyForSession should still work via leastConnections fallback")
	}
}

// TestPendingKeySkipsExcludedKey verifies that if the pending key is in
// the excludeKeys list, it falls through to leastConnectionsKey.
func TestPendingKeySkipsExcludedKey(t *testing.T) {
	p := newTestProxy([]string{"key-a", "key-b"})

	k, _ := p.pickKeyForNewConversation()
	// Request second message with the pending key excluded
	secondKey, _ := p.pickPoolKeyForSession("conv-excl-test-uuid-5678", k)
	if secondKey == k {
		t.Errorf("expected different key when pending key is excluded, got same: %s", k)
	}
}

func TestIsChatPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/windsurf.AIService/GetChatMessage", true},
		{"/windsurf.AIService/GetChatMessageBurst", true},
		{"/windsurf.AIService/GetCompletions", true},
		{"/windsurf.AIService/ListConversations", false},
		{"/health", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isChatPath(tc.path); got != tc.want {
			t.Errorf("isChatPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
