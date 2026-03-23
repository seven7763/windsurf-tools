package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInjectCodeiumConfigWithHomeDirWritesBothKeyVariants(t *testing.T) {
	homeDir := t.TempDir()
	configPath := filepath.Join(homeDir, ".codeium", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"theme":"dark"}`), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := injectCodeiumConfigWithHomeDir(homeDir, "sk-ws-test"); err != nil {
		t.Fatalf("injectCodeiumConfigWithHomeDir() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got["api_key"] != "sk-ws-test" {
		t.Fatalf("api_key = %#v, want sk-ws-test", got["api_key"])
	}
	if got["apiKey"] != "sk-ws-test" {
		t.Fatalf("apiKey = %#v, want sk-ws-test", got["apiKey"])
	}
	if got["theme"] != "dark" {
		t.Fatalf("theme = %#v, want dark", got["theme"])
	}
}
