package api

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"windsurf-tools-linux/internal/models"
	"windsurf-tools-linux/internal/store"
)

//go:embed web/*
var webAssets embed.FS

type Server struct {
	store    *store.Store
	readOnly bool
	mux      *http.ServeMux
	files    http.Handler
}

type StateResponse struct {
	DataDir        string       `json:"data_dir"`
	GeneratedAt    string       `json:"generated_at"`
	Summary        Summary      `json:"summary"`
	Accounts       []AccountDTO `json:"accounts"`
	Settings       SettingsDTO  `json:"settings"`
	Compatibility  []string     `json:"compatibility"`
	ListenBehavior string       `json:"listen_behavior"`
}

type Summary struct {
	TotalAccounts      int         `json:"total_accounts"`
	ActiveAccounts     int         `json:"active_accounts"`
	LowQuotaAccounts   int         `json:"low_quota_accounts"`
	ExpiringSoon       int         `json:"expiring_soon"`
	MissingCredentials int         `json:"missing_credentials"`
	PlanCounts         []PlanCount `json:"plan_counts"`
}

type PlanCount struct {
	Plan  string `json:"plan"`
	Count int    `json:"count"`
}

type AccountDTO struct {
	models.Account
	Expired             bool `json:"expired"`
	ExpiringSoon        bool `json:"expiring_soon"`
	LowQuota            bool `json:"low_quota"`
	HasPassword         bool `json:"has_password"`
	HasToken            bool `json:"has_token"`
	HasRefreshToken     bool `json:"has_refresh_token"`
	HasWindsurfAPIKey   bool `json:"has_windsurf_api_key"`
	HasAnyCredential    bool `json:"has_any_credential"`
	QuotaPercentNumeric int  `json:"quota_percent_numeric"`
}

type SettingsDTO struct {
	ConcurrentLimit            int    `json:"concurrent_limit"`
	AutoRefreshTokens          bool   `json:"auto_refresh_tokens"`
	AutoRefreshQuotas          bool   `json:"auto_refresh_quotas"`
	QuotaRefreshPolicy         string `json:"quota_refresh_policy"`
	QuotaCustomIntervalMinutes int    `json:"quota_custom_interval_minutes"`
	DebugLog                   bool   `json:"debug_log"`
	ImportConcurrency          int    `json:"import_concurrency"`
	OpenAIRelayEnabled         bool   `json:"openai_relay_enabled"`
	OpenAIRelayPort            int    `json:"openai_relay_port"`
}

func New(store *store.Store, readOnly bool) (*Server, error) {
	assetRoot, err := fs.Sub(webAssets, "web")
	if err != nil {
		return nil, fmt.Errorf("sub web assets: %w", err)
	}
	s := &Server{
		store:    store,
		readOnly: readOnly,
		mux:      http.NewServeMux(),
		files:    http.FileServer(http.FS(assetRoot)),
	}
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/state", s.handleState)
	s.mux.HandleFunc("/api/accounts", s.handleAccounts)
	s.mux.HandleFunc("/api/accounts/", s.handleAccountByID)
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"generated":  time.Now().UTC().Format(time.RFC3339),
		"read_only":  s.readOnly,
		"data_dir":   s.store.DataDir(),
		"app_server": "windsurf-tools-linux",
	})
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	accounts := s.store.GetAllAccounts()
	response := StateResponse{
		DataDir:        s.store.DataDir(),
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		Summary:        buildSummary(accounts),
		Accounts:       buildAccountDTOs(accounts),
		Settings:       toSettingsDTO(s.store.GetSettings()),
		ListenBehavior: "default bind is 127.0.0.1, so the control plane stays local unless you intentionally expose it",
		Compatibility: []string{
			"reads and writes the same accounts.json and settings.json layout as the desktop project",
			"does not implement MITM, traffic interception, automatic account rotation, or quota-bypass behavior",
			"focuses on local account inventory, quota snapshots, search, and manual record maintenance",
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, buildAccountDTOs(s.store.GetAllAccounts()))
	case http.MethodPost:
		if s.readOnly {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "server is running in read-only mode"})
			return
		}
		var payload models.Account
		if err := decodeJSON(r.Body, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		payload = normalizeAccountPayload(payload)
		if strings.TrimSpace(payload.Email) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
			return
		}
		saved, err := s.store.SaveAccount(payload)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, toAccountDTO(saved))
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleAccountByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/accounts/")
	if id == "" || strings.ContainsRune(id, '/') {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w, http.MethodDelete)
		return
	}
	if s.readOnly {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "server is running in read-only mode"})
		return
	}
	if err := s.store.DeleteAccount(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	s.files.ServeHTTP(w, r)
}

func decodeJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func buildAccountDTOs(accounts []models.Account) []AccountDTO {
	items := make([]AccountDTO, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, toAccountDTO(account))
	}
	sort.SliceStable(items, func(i int, j int) bool {
		if items[i].Expired != items[j].Expired {
			return !items[i].Expired && items[j].Expired
		}
		return strings.ToLower(items[i].Email) < strings.ToLower(items[j].Email)
	})
	return items
}

func toAccountDTO(account models.Account) AccountDTO {
	return AccountDTO{
		Account:             account,
		Expired:             isExpired(account.SubscriptionExpiresAt),
		ExpiringSoon:        isExpiringSoon(account.SubscriptionExpiresAt),
		LowQuota:            isLowQuota(account),
		HasPassword:         strings.TrimSpace(account.Password) != "",
		HasToken:            strings.TrimSpace(account.Token) != "",
		HasRefreshToken:     strings.TrimSpace(account.RefreshToken) != "",
		HasWindsurfAPIKey:   strings.TrimSpace(account.WindsurfAPIKey) != "",
		HasAnyCredential:    hasAnyCredential(account),
		QuotaPercentNumeric: quotaPercent(account),
	}
}

