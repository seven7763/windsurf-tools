import { SWITCH_PLAN_FILTER_TONES, type SwitchPlanTone } from './settingsModel'

/** 号池计划分布图用色（与 cockpit 类仪表盘区分度） */
export const PLAN_TONE_CHART_COLORS: Record<SwitchPlanTone, string> = {
  pro: '#2563eb',
  max: '#7c3aed',
  team: '#4f46e5',
  enterprise: '#475569',
  trial: '#d97706',
  free: '#94a3b8',
  unknown: '#71717a',
}

export type PlanToneCounts = Partial<Record<SwitchPlanTone, number>>

/** 号池总数 */
export function sumPlanToneCounts(counts: PlanToneCounts): number {
  return SWITCH_PLAN_FILTER_TONES.reduce((s, t) => s + (counts[t] ?? 0), 0)
}

/**
 * 从 -90° 起算的 conic-gradient，首段在 12 点方向，用于环形图底。
 */
export function buildPlanPoolConicGradient(counts: PlanToneCounts): string | null {
  const total = sumPlanToneCounts(counts)
  if (total <= 0) return null
  let acc = 0
  const parts: string[] = []
  for (const t of SWITCH_PLAN_FILTER_TONES) {
    const c = counts[t] ?? 0
    if (c <= 0) continue
    const pct = (c / total) * 100
    const start = acc
    acc += pct
    parts.push(`${PLAN_TONE_CHART_COLORS[t]} ${start}% ${acc}%`)
  }
  if (parts.length === 0) return null
  return `conic-gradient(from -90deg, ${parts.join(', ')})`
}

/** 在「部分计划」下，号池内会被轮换范围覆盖的账号数 */
export function countAccountsInFilter(
  counts: PlanToneCounts,
  selectedTones: Set<string>,
): number {
  let n = 0
  for (const t of SWITCH_PLAN_FILTER_TONES) {
    if (selectedTones.has(t)) {
      n += counts[t] ?? 0
    }
  }
  return n
}
