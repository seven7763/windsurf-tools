package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// PatchService handles the seamless switching patch for Windsurf's extension.js
type PatchService struct{}

func NewPatchService() *PatchService {
	return &PatchService{}
}

// getExtensionJSRelativePath returns the platform-specific path to extension.js
func getExtensionJSRelativePath() string {
	if runtime.GOOS == "darwin" {
		return filepath.Join("Contents", "Resources", "app", "extensions", "windsurf", "dist", "extension.js")
	}
	// Windows / Linux
	return filepath.Join("resources", "app", "extensions", "windsurf", "dist", "extension.js")
}

// FindWindsurfPath auto-detects the Windsurf installation directory
func (p *PatchService) FindWindsurfPath() (string, error) {
	var candidates []string

	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "Programs", "Windsurf"))
		}
		candidates = append(candidates,
			`C:\Program Files\Windsurf`,
			`C:\Program Files (x86)\Windsurf`,
			`D:\Program\Windsurf`,
		)
	case "darwin":
		home, _ := os.UserHomeDir()
		candidates = append(candidates,
			"/Applications/Windsurf.app",
			filepath.Join(home, "Applications", "Windsurf.app"),
		)
	default: // linux
		home, _ := os.UserHomeDir()
		candidates = append(candidates,
			"/opt/Windsurf",
			"/usr/share/windsurf",
			filepath.Join(home, ".local", "share", "Windsurf"),
		)
	}

	relPath := getExtensionJSRelativePath()
	for _, c := range candidates {
		extFile := filepath.Join(c, relPath)
		if _, err := os.Stat(extFile); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("未找到Windsurf安装路径")
}

// PatchResult represents the result of applying the patch
type PatchResult struct {
	Success        bool     `json:"success"`
	AlreadyPatched bool     `json:"already_patched"`
	Modifications  []string `json:"modifications"`
	BackupFile     string   `json:"backup_file"`
	Message        string   `json:"message"`
}

// ApplyPatch patches extension.js to inject OAuth callback handler for seamless switching
func (p *PatchService) ApplyPatch(windsurfPath string) (*PatchResult, error) {
	extensionFile := filepath.Join(windsurfPath, getExtensionJSRelativePath())

	if _, err := os.Stat(extensionFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("extension.js 文件不存在: %s", extensionFile)
	}

	// 1. 管理备份文件（最多保留3份）
	parentDir := filepath.Dir(extensionFile)
	p.cleanOldBackups(parentDir, 3)

	// 创建新备份
	backupFile := extensionFile + fmt.Sprintf(".backup.%s", time.Now().Format("20060102_150405"))
	data, err := os.ReadFile(extensionFile)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	if err := os.WriteFile(backupFile, data, 0644); err != nil {
		return nil, fmt.Errorf("备份失败: %w", err)
	}

	content := string(data)
	modifiedContent := content
	var modifications []string

	// 2. 修改1: 添加全局 OAuth 回调处理器
	// 原始: this._uriHandler.event(X=>{"/refresh-authentication-session"===X.path&&(0,Y.refreshAuthenticationSession)()})
	// 注入 handleUri + handleAuthToken 逻辑
	pattern1 := regexp.MustCompile(
		`this\._uriHandler\.event\((\w+)=>\{"\/refresh-authentication-session"===(\w+)\.path&&\(0,(\w+)\.refreshAuthenticationSession\)\(\)\}\)`,
	)

	if matches := pattern1.FindStringSubmatch(modifiedContent); matches != nil {
		varName1 := matches[1]
		varName2 := matches[2]
		moduleName := matches[3]

		if varName1 == varName2 {
			replacement := fmt.Sprintf(
				`this._uriHandler.event(async %s=>{if("/refresh-authentication-session"===%s.path){(0,%s.refreshAuthenticationSession)()}else{try{const t=u.handleUri(%s);await this.handleAuthToken(t)}catch(e){console.error("[Windsurf] Failed to handle OAuth callback:",e)}}})`,
				varName1, varName1, moduleName, varName1,
			)
			modifiedContent = strings.Replace(modifiedContent, matches[0], replacement, 1)
			modifications = append(modifications, "OAuth回调处理器")
		}
	}

	// 3. 修改2: 移除180秒超时限制 (可选)
	pattern2 := regexp.MustCompile(
		`,new Promise\((\w+),(\w+)\)=>setTimeout\(\(\)=>\{(\w+)\(new (\w+)\)\},18e4\)\)`,
	)

	if matches := pattern2.FindStringSubmatch(modifiedContent); matches != nil {
		rejectVar1 := matches[2]
		rejectVar2 := matches[3]
		if rejectVar1 == rejectVar2 {
			modifiedContent = strings.Replace(modifiedContent, matches[0], "", 1)
			modifications = append(modifications, "移除超时限制")
		}
	} else {
		modifications = append(modifications, "超时限制已被官方移除(跳过)")
	}

	// 4. 检查是否已经打过补丁
	if modifiedContent == content {
		return &PatchResult{
			Success:        true,
			AlreadyPatched: true,
			Message:        "补丁已经应用过了",
		}, nil
	}

	// 5. 写入修改后的文件
	if err := os.WriteFile(extensionFile, []byte(modifiedContent), 0644); err != nil {
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}

	// 6. 重启 Windsurf
	_ = restartWindsurfFromInstall("")

	return &PatchResult{
		Success:       true,
		Modifications: modifications,
		BackupFile:    backupFile,
		Message:       "补丁应用成功，Windsurf正在重启",
	}, nil
}

