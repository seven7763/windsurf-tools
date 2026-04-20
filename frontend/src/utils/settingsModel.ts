import { models } from '../../wailsjs/go/models'

/** 与 backend/utils/plan_tone.go PlanTone 顺序一致，用于排序与全选判定 */
export const SWITCH_PLAN_FILTER_TONES = [
  'pro',
  'max',
  'team',
  'enterprise',
  'trial',
  'free',
  'unknown',
] as const

export type SwitchPlanTone = (typeof SWITCH_PLAN_FILTER_TONES)[number]

/** 多选勾选列表（不含「全部」） */
export const switchPlanFilterToneOptions: Array<{ value: SwitchPlanTone; label: string }> = [
  { value: 'pro', label: 'Pro' },
  { value: 'max', label: 'Max / Ultimate' },
  { value: 'team', label: 'Teams' },
  { value: 'enterprise', label: 'Enterprise' },
  { value: 'trial', label: 'Trial' },
  { value: 'free', label: 'Free' },
  { value: 'unknown', label: '未识别' },
]

const TONE_ORDER = new Map(SWITCH_PLAN_FILTER_TONES.map((t, i) => [t, i]))

/** 下拉/兼容：含「全部」与单选旧值 */
export const switchPlanFilterOptions: Array<{ value: string; label: string }> = [
  { value: 'all', label: '全部计划（不限制）' },
  ...switchPlanFilterToneOptions.map((o) => ({ value: o.value, label: `仅 ${o.label}` })),
]

/** 与 backend/models/settings.go + wailsjs models.Settings 对齐 */
export function createDefaultSettings(): models.Settings {
  return new models.Settings({
    concurrent_limit: 5,
    auto_refresh_tokens: false,
    auto_refresh_quotas: false,
    quota_refresh_policy: 'hybrid',
    quota_custom_interval_minutes: 360,
    auto_switch_plan_filter: 'all',
    auto_switch_on_quota_exhausted: true,
    quota_hot_poll_seconds: 12,
    minimize_to_tray: false,
    silent_start: false,
    openai_relay_enabled: false,
    openai_relay_port: 8787,
    openai_relay_secret: '',
    debug_log: false,
    import_concurrency: 3,
    forge_enabled: false,
    static_cache_intercept: true,
    mitm_full_capture: false,
    mitm_debug_dump: false,
  })
}

export function normalizeSettings(raw: unknown): models.Settings {
  const base = createDefaultSettings()
  if (!raw || typeof raw !== 'object') {
    return base
  }
  const s = raw as Record<string, unknown>
  return new models.Settings({
    concurrent_limit: Math.max(1, Number(s.concurrent_limit) || 5),
    auto_refresh_tokens: Boolean(s.auto_refresh_tokens),
    auto_refresh_quotas: Boolean(s.auto_refresh_quotas),
    quota_refresh_policy: String(s.quota_refresh_policy || 'hybrid'),
    quota_custom_interval_minutes: clampQuotaMinutes(Number(s.quota_custom_interval_minutes)),
    auto_switch_plan_filter: normalizeSwitchPlanFilter(String(s.auto_switch_plan_filter ?? 'all')),
    auto_switch_on_quota_exhausted:
      'auto_switch_on_quota_exhausted' in s ? Boolean(s.auto_switch_on_quota_exhausted) : true,
    quota_hot_poll_seconds: clampHotPollSeconds(
      'quota_hot_poll_seconds' in s ? Number(s.quota_hot_poll_seconds) : 12,
    ),
    minimize_to_tray: Boolean(s.minimize_to_tray),
    silent_start: 'silent_start' in s ? Boolean(s.silent_start) : base.silent_start,
    openai_relay_enabled: 'openai_relay_enabled' in s ? Boolean(s.openai_relay_enabled) : base.openai_relay_enabled,
    openai_relay_port: Math.max(1, Math.min(65535, Number(s.openai_relay_port) || 8787)),
    openai_relay_secret: String(s.openai_relay_secret ?? ''),
    debug_log: 'debug_log' in s ? Boolean(s.debug_log) : false,
    import_concurrency: Math.max(1, Math.min(20, Number(s.import_concurrency) || 3)),
    forge_enabled: 'forge_enabled' in s ? Boolean(s.forge_enabled) : false,
    static_cache_intercept: 'static_cache_intercept' in s ? Boolean(s.static_cache_intercept) : true,
    mitm_full_capture: 'mitm_full_capture' in s ? Boolean(s.mitm_full_capture) : false,
    mitm_debug_dump: 'mitm_debug_dump' in s ? Boolean(s.mitm_debug_dump) : false,
  })
}

