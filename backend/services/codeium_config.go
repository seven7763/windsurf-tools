package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InjectCodeiumConfig 写入 ~/.codeium/config.json 注入 API Key。
// 兼容不同 Windsurf/Codeium 版本，同时写入 snake_case 与 camelCase。
func InjectCodeiumConfig(apiKey string) error {
	if apiKey == "" {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return injectCodeiumConfigWithHomeDir(home, apiKey)
}

func InjectCodeiumConfigAtHome(homeDir, apiKey string) error {
	if apiKey == "" {
		return nil
	}
	return injectCodeiumConfigWithHomeDir(homeDir, apiKey)
}

func injectCodeiumConfigWithHomeDir(homeDir, apiKey string) error {
	dir, err := codeiumConfigDirFromHome(homeDir)
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
	config["apiKey"] = apiKey

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 codeium config: %w", err)
	}
	return robustWriteFile(configPath, data)
}

// robustWriteFile 兼容管理员 Windsurf 锁定文件：直写 → 临时文件+rename → PowerShell。
func robustWriteFile(filePath string, data []byte) error {
	if err := os.WriteFile(filePath, data, 0644); err == nil {
		return nil
	} else {
		log.Printf("[写入] 直写 %s 失败(%v)，尝试备选方案", filepath.Base(filePath), err)
	}
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err == nil {
		if err := os.Rename(tmpPath, filePath); err == nil {
			return nil
		}
		_ = os.Remove(tmpPath)
	}
	if runtime.GOOS == "windows" {
		return writeFileViaPowerShell(filePath, data)
	}
	return fmt.Errorf("写入 %s 失败（所有方式均失败）", filepath.Base(filePath))
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

	// 无备份时清除注入过的 key 字段
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	config := make(map[string]interface{})
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	delete(config, "api_key")
	delete(config, "apiKey")
	newData, _ := json.MarshalIndent(config, "", "  ")
	_ = os.WriteFile(configPath, newData, 0644)
	return nil
}

func codeiumConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录: %w", err)
	}
	return codeiumConfigDirFromHome(home)
}

func codeiumConfigDirFromHome(home string) (string, error) {
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("获取用户目录: 为空")
	}
	dir := filepath.Join(home, ".codeium")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建 .codeium 目录: %w", err)
	}
	return dir, nil
}
