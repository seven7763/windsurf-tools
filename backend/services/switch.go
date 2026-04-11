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
	configRoot, configErr := os.UserConfigDir()
	if configErr != nil || strings.TrimSpace(configRoot) == "" {
		configRoot = filepath.Join(home, ".config")
	}
	return windsurfAuthPathCandidatesFor(runtime.GOOS, home, os.Getenv("APPDATA"), configRoot)
}

func windsurfAuthPathCandidatesFor(goos, home, appData, configRoot string) []string {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	configRoot = strings.TrimSpace(configRoot)
	if configRoot == "" {
		configRoot = filepath.Join(home, ".config")
	}

	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "windows":
		if strings.TrimSpace(appData) == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return uniqueCandidatePaths([]string{
			filepath.Join(appData, ".codeium", "windsurf", "config", "windsurf_auth.json"),
			filepath.Join(appData, "Windsurf", "User", "globalStorage", "windsurf_auth.json"),
			filepath.Join(appData, "Codeium", "User", "globalStorage", "windsurf_auth.json"),
		})
	case "darwin":
		return uniqueCandidatePaths([]string{
			filepath.Join(home, ".codeium", "windsurf", "config", "windsurf_auth.json"),
			filepath.Join(home, "Library", "Application Support", "Windsurf", "User", "globalStorage", "windsurf_auth.json"),
		})
	default:
		return uniqueCandidatePaths([]string{
			filepath.Join(home, ".codeium", "windsurf", "config", "windsurf_auth.json"),
			filepath.Join(configRoot, "Windsurf", "User", "globalStorage", "windsurf_auth.json"),
			filepath.Join(configRoot, "windsurf", "User", "globalStorage", "windsurf_auth.json"),
			filepath.Join(configRoot, "Codeium", "User", "globalStorage", "windsurf_auth.json"),
			filepath.Join(configRoot, "codeium", "User", "globalStorage", "windsurf_auth.json"),
		})
	}
}

func uniqueCandidatePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "." || p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func preferredAuthWritePathFor(goos string, cands []string) string {
	if len(cands) == 0 {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "linux":
		for _, p := range cands {
			normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(p)))
			if strings.Contains(normalized, "/windsurf/user/globalstorage/windsurf_auth.json") {
				return p
			}
		}
	}
	return cands[0]
}

// resolveAuthPath 优先返回已存在的 auth 文件路径，否则返回首选写入路径。
// Linux/Kali 首次安装更偏向 Windsurf 的 globalStorage 位置，避免写回旧的 ~/.codeium 路径。
func (s *SwitchService) resolveAuthPath() (string, error) {
	cands := s.windsurfAuthPathCandidates()
	if len(cands) == 0 {
		return "", fmt.Errorf("无法解析用户主目录")
	}
	best, err := s.resolveBestAuthState()
	if err == nil && best != nil {
		return best.Path, nil
	}
	return preferredAuthWritePathFor(runtime.GOOS, cands), nil
}

// GetWindsurfAuthPath 返回当前用于读写的 windsurf_auth.json 路径（与 GetCurrentAuth / SwitchAccount 一致）。
func (s *SwitchService) GetWindsurfAuthPath() (string, error) {
	return s.resolveAuthPath()
}

// SwitchAccount writes the token into windsurf_auth.json for seamless switching
func (s *SwitchService) SwitchAccount(token, email string) error {
	cands := s.windsurfAuthPathCandidates()
	if len(cands) == 0 {
		return fmt.Errorf("获取auth路径失败: 无法解析用户主目录")
	}

	// ★ 读取旧邮箱，只有切换到不同邮箱时才清除 state.vscdb 的旧 session
	// 同邮箱换 token 不清，避免 IDE 重启后需要重新登录
	oldEmail := ""
	if oldAuth, err := s.GetCurrentAuth(); err == nil && oldAuth != nil {
		oldEmail = strings.TrimSpace(strings.ToLower(oldAuth.Email))
	}
	newEmail := strings.TrimSpace(strings.ToLower(email))

	var writeErrs []string
	for _, authPath := range cands {
		if err := WriteAuthFile(authPath, token, email); err != nil {
			writeErrs = append(writeErrs, fmt.Sprintf("%s: %v", authPath, err))
		}
	}
	if len(writeErrs) > 0 {
		return fmt.Errorf("写入auth文件失败: %s", strings.Join(writeErrs, "; "))
	}

	// 只有邮箱变化时才清除旧 session 缓存
	if newEmail != oldEmail {
		log.Printf("[切号] 邮箱变更 %s → %s，清理旧会话缓存", oldEmail, newEmail)
		if err := s.clearWindsurfSessionCache(email); err != nil {
			return fmt.Errorf("清理 Windsurf 本地会话缓存失败: %w", err)
		}
	} else {
		log.Printf("[切号] 同邮箱 %s 换 token，跳过会话缓存清理", newEmail)
	}
	return nil
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
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("PowerShell写入失败: %w, output: %s", err, string(out))
	}
	return nil
}

