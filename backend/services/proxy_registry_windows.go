//go:build windows

package services

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	proxyOverrideValue  = "ProxyOverride"
)

// AddProxyOverride 添加域名到 IE/系统代理白名单 (ProxyOverride)
// 防止系统代理干扰 hosts 劫持后的本地 MITM 代理
func AddProxyOverride() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	existing, _, _ := k.GetStringValue(proxyOverrideValue)
	for _, domain := range hostsTargets {
		if !strings.Contains(existing, domain) {
			if existing != "" {
				existing += ";"
			}
			existing += domain
		}
	}
	return k.SetStringValue(proxyOverrideValue, existing)
}

// RemoveProxyOverride 从 ProxyOverride 移除我们添加的域名
func RemoveProxyOverride() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil // 打不开不报错
	}
	defer k.Close()

	existing, _, err := k.GetStringValue(proxyOverrideValue)
	if err != nil {
		return nil
	}

	parts := strings.Split(existing, ";")
	var kept []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		skip := false
		for _, domain := range hostsTargets {
			if p == domain {
				skip = true
				break
			}
		}
		if !skip && p != "" {
			kept = append(kept, p)
		}
	}
	return k.SetStringValue(proxyOverrideValue, strings.Join(kept, ";"))
}