/** 规范化存储：all；或逗号分隔的合法 tone（去重、按固定顺序排序）。支持旧版单值 pro / trial 等。 */
export function normalizeSwitchPlanFilter(v: string | undefined | null): string {
  if (v == null || v === '' || v === 'undefined') {
    return 'all'
  }
  let s = String(v).trim().toLowerCase().replace(/，/g, ',')
  if (s === 'all') {
    return 'all'
  }
  const allowed = new Set<string>(SWITCH_PLAN_FILTER_TONES as unknown as string[])
  const parts = [
    ...new Set(
      s
        .split(',')
        .map((x) => x.trim())
        .filter(Boolean)
        .filter((x) => allowed.has(x)),
    ),
  ]
  if (parts.length === 0) {
    return 'all'
  }
  if (parts.length >= SWITCH_PLAN_FILTER_TONES.length) {
    return 'all'
  }
  parts.sort((a, b) => (TONE_ORDER.get(a as SwitchPlanTone) ?? 0) - (TONE_ORDER.get(b as SwitchPlanTone) ?? 0))
  return parts.join(',')
}

/** 用于界面展示当前范围文案 */
export function formatSwitchPlanFilterSummary(filter: string | undefined | null): string {
  const n = normalizeSwitchPlanFilter(filter ?? 'all')
  if (n === 'all') {
    return '全部计划（不限制）'
  }
  const labelByValue = Object.fromEntries(switchPlanFilterToneOptions.map((o) => [o.value, o.label]))
  return n
    .split(',')
    .map((t) => labelByValue[t] || t)
    .join('、')
}

export function clampQuotaMinutes(m: number): number {
  if (!Number.isFinite(m) || m <= 0) {
    return 360
  }
  return Math.min(10080, Math.max(5, Math.round(m)))
}

/** 当前活跃席位额度快查间隔（秒），与后端 clampQuotaHotPollSeconds 一致 */
export function clampHotPollSeconds(sec: number): number {
  if (!Number.isFinite(sec) || sec <= 0) {
    return 12
  }
  return Math.min(60, Math.max(5, Math.round(sec)))
}

/** 与后端 JSON 字段一致，便于 reactive + v-model */
export type SettingsForm = {
  concurrent_limit: number
  auto_refresh_tokens: boolean
  auto_refresh_quotas: boolean
  quota_refresh_policy: string
  quota_custom_interval_minutes: number
  /** 无感下一席位：all 或逗号分隔多选，如 trial,pro */
  auto_switch_plan_filter: string
  /** 额度用尽时自动切下一席（需开启定期同步额度） */
  auto_switch_on_quota_exhausted: boolean
  /** 当前活跃席位快查间隔（秒），用尽轮换依赖此轮询 */
  quota_hot_poll_seconds: number
  /** 关闭窗口时最小化到系统托盘 */
  minimize_to_tray: boolean
  /** 启动时不显示主窗口（托盘仍可打开） */
  silent_start: boolean
  /** OpenAI 兼容中转服务器 */
  openai_relay_enabled: boolean
  openai_relay_port: number
  openai_relay_secret: string
  /** 调试日志：开启后将切号/代理/额度判定写入 debug.log */
  debug_log: boolean
  /** 导入并发数 1～20 */
  import_concurrency: number
  /** GetUserStatus/GetPlanStatus 伪造为 Enterprise + 无限积分 */
  forge_enabled: boolean
  /** 静态响应缓存拦截 (.bin 文件直返) */
  static_cache_intercept: boolean
  /** MITM 全量抓包落盘 */
  mitm_full_capture: boolean
  /** MITM protobuf dump 诊断 */
  mitm_debug_dump: boolean
}

