package services

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const hostsMarker = "# windsurf-tools-mitm"

// hostsTargets 需要劫持的所有域名 (与 kaifa 一致)
var hostsTargets = []string{
	"server.self-serve.windsurf.com",
	"server.codeium.com",
}

// GetHostsFilePath returns the system hosts file path.
func GetHostsFilePath() string {
	if runtime.GOOS == "windows" {
		return `C:\Windows\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

func hostsBackupPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, caDirName, caSubDir)
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "hosts.bak")
}

// AddHostsEntry adds 127.0.0.1 mapping for domain to the system hosts file.
// When domain is empty or matches TargetDomain, all hostsTargets are added.
func AddHostsEntry(domain string) error {
	path := GetHostsFilePath()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 hosts 文件失败: %w", err)
	}
	content := string(data)

	// 如果已经有标记行，说明已劫持
	if strings.Contains(content, hostsMarker) {
		return nil
	}

	// 备份原始 hosts
	_ = os.WriteFile(hostsBackupPath(), data, 0644)

	// 添加所有目标域名
	var lines []string
	for _, target := range hostsTargets {
		lines = append(lines, fmt.Sprintf("127.0.0.1 %s %s", target, hostsMarker))
	}
	addition := "\n" + strings.Join(lines, "\n") + "\n"
	newContent := content + addition

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("写入 hosts 文件失败(需要管理员权限): %w", err)
	}

	return flushDNS()
}

// RemoveHostsEntry removes our hosts entries.
// When domain is empty or matches TargetDomain, all hostsTargets marker lines are removed.
func RemoveHostsEntry(domain string) error {
	path := GetHostsFilePath()

	// 优先从备份恢复
	backup := hostsBackupPath()
	if backupData, err := os.ReadFile(backup); err == nil && len(backupData) > 0 {
		if err := os.WriteFile(path, backupData, 0644); err == nil {
			_ = os.Remove(backup)
			return flushDNS()
		}
	}

	// 备份不可用时按标记逐行清理
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 hosts 文件失败: %w", err)
	}

	var newLines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, hostsMarker) {
			continue // skip our lines
		}
		newLines = append(newLines, line)
	}

	newContent := strings.Join(newLines, "\n")
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("写入 hosts 文件失败(需要管理员权限): %w", err)
	}

	return flushDNS()
}

// IsHostsMapped checks if the hosts hijacking is active (marker present).
func IsHostsMapped(domain string) bool {
	data, err := os.ReadFile(GetHostsFilePath())
	if err != nil {
		return false
	}
	return strings.Contains(string(data), hostsMarker)
}

// flushDNS clears the system DNS cache so hosts changes take effect immediately.
func flushDNS() error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("ipconfig", "/flushdns")
		hideWindow(cmd)
		return cmd.Run()
	case "darwin":
		_ = exec.Command("dscacheutil", "-flushcache").Run()
		_ = exec.Command("killall", "-HUP", "mDNSResponder").Run()
		return nil
	default:
		_ = exec.Command("systemd-resolve", "--flush-caches").Run()
		return nil
	}
}
