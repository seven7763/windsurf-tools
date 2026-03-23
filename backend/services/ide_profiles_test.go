package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildIDEProfilePaths(t *testing.T) {
	baseDir := filepath.Join("C:", "tmp", "windsurf-tools")
	paths := BuildIDEProfilePaths(baseDir, "acc-123", "User.Name+tag@example.com")

	if paths.ProfileSlug == "" {
		t.Fatal("ProfileSlug should not be empty")
	}
	if strings.Contains(paths.ProfileSlug, "@") {
		t.Fatalf("ProfileSlug should be sanitized, got %q", paths.ProfileSlug)
	}
	if !strings.Contains(paths.AuthPath, ".codeium") {
		t.Fatalf("AuthPath should point to .codeium profile, got %q", paths.AuthPath)
	}
	if !strings.Contains(paths.UserDataDir, "Windsurf") {
		t.Fatalf("UserDataDir should contain Windsurf profile path, got %q", paths.UserDataDir)
	}
}

func TestPrepareIsolatedIDEProfileCopiesBootstrapFiles(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	var sourceUserDir string
	switch runtime.GOOS {
	case "windows":
		appData := filepath.Join(homeDir, "AppData", "Roaming")
		t.Setenv("APPDATA", appData)
		sourceUserDir = filepath.Join(appData, "Windsurf", "User")
	case "darwin":
		sourceUserDir = filepath.Join(homeDir, "Library", "Application Support", "Windsurf", "User")
	default:
		sourceUserDir = filepath.Join(homeDir, ".config", "Windsurf", "User")
	}

	for path, content := range map[string]string{
		filepath.Join(homeDir, ".codeium", "config.json"):                  `{"api_key":"sk-main","apiKey":"sk-main"}`,
		filepath.Join(homeDir, ".codeium", "installation_id"):              "install-id",
		filepath.Join(homeDir, ".codeium", "windsurf", "user_settings.pb"): "pb",
		filepath.Join(homeDir, ".codeium", "windsurf", "installation_id"):  "ws-install-id",
		filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json"):  `{"mcp":true}`,
		filepath.Join(sourceUserDir, "settings.json"):                      `{"update.mode":"none"}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}

	profile := BuildIDEProfilePaths(t.TempDir(), "acc-123", "user@example.com")
	if err := PrepareIsolatedIDEProfile(profile); err != nil {
		t.Fatalf("PrepareIsolatedIDEProfile() error = %v", err)
	}

	wantCopied := []string{
		filepath.Join(profile.HomeDir, ".codeium", "config.json"),
		filepath.Join(profile.HomeDir, ".codeium", "installation_id"),
		filepath.Join(profile.HomeDir, ".codeium", "windsurf", "user_settings.pb"),
		filepath.Join(profile.HomeDir, ".codeium", "windsurf", "installation_id"),
		filepath.Join(profile.UserDataDir, "User", "settings.json"),
	}
	for _, path := range wantCopied {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected bootstrap file %q to exist: %v", path, err)
		}
	}
}

func TestWriteIDEProfileMetadata(t *testing.T) {
	profile := BuildIDEProfilePaths(t.TempDir(), "acc-123", "user@example.com")
	if err := WriteIDEProfileMetadata(profile, "acc-123", "user@example.com"); err != nil {
		t.Fatalf("WriteIDEProfileMetadata() error = %v", err)
	}

	data, err := os.ReadFile(profile.MetadataPath())
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", profile.MetadataPath(), err)
	}
	var meta IDEProfileMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if meta.AccountID != "acc-123" {
		t.Fatalf("AccountID = %q, want acc-123", meta.AccountID)
	}
	if meta.Email != "user@example.com" {
		t.Fatalf("Email = %q, want user@example.com", meta.Email)
	}
	if filepath.Clean(meta.UserDataDir) != filepath.Clean(profile.UserDataDir) {
		t.Fatalf("UserDataDir = %q, want %q", meta.UserDataDir, profile.UserDataDir)
	}
}

func TestProfileCommandLineMatches(t *testing.T) {
	target := normalizeProfilePath(`C:\Users\Hei\AppData\Roaming\WindsurfTools\ide_profiles\user-a\Roaming\Windsurf`)
	line := `C:\Users\Hei\AppData\Local\Programs\Windsurf\Windsurf.exe --new-window --user-data-dir=C:\Users\Hei\AppData\Roaming\WindsurfTools\ide_profiles\user-a\Roaming\Windsurf`
	if !profileCommandLineMatches(line, target) {
		t.Fatal("profileCommandLineMatches() = false, want true")
	}
	if profileCommandLineMatches(`C:\Users\Hei\AppData\Local\Programs\Windsurf\Windsurf.exe --new-window`, target) {
		t.Fatal("profileCommandLineMatches() = true for command without user-data-dir, want false")
	}
}

func TestParseCommandLinesJSON(t *testing.T) {
	lines, err := parseCommandLinesJSON([]byte(`["cmd-a","cmd-b"]`))
	if err != nil {
		t.Fatalf("parseCommandLinesJSON(array) error = %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("parseCommandLinesJSON(array) len = %d, want 2", len(lines))
	}

	single, err := parseCommandLinesJSON([]byte(`"cmd-a"`))
	if err != nil {
		t.Fatalf("parseCommandLinesJSON(single) error = %v", err)
	}
	if len(single) != 1 || single[0] != "cmd-a" {
		t.Fatalf("parseCommandLinesJSON(single) = %#v, want [cmd-a]", single)
	}
}
