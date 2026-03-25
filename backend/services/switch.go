package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	return WriteAuthFile(authPath, token, email)
}

// WriteAuthFile writes a Windsurf auth payload to the given path.
// Windows 下兼容管理员 Windsurf 锁定文件的情况：先尝试直写，失败则用临时文件+重命名。
func WriteAuthFile(authPath, token, email string) error {
	if strings.TrimSpace(authPath) == "" {
		return fmt.Errorf("auth 路径为空")
	}

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

	auth := WindsurfAuthJSON{
		Token:     token,
		Email:     email,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化auth失败: %w", err)
	}

	// ① 直接写入
	if err := os.WriteFile(authPath, data, 0644); err == nil {
		return verifyAuthFileWrite(authPath, token)
	} else {
		log.Printf("[切号] 直写auth失败(%v)，尝试临时文件+重命名", err)
	}

	// ② 直写失败（文件被 Windsurf 锁定 / 权限不足）→ 临时文件 + rename
	tmpPath := authPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		// ③ 临时文件也写不了，Windows 上用 PowerShell 强制写入
		if runtime.GOOS == "windows" {
			if psErr := writeFileViaPowerShell(authPath, data); psErr != nil {
				return fmt.Errorf("写入auth文件失败（直写/tmp/PS均失败）: %w", psErr)
			}
			return verifyAuthFileWrite(authPath, token)
		}
		return fmt.Errorf("写入临时auth文件失败: %w", err)
	}

	// 尝试重命名覆盖（Windows 下比直写更容易绕过文件锁）
	if err := os.Rename(tmpPath, authPath); err != nil {
		_ = os.Remove(tmpPath)
		// rename 也失败，Windows 上用 PowerShell
		if runtime.GOOS == "windows" {
			if psErr := writeFileViaPowerShell(authPath, data); psErr != nil {
				return fmt.Errorf("重命名+PS写入auth均失败: %w", psErr)
			}
			return verifyAuthFileWrite(authPath, token)
		}
		return fmt.Errorf("重命名auth文件失败: %w", err)
	}

	return verifyAuthFileWrite(authPath, token)
}

// verifyAuthFileWrite 写入后回读验证 token 是否确实落盘。
func verifyAuthFileWrite(authPath, expectToken string) error {
	raw, err := os.ReadFile(authPath)
	if err != nil {
		return fmt.Errorf("auth写入后回读失败: %w", err)
	}
	var check WindsurfAuthJSON
	if err := json.Unmarshal(raw, &check); err != nil {
		return fmt.Errorf("auth写入后解析失败: %w", err)
	}
	if check.Token != expectToken {
		return errors.New("auth写入验证失败：回读 token 与写入值不一致，文件可能被 Windsurf 覆盖")
	}
	return nil
}

// writeFileViaPowerShell Windows 下通过 PowerShell Set-Content 写入（绕过部分文件锁场景）。
func writeFileViaPowerShell(filePath string, data []byte) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("非 Windows 平台")
	}
	// 用 [IO.File]::WriteAllText 强制写入，比 Set-Content 更可靠
	escaped := strings.ReplaceAll(string(data), "'", "''")
	ps := fmt.Sprintf(`[IO.File]::WriteAllText('%s','%s')`, filePath, escaped)
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("PowerShell写入失败: %w, output: %s", err, string(out))
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
		scheme := strings.SplitN(u, "://", 2)[0]
		if runtime.GOOS == "windows" && !isProtocolHandlerRegistered(scheme) {
			continue
		}
		tryOpenURL(u)
		time.Sleep(50 * time.Millisecond)
	}
}

// isProtocolHandlerRegistered 检查 Windows 注册表中协议处理器是否存在（避免弹出「获取打开此链接的应用」）。
func isProtocolHandlerRegistered(scheme string) bool {
	if runtime.GOOS != "windows" {
		return true
	}
	out, err := exec.Command("reg", "query", `HKCR\`+scheme, "/ve").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "url:")
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
