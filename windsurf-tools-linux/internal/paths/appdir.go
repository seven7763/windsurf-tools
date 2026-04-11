package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	AppDirName       = "WindsurfTools"
	LegacyAppDirName = "windsurf-tools-wails"
	EnvDataDir       = "WINDSURF_TOOLS_DATA_DIR"
)

func ResolveAppConfigDir(explicitDir string) (string, error) {
	if dir := strings.TrimSpace(explicitDir); dir != "" {
		return ensureDir(dir)
	}
	if dir := strings.TrimSpace(os.Getenv(EnvDataDir)); dir != "" {
		return ensureDir(dir)
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return resolveUnderConfigRoot(configRoot)
}

func ensureDir(dir string) (string, error) {
	clean := filepath.Clean(dir)
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return "", fmt.Errorf("mkdir data dir: %w", err)
	}
	return clean, nil
}

func resolveUnderConfigRoot(configRoot string) (string, error) {
	preferred := filepath.Join(configRoot, AppDirName)
	legacy := filepath.Join(configRoot, LegacyAppDirName)

	switch {
	case hasAnyDataFile(preferred):
		return ensureDir(preferred)
	case hasAnyDataFile(legacy):
		if err := migrateLegacyToPreferred(legacy, preferred); err != nil {
			return ensureDir(legacy)
		}
		return ensureDir(preferred)
	default:
		return ensureDir(preferred)
	}
}

func hasAnyDataFile(dir string) bool {
	for _, name := range []string{"accounts.json", "settings.json"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func migrateLegacyToPreferred(legacy string, preferred string) error {
	if _, err := ensureDir(preferred); err != nil {
		return err
	}
	for _, name := range []string{"accounts.json", "settings.json"} {
		src := filepath.Join(legacy, name)
		dst := filepath.Join(preferred, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	marker := filepath.Join(legacy, ".migrated_to_WindsurfTools")
	_ = os.WriteFile(marker, []byte(preferred+"\n"), 0o644)
	return nil
}
