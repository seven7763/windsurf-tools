import type { Account, HealthFilter, PlanFilter } from '../types/windsurf'

type HealthTone = Exclude<HealthFilter, 'all'>
type PlanTone = Exclude<PlanFilter, 'all'>

const DASH = '—'
const ASIA_SHANGHAI = 'Asia/Shanghai'

function parseDateValue(value?: string): number {
  if (!value) {
    return 0
  }

  const time = new Date(value).getTime()
  return Number.isNaN(time) ? 0 : time
}

export function formatDateTime(value?: string, fallback = DASH): string {
  if (!value) {
    return fallback
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }

  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: ASIA_SHANGHAI,
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

export function formatCompactDate(value?: string, fallback = DASH): string {
  if (!value) {
    return fallback
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }

  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: ASIA_SHANGHAI,
    month: 'numeric',
    day: 'numeric',
  }).format(date)
}

/** 订阅/试用结束时间；空时提示同步（后端从 GetPlanStatus / JWT 拉取） */
export function formatSubscriptionExpiryLine(value?: string): string {
  if (!value) {
    return '订阅/试用到期 — 暂无（请先「同步额度」）'
  }
  return `订阅/试用到期（东八区） ${formatDateTime(value)}`
}

export function truncateMiddle(value?: string, head = 10, tail = 6): string {
  if (!value) {
    return DASH
  }

  if (value.length <= head + tail + 3) {
    return value
  }

  return `${value.slice(0, head)}...${value.slice(-tail)}`
}

export function parsePercent(value?: string): number | null {
  if (!value) {
    return null
  }

  const normalized = value.replace('%', '').trim()
  const numeric = Number(normalized)
  return Number.isFinite(numeric) ? numeric : null
}

export function getMonthlyRemaining(account: Account): number | null {
  const total = account.total_quota ?? 0
  if (total <= 0) {
    return null
  }

  const used = Math.max(0, account.used_quota ?? 0)
  const remaining = Math.max(0, total - used)
  return (remaining / total) * 100
}

export function getLowestRemaining(account: Account): number | null {
  const candidates = [
    parsePercent(account.daily_remaining),
    parsePercent(account.weekly_remaining),
    getMonthlyRemaining(account),
  ].filter((value): value is number => value !== null)

  if (!candidates.length) {
    return null
  }

  return Math.min(...candidates)
}

export function getPlanTone(plan?: string): PlanTone {
  const normalized = (plan ?? '').toLowerCase().replaceAll('_', ' ').trim()
  if (!normalized || normalized === 'unknown') {
    return 'unknown'
  }

  if (normalized.includes('trial')) {
    return 'trial'
  }
  if (normalized.includes('max') || normalized.includes('ultimate')) {
    return 'max'
  }
  if (normalized.includes('enterprise')) {
    return 'enterprise'
  }
  if (normalized.includes('team')) {
    return 'team'
  }
  if (normalized.includes('pro')) {
    return 'pro'
  }
  if (normalized.includes('free')) {
    return 'free'
  }
  if (normalized.includes('basic')) {
    return 'free'
  }
  return 'unknown'
}

export function getPlanLabel(plan?: string): string {
  if (!plan) {
    return 'UNKNOWN'
  }

  switch (getPlanTone(plan)) {
    case 'trial':
      return 'TRIAL'
    case 'max':
      return 'MAX'
    case 'enterprise':
      return 'ENTERPRISE'
    case 'team':
      return 'TEAMS'
    case 'pro':
      return 'PRO'
    case 'free':
      return 'FREE'
    default:
      return plan.toUpperCase()
  }
}

export function getAccountHealth(account: Account): HealthTone {
  const status = account.status?.toLowerCase() ?? ''
  const now = Date.now()
  const expiresAt = parseDateValue(account.subscription_expires_at)

  if (status === 'expired' || status === 'disabled' || (expiresAt > 0 && expiresAt < now)) {
    return 'expired'
  }

  const lowest = getLowestRemaining(account)
  if (lowest === null) {
    return 'unknown'
  }

  if (lowest < 20) {
    return 'critical'
  }

  return 'healthy'
}

export function getHealthLabel(health: HealthTone): string {
  switch (health) {
    case 'healthy':
      return '状态稳定'
    case 'critical':
      return '额度偏低'
    case 'expired':
      return '已过期'
    default:
      return '待补全'
  }
}

export function getTokenSource(account: Account): string {
  if (account.windsurf_api_key) {
    return 'API Key'
  }
  if (account.refresh_token) {
    return 'Refresh Token'
  }
  if (account.token) {
    return 'JWT'
  }
  return '未记录'
}

export function getTokenPreview(account: Account): string {
  if (account.windsurf_api_key) {
    return truncateMiddle(account.windsurf_api_key, 14, 6)
  }
  if (account.refresh_token) {
    return truncateMiddle(account.refresh_token, 14, 6)
  }
  if (account.token) {
    return truncateMiddle(account.token, 14, 6)
  }
  return DASH
}

export function getPrimaryTimestamp(account: Account): number {
  return Math.max(
    parseDateValue(account.last_quota_update),
    parseDateValue(account.last_login_at),
    parseDateValue(account.created_at),
  )
}

export function matchesSearch(account: Account, query: string): boolean {
  if (!query) {
    return true
  }

  const keyword = query.trim().toLowerCase()
  if (!keyword) {
    return true
  }

  const haystack = [
    account.nickname,
    account.email,
    account.plan_name,
    account.remark,
    account.status,
    account.tags,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()

  return haystack.includes(keyword)
}

export function matchesPlan(account: Account, filter: PlanFilter): boolean {
  if (filter === 'all') {
    return true
  }

  return getPlanTone(account.plan_name) === filter
}

export function matchesHealth(account: Account, filter: HealthFilter): boolean {
  if (filter === 'all') {
    return true
  }

  return getAccountHealth(account) === filter
}

export function formatQuota(value?: string): string {
  return value?.trim() || DASH
}

export function formatMonthlyUsage(account: Account): string {
  const total = account.total_quota ?? 0
  if (total <= 0) {
    return DASH
  }

  return `${account.used_quota ?? 0} / ${total}`
}
