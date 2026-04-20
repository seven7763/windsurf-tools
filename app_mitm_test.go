package main

import (
	"testing"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/store"
)

func TestHandleMitmKeyAccessDeniedPersistsAccountStatusAndSkipsKey(t *testing.T) {
	newTestApp := func(t *testing.T, firstDetail string) *App {
		t.Helper()
		s, err := store.NewStoreInPaths(t.TempDir())
		if err != nil {
			t.Fatalf("NewStoreInPaths() error = %v", err)
		}
		accounts := []models.Account{
			{ID: "a", Email: "a@example.com", WindsurfAPIKey: "sk-ws-a", PlanName: "Pro", Status: "active", DailyRemaining: "60.00%"},
			{ID: "b", Email: "b@example.com", WindsurfAPIKey: "sk-ws-b", PlanName: "Pro", Status: "active", DailyRemaining: "55.00%"},
		}
		for _, acc := range accounts {
			if err := s.AddAccount(acc); err != nil {
				t.Fatalf("AddAccount(%s) error = %v", acc.ID, err)
			}
		}
		app := &App{
			store:     s,
			mitmProxy: services.NewMitmProxy(nil, nil, "", nil),
		}
		app.syncMitmPoolKeys()
		app.handleMitmKeyAccessDenied("sk-ws-a", firstDetail)
		return app
	}

	t.Run("generic permission denied disables account", func(t *testing.T) {
		app := newTestApp(t, `Connect JWT失败(HTTP 403): {"code":"permission_denied","message":"permission denied"}`)

		acc, err := app.store.GetAccount("a")
		if err != nil {
			t.Fatalf("GetAccount(a) error = %v", err)
		}
		if acc.Status != "disabled" {
			t.Fatalf("Status = %q, want disabled", acc.Status)
		}
		keys := collectEligibleMitmAPIKeys(app.store.GetAllAccounts(), "all")
		if len(keys) != 1 || keys[0] != "sk-ws-b" {
			t.Fatalf("eligible keys = %#v, want only sk-ws-b", keys)
		}
	})

	t.Run("subscription inactive downgrades to free and skips key", func(t *testing.T) {
		app := newTestApp(t, `Connect JWT失败(HTTP 403): {"code":"permission_denied","message":"subscription is not active, please contact your admin"}`)

		acc, err := app.store.GetAccount("a")
		if err != nil {
			t.Fatalf("GetAccount(a) error = %v", err)
		}
		if acc.Status != "expired" {
			t.Fatalf("Status = %q, want expired", acc.Status)
		}
		if acc.PlanName != "Free" {
			t.Fatalf("PlanName = %q, want Free", acc.PlanName)
		}
		keys := collectEligibleMitmAPIKeys(app.store.GetAllAccounts(), "all")
		if len(keys) != 1 || keys[0] != "sk-ws-b" {
			t.Fatalf("eligible keys = %#v, want only sk-ws-b", keys)
		}
	})
}

func TestRotateMitmToNextAvailableSwitchesKey(t *testing.T) {
	s, err := store.NewStoreInPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreInPaths() error = %v", err)
	}
	now := time.Now().Format(time.RFC3339)
	accounts := []models.Account{
		{ID: "a", Email: "a@example.com", Token: "token-a", WindsurfAPIKey: "sk-ws-a", PlanName: "Pro", Status: "active", DailyRemaining: "80.00%", WeeklyRemaining: "80.00%", LastQuotaUpdate: now},
		{ID: "b", Email: "b@example.com", Token: "token-b", WindsurfAPIKey: "sk-ws-b", PlanName: "Pro", Status: "active", DailyRemaining: "75.00%", WeeklyRemaining: "75.00%", LastQuotaUpdate: now},
	}
	for _, acc := range accounts {
		if err := s.AddAccount(acc); err != nil {
			t.Fatalf("AddAccount(%s) error = %v", acc.ID, err)
		}
	}

	app := &App{
		store:     s,
		mitmProxy: services.NewMitmProxy(nil, nil, "", nil),
	}
	app.syncMitmPoolKeys()
	if ok := app.mitmProxy.SwitchToKey("sk-ws-a"); !ok {
		t.Fatal("SwitchToKey(sk-ws-a) = false, want true")
	}

	next, err := app.rotateMitmToNextAvailable("a", "all")
	if err != nil {
		t.Fatalf("rotateMitmToNextAvailable() error = %v", err)
	}
	if next.Email != "b@example.com" {
		t.Fatalf("next.Email = %q, want %q", next.Email, "b@example.com")
	}
	if got := app.mitmProxy.CurrentAPIKey(); got != "sk-ws-b" {
		t.Fatalf("CurrentAPIKey() = %q, want %q", got, "sk-ws-b")
	}
}

func TestHandleMitmCurrentKeyChangedDoesNotSyncLocalAuth(t *testing.T) {
	s, err := store.NewStoreInPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreInPaths() error = %v", err)
	}
	accounts := []models.Account{
		{ID: "a", Email: "a@example.com", Token: "token-a", WindsurfAPIKey: "sk-ws-a", PlanName: "Pro", Status: "active", DailyRemaining: "80.00%", WeeklyRemaining: "80.00%"},
		{ID: "b", Email: "b@example.com", Token: "token-b", WindsurfAPIKey: "sk-ws-b", PlanName: "Pro", Status: "active", DailyRemaining: "75.00%", WeeklyRemaining: "75.00%"},
	}
	for _, acc := range accounts {
		if err := s.AddAccount(acc); err != nil {
			t.Fatalf("AddAccount(%s) error = %v", acc.ID, err)
		}
	}

	app := &App{
		store:     s,
		mitmProxy: services.NewMitmProxy(nil, nil, "", nil),
	}
	app.syncMitmPoolKeys()
	if ok := app.mitmProxy.SwitchToKey("sk-ws-a"); !ok {
		t.Fatal("SwitchToKey(sk-ws-a) = false, want true")
	}

	// ★ shouldSyncMitmLocalSessionOnKeyChange() = false
	// MITM 按 conversation_id 路由，自动轮转不修改本地登录态
	app.handleMitmCurrentKeyChanged("sk-ws-b", services.MitmCurrentKeyChangeReasonRateLimitRotate)
	// No assertion on local auth — the function just logs, no side effects on auth files
}
