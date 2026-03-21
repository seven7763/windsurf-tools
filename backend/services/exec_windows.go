//go:build windows

package services

import (
	"os/exec"
	"syscall"
)

// hideWindow sets SysProcAttr to hide the console window on Windows.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
