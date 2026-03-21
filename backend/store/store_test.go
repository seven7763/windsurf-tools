package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"windsurf-tools-wails/backend/models"
)

func TestGetAccountReturnsIndependentCopy(t *testing.T) {
	dir := t.TempDir()
	accPath := filepath.Join(dir, "accounts.json")
	settingsPath := filepath.Join(dir, "settings.json")
	rawAcc := []models.Account{
		{ID: "a1", Email: "orig@example.com", PlanName: "Pro"},
	}
	b, err := json.MarshalIndent(rawAcc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(accPath, b, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Store{
		dataDir:      dir,
		accountsFile: accPath,
		settingsFile: settingsPath,
		accounts:     make([]models.Account, 0),
		settings:     models.DefaultSettings(),
	}
	s.load()

	got1, err := s.GetAccount("a1")
	if err != nil {
		t.Fatal(err)
	}
	got1.Email = "mutated@example.com"

	got2, err := s.GetAccount("a1")
	if err != nil {
		t.Fatal(err)
	}
	if got2.Email != "orig@example.com" {
		t.Fatalf("GetAccount 应返回拷贝，修改返回值不应写回存储: got %q", got2.Email)
	}
}

func TestNewStoreInPaths(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStoreInPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAccount("none"); err == nil {
		t.Fatal("expect error for missing id")
	}
	if s.DataDir() != dir {
		t.Fatalf("DataDir: got %q want %q", s.DataDir(), dir)
	}
}
