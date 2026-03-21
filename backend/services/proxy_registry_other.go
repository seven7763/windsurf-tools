//go:build !windows

package services

// AddProxyOverride 非 Windows 无需注册表操作
func AddProxyOverride() error { return nil }

// RemoveProxyOverride 非 Windows 无需清理
func RemoveProxyOverride() error { return nil }
