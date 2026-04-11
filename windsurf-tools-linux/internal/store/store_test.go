package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"windsurf-tools-linux/internal/models"
)

func TestGetAccountReturnsCopy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeAccountsFile(t, dir, []models.Account{{ID: "a1", Email: "alpha@example.com", PlanName: "pro"}})

	s, err := NewInDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAccount("a1")
	if err != nil {
		t.Fatal(err)
	}
	got.Email = "mutated@example.com"

	again, err := s.GetAccount("a1")
	if err != nil {
		t.Fatal(err)
	}
	if again.Email != "alpha@example.com" {
		t.Fatalf("GetAccount() leaked internal state: got %q", again.Email)
	}
}

func TestSaveAccountCreatesAndUpdates(t *testing.T) {
	t.Parallel()

	s, err := NewInDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	created, err := s.SaveAccount(models.Account{Email: "alpha@example.com", PlanName: "trial"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("SaveAccount() should assign an ID")
	}

	created.Nickname = "Alpha"
	updated, err := s.SaveAccount(created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Nickname != "Alpha" {
		t.Fatalf("SaveAccount() update nickname = %q, want %q", updated.Nickname, "Alpha")
	}

	all := s.GetAllAccounts()
	if len(all) != 1 {
		t.Fatalf("len(GetAllAccounts()) = %d, want 1", len(all))
	}
}

func TestSaveAccountRejectsDuplicateIdentity(t *testing.T) {
	t.Parallel()

	s, err := NewInDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccount(models.Account{Email: "alpha@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAccount(models.Account{Email: "ALPHA@example.com"}); err == nil {
		t.Fatal("expected duplicate identity error")
	}
}

func TestDeleteAccountRemovesEntry(t *testing.T) {
	t.Parallel()

	s, err := NewInDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.SaveAccount(models.Account{Email: "alpha@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAccount(created.ID); err == nil {
		t.Fatal("expected missing account after delete")
	}
}

func TestSaveAccountUpdateRejectsConflictWithAnotherRecord(t *testing.T) {
	t.Parallel()

	s, err := NewInDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.SaveAccount(models.Account{Email: "alpha@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveAccount(models.Account{Email: "beta@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	second.Email = first.Email
	if _, err := s.SaveAccount(second); err == nil {
		t.Fatal("expected duplicate conflict when updating into another record's identity")
	}
}

func TestLoadSettingsIgnoresLegacyFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	raw := []byte(`{"mitm_proxy_enabled":true,"mitm_proxy_port":8443,"seamless_switch":true,"proxy_enabled":true}`)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := NewInDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := s.GetSettings()
	if !got.MitmProxyEnabled {
		t.Fatal("expected mitm_proxy_enabled to load")
	}
	if !got.ProxyEnabled {
		t.Fatal("expected proxy_enabled to load")
	}
}

func writeAccountsFile(t *testing.T, dir string, accounts []models.Account) {
	t.Helper()
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
