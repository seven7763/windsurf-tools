// Package paths 集中管理本应用跨平台数据目录（号池 accounts.json、settings.json 等）。
//
// 目录约定（均位于 os.UserConfigDir() 下，随系统而变）：
//   - Windows:  %APPDATA%\WindsurfTools
//   - macOS:    ~/Library/Application Support/WindsurfTools
//   - Linux:    $XDG_CONFIG_HOME 或 ~/.config/WindsurfTools
//
// 旧版目录名 windsurf-tools-wails 会在首次启动时自动迁移至 WindsurfTools。
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// AppDirName 号池与设置根目录（无「wails」后缀，便于在访达/资源管理器中识别）
	AppDirName = "WindsurfTools"
	// LegacyAppDirName 历史版本目录，仅用于一次性迁移
	LegacyAppDirName = "windsurf-tools-wails"
)

func hasAnyDataFile(dir string) bool {
	for _, name := range []string{"accounts.json", "settings.json"} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

// copyFile copies src to dst with mode 0644; overwrites dst if exists.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// migrateLegacyToPreferred copies accounts/settings from legacy to preferred when needed.
func migrateLegacyToPreferred(legacy, preferred string) error {
	if err := os.MkdirAll(preferred, 0755); err != nil {
		return fmt.Errorf("mkdir preferred: %w", err)
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
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
	}
	marker := filepath.Join(legacy, ".migrated_to_WindsurfTools")
	_ = os.WriteFile(marker, []byte(preferred+"\n"), 0644)
	return nil
}

// resolveUnderConfigRoot 在给定「配置根」（如 UserConfigDir）下解析应用数据目录，便于单测注入临时目录。
func resolveUnderConfigRoot(configRoot string) (string, error) {
	preferred := filepath.Join(configRoot, AppDirName)
	legacy := filepath.Join(configRoot, LegacyAppDirName)

	switch {
	case hasAnyDataFile(preferred):
		return preferred, nil
	case hasAnyDataFile(legacy):
		if err := migrateLegacyToPreferred(legacy, preferred); err != nil {
			// 迁移失败时继续使用旧目录，避免丢数据
			return legacy, nil
		}
		return preferred, nil
	default:
		if err := os.MkdirAll(preferred, 0755); err != nil {
			return "", fmt.Errorf("mkdir app data: %w", err)
		}
		return preferred, nil
	}
}

// ResolveAppConfigDir 返回号池与全局设置所在目录；若仅有旧版目录则迁移后返回新路径。
func ResolveAppConfigDir() (string, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return resolveUnderConfigRoot(configRoot)
}
