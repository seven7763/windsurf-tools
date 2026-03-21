package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SwitchService handles seamless account switching
type SwitchService struct{}

func NewSwitchService() *SwitchService {
	return &SwitchService{}
}

// WindsurfAuthJSON is the structure of windsurf_auth.json
type WindsurfAuthJSON struct {
	Token     string `json:"token"`
	Email     string `json:"email,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// windsurfAuthPathCandidates 返回可能的 windsurf_auth.json 路径（顺序：优先 ~/.codeium，其次平台备选）。
// macOS 部分安装/版本会把会话写在 Application Support 下，需逐个探测。
func (s *SwitchService) windsurfAuthPathCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		base := filepath.Join(appdata, ".codeium", "windsurf", "config")
		return []string{filepath.Join(base, "windsurf_auth.json")}
	case "darwin":
		return []string{
			filepath.Join(home, ".codeium", "windsurf", "config", "windsurf_auth.json"),
			filepath.Join(home, "Library", "Application Support", "Windsurf", "User", "globalStorage", "windsurf_auth.json"),
		}
	default:
		base := filepath.Join(home, ".codeium", "windsurf", "config")
		return []string{filepath.Join(base, "windsurf_auth.json")}
	}
}

// resolveAuthPath 优先返回已存在的 auth 文件路径，否则返回首选写入路径（与旧版行为一致：~/.codeium/...）。
func (s *SwitchService) resolveAuthPath() (string, error) {
	cands := s.windsurfAuthPathCandidates()
	if len(cands) == 0 {
		return "", fmt.Errorf("无法解析用户主目录")
	}
	for _, p := range cands {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return cands[0], nil
}

// GetWindsurfAuthPath 返回当前用于读写的 windsurf_auth.json 路径（与 GetCurrentAuth / SwitchAccount 一致）。
func (s *SwitchService) GetWindsurfAuthPath() (string, error) {
	return s.resolveAuthPath()
}

// SwitchAccount writes the token into windsurf_auth.json for seamless switching
func (s *SwitchService) SwitchAccount(token, email string) error {
	authPath, err := s.resolveAuthPath()
	if err != nil {
		return fmt.Errorf("获取auth路径失败: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(authPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// Backup existing file
	if _, err := os.Stat(authPath); err == nil {
		backupPath := authPath + fmt.Sprintf(".bak.%d", time.Now().Unix())
		if data, err := os.ReadFile(authPath); err == nil {
			_ = os.WriteFile(backupPath, data, 0644)
		}
	}

	// Write new auth
	auth := WindsurfAuthJSON{
		Token:     token,
		Email:     email,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化auth失败: %w", err)
	}

	if err := os.WriteFile(authPath, data, 0644); err != nil {
		return fmt.Errorf("写入auth文件失败: %w", err)
	}

	return nil
}

// GetCurrentAuth 读取当前登录会话：按候选路径依次尝试，兼容 macOS 多安装路径。
func (s *SwitchService) GetCurrentAuth() (*WindsurfAuthJSON, error) {
	cands := s.windsurfAuthPathCandidates()
	if len(cands) == 0 {
		return nil, fmt.Errorf("无法解析用户主目录")
	}
	var lastErr error
	for _, authPath := range cands {
		data, err := os.ReadFile(authPath)
		if err != nil {
			lastErr = err
			continue
		}
		var auth WindsurfAuthJSON
		if err := json.Unmarshal(data, &auth); err != nil {
			return nil, fmt.Errorf("解析auth文件失败: %w", err)
		}
		return &auth, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("读取auth文件失败: %w", lastErr)
	}
	return nil, fmt.Errorf("未找到 windsurf_auth")
}

// TryOpenWindsurfRefreshURIs 尝试唤起 Windsurf 内置的会话刷新（扩展里注册的 path，具体 scheme 随版本可能变化；失败可忽略）。
func (s *SwitchService) TryOpenWindsurfRefreshURIs() {
	candidates := []string{
		"windsurf://refresh-authentication-session",
		"windsurf://windsurf/refresh-authentication-session",
		"codeium://refresh-authentication-session",
	}
	for _, u := range candidates {
		tryOpenURL(u)
		time.Sleep(150 * time.Millisecond)
	}
}

func tryOpenURL(u string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		_ = exec.Command("open", u).Start()
	default:
		_ = exec.Command("xdg-open", u).Start()
	}
}

// WindsurfInstallExePath 解析安装根目录或 .exe 路径，返回 Windows 下 Windsurf.exe 绝对路径；非 Windows 返回空。
func WindsurfInstallExePath(installRoot string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	root := strings.TrimSpace(installRoot)
	if root == "" {
		return ""
	}
	if strings.EqualFold(filepath.Ext(root), ".exe") {
		if _, err := os.Stat(root); err == nil {
			return root
		}
		return ""
	}
	exe := filepath.Join(root, "Windsurf.exe")
	if _, err := os.Stat(exe); err == nil {
		return exe
	}
	return ""
}
