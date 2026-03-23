package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type IDEProfilePaths struct {
	ProfileSlug     string
	RootDir         string
	HomeDir         string
	AppDataDir      string
	LocalAppDataDir string
	UserDataDir     string
	AuthPath        string
}

type IDEProfileMetadata struct {
	AccountID    string `json:"account_id"`
	Email        string `json:"email"`
	ProfileSlug  string `json:"profile_slug"`
	RootDir      string `json:"root_dir"`
	UserDataDir  string `json:"user_data_dir"`
	LastOpenedAt string `json:"last_opened_at"`
}

func (p IDEProfilePaths) MetadataPath() string {
	return filepath.Join(p.RootDir, "profile.json")
}

var profileSlugSanitizer = regexp.MustCompile(`[^a-z0-9._-]+`)

func BuildIDEProfilePaths(baseDir, accountID, email string) IDEProfilePaths {
	slugSource := strings.TrimSpace(strings.ToLower(email))
	if slugSource == "" {
		slugSource = strings.TrimSpace(strings.ToLower(accountID))
	}
	if slugSource == "" {
		slugSource = "account"
	}
	slugSource = strings.ReplaceAll(slugSource, "@", "-")
	slugSource = strings.ReplaceAll(slugSource, string(filepath.Separator), "-")
	slug := profileSlugSanitizer.ReplaceAllString(slugSource, "-")
	slug = strings.Trim(slug, "-._")
	if slug == "" {
		slug = "account"
	}
	if len(slug) > 48 {
		slug = slug[:48]
	}

	rootDir := filepath.Join(baseDir, "ide_profiles", slug)
	homeDir := filepath.Join(rootDir, "home")
	appDataDir := filepath.Join(rootDir, "Roaming")
	localAppDataDir := filepath.Join(rootDir, "Local")

	authPath := filepath.Join(appDataDir, ".codeium", "windsurf", "config", "windsurf_auth.json")
	userDataDir := filepath.Join(appDataDir, "Windsurf")
	if runtime.GOOS == "darwin" {
		userDataDir = filepath.Join(homeDir, "Library", "Application Support", "Windsurf")
		authPath = filepath.Join(homeDir, ".codeium", "windsurf", "config", "windsurf_auth.json")
	}

	return IDEProfilePaths{
		ProfileSlug:     slug,
		RootDir:         rootDir,
		HomeDir:         homeDir,
		AppDataDir:      appDataDir,
		LocalAppDataDir: localAppDataDir,
		UserDataDir:     userDataDir,
		AuthPath:        authPath,
	}
}

func LaunchWindsurfWithProfile(installRoot string, profile IDEProfilePaths) error {
	switch runtime.GOOS {
	case "windows":
		exePath := WindsurfInstallExePath(installRoot)
		if exePath == "" {
			localAppData := os.Getenv("LOCALAPPDATA")
			exePath = filepath.Join(localAppData, "Programs", "Windsurf", "Windsurf.exe")
		}
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("未找到 Windsurf 可执行文件: %w", err)
		}
		if err := os.MkdirAll(profile.UserDataDir, 0755); err != nil {
			return fmt.Errorf("创建 profile 目录失败: %w", err)
		}
		cmd := exec.Command(exePath, "--new-window", "--user-data-dir="+profile.UserDataDir)
		cmd.Env = append(os.Environ(),
			"APPDATA="+profile.AppDataDir,
			"LOCALAPPDATA="+profile.LocalAppDataDir,
			"USERPROFILE="+profile.HomeDir,
			"HOME="+profile.HomeDir,
		)
		hideWindow(cmd)
		return cmd.Start()
	case "darwin":
		if err := os.MkdirAll(profile.UserDataDir, 0755); err != nil {
			return fmt.Errorf("创建 profile 目录失败: %w", err)
		}
		cmd := exec.Command("open", "-na", "Windsurf", "--args", "--new-window", "--user-data-dir="+profile.UserDataDir)
		cmd.Env = append(os.Environ(), "HOME="+profile.HomeDir)
		return cmd.Start()
	default:
		if err := os.MkdirAll(profile.UserDataDir, 0755); err != nil {
			return fmt.Errorf("创建 profile 目录失败: %w", err)
		}
		cmd := exec.Command("windsurf", "--new-window", "--user-data-dir="+profile.UserDataDir)
		cmd.Env = append(os.Environ(), "HOME="+profile.HomeDir)
		hideWindow(cmd)
		return cmd.Start()
	}
}

func WriteIDEProfileMetadata(profile IDEProfilePaths, accountID, email string) error {
	data, err := json.MarshalIndent(IDEProfileMetadata{
		AccountID:    strings.TrimSpace(accountID),
		Email:        strings.TrimSpace(email),
		ProfileSlug:  profile.ProfileSlug,
		RootDir:      filepath.Clean(profile.RootDir),
		UserDataDir:  filepath.Clean(profile.UserDataDir),
		LastOpenedAt: time.Now().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化独立 profile 元数据失败: %w", err)
	}
	if err := os.MkdirAll(profile.RootDir, 0755); err != nil {
		return fmt.Errorf("创建独立 profile 根目录失败: %w", err)
	}
	if err := os.WriteFile(profile.MetadataPath(), data, 0644); err != nil {
		return fmt.Errorf("写入独立 profile 元数据失败: %w", err)
	}
	return nil
}

func IsIDEProfileRunning(profile IDEProfilePaths) (bool, error) {
	target := normalizeProfilePath(profile.UserDataDir)
	if target == "" {
		return false, nil
	}
	cmdlines, err := windsurfProcessCommandLines()
	if err != nil {
		return false, err
	}
	for _, line := range cmdlines {
		if profileCommandLineMatches(line, target) {
			return true, nil
		}
	}
	return false, nil
}

func windsurfProcessCommandLines() ([]string, error) {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("powershell", "-NoProfile", "-Command", "(Get-CimInstance Win32_Process -Filter \"Name = 'Windsurf.exe'\" | Select-Object -ExpandProperty CommandLine | ConvertTo-Json -Compress)")
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("读取 Windsurf 进程命令行失败: %w", err)
		}
		return parseCommandLinesJSON(out)
	case "darwin", "linux":
		out, err := exec.Command("ps", "-ax", "-o", "command=").Output()
		if err != nil {
			return nil, fmt.Errorf("读取 Windsurf 进程列表失败: %w", err)
		}
		lines := strings.Split(string(out), "\n")
		outLines := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				outLines = append(outLines, line)
			}
		}
		return outLines, nil
	default:
		return nil, nil
	}
}

func parseCommandLinesJSON(data []byte) ([]string, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
		return arr, nil
	}
	var single string
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
		if strings.TrimSpace(single) == "" {
			return nil, nil
		}
		return []string{single}, nil
	}
	return nil, fmt.Errorf("无法解析 Windsurf 进程命令行")
}

func profileCommandLineMatches(commandLine, normalizedUserDataDir string) bool {
	line := normalizeProfilePath(commandLine)
	if line == "" || normalizedUserDataDir == "" {
		return false
	}
	if !strings.Contains(line, "--user-data-dir") {
		return false
	}
	return strings.Contains(line, normalizedUserDataDir)
}

func normalizeProfilePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = filepath.Clean(value)
	value = strings.ReplaceAll(value, "\\", "/")
	return strings.ToLower(value)
}
