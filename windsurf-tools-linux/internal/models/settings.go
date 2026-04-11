package models

type Settings struct {
	ProxyEnabled               bool   `json:"proxy_enabled"`
	ProxyURL                   string `json:"proxy_url"`
	WindsurfPath               string `json:"windsurf_path"`
	ConcurrentLimit            int    `json:"concurrent_limit"`
	AutoRefreshTokens          bool   `json:"auto_refresh_tokens"`
	AutoRefreshQuotas          bool   `json:"auto_refresh_quotas"`
	QuotaRefreshPolicy         string `json:"quota_refresh_policy"`
	QuotaCustomIntervalMinutes int    `json:"quota_custom_interval_minutes"`
	AutoSwitchPlanFilter       string `json:"auto_switch_plan_filter"`
	AutoSwitchOnQuotaExhausted bool   `json:"auto_switch_on_quota_exhausted"`
	QuotaHotPollSeconds        int    `json:"quota_hot_poll_seconds"`
	RestartWindsurfAfterSwitch bool   `json:"restart_windsurf_after_switch"`
	MinimizeToTray             bool   `json:"minimize_to_tray"`
	ShowDesktopToolbar         bool   `json:"show_desktop_toolbar"`
	SilentStart                bool   `json:"silent_start"`
	MitmOnly                   bool   `json:"mitm_only"`
	MitmTunMode                bool   `json:"mitm_tun_mode"`
	MitmProxyEnabled           bool   `json:"mitm_proxy_enabled"`
	MitmDebugDump              bool   `json:"mitm_debug_dump"`
	MitmFullCapture            bool   `json:"mitm_full_capture"`
	DebugLog                   bool   `json:"debug_log"`
	ImportConcurrency          int    `json:"import_concurrency"`
	OpenAIRelayEnabled         bool   `json:"openai_relay_enabled"`
	OpenAIRelayPort            int    `json:"openai_relay_port"`
	OpenAIRelaySecret          string `json:"openai_relay_secret"`
}

func DefaultSettings() Settings {
	return Settings{
		ProxyEnabled:               false,
		ConcurrentLimit:            5,
		AutoRefreshTokens:          false,
		AutoRefreshQuotas:          false,
		QuotaRefreshPolicy:         "hybrid",
		QuotaCustomIntervalMinutes: 360,
		AutoSwitchPlanFilter:       "all",
		AutoSwitchOnQuotaExhausted: true,
		QuotaHotPollSeconds:        12,
		RestartWindsurfAfterSwitch: true,
		MinimizeToTray:             false,
		ShowDesktopToolbar:         false,
		SilentStart:                false,
		MitmOnly:                   false,
		MitmTunMode:                false,
		MitmProxyEnabled:           false,
		MitmDebugDump:              false,
		MitmFullCapture:            false,
		DebugLog:                   false,
		ImportConcurrency:          3,
		OpenAIRelayEnabled:         false,
		OpenAIRelayPort:            8787,
	}
}
