//go:build !darwin

package services

// DarwinBatchSetup is a no-op on non-macOS platforms.
func DarwinBatchSetup() error { return nil }

// DarwinBatchTeardown is a no-op on non-macOS platforms.
func DarwinBatchTeardown() error { return nil }
