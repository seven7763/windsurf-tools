package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"windsurf-tools-linux/internal/models"
	"windsurf-tools-linux/internal/paths"
)

type Store struct {
	dataDir      string
	accountsFile string
	settingsFile string
	mu           sync.RWMutex
	accounts     []models.Account
	settings     models.Settings
}

func New(explicitDir string) (*Store, error) {
	dir, err := paths.ResolveAppConfigDir(explicitDir)
	if err != nil {
		return nil, err
	}
	return NewInDir(dir)
}

func NewInDir(appDir string) (*Store, error) {
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}

	s := &Store{
		dataDir:      appDir,
		accountsFile: filepath.Join(appDir, "accounts.json"),
		settingsFile: filepath.Join(appDir, "settings.json"),
		accounts:     make([]models.Account, 0),
		settings:     models.DefaultSettings(),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) DataDir() string {
	return s.dataDir
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if data, err := os.ReadFile(s.accountsFile); err == nil {
		if err := json.Unmarshal(data, &s.accounts); err != nil {
			return fmt.Errorf("decode accounts.json: %w", err)
		}
	}

	if data, err := os.ReadFile(s.settingsFile); err == nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("decode settings.json raw map: %w", err)
		}
		if err := json.Unmarshal(data, &s.settings); err != nil {
			return fmt.Errorf("decode settings.json: %w", err)
		}
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

	return nil
}

func (s *Store) GetAllAccounts() []models.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Account, len(s.accounts))
	copy(out, s.accounts)
	return out
}

func (s *Store) GetAccount(id string) (models.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, acc := range s.accounts {
		if acc.ID == id {
			return acc, nil
		}
	}
	return models.Account{}, fmt.Errorf("account not found")
}

func (s *Store) SaveAccount(acc models.Account) (models.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	acc.ID = stringsOr(acc.ID, uuid.NewString())
	acc.Email = trim(acc.Email)
	acc.Nickname = trim(acc.Nickname)
	acc.PlanName = stringsOr(trim(acc.PlanName), "unknown")
	acc.Status = stringsOr(trim(acc.Status), "active")
	acc.CreatedAt = trim(acc.CreatedAt)

	updateIndex := -1
	for i, existing := range s.accounts {
		if existing.ID == acc.ID {
			updateIndex = i
			if acc.CreatedAt == "" {
				acc.CreatedAt = existing.CreatedAt
			}
			continue
		}
		if AccountsConflict(existing, acc) {
			return models.Account{}, fmt.Errorf("account already exists")
		}
	}

	if updateIndex >= 0 {
		s.accounts[updateIndex] = acc
		return acc, s.saveAccountsLocked()
	}

	acc.CreatedAt = stringsOr(acc.CreatedAt, now)
	s.accounts = append(s.accounts, acc)
	return acc, s.saveAccountsLocked()
}

func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, acc := range s.accounts {
		if acc.ID == id {
			s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
			return s.saveAccountsLocked()
		}
	}
	return fmt.Errorf("account not found")
}

func (s *Store) GetSettings() models.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Store) saveAccountsLocked() error {
	data, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal accounts: %w", err)
	}
	return atomicWriteFile(s.accountsFile, data)
}

func atomicWriteFile(filePath string, data []byte) error {
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return os.WriteFile(filePath, data, 0o644)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return os.WriteFile(filePath, data, 0o644)
	}
	return nil
}

func trim(value string) string {
	return strings.TrimSpace(value)
}

func stringsOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
