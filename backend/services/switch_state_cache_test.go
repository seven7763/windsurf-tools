package services

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	testWindsurfSecretSessions   = `secret://{"extensionId":"codeium.windsurf","key":"windsurf_auth.sessions"}`
	testWindsurfSecretAPIServer  = `secret://{"extensionId":"codeium.windsurf","key":"windsurf_auth.apiServerUrl"}`
	testWindsurfHistoricalAuth   = "windsurf_auth-Old User"
	testWindsurfHistoricalUsages = "windsurf_auth-Old User-usages"
)

func TestSwitchAccountClearsWindsurfSessionCache(t *testing.T) {
	configureSwitchTestEnv(t)
	svc := NewSwitchService()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}
	roots := windsurfUserRootCandidatesFor(runtime.GOOS, homeDir, os.Getenv("APPDATA"), configRoot)
	if len(roots) == 0 {
		t.Fatal("windsurfUserRootCandidatesFor() returned no roots")
	}

	globalDBPath := filepath.Join(roots[0], "globalStorage", "state.vscdb")
	workspaceDBPath := filepath.Join(roots[0], "workspaceStorage", "ws-1", "state.vscdb")

	mainState, err := json.Marshal(map[string]any{
		"lastLoginEmail":                  "old@example.com",
		"windsurf.state.cachedUserStatus": map[string]any{"planName": "Pro"},
		"apiServerUrl":                    "https://api.windsurf.com",
		"keep":                            true,
	})
	if err != nil {
		t.Fatalf("Marshal(mainState) error = %v", err)
	}

	createStateDB(t, globalDBPath, map[string]string{
		windsurfStateKeyMainStore:    string(mainState),
		windsurfStateKeyCurrentAuth:  "Old User",
		windsurfStateKeyAuthStatus:   `{"apiKey":"old-key"}`,
		windsurfStateKeyPlanInfo:     `{"planName":"Pro"}`,
		testWindsurfSecretSessions:   `{"data":"[]","type":"secret"}`,
		testWindsurfSecretAPIServer:  `{"data":"https://api.windsurf.com","type":"secret"}`,
		testWindsurfHistoricalAuth:   `[]`,
		testWindsurfHistoricalUsages: `[{"extensionId":"codeium.windsurf"}]`,
	})
	createStateDB(t, workspaceDBPath, map[string]string{
		windsurfStateKeyCurrentAuth: "Old User",
	})

	if err := svc.SwitchAccount("jwt-token", "new@example.com"); err != nil {
		t.Fatalf("SwitchAccount() error = %v", err)
	}

	assertStateDBKeyMissing(t, globalDBPath, windsurfStateKeyCurrentAuth)
	assertStateDBKeyMissing(t, globalDBPath, windsurfStateKeyAuthStatus)
	assertStateDBKeyMissing(t, globalDBPath, windsurfStateKeyPlanInfo)
	assertStateDBKeyMissing(t, globalDBPath, testWindsurfSecretSessions)
	assertStateDBKeyMissing(t, globalDBPath, testWindsurfHistoricalAuth)
	assertStateDBKeyMissing(t, globalDBPath, testWindsurfHistoricalUsages)
	assertStateDBKeyPresent(t, globalDBPath, testWindsurfSecretAPIServer)
	assertStateDBKeyMissing(t, workspaceDBPath, windsurfStateKeyCurrentAuth)

	rawState := readStateDBValue(t, globalDBPath, windsurfStateKeyMainStore)
	var state map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawState), &state); err != nil {
		t.Fatalf("Unmarshal(codeium.windsurf) error = %v", err)
	}

	var lastLoginEmail string
	if err := json.Unmarshal(state["lastLoginEmail"], &lastLoginEmail); err != nil {
		t.Fatalf("Unmarshal(lastLoginEmail) error = %v", err)
	}
	if lastLoginEmail != "new@example.com" {
		t.Fatalf("lastLoginEmail = %q, want %q", lastLoginEmail, "new@example.com")
	}
	if _, ok := state["windsurf.state.cachedUserStatus"]; ok {
		t.Fatal("windsurf.state.cachedUserStatus should be removed")
	}
}

func createStateDB(t *testing.T, dbPath string, entries map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dbPath, err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(%q) error = %v", dbPath, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("CREATE TABLE %q error = %v", dbPath, err)
	}
	for key, value := range entries {
		if _, err := db.Exec(`INSERT OR REPLACE INTO ItemTable(key, value) VALUES(?, ?)`, key, value); err != nil {
			t.Fatalf("INSERT %q into %q error = %v", key, dbPath, err)
		}
	}
}

func assertStateDBKeyMissing(t *testing.T, dbPath, key string) {
	t.Helper()
	if _, ok := readStateDBValueIfPresent(t, dbPath, key); ok {
		t.Fatalf("expected %q to be removed from %q", key, dbPath)
	}
}

func assertStateDBKeyPresent(t *testing.T, dbPath, key string) {
	t.Helper()
	if _, ok := readStateDBValueIfPresent(t, dbPath, key); !ok {
		t.Fatalf("expected %q to remain in %q", key, dbPath)
	}
}

func readStateDBValue(t *testing.T, dbPath, key string) string {
	t.Helper()
	value, ok := readStateDBValueIfPresent(t, dbPath, key)
	if !ok {
		t.Fatalf("expected %q to exist in %q", key, dbPath)
	}
	return value
}

func readStateDBValueIfPresent(t *testing.T, dbPath, key string) (string, bool) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open(%q) error = %v", dbPath, err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	var value string
	err = db.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatalf("SELECT %q from %q error = %v", key, dbPath, err)
	}
	return value, true
}