// RestorePatch restores extension.js from the latest backup
func (p *PatchService) RestorePatch(windsurfPath string) error {
	extensionFile := filepath.Join(windsurfPath, getExtensionJSRelativePath())
	parentDir := filepath.Dir(extensionFile)

	backup, err := p.findLatestBackup(parentDir)
	if err != nil {
		return err
	}

	if err := copyFile(backup, extensionFile); err != nil {
		return fmt.Errorf("还原失败: %w", err)
	}

	_ = restartWindsurfFromInstall("")
	return nil
}

// RestartWindsurfFromInstall 结束 Windsurf 进程并重新启动。installRoot 为安装目录（内含 Windsurf.exe）或可执行文件完整路径；空则使用 Windows 默认 LocalAppData 路径。
func (p *PatchService) RestartWindsurfFromInstall(installRoot string) error {
	return restartWindsurfFromInstall(installRoot)
}

// CheckPatchStatus checks whether the patch is currently applied
func (p *PatchService) CheckPatchStatus(windsurfPath string) (bool, error) {
	extensionFile := filepath.Join(windsurfPath, getExtensionJSRelativePath())

	data, err := os.ReadFile(extensionFile)
	if err != nil {
		return false, fmt.Errorf("读取文件失败: %w", err)
	}

	return strings.Contains(string(data), "Failed to handle OAuth callback"), nil
}

// ── Helpers ──────────────────────────────────────────────────────

func (p *PatchService) cleanOldBackups(dir string, maxKeep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "extension.js.backup.") {
			backups = append(backups, filepath.Join(dir, e.Name()))
		}
	}

	sort.Strings(backups) // 文件名含时间戳，按名称排序即按时间
	for len(backups) >= maxKeep {
		_ = os.Remove(backups[0])
		backups = backups[1:]
	}
}

func (p *PatchService) findLatestBackup(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("读取目录失败: %w", err)
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "extension.js.backup.") {
			backups = append(backups, filepath.Join(dir, e.Name()))
		}
	}

	if len(backups) == 0 {
		return "", fmt.Errorf("未找到备份文件，无法还原")
	}

	sort.Strings(backups)
	return backups[len(backups)-1], nil // 最新的
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func restartWindsurfFromInstall(installRoot string) error {
	switch runtime.GOOS {
	case "windows":
		killCmd := exec.Command("taskkill", "/F", "/IM", "Windsurf.exe")
		hideWindow(killCmd)
		_ = killCmd.Run()
		time.Sleep(2 * time.Second)

		exePath := WindsurfInstallExePath(installRoot)
		if exePath == "" {
			localAppData := os.Getenv("LOCALAPPDATA")
			exePath = filepath.Join(localAppData, "Programs", "Windsurf", "Windsurf.exe")
		}
		if _, err := os.Stat(exePath); err != nil {
			return fmt.Errorf("未找到Windsurf可执行文件")
		}
		startCmd := exec.Command("cmd", "/C", "start", "", exePath)
		hideWindow(startCmd)
		return startCmd.Start()

	case "darwin":
		_ = exec.Command("pkill", "-f", "Windsurf").Run()
		time.Sleep(2 * time.Second)
		return exec.Command("open", "-a", "Windsurf").Start()

	default: // linux
		_ = exec.Command("pkill", "-f", "windsurf").Run()
		time.Sleep(2 * time.Second)
		return exec.Command("windsurf").Start()
	}
}
