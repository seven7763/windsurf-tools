package services

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PrepareIsolatedIDEProfile bootstraps the minimal user-state files required by
// Windsurf before launching an isolated window. It intentionally copies only a
// small whitelist so each profile keeps an independent login state.
func PrepareIsolatedIDEProfile(profile IDEProfilePaths) error {
	for _, dir := range []string{
		profile.RootDir,
		profile.HomeDir,
		profile.AppDataDir,
		profile.LocalAppDataDir,
		profile.UserDataDir,
	} {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建隔离 profile 目录失败 (%s): %w", dir, err)
		}
	}

	if err := bootstrapCodeiumProfile(profile); err != nil {
		return err
	}
	if err := bootstrapWindsurfUserProfile(profile); err != nil {
		return err
	}
	return nil
}

func bootstrapCodeiumProfile(profile IDEProfilePaths) error {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}

	srcRoot := filepath.Join(home, ".codeium")
	dstRoot := filepath.Join(profile.HomeDir, ".codeium")
	relPaths := []string{
		"config.json",
		"installation_id",
		filepath.Join("memories", "global_rules.md"),
		filepath.Join("windsurf", "installation_id"),
		filepath.Join("windsurf", "mcp_config.json"),
		filepath.Join("windsurf", "native_storage_migrations.lock"),
		filepath.Join("windsurf", "onboarding.json"),
		filepath.Join("windsurf", "onboarding.json.lock"),
		filepath.Join("windsurf", "user_settings.pb"),
		filepath.Join("windsurf", "memories", "global_rules.md"),
	}

	for _, rel := range relPaths {
		if err := copyFileIfExists(filepath.Join(srcRoot, rel), filepath.Join(dstRoot, rel)); err != nil {
			return fmt.Errorf("同步独立开窗 Codeium bootstrap 失败 (%s): %w", rel, err)
		}
	}
	return nil
}

func bootstrapWindsurfUserProfile(profile IDEProfilePaths) error {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}

	srcUserDir := currentWindsurfUserDir(home)
	dstUserDir := filepath.Join(profile.UserDataDir, "User")
	for _, rel := range []string{"settings.json", "argv.json"} {
		if err := copyFileIfExists(filepath.Join(srcUserDir, rel), filepath.Join(dstUserDir, rel)); err != nil {
			return fmt.Errorf("同步独立开窗 Windsurf 用户设置失败 (%s): %w", rel, err)
		}
	}
	return nil
}

func currentWindsurfUserDir(home string) string {
	switch runtime.GOOS {
	case "windows":
		appData := strings.TrimSpace(os.Getenv("APPDATA"))
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Windsurf", "User")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Windsurf", "User")
	default:
		return filepath.Join(home, ".config", "Windsurf", "User")
	}
}

func copyFileIfExists(src, dst string) error {
	if strings.TrimSpace(src) == "" || strings.TrimSpace(dst) == "" {
		return nil
	}

	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
