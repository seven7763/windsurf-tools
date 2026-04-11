package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	privilegeLookPath = exec.LookPath
	privilegeCommand  = func(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }
)

func buildPrivilegeCommand(goos string, euid int, lookPath func(string) (string, error), target string, args ...string) (string, []string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil, fmt.Errorf("target command is empty")
	}

	resolvedTarget := target
	if !filepath.IsAbs(target) {
		path, err := lookPath(target)
		if err != nil {
			return "", nil, fmt.Errorf("无法找到命令 %s: %w", target, err)
		}
		resolvedTarget = path
	}

	if strings.ToLower(strings.TrimSpace(goos)) != "linux" || euid == 0 {
		return resolvedTarget, args, nil
	}

	if pkexecPath, err := lookPath("pkexec"); err == nil {
		return pkexecPath, append([]string{resolvedTarget}, args...), nil
	}
	if sudoPath, err := lookPath("sudo"); err == nil {
		return sudoPath, append([]string{resolvedTarget}, args...), nil
	}
	return "", nil, fmt.Errorf("Linux 需要 root 权限，请使用 root 启动，或确保系统已安装 pkexec/sudo")
}

func runCommandWithPrivilege(target string, args ...string) ([]byte, error) {
	name, finalArgs, err := buildPrivilegeCommand(runtime.GOOS, os.Geteuid(), privilegeLookPath, target, args...)
	if err != nil {
		return nil, err
	}
	cmd := privilegeCommand(name, finalArgs...)
	hideWindow(cmd)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return output, fmt.Errorf("%s %s: %w", name, strings.Join(finalArgs, " "), runErr)
	}
	return output, nil
}

func writeSystemFile(path string, data []byte, perm os.FileMode) error {
	if err := os.WriteFile(path, data, perm); err == nil {
		return nil
	} else if runtime.GOOS != "linux" {
		return err
	}

	tmp, err := os.CreateTemp("", "windsurf-tools-system-write-*")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("设置临时文件权限失败: %w", err)
	}

	mode := fmt.Sprintf("%04o", perm.Perm())
	output, err := runCommandWithPrivilege("install", "-m", mode, tmpPath, path)
	if err != nil {
		return fmt.Errorf("提权写入系统文件失败: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func removeSystemFile(path string) error {
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		return nil
	} else if runtime.GOOS != "linux" {
		return err
	}
	output, err := runCommandWithPrivilege("rm", "-f", path)
	if err != nil {
		return fmt.Errorf("提权删除系统文件失败: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