export function settingsToForm(s: models.Settings): SettingsForm {
  return {
    concurrent_limit: s.concurrent_limit || 5,
    auto_refresh_tokens: s.auto_refresh_tokens,
    auto_refresh_quotas: s.auto_refresh_quotas,
    quota_refresh_policy: s.quota_refresh_policy || 'hybrid',
    quota_custom_interval_minutes: clampQuotaMinutes(s.quota_custom_interval_minutes),
    auto_switch_plan_filter: normalizeSwitchPlanFilter(s.auto_switch_plan_filter),
    auto_switch_on_quota_exhausted: s.auto_switch_on_quota_exhausted !== false,
    quota_hot_poll_seconds: clampHotPollSeconds(s.quota_hot_poll_seconds ?? 12),
    minimize_to_tray: s.minimize_to_tray === true,
    silent_start: s.silent_start === true,
    openai_relay_enabled: s.openai_relay_enabled === true,
    openai_relay_port: Math.max(1, Number(s.openai_relay_port) || 8787),
    openai_relay_secret: String(s.openai_relay_secret ?? ''),
    debug_log: (s as any).debug_log === true,
    import_concurrency: Math.max(1, Math.min(20, Number((s as any).import_concurrency) || 3)),
    forge_enabled: (s as any).forge_enabled === true,
    static_cache_intercept: (s as any).static_cache_intercept !== false,
    mitm_full_capture: (s as any).mitm_full_capture === true,
    mitm_debug_dump: (s as any).mitm_debug_dump === true,
  }
}

export function formToSettings(form: SettingsForm): models.Settings {
  return new models.Settings({
    concurrent_limit: Math.max(1, Math.round(form.concurrent_limit) || 5),
    auto_refresh_tokens: form.auto_refresh_tokens,
    auto_refresh_quotas: form.auto_refresh_quotas,
    quota_refresh_policy: form.quota_refresh_policy || 'hybrid',
    quota_custom_interval_minutes: clampQuotaMinutes(form.quota_custom_interval_minutes),
    auto_switch_plan_filter: normalizeSwitchPlanFilter(form.auto_switch_plan_filter),
    auto_switch_on_quota_exhausted: form.auto_switch_on_quota_exhausted,
    quota_hot_poll_seconds: clampHotPollSeconds(form.quota_hot_poll_seconds),
    minimize_to_tray: form.minimize_to_tray,
    silent_start: form.silent_start,
    openai_relay_enabled: form.openai_relay_enabled,
    openai_relay_port: Math.max(1, Math.min(65535, Math.round(form.openai_relay_port) || 8787)),
    openai_relay_secret: (form.openai_relay_secret ?? '').trim(),
    debug_log: form.debug_log,
    import_concurrency: Math.max(1, Math.min(20, Math.round(form.import_concurrency) || 3)),
    forge_enabled: form.forge_enabled,
    static_cache_intercept: form.static_cache_intercept,
    mitm_full_capture: form.mitm_full_capture,
    mitm_debug_dump: form.mitm_debug_dump,
  })
}

export const quotaPolicyOptions: Array<{ value: string; label: string }> = [
  { value: 'hybrid', label: '美东换日或满 24h（推荐）' },
  { value: 'interval_24h', label: '固定每 24 小时' },
  { value: 'us_calendar', label: '仅美东日历跨日' },
  { value: 'local_calendar', label: '本机时区跨日' },
  { value: 'interval_1h', label: '每 1 小时' },
  { value: 'interval_6h', label: '每 6 小时' },
  { value: 'interval_12h', label: '每 12 小时' },
  { value: 'custom', label: '自定义间隔（分钟）' },
]
