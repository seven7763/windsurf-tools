package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	windsurfStateKeyCurrentAuth = "codeium.windsurf-windsurf_auth"
	windsurfStateKeyAuthStatus  = "windsurfAuthStatus"
	windsurfStateKeyPlanInfo    = "windsurf.settings.cachedPlanInfo"
	windsurfStateKeyMainStore   = "codeium.windsurf"
	windsurfStateKeySessions    = `secret://{"extensionId":"codeium.windsurf","key":"windsurf_auth.sessions"}`

	windsurfStateAuthKeyPrefix = "windsurf_auth-"
)

func (s *SwitchService) windsurfStateDBPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	configRoot, configErr := os.UserConfigDir()
	if configErr != nil || strings.TrimSpace(configRoot) == "" {
		configRoot = filepath.Join(home, ".config")
	}
	return windsurfStateDBPathsFor(runtime.GOOS, home, os.Getenv("APPDATA"), configRoot)
}

func windsurfStateDBPathsFor(goos, home, appData, configRoot string) []string {
	roots := windsurfUserRootCandidatesFor(goos, home, appData, configRoot)
	if len(roots) == 0 {
		return nil
	}
	paths := make([]string, 0, len(roots)*2)
	for _, root := range roots {
		globalPath := filepath.Join(root, "globalStorage", "state.vscdb")
		if _, err := os.Stat(globalPath); err == nil {
			paths = append(paths, globalPath)
		}

		workspaceMatches, err := filepath.Glob(filepath.Join(root, "workspaceStorage", "*", "state.vscdb"))
		if err == nil {
			paths = append(paths, workspaceMatches...)
		}
	}
	return uniqueCandidatePaths(paths)
}

func windsurfUserRootCandidatesFor(goos, home, appData, configRoot string) []string {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	configRoot = strings.TrimSpace(configRoot)
	if configRoot == "" {
		configRoot = filepath.Join(home, ".config")
	}

	switch strings.ToLower(strings.TrimSpace(goos)) {
	case "windows":
		if strings.TrimSpace(appData) == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return uniqueCandidatePaths([]string{
			filepath.Join(appData, "Windsurf", "User"),
			filepath.Join(appData, "Codeium", "User"),
		})
	case "darwin":
		appSupport := filepath.Join(home, "Library", "Application Support")
		return uniqueCandidatePaths([]string{
			filepath.Join(appSupport, "Windsurf", "User"),
			filepath.Join(appSupport, "Codeium", "User"),
		})
	default:
		return uniqueCandidatePaths([]string{
			filepath.Join(configRoot, "Windsurf", "User"),
			filepath.Join(configRoot, "windsurf", "User"),
			filepath.Join(configRoot, "Codeium", "User"),
			filepath.Join(configRoot, "codeium", "User"),
		})
	}
}

func (s *SwitchService) clearWindsurfSessionCache(email string) error {
	dbPaths := s.windsurfStateDBPaths()
	if len(dbPaths) == 0 {
		return nil
	}

	var errs []string
	for _, dbPath := range dbPaths {
		changed, err := clearWindsurfSessionCacheInDB(dbPath, email)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", dbPath, err))
			continue
		}
		if changed {
			log.Printf("[切号] 已清理 Windsurf 会话缓存: %s", dbPath)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func clearWindsurfSessionCacheInDB(dbPath, email string) (bool, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return false, fmt.Errorf("打开 state.vscdb 失败: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA busy_timeout = 2000`); err != nil {
		return false, fmt.Errorf("设置 busy_timeout 失败: %w", err)
	}

	var hasItemTable int
	if err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='ItemTable'`).Scan(&hasItemTable); err != nil {
		return false, fmt.Errorf("检查 ItemTable 失败: %w", err)
	}
	if hasItemTable == 0 {
		return false, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	changed := false
	if deleted, err := deleteWindsurfSessionKeys(tx); err != nil {
		return false, err
	} else if deleted {
		changed = true
	}

	if updated, err := updateCodeiumWindsurfState(tx, strings.TrimSpace(email)); err != nil {
		return false, err
	} else if updated {
		changed = true
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("提交会话缓存清理事务失败: %w", err)
	}
	return changed, nil
}

func deleteWindsurfSessionKeys(tx *sql.Tx) (bool, error) {
	res, err := tx.Exec(`
		DELETE FROM ItemTable
		WHERE key IN (?, ?, ?)
		   OR key = ?
		   OR key LIKE ?
	`,
		windsurfStateKeyCurrentAuth,
		windsurfStateKeyAuthStatus,
		windsurfStateKeyPlanInfo,
		windsurfStateKeySessions,
		windsurfStateAuthKeyPrefix+"%",
	)
	if err != nil {
		return false, fmt.Errorf("删除旧会话键失败: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("读取旧会话键删除结果失败: %w", err)
	}
	return rows > 0, nil
}

func updateCodeiumWindsurfState(tx *sql.Tx, email string) (bool, error) {
	var raw string
	err := tx.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, windsurfStateKeyMainStore).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("读取 codeium.windsurf 失败: %w", err)
	}

	patched, changed, err := sanitizeCodeiumWindsurfState(raw, email)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	if _, err := tx.Exec(`UPDATE ItemTable SET value = ? WHERE key = ?`, patched, windsurfStateKeyMainStore); err != nil {
		return false, fmt.Errorf("更新 codeium.windsurf 失败: %w", err)
	}
	return true, nil
}

func sanitizeCodeiumWindsurfState(raw, email string) (string, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return raw, false, nil
	}

	var state map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return "", false, fmt.Errorf("解析 codeium.windsurf 失败: %w", err)
	}

	changed := false
	if email != "" {
		wantEmail, err := json.Marshal(email)
		if err != nil {
			return "", false, fmt.Errorf("序列化 lastLoginEmail 失败: %w", err)
		}
		if got, ok := state["lastLoginEmail"]; !ok || string(got) != string(wantEmail) {
			state["lastLoginEmail"] = wantEmail
			changed = true
		}
	} else if _, ok := state["lastLoginEmail"]; ok {
		delete(state, "lastLoginEmail")
		changed = true
	}

	if _, ok := state["windsurf.state.cachedUserStatus"]; ok {
		delete(state, "windsurf.state.cachedUserStatus")
		changed = true
	}

	if !changed {
		return raw, false, nil
	}

	data, err := json.Marshal(state)
	if err != nil {
		return "", false, fmt.Errorf("序列化 codeium.windsurf 失败: %w", err)
	}
	return string(data), true, nil
}
