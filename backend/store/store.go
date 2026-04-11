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
		if json.Valid(b) {
			_ = json.Unmarshal(b, &s.accounts)
		} else {
			// accounts.json 损坏，尝试从 .bak 恢复
			bakPath := s.accountsFile + ".bak"
			if bakData, bakErr := os.ReadFile(bakPath); bakErr == nil && json.Valid(bakData) {
				_ = json.Unmarshal(bakData, &s.accounts)
				// 恢复成功，覆盖损坏的文件
				_ = os.WriteFile(s.accountsFile, bakData, 0644)
				fmt.Printf("[Store] accounts.json 已损坏，已从 .bak 恢复 (%d bytes)\n", len(bakData))
			} else {
				fmt.Printf("[Store] ⚠ accounts.json 已损坏且无有效 .bak 可恢复 (%d bytes)\n", len(b))
			}
		}
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
// 加固措施：写入前校验 JSON 有效性、先备份旧文件、fsync 刷盘再 rename。
func atomicWriteFile(filePath string, data []byte) error {
	// 1. 写入前校验 JSON 合法性，防止写入损坏数据
	if !json.Valid(data) {
		return fmt.Errorf("atomicWriteFile: 拒绝写入非法 JSON 到 %s (%d bytes)", filepath.Base(filePath), len(data))
	}

	// 2. 如果目标文件存在，先创建 .bak 备份
	if _, err := os.Stat(filePath); err == nil {
		bakPath := filePath + ".bak"
		// 静默失败: 备份不是关键路径
		if bakData, readErr := os.ReadFile(filePath); readErr == nil && json.Valid(bakData) {
			_ = os.WriteFile(bakPath, bakData, 0644)
		}
	}

	// 3. 使用带 pid 的临时文件名，避免并发/残留冲突
	tmpPath := fmt.Sprintf("%s.tmp.%d", filePath, os.Getpid())
	_ = os.Remove(tmpPath) // 清理可能的残留

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		// 创建 tmp 失败，回退直接写
		return os.WriteFile(filePath, data, 0644)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return os.WriteFile(filePath, data, 0644)
	}

	// 4. fsync 确保数据落盘
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return os.WriteFile(filePath, data, 0644)
	}
	f.Close()

	// 5. rename 原子替换
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
