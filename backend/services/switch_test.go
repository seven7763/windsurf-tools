package services

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func configureSwitchTestEnv(t *testing.T) string {
	t.Helper()

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	switch runtime.GOOS {
	case "windows":
		t.Setenv("USERPROFILE", homeDir)
		appData := filepath.Join(homeDir, "AppData", "Roaming")
		t.Setenv("APPDATA", appData)
		return filepath.Join(appData, ".codeium", "windsurf", "config", "windsurf_auth.json")
	case "darwin":
		t.Setenv("USERPROFILE", homeDir)
		return filepath.Join(homeDir, ".codeium", "windsurf", "config", "windsurf_auth.json")
	default:
		t.Setenv("USERPROFILE", homeDir)
		return filepath.Join(homeDir, ".codeium", "windsurf", "config", "windsurf_auth.json")
	}
}

func TestSwitchServiceSwitchAccountAndGetCurrentAuth(t *testing.T) {
	wantPath := configureSwitchTestEnv(t)
	svc := NewSwitchService()

	if err := svc.SwitchAccount("jwt-token", "user@example.com"); err != nil {
		t.Fatalf("SwitchAccount() error = %v", err)
	}

	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("auth file not written to expected path %q: %v", wantPath, err)
	}

	auth, err := svc.GetCurrentAuth()
	if err != nil {
		t.Fatalf("GetCurrentAuth() error = %v", err)
	}
	if auth.Email != "user@example.com" {
		t.Fatalf("GetCurrentAuth().Email = %q, want %q", auth.Email, "user@example.com")
	}
	if auth.Token != "jwt-token" {
		t.Fatalf("GetCurrentAuth().Token = %q, want %q", auth.Token, "jwt-token")
	}
	if auth.Timestamp == 0 {
		t.Fatal("GetCurrentAuth().Timestamp should be populated")
	}
}

func TestSwitchServiceGetWindsurfAuthPathPrefersExistingFile(t *testing.T) {
	wantPath := configureSwitchTestEnv(t)
	if err := os.MkdirAll(filepath.Dir(wantPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wantPath, []byte(`{"token":"tok","email":"reader@example.com"}`), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewSwitchService()
	gotPath, err := svc.GetWindsurfAuthPath()
	if err != nil {
		t.Fatalf("GetWindsurfAuthPath() error = %v", err)
	}
	if filepath.Clean(gotPath) != filepath.Clean(wantPath) {
		t.Fatalf("GetWindsurfAuthPath() = %q, want %q", gotPath, wantPath)
	}
}
