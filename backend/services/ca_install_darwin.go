//go:build darwin

package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// darwinInstallCAViaTerminal opens Terminal.app, runs `sudo security add-trusted-cert ...`
// (which prompts for the user's password with a TTY-backed authorization context
// that SecTrustSettings API accepts), and polls for successful trust installation.
//
// Timeline:
//   1. Write a wrapper shell script to /tmp that invokes sudo security add-trusted-cert.
//   2. Tell Terminal.app (via AppleScript) to run that script in a new window.
//   3. Poll the keychain (for up to 90 s) to detect when trust is actually in place.
//   4. Clean up the temp script.
func darwinInstallCAViaTerminal(certPath string) error {
	// 1. Prepare a self-closing shell script the user will approve with `sudo`.
	tmpScript := filepath.Join(os.TempDir(), "windsurf-tools-install-ca.sh")
	scriptContent := fmt.Sprintf(`#!/bin/bash
set -e
echo ""
echo "============================================"
echo "  Windsurf Tools — 安装 MITM CA 证书"
echo "============================================"
echo ""
echo "请输入你的 macOS 登录密码（不会显示）："
sudo /usr/bin/security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain %q
echo ""
echo "✅ CA 证书已安装并信任。此窗口将在 3 秒后自动关闭。"
sleep 3
osascript -e 'tell application "Terminal" to close (every window whose name contains "windsurf-tools-install-ca")' >/dev/null 2>&1 || true
exit 0
`, certPath)
	if err := os.WriteFile(tmpScript, []byte(scriptContent), 0755); err != nil {
		return fmt.Errorf("写入安装脚本失败: %w", err)
	}
	defer os.Remove(tmpScript)

	// 2. Ask Terminal to open and run it.
	openCmd := fmt.Sprintf(`tell application "Terminal"
	activate
	do script "bash %s"
end tell`, shellQuote(tmpScript))
	if err := exec.Command("osascript", "-e", openCmd).Run(); err != nil {
		return fmt.Errorf("调起 Terminal.app 失败: %w", err)
	}

	// 3. Poll keychain trust settings until CA is trusted (or timeout).
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		if isCATrusted() {
			InvalidateCACache()
			return nil
		}
	}
	return fmt.Errorf("未检测到 CA 信任设置；请确认 Terminal 窗口里密码输入成功")
}

// isCATrusted returns true when our CA is recognized by the system trust chain
// (i.e. `security verify-cert` succeeds against it).
func isCATrusted() bool {
	// 先确认证书入库
	if err := exec.Command("security", "find-certificate", "-c", "Windsurf Tools CA",
		"/Library/Keychains/System.keychain").Run(); err != nil {
		return false
	}
	// verify-cert 用全部已配置策略校验证书链。trust 未生效时会返回非 0。
	out, err := exec.Command("security", "verify-cert", "-c", caCertPath()).CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "successful")
}
