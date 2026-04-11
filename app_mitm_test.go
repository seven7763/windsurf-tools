package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/services"
	"windsurf-tools-wails/backend/store"
)

func setupTestAuthEnv(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	origAppData := os.Getenv("APPDATA")
	origHome := os.Getenv("HOME")
	origXDGConfigHome := os.Getenv("XDG_CONFIG_HOME")
	if err := os.Setenv("APPDATA", tmp); err != nil {
		t.Fatalf("Setenv(APPDATA) error = %v", err)
	}
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("Setenv(HOME) error = %v", err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, ".config")); err != nil {
		t.Fatalf("Setenv(XDG_CONFIG_HOME) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("APPDATA", origAppData)
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("XDG_CONFIG_HOME", origXDGConfigHome)
	})
	return tmp
}

func readWindsurfAuthForTest(t *testing.T, switchSvc *services.SwitchService) services.WindsurfAuthJSON {
	t.Helper()
	authPath, err := switchSvc.GetWindsurfAuthPath()
	if err != nil {
		t.Fatalf("GetWindsurfAuthPath() error = %v", err)
	}
	raw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", authPath, err)
	}
	var auth services.WindsurfAuthJSON
	if err := json.Unmarshal(raw, &auth); err != nil {
		t.Fatalf("Unmarshal(auth) error = %v", err)
	}
	return auth
}

func waitForWindsurfAuthForTest(t *testing.T, switchSvc *services.SwitchService, wantEmail string) services.WindsurfAuthJSON {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		authPath, err := switchSvc.GetWindsurfAuthPath()
		if err == nil {
			if raw, readErr := os.ReadFile(authPath); readErr == nil {
				var auth services.WindsurfAuthJSON
				if jsonErr := json.Unmarshal(raw, &auth); jsonErr == nil && auth.Email == wantEmail {
					return auth
				}
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	return readWindsurfAuthForTest(t, switchSvc)
}

func TestSyncMitmLocalAuthWritesWindsurfAuthFile(t *testing.T) {
	tmp := setupTestAuthEnv(t)

	s, err := store.NewStoreInPaths(filepath.Join(tmp, "WindsurfTools"))
	if err != nil {
		t.Fatalf("NewStoreInPaths() error = %v", err)
	}
	app := &App{
		store:     s,
		switchSvc: services.NewSwitchService(),
	}

	acc := models.Account{
		ID:             "acc-1",
		Email:          "user@example.com",
		Token:          "token-123",
		WindsurfAPIKey: "sk-ws-1",
	}

	if err := app.syncMitmLocalAuth(acc); err != nil {
		t.Fatalf("syncMitmLocalAuth() error = %v", err)
	}

	auth := readWindsurfAuthForTest(t, app.switchSvc)
	if auth.Token != "token-123" {
		t.Fatalf("auth.Token = %q, want %q", auth.Token, "token-123")
	}
	if auth.Email != "user@example.com" {
		t.Fatalf("auth.Email = %q, want %q", auth.Email, "user@example.com")
	}
}

func TestSyncMitmLocalAuthRequiresToken(t *testing.T) {
	app := &App{switchSvc: services.NewSwitchService()}

	err := app.syncMitmLocalAuth(models.Account{Email: "user@example.com"})
	if err == nil {
		t.Fatal("syncMitmLocalAuth() error = nil, want token error")
	}
}

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

func TestRotateMitmToNextAvailableSyncsLocalAuth(t *testing.T) {
	tmp := setupTestAuthEnv(t)

	s, err := store.NewStoreInPaths(filepath.Join(tmp, "WindsurfTools"))
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
		switchSvc: services.NewSwitchService(),
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
	auth := readWindsurfAuthForTest(t, app.switchSvc)
	if auth.Email != "b@example.com" || auth.Token != "token-b" {
		t.Fatalf("auth = %#v, want b@example.com/token-b", auth)
	}
}

func TestHandleMitmCurrentKeyChangedSyncsLocalAuthAfterAutoRotate(t *testing.T) {
	tmp := setupTestAuthEnv(t)

	s, err := store.NewStoreInPaths(filepath.Join(tmp, "WindsurfTools"))
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
		switchSvc: services.NewSwitchService(),
		mitmProxy: services.NewMitmProxy(nil, nil, "", nil),
	}
	app.syncMitmPoolKeys()
	if ok := app.mitmProxy.SwitchToKey("sk-ws-a"); !ok {
		t.Fatal("SwitchToKey(sk-ws-a) = false, want true")
	}
	if err := app.syncMitmLocalAuth(accounts[0]); err != nil {
		t.Fatalf("syncMitmLocalAuth(current) error = %v", err)
	}

	app.handleMitmCurrentKeyChanged("sk-ws-b", services.MitmCurrentKeyChangeReasonRateLimitRotate)

	// ★ shouldSyncMitmLocalSessionOnKeyChange() = false
	// MITM 按 conversation_id 路由，自动轮转不修改本地登录态
	auth := readWindsurfAuthForTest(t, app.switchSvc)
	if auth.Email != "a@example.com" || auth.Token != "token-a" {
		t.Fatalf("auth should stay unchanged after auto-rotate: got %#v", auth)
	}
}
