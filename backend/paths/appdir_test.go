package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveUnderConfigRoot_PrefersNewWhenPresent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, AppDirName), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, AppDirName, "accounts.json"), []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	dir, err := resolveUnderConfigRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, AppDirName)
	if filepath.Clean(dir) != want {
		t.Fatalf("got %q want %q", dir, want)
	}
}

func TestResolveUnderConfigRoot_MigratesLegacy(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, LegacyAppDirName)
	if err := os.MkdirAll(legacy, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "accounts.json"), []byte("[]"), 0644); err != nil {
		t.Fatal(err)
	}
	dir, err := resolveUnderConfigRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	preferred := filepath.Join(root, AppDirName)
	if filepath.Clean(dir) != preferred {
		t.Fatalf("got %q want %q", dir, preferred)
	}
	if _, err := os.Stat(filepath.Join(preferred, "accounts.json")); err != nil {
		t.Fatal("migration did not copy accounts.json:", err)
	}
}
