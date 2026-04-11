package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAppConfigDirUsesExplicitDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "custom")
	got, err := ResolveAppConfigDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("ResolveAppConfigDir() = %q, want %q", got, dir)
	}
}

func TestResolveUnderConfigRootPrefersNewDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	preferred := filepath.Join(root, AppDirName)
	if err := os.MkdirAll(preferred, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(preferred, "accounts.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveUnderConfigRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != preferred {
		t.Fatalf("resolveUnderConfigRoot() = %q, want %q", got, preferred)
	}
}

func TestResolveUnderConfigRootMigratesLegacyDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	legacy := filepath.Join(root, LegacyAppDirName)
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "accounts.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveUnderConfigRoot(root)
	if err != nil {
		t.Fatal(err)
	}

	preferred := filepath.Join(root, AppDirName)
	if got != preferred {
		t.Fatalf("resolveUnderConfigRoot() = %q, want %q", got, preferred)
	}
	if _, err := os.Stat(filepath.Join(preferred, "accounts.json")); err != nil {
		t.Fatalf("expected migrated accounts.json: %v", err)
	}
}
