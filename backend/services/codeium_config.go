package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// InjectCodeiumConfig 写入 ~/.codeium/config.json 注入 API Key
// Windsurf/Codeium 启动时会读取此文件中的 api_key
func InjectCodeiumConfig(apiKey string) error {
	if apiKey == "" {
		return nil
	}
	dir, err := codeiumConfigDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(dir, "config.json")
	backupPath := filepath.Join(dir, "config.json.bak")

	// 备份原始文件
	if data, err := os.ReadFile(configPath); err == nil {
		_ = os.WriteFile(backupPath, data, 0644)
	}

	// 读取或创建新的配置
	config := make(map[string]interface{})
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &config)
	}

	config["api_key"] = apiKey

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 codeium config: %w", err)
	}
	return os.WriteFile(configPath, data, 0644)
}

// RestoreCodeiumConfig 恢复 ~/.codeium/config.json
func RestoreCodeiumConfig() error {
	dir, err := codeiumConfigDir()
	if err != nil {
		return nil
	}
	configPath := filepath.Join(dir, "config.json")
	backupPath := filepath.Join(dir, "config.json.bak")

	if backupData, err := os.ReadFile(backupPath); err == nil {
		_ = os.WriteFile(configPath, backupData, 0644)
		_ = os.Remove(backupPath)
		return nil
	}

	// 无备份时清除 api_key 字段
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	config := make(map[string]interface{})
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	delete(config, "api_key")
	newData, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(configPath, newData, 0644)
	return nil
}

func codeiumConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录: %w", err)
	}
	dir := filepath.Join(home, ".codeium")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建 .codeium 目录: %w", err)
	}
	return dir, nil
}
