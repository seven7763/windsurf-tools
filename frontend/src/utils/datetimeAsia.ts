/** 展示用：统一按 Asia/Shanghai（与产品「东八区」说明一致） */

const SHANGHAI = 'Asia/Shanghai'

/** 解析 ISO / RFC3339；失败返回 null */
export function parseDateLoose(s: string): Date | null {
  if (!s?.trim()) return null
  const d = new Date(s.trim())
  return Number.isNaN(d.getTime()) ? null : d
}

/**
 * 完整日期时间（东八区），避免误用本地时区却标注 GMT+8。
 * 例：2026年4月15日 01:58（东八区）
 */
export function formatDateTimeAsiaShanghai(iso: string): string {
  const d = parseDateLoose(iso)
  if (!d) return iso
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: SHANGHAI,
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(d)
}

/** 相对「下次刷新」的简短中文（用于额度条下方） */
export function formatResetCountdownZH(targetIso: string, now: Date = new Date()): string {
  const t = parseDateLoose(targetIso)
  if (!t) return ''
  const diff = t.getTime() - now.getTime()
  if (diff <= 0) return '即将刷新'
  const m = Math.ceil(diff / 60000)
  const h = Math.floor(m / 60)
  const d = Math.floor(h / 24)
  if (d >= 2) return `约 ${d} 天后刷新`
  if (d === 1) return '约 1 天后刷新'
  if (h >= 2) return `约 ${h} 小时后刷新`
  if (h === 1) return '约 1 小时后刷新'
  if (m >= 2) return `约 ${m} 分钟后刷新`
  return '即将刷新'
}

/** 同步时间一行（last_quota_update 常为 RFC3339 或无 Z） */
export function formatSyncTimeLine(iso: string): string {
  const d = parseDateLoose(iso)
  if (!d) return iso
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: SHANGHAI,
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(d)
}