func buildSummary(accounts []models.Account) Summary {
	out := Summary{TotalAccounts: len(accounts)}
	planCounts := map[string]int{}
	for _, account := range accounts {
		if strings.TrimSpace(account.Status) == "" || strings.EqualFold(strings.TrimSpace(account.Status), "active") {
			out.ActiveAccounts++
		}
		if isLowQuota(account) {
			out.LowQuotaAccounts++
		}
		if isExpiringSoon(account.SubscriptionExpiresAt) {
			out.ExpiringSoon++
		}
		if !hasAnyCredential(account) {
			out.MissingCredentials++
		}
		plan := strings.TrimSpace(account.PlanName)
		if plan == "" {
			plan = "unknown"
		}
		planCounts[strings.ToLower(plan)]++
	}
	for plan, count := range planCounts {
		out.PlanCounts = append(out.PlanCounts, PlanCount{Plan: plan, Count: count})
	}
	sort.Slice(out.PlanCounts, func(i int, j int) bool {
		if out.PlanCounts[i].Count != out.PlanCounts[j].Count {
			return out.PlanCounts[i].Count > out.PlanCounts[j].Count
		}
		return out.PlanCounts[i].Plan < out.PlanCounts[j].Plan
	})
	return out
}

func toSettingsDTO(settings models.Settings) SettingsDTO {
	return SettingsDTO{
		ConcurrentLimit:            settings.ConcurrentLimit,
		AutoRefreshTokens:          settings.AutoRefreshTokens,
		AutoRefreshQuotas:          settings.AutoRefreshQuotas,
		QuotaRefreshPolicy:         settings.QuotaRefreshPolicy,
		QuotaCustomIntervalMinutes: settings.QuotaCustomIntervalMinutes,
		DebugLog:                   settings.DebugLog,
		ImportConcurrency:          settings.ImportConcurrency,
		OpenAIRelayEnabled:         settings.OpenAIRelayEnabled,
		OpenAIRelayPort:            settings.OpenAIRelayPort,
	}
}

func normalizeAccountPayload(account models.Account) models.Account {
	account.Email = strings.TrimSpace(account.Email)
	account.Nickname = strings.TrimSpace(account.Nickname)
	account.PlanName = strings.TrimSpace(account.PlanName)
	account.Status = strings.TrimSpace(account.Status)
	account.Tags = strings.TrimSpace(account.Tags)
	account.Remark = strings.TrimSpace(account.Remark)
	account.Password = strings.TrimSpace(account.Password)
	account.Token = strings.TrimSpace(account.Token)
	account.RefreshToken = strings.TrimSpace(account.RefreshToken)
	account.WindsurfAPIKey = strings.TrimSpace(account.WindsurfAPIKey)
	account.DailyRemaining = strings.TrimSpace(account.DailyRemaining)
	account.WeeklyRemaining = strings.TrimSpace(account.WeeklyRemaining)
	account.DailyResetAt = strings.TrimSpace(account.DailyResetAt)
	account.WeeklyResetAt = strings.TrimSpace(account.WeeklyResetAt)
	account.SubscriptionExpiresAt = strings.TrimSpace(account.SubscriptionExpiresAt)
	account.TokenExpiresAt = strings.TrimSpace(account.TokenExpiresAt)
	account.LastLoginAt = strings.TrimSpace(account.LastLoginAt)
	account.LastQuotaUpdate = strings.TrimSpace(account.LastQuotaUpdate)
	return account
}

func hasAnyCredential(account models.Account) bool {
	return strings.TrimSpace(account.Password) != "" ||
		strings.TrimSpace(account.Token) != "" ||
		strings.TrimSpace(account.RefreshToken) != "" ||
		strings.TrimSpace(account.WindsurfAPIKey) != ""
}

func isExpiringSoon(value string) bool {
	expiry, ok := parseTime(value)
	if !ok {
		return false
	}
	now := time.Now().UTC()
	return expiry.After(now) && expiry.Before(now.Add(72*time.Hour))
}

func isExpired(value string) bool {
	expiry, ok := parseTime(value)
	if !ok {
		return false
	}
	return expiry.Before(time.Now().UTC())
}

func isLowQuota(account models.Account) bool {
	for _, candidate := range []string{account.DailyRemaining, account.WeeklyRemaining} {
		if percent, ok := parsePercent(candidate); ok && percent <= 15 {
			return true
		}
	}
	return account.TotalQuota > 0 && account.UsedQuota >= account.TotalQuota
}

func quotaPercent(account models.Account) int {
	for _, candidate := range []string{account.DailyRemaining, account.WeeklyRemaining} {
		if percent, ok := parsePercent(candidate); ok {
			return percent
		}
	}
	if account.TotalQuota <= 0 {
		return -1
	}
	remaining := account.TotalQuota - account.UsedQuota
	if remaining < 0 {
		remaining = 0
	}
	return remaining * 100 / account.TotalQuota
}

func parsePercent(value string) (int, bool) {
	clean := strings.TrimSpace(strings.TrimSuffix(value, "%"))
	if clean == "" {
		return 0, false
	}
	floatValue, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, false
	}
	return int(floatValue + 0.5), true
}

func parseTime(value string) (time.Time, bool) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, clean); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.ParseInLocation("2006-01-02 15:04:05", clean, time.Local); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}