// ── MITM 原始登录态 备份/恢复 ──

const mitmOrigAuthSuffix = ".mitm_original"

// BackupOriginalAuth 在 MITM 启动时调用，备份所有 windsurf_auth.json 的原始内容。
// 如果备份文件已存在则跳过（避免多次启动覆盖真正的原始态）。
func (s *SwitchService) BackupOriginalAuth() {
	for _, authPath := range s.windsurfAuthPathCandidates() {
		backupPath := authPath + mitmOrigAuthSuffix
		if _, err := os.Stat(backupPath); err == nil {
			continue // 已备份，跳过
		}
		data, err := os.ReadFile(authPath)
		if err != nil {
			continue // 文件不存在
		}
		_ = os.WriteFile(backupPath, data, 0644)
		log.Printf("[切号] 已备份原始 auth: %s", authPath)
	}
}

// RestoreOriginalAuth 在 MITM 退出时调用，恢复所有 windsurf_auth.json 为 MITM 启动前的内容。
func (s *SwitchService) RestoreOriginalAuth() {
	for _, authPath := range s.windsurfAuthPathCandidates() {
		backupPath := authPath + mitmOrigAuthSuffix
		data, err := os.ReadFile(backupPath)
		if err != nil {
			continue // 无备份
		}
		if err := os.WriteFile(authPath, data, 0644); err != nil {
			log.Printf("[切号] 恢复原始 auth 失败: %s: %v", authPath, err)
			continue
		}
		_ = os.Remove(backupPath)
		log.Printf("[切号] 已恢复原始 auth: %s", authPath)
	}
}

// GetCurrentAuth 读取当前登录会话：按候选路径依次尝试，兼容 macOS 多安装路径。
func (s *SwitchService) GetCurrentAuth() (*WindsurfAuthJSON, error) {
	best, err := s.resolveBestAuthState()
	if err != nil {
		return nil, err
	}
	if best == nil {
		return nil, fmt.Errorf("未找到 windsurf_auth")
	}
	return &best.Auth, nil
}

type authFileState struct {
	Path    string
	Auth    WindsurfAuthJSON
	ModTime time.Time
}

func (s *SwitchService) resolveBestAuthState() (*authFileState, error) {
	cands := s.windsurfAuthPathCandidates()
	if len(cands) == 0 {
		return nil, fmt.Errorf("无法解析用户主目录")
	}
	var best *authFileState
	var lastErr error
	for _, authPath := range cands {
		state, err := readAuthFileState(authPath)
		if err != nil {
			lastErr = err
			continue
		}
		if isNewerAuthState(best, state) {
			best = state
		}
	}
	if best != nil {
		return best, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("读取auth文件失败: %w", lastErr)
	}
	return nil, nil
}

func readAuthFileState(authPath string) (*authFileState, error) {
	data, err := os.ReadFile(authPath)
	if err != nil {
		return nil, err
	}
	var auth WindsurfAuthJSON
	if err := json.Unmarshal(data, &auth); err != nil {
		return nil, fmt.Errorf("%s: 解析auth文件失败: %w", authPath, err)
	}
	info, statErr := os.Stat(authPath)
	modTime := time.Time{}
	if statErr == nil {
		modTime = info.ModTime()
	}
	return &authFileState{
		Path:    authPath,
		Auth:    auth,
		ModTime: modTime,
	}, nil
}

func isNewerAuthState(current, candidate *authFileState) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	currentStamp := authStateTimestamp(current)
	candidateStamp := authStateTimestamp(candidate)
	if candidateStamp != currentStamp {
		return candidateStamp > currentStamp
	}
	return candidate.ModTime.After(current.ModTime)
}

func authStateTimestamp(state *authFileState) int64 {
	if state == nil {
		return 0
	}
	if state.Auth.Timestamp > 0 {
		return state.Auth.Timestamp
	}
	if !state.ModTime.IsZero() {
		return state.ModTime.Unix()
	}
	return 0
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
	cmd := exec.Command("reg", "query", `HKCR\`+scheme, "/ve")
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "url:")
}

func tryOpenURL(u string) {
	switch runtime.GOOS {
	case "windows":
		openCmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
		hideWindow(openCmd)
		_ = openCmd.Start()
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
