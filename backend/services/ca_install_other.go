//go:build !darwin

package services

import "errors"

// darwinInstallCAViaTerminal stub for non-macOS builds — never called because
// the caller already switches on runtime.GOOS, but must exist for the compiler.
func darwinInstallCAViaTerminal(_ string) error {
	return errors.New("darwinInstallCAViaTerminal is only implemented on macOS")
}
