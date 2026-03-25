package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"windsurf-tools-wails/backend/models"
	"windsurf-tools-wails/backend/paths"
)

type Store struct {
	dataDir      string
	accountsFile string
	settingsFile string
	mu           sync.RWMutex
	accounts     []models.Account
	settings     models.Settings
}

// DataDir 返回号池与 settings.json 所在目录（跨平台统一为 UserConfigDir/WindsurfTools）。
func (s *Store) DataDir() string {
	return s.dataDir
}

// NewStoreInPaths 在指定目录创建/加载账号与设置文件（accounts.json、settings.json）。
func NewStoreInPaths(appDir string) (*Store, error) {
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	s := &Store{
		dataDir:      appDir,
		accountsFile: filepath.Join(appDir, "accounts.json"),
		settingsFile: filepath.Join(appDir, "settings.json"),
		accounts:     make([]models.Account, 0),
		settings:     models.DefaultSettings(),
	}

	s.load()
	return s, nil
}

func NewStore() (*Store, error) {
	dir, err := paths.ResolveAppConfigDir()
	if err != nil {
		return nil, err
	}
	return NewStoreInPaths(dir)
}

func (s *Store) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, err := os.ReadFile(s.accountsFile); err == nil {
		_ = json.Unmarshal(b, &s.accounts)
	}
	if b, err := os.ReadFile(s.settingsFile); err == nil {
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(b, &raw)
		_ = json.Unmarshal(b, &s.settings)
		// 旧版 settings.json 无此字段时默认开启（与 models.DefaultSettings 一致）
		if _, ok := raw["auto_switch_on_quota_exhausted"]; !ok {
			s.settings.AutoSwitchOnQuotaExhausted = true
		}
		if _, ok := raw["quota_hot_poll_seconds"]; !ok {
			s.settings.QuotaHotPollSeconds = 12
		}
		if _, ok := raw["restart_windsurf_after_switch"]; !ok {
			s.settings.RestartWindsurfAfterSwitch = true
		}
	}
}

func (s *Store) saveAccounts() error {
	b, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.accountsFile, b)
}

func (s *Store) saveSettings() error {
	b, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.settingsFile, b)
}

// atomicWriteFile 原子写入：先写临时文件再 rename，防止进程崩溃时损坏 JSON。
// rename 失败时回退到直接写入（Windows 下跨卷 rename 可能失败）。
func atomicWriteFile(filePath string, data []byte) error {
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		// tmp 写入失败，直接写目标文件
		return os.WriteFile(filePath, data, 0644)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return os.WriteFile(filePath, data, 0644)
	}
	return nil
}

// ── Account Operations ──

func (s *Store) AddAccount(acc models.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if AccountsConflict(s.accounts[i], acc) {
			return fmt.Errorf("账号已存在，不可重复导入")
		}
	}
	s.accounts = append(s.accounts, acc)
	return s.saveAccounts()
}

func (s *Store) GetAllAccounts() []models.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make([]models.Account, len(s.accounts))
	copy(copied, s.accounts)
	return copied
}

// AccountCount 返回号池总数（轻量，不拷贝切片）。
func (s *Store) AccountCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.accounts)
}

// AddAccountsBatch 批量添加账号，仅在所有写入完成后执行一次持久化；返回每条记录的错误（nil 表示成功）。
func (s *Store) AddAccountsBatch(accs []models.Account) []error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := make([]error, len(accs))
	added := false
	for i, acc := range accs {
		dup := false
		for j := range s.accounts {
			if AccountsConflict(s.accounts[j], acc) {
				errs[i] = fmt.Errorf("账号已存在，不可重复导入")
				dup = true
				break
			}
		}
		if !dup {
			s.accounts = append(s.accounts, acc)
			added = true
		}
	}
	if added {
		if err := s.saveAccounts(); err != nil {
			for i := range errs {
				if errs[i] == nil {
					errs[i] = err
				}
			}
		}
	}
	return errs
}

// GetAccount 返回账号值的拷贝，避免调用方持有指向内部切片的指针导致数据竞争。
func (s *Store) GetAccount(id string) (models.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			return s.accounts[i], nil
		}
	}
	return models.Account{}, fmt.Errorf("account not found")
}

func (s *Store) UpdateAccount(acc models.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == acc.ID {
			s.accounts[i] = acc
			return s.saveAccounts()
		}
	}
	return fmt.Errorf("account not found")
}

func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.accounts {
		if s.accounts[i].ID == id {
			s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
			return s.saveAccounts()
		}
	}
	return fmt.Errorf("account not found")
}

// ── Settings Operations ──

func (s *Store) GetSettings() models.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Store) UpdateSettings(st models.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = st
	return s.saveSettings()
}
