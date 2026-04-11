package models

// Settings 全局设置
type Settings struct {
	ProxyEnabled               bool   `json:"proxy_enabled"`
	ProxyURL                   string `json:"proxy_url"`
	WindsurfPath               string `json:"windsurf_path"`
	ConcurrentLimit            int    `json:"concurrent_limit"`
	AutoRefreshTokens          bool   `json:"auto_refresh_tokens"`
	AutoRefreshQuotas          bool   `json:"auto_refresh_quotas"`
	QuotaRefreshPolicy         string `json:"quota_refresh_policy"`          // hybrid | interval_* | us_calendar | local_calendar | custom
	QuotaCustomIntervalMinutes int    `json:"quota_custom_interval_minutes"` // 仅 policy=custom 时使用，默认由后端钳制
	// AutoSwitchPlanFilter 无感「下一席位」计划池：all 不限制；否则逗号分隔多选，如 trial,pro（与 PlanTone 一致）
	AutoSwitchPlanFilter string `json:"auto_switch_plan_filter"`
	// AutoSwitchOnQuotaExhausted 在自动同步额度后，若当前 Windsurf 登录账号额度用尽则尝试切到下一席（依赖 windsurf_auth 与号池匹配）
	AutoSwitchOnQuotaExhausted bool `json:"auto_switch_on_quota_exhausted"`
	// QuotaHotPollSeconds 开启「用尽切号」时，仅对当前 Windsurf 会话高频拉额度（秒）；号池其余账号只走 QuotaRefreshPolicy 的定期同步，不在此轮询。范围 5～60
	QuotaHotPollSeconds int `json:"quota_hot_poll_seconds"`
	// RestartWindsurfAfterSwitch 仅对写入 windsurf_auth.json 的切号生效；MITM 代理换号不触发、一般也无需重启 IDE。运行中的 IDE 会缓存登录态，仅改文件往往不会立即生效
	RestartWindsurfAfterSwitch bool `json:"restart_windsurf_after_switch"`

	// MinimizeToTray 点击关闭时最小化到系统托盘而不退出（需系统支持托盘图标）
	MinimizeToTray bool `json:"minimize_to_tray"`
	// ShowDesktopToolbar 启用桌面小横条模式：小窗口置顶展示当前账号与额度（可配合托盘菜单）
	ShowDesktopToolbar bool `json:"show_desktop_toolbar"`
	// SilentStart 启动时不显示主窗口（仍可在托盘打开；也可用命令行 --silent）
	SilentStart bool `json:"silent_start"`

	// MitmOnly 仅使用 MITM 多号轮换：不写入 windsurf_auth、不在额度用尽时执行「文件切号」（仍同步号池额度与 JWT 供代理使用）
	MitmOnly bool `json:"mitm_only"`
	// MitmTunMode 在 UI 中展示 TUN/全局代理（如 Clash）与本机 MITM 共存的说明；不修改网络栈
	MitmTunMode bool `json:"mitm_tun_mode"`

	// ── MITM 代理 ──
	// MitmProxyEnabled 仅对无界面服务 / daemon 生效：启动后自动拉起 MITM 代理（hosts 劫持 + JWT 替换 + 多号轮换）
	MitmProxyEnabled bool `json:"mitm_proxy_enabled"`
	// MitmDebugDump 开启后，MITM 拦截 GetChatMessage 时将请求/响应的 protobuf 字段树写入 proto_dumps/ 目录
	MitmDebugDump bool `json:"mitm_debug_dump"`
	// MitmFullCapture 开启后，全量记录 MITM 代理的所有请求/响应到 capture/ 目录（JSONL + body 文件）
	MitmFullCapture bool `json:"mitm_full_capture"`

	// ── 静态响应缓存 ──
	StaticCacheIntercept bool `json:"static_cache_intercept"`

	// ── GetUserStatus 伪造 ──
	ForgeEnabled           bool   `json:"forge_enabled"`
	FakeCredits            int    `json:"fake_credits"`
	FakeCreditsPremium     int    `json:"fake_credits_premium"`
	FakeCreditsOther       int    `json:"fake_credits_other"`
	FakeCreditsUsed        int    `json:"fake_credits_used"`
	FakeSubscriptionType   string `json:"fake_subscription_type"`
	FakeBillingExtendYears int    `json:"fake_billing_extend_years"`

	// DebugLog 开启后将切号/代理/额度判定等关键日志写入文件 debug.log
	DebugLog bool `json:"debug_log"`
	// ImportConcurrency 导入时最大并发数（默认 3）
	ImportConcurrency int `json:"import_concurrency"`

	// ── OpenAI 中转 ──
	// OpenAIRelayEnabled 启用本地 OpenAI 兼容 API 中转服务器
	OpenAIRelayEnabled bool `json:"openai_relay_enabled"`
	// OpenAIRelayPort 中转服务器监听端口（默认 8787）
	OpenAIRelayPort int `json:"openai_relay_port"`
	// OpenAIRelaySecret Bearer token 鉴权密钥（空则不鉴权）
	OpenAIRelaySecret string `json:"openai_relay_secret"`
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
		StaticCacheIntercept:       true,
		ForgeEnabled:               false,
		FakeCredits:                10000000,
		FakeCreditsPremium:         150000,
		FakeCreditsOther:           25000,
		FakeCreditsUsed:            0,
		FakeSubscriptionType:       "Enterprise",
		FakeBillingExtendYears:     10,
		DebugLog:                   false,
		ImportConcurrency:          3,
		OpenAIRelayEnabled:         false,
		OpenAIRelayPort:            8787,
		OpenAIRelaySecret:          "",
	}
}
