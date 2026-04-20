//go:build darwin

package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DarwinBatchSetup 收集 MITM 启动时需要的特权命令（写入 hosts + 刷 DNS），
// 通过 SetPrivilegedPrelude 注册给 porthelper 的 osascript 调用，实现 **单次密码** 完成全部设置。
// 注意：CA 安装 **不** 放在这里 —— 因为 macOS 14+ 下 osascript 的 admin 上下文
// 无法给 SecTrustSettings API 授权，会导致假安装（证书入库但无信任设置）。CA 走
// InstallCA() → darwinInstallCAViaTerminal 单独流程。
// 调用方必须先调用本函数，再调用 mitmProxy.Start()。
func DarwinBatchSetup() error {
	var cmds []string

	// ① 写入 hosts（如果尚未劫持）
	hostsPath := GetHostsFilePath()
	if !IsHostsMapped("") {
		if data, err := os.ReadFile(hostsPath); err == nil {
			// 备份原始 hosts
			_ = os.WriteFile(hostsBackupPath(), data, 0644)
			var lines []string
			for _, target := range hostsTargets {
				lines = append(lines, fmt.Sprintf("127.0.0.1 %s %s", target, hostsMarker))
			}
			newContent := string(data) + "\n" + strings.Join(lines, "\n") + "\n"
			tmpHosts := filepath.Join(os.TempDir(), "windsurf-tools-hosts-setup")
			if err := os.WriteFile(tmpHosts, []byte(newContent), 0644); err == nil {
				cmds = append(cmds, fmt.Sprintf("/usr/bin/install -m 0644 %s %s",
					shellQuote(tmpHosts), shellQuote(hostsPath)))
			}
		}
	}

	// ③ 刷新 DNS（放最后，不影响主流程）
	cmds = append(cmds, "/usr/bin/dscacheutil -flushcache",
		"/usr/bin/killall -HUP mDNSResponder 2>/dev/null || true")

	// 把命令合并成一段 shell 脚本，注册给 porthelper 用同一个 osascript 提权执行
	prelude := strings.Join(cmds, "; ")
	SetPrivilegedPrelude(prelude)

	// 立即使 CA 缓存失效，启动完成后 IsCAInstalled() 会重新探测
	InvalidateCACache()
	return nil
}

// DarwinBatchTeardown 在 macOS 上将所有特权清理操作合并为一次 osascript 调用，
// 用户只需输入一次管理员密码即可完成：恢复 hosts、卸载 CA、恢复 pf 规则。
func DarwinBatchTeardown() error {
	var cmds []string

	// ① 恢复 hosts（从备份或逐行清理）
	hostsPath := GetHostsFilePath()
	backupPath := hostsBackupPath()
	if backupData, err := os.ReadFile(backupPath); err == nil && len(backupData) > 0 {
		// 有备份：写备份到临时文件，install 替换 /etc/hosts
		tmpHosts := filepath.Join(os.TempDir(), "windsurf-tools-hosts-restore")
		if err := os.WriteFile(tmpHosts, backupData, 0644); err == nil {
			cmds = append(cmds, fmt.Sprintf("/usr/bin/install -m 0644 %s %s",
				shellQuote(tmpHosts), shellQuote(hostsPath)))
		}
	} else {
		// 无备份：逐行清理 marker
		if data, err := os.ReadFile(hostsPath); err == nil {
			var lines []string
			for _, line := range strings.Split(string(data), "\n") {
				if !strings.Contains(line, hostsMarker) {
					lines = append(lines, line)
				}
			}
			cleanContent := strings.Join(lines, "\n")
			if !strings.HasSuffix(cleanContent, "\n") {
				cleanContent += "\n"
			}
			tmpHosts := filepath.Join(os.TempDir(), "windsurf-tools-hosts-restore")
			if err := os.WriteFile(tmpHosts, []byte(cleanContent), 0644); err == nil {
				cmds = append(cmds, fmt.Sprintf("/usr/bin/install -m 0644 %s %s",
					shellQuote(tmpHosts), shellQuote(hostsPath)))
			}
		}
	}

	// ② 卸载 CA（仅当钥匙串中确实存在时）
	certPath := caCertPath()
	if IsCAInstalled() {
		cmds = append(cmds, fmt.Sprintf("/usr/bin/security remove-trusted-cert -d %s", shellQuote(certPath)))
	}

	// ③ 刷新 DNS
	cmds = append(cmds, "dscacheutil -flushcache", "killall -HUP mDNSResponder 2>/dev/null || true")

	if len(cmds) == 0 {
		return nil
	}

	script := strings.Join(cmds, " ; ")
	if _, err := runShellScriptWithPrivilege(script); err != nil {
		return fmt.Errorf("批量恢复失败（请检查管理员密码是否正确）: %w", err)
	}

	// 清理临时文件和备份
	_ = os.Remove(filepath.Join(os.TempDir(), "windsurf-tools-hosts-restore"))
	_ = os.Remove(backupPath)
	return nil
}
