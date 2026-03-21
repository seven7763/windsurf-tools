<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useAccountStore } from '../stores/useAccountStore'
import { useSystemStore } from '../stores/useSystemStore'
import { useSettingsStore } from '../stores/useSettingsStore'
import { useMitmStatusStore } from '../stores/useMitmStatusStore'
import SmartInsightsCard from '../components/dashboard/SmartInsightsCard.vue'
import MitmPanel from '../components/MitmPanel.vue'
import PlanDistributionDonut from '../components/settings/PlanDistributionDonut.vue'
import { computeDashboardInsights } from '../utils/dashboardInsights'
import { KeyRound, Users, CheckCircle, AlertTriangle, Activity, BarChart3, Wifi } from 'lucide-vue-next'
import { showToast } from '../utils/toast'
import { getPlanTone } from '../utils/account'
import { SWITCH_PLAN_FILTER_TONES, switchPlanFilterToneOptions, type SwitchPlanTone } from '../utils/settingsModel'
import { PLAN_TONE_CHART_COLORS } from '../utils/planToneChart'

const accountStore = useAccountStore()
const systemStore = useSystemStore()
const settingsStore = useSettingsStore()
const mitmStore = useMitmStatusStore()

const mitmOnly = computed(() => settingsStore.settings?.mitm_only === true)
const status = computed(() => mitmStore.status)

const dashboardInsights = computed(() =>
  computeDashboardInsights({
    settings: settingsStore.settings ?? null,
    accounts: accountStore.accounts,
    mitmStatus: status.value,
    mitmOnly: mitmOnly.value,
    patchApplied: systemStore.patchStatus,
    windsurfPath: systemStore.windsurfPath,
  })
)

const currentOnlineAccount = computed(() => {
  const email = systemStore.currentAuthEmail
  if (!email?.trim()) return null
  const e = email.trim().toLowerCase()
  return accountStore.accounts.find((a) => (a.email || '').trim().toLowerCase() === e) ?? null
})

onMounted(() => {
  accountStore.fetchAccounts()
  settingsStore.fetchSettings()
  systemStore.initSystemEnvironment()
  mitmStore.startPolling()
})

onUnmounted(() => {
  mitmStore.stopPolling()
})

const handleRefreshAllQuotas = async () => {
  try {
    const map = await accountStore.refreshAllQuotas()
    const entries = Object.entries(map || {})
    const synced = entries.filter(([, v]) => String(v).includes('已同步')).length
    showToast(`额度已同步：${synced} / ${entries.length} 个账号`, 'success')
  } catch (e: unknown) {
    showToast(`同步额度失败: ${String(e)}`, 'error')
  }
}

const handleRefreshAllTokens = async () => {
  try {
    const map = await accountStore.refreshAllTokens()
    const entries = Object.entries(map || {})
    const ok = entries.filter(([, v]) => String(v).includes('成功')).length
    showToast(`凭证刷新：${ok} / ${entries.length}`, 'success')
  } catch (e: unknown) {
    showToast(`刷新凭证失败: ${String(e)}`, 'error')
  }
}

// Setup KPIs
const totalAccounts = computed(() => accountStore.accounts.length)

const parseQuota = (str: string | undefined | null) => {
  if (!str) return null
  const n = parseFloat(String(str).replace('%', '').trim())
  return Number.isFinite(n) ? n : null
}

const lowQuotaCount = computed(() => {
  return accountStore.accounts.filter(a => {
    const q = parseQuota(a.daily_remaining)
    return q !== null && q > 0 && q < 20
  }).length
})

const expiredCount = computed(() => {
  return accountStore.accounts.filter(a => {
    const q = parseQuota(a.daily_remaining)
    return q === 0
  }).length
})

const normalCount = computed(() => {
  const c = totalAccounts.value - lowQuotaCount.value - expiredCount.value
  return c < 0 ? 0 : c
})

const avgQuota = computed(() => {
  const valid = accountStore.accounts.map(a => parseQuota(a.daily_remaining)).filter(q => q !== null) as number[]
  if (valid.length === 0) return '0%'
  const sum = valid.reduce((acc, curr) => acc + curr, 0)
  return Math.round(sum / valid.length) + '%'
})

const healthRate = computed(() => {
  if (totalAccounts.value === 0) return 0
  return Math.round((normalCount.value / totalAccounts.value) * 100)
})

// Setup Plans Breakdown
const planToneCounts = computed<Partial<Record<SwitchPlanTone, number>>>(() => {
  const counts: Partial<Record<SwitchPlanTone, number>> = {}
  for (const tone of SWITCH_PLAN_FILTER_TONES) {
    counts[tone] = 0
  }
  for (const account of accountStore.accounts) {
    const tone = getPlanTone(account.plan_name) as SwitchPlanTone
    counts[tone] = (counts[tone] ?? 0) + 1
  }
  return counts
})

const planLabelMap = new Map(switchPlanFilterToneOptions.map((option) => [option.value, option.label]))

const planRows = computed(() => {
  const total = accountStore.accounts.length
  return SWITCH_PLAN_FILTER_TONES.map((tone) => {
    const count = planToneCounts.value[tone] ?? 0
    return {
      tone,
      label: planLabelMap.get(tone) ?? tone,
      count,
      pct: total > 0 ? (count / total) * 100 : 0,
      color: PLAN_TONE_CHART_COLORS[tone],
    }
  }).filter((row) => row.count > 0)
})

const circumference = 2 * Math.PI * 45 // radius 45
const dashOffset = computed(() => circumference * (1 - healthRate.value / 100))
</script>

<template>
  <div class="p-6 md:p-8 max-w-6xl w-full mx-auto pb-10">
    
    <!-- Top Header & Buttons -->
    <div class="flex items-start justify-between mb-8 shrink-0 flex-wrap gap-4">
      <div>
        <h1 class="text-[32px] font-[800] text-gray-900 dark:text-gray-100 tracking-tight leading-none">控制台</h1>
        <div class="flex items-center gap-2 mt-4">
          <p class="text-[13px] text-gray-500 font-medium">系统状态与资产概览</p>
          <div
            v-if="!currentOnlineAccount"
            class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-black/5 dark:bg-white/5 text-[11px] text-gray-500 font-medium ml-2"
          >
            <Wifi class="w-3.5 h-3.5" />
            未检测到在线账号
          </div>
          <div
            v-else
            class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-500/10 text-[11px] text-emerald-600 dark:text-emerald-400 font-medium ml-2"
          >
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.8)] animate-pulse"></span>
            当前在线: {{ currentOnlineAccount.email }}
          </div>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <button
          @click="handleRefreshAllQuotas"
          :disabled="accountStore.actionLoading"
          class="no-drag-region flex items-center gap-1.5 px-4 py-2.5 bg-emerald-50 dark:bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 rounded-full font-bold text-[13px] hover:bg-emerald-100 dark:hover:bg-emerald-500/20 transition-all ios-btn"
        >
          <BarChart3 class="w-[18px] h-[18px]" stroke-width="2.5" />
          同步额度
        </button>
        <button
          @click="handleRefreshAllTokens"
          :disabled="accountStore.actionLoading"
          class="no-drag-region flex items-center gap-1.5 px-4 py-2.5 bg-black/5 dark:bg-white/10 text-gray-700 dark:text-gray-200 rounded-full font-bold text-[13px] hover:bg-black/10 dark:hover:bg-white/15 transition-all ios-btn"
        >
          <KeyRound class="w-[18px] h-[18px]" stroke-width="2.5" />
          刷新凭证
        </button>
      </div>
    </div>

    <!-- Smart Insights -->
    <SmartInsightsCard :insights="dashboardInsights" />

    <!-- 4 KPI Cards -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6 shrink-0">
      <div class="ios-glass bg-white/60 dark:bg-[#1C1C1E]/60 rounded-[24px] p-5 border border-black/5 dark:border-white/5 flex flex-col justify-between">
        <div class="text-[32px] font-extrabold text-gray-900 dark:text-gray-100 leading-none mb-3 tracking-tight">{{ totalAccounts }}</div>
        <div class="flex items-center text-[12px] text-gray-500 dark:text-gray-400 font-medium">
          <Users class="w-4 h-4 mr-1.5 opacity-70" stroke-width="2.5" /> 总账号
        </div>
      </div>
      <div class="ios-glass bg-white/60 dark:bg-[#1C1C1E]/60 rounded-[24px] p-5 border border-black/5 dark:border-white/5 flex flex-col justify-between">
        <div class="text-[32px] font-extrabold text-emerald-500 leading-none mb-3 tracking-tight">{{ normalCount }}</div>
        <div class="flex items-center text-[12px] text-gray-500 dark:text-gray-400 font-medium">
          <CheckCircle class="w-4 h-4 mr-1.5 text-emerald-500 opacity-80" stroke-width="2.5" /> 状态正常
        </div>
      </div>
      <div class="ios-glass bg-white/60 dark:bg-[#1C1C1E]/60 rounded-[24px] p-5 border border-black/5 dark:border-white/5 flex flex-col justify-between">
        <div class="text-[32px] font-extrabold text-gray-900 dark:text-gray-100 leading-none mb-3 tracking-tight">{{ lowQuotaCount }}</div>
        <div class="flex items-center text-[12px] text-gray-500 dark:text-gray-400 font-medium">
          <AlertTriangle class="w-4 h-4 mr-1.5 opacity-70" stroke-width="2.5" /> 额度偏低
        </div>
      </div>
      <div class="ios-glass bg-white/60 dark:bg-[#1C1C1E]/60 rounded-[24px] p-5 border border-black/5 dark:border-white/5 flex flex-col justify-between">
        <div class="text-[32px] font-extrabold text-gray-900 dark:text-gray-100 leading-none mb-3 tracking-tight">{{ avgQuota }}</div>
        <div class="flex items-center text-[12px] text-gray-500 dark:text-gray-400 font-medium">
          <Activity class="w-4 h-4 mr-1.5 opacity-70" stroke-width="2.5" /> 平均日额度
        </div>
      </div>
    </div>

    <!-- Main Grid Layout -->
    <div class="grid grid-cols-1 md:grid-cols-[1.25fr_1fr] lg:grid-cols-[1.5fr_1fr] gap-6">
      
      <!-- Left Column -->
      <div class="flex flex-col gap-6">
        <MitmPanel />
      </div>

      <!-- Right Column -->
      <div class="flex flex-col gap-6">

        <!-- Donut Chart Card -->
        <div class="ios-glass bg-white/60 dark:bg-[#1C1C1E]/60 rounded-[28px] p-6 border border-black/5 dark:border-white/5 flex flex-col items-center justify-center h-[260px]">
          <div class="relative w-[120px] h-[120px] mb-6">
            <svg class="w-full h-full -rotate-90 transform drop-shadow-sm" viewBox="0 0 100 100">
              <!-- Background Circle -->
              <circle
                cx="50" cy="50" r="45"
                fill="none"
                class="stroke-gray-100 dark:stroke-gray-800"
                stroke-width="10"
              />
              <!-- Progress Circle -->
              <circle
                cx="50" cy="50" r="45"
                fill="none"
                class="stroke-emerald-400"
                stroke-width="10"
                stroke-linecap="round"
                :stroke-dasharray="circumference"
                :stroke-dashoffset="dashOffset"
                style="transition: stroke-dashoffset 1.5s cubic-bezier(0.2, 0.8, 0.2, 1);"
              />
            </svg>
            <div class="absolute inset-0 flex flex-col items-center justify-center mt-1">
              <span class="text-[26px] font-extrabold text-gray-900 dark:text-gray-100 leading-none">{{ healthRate }}%</span>
              <span class="text-[11px] text-gray-500 font-bold mt-1 tracking-wide">健康率</span>
            </div>
          </div>
          <div class="w-full grid grid-cols-3 text-center px-2">
            <div class="flex flex-col">
              <span class="text-[18px] font-bold text-emerald-500 leading-tight">{{ normalCount }}</span>
              <span class="text-[11px] text-gray-500 mt-1">正常</span>
            </div>
            <div class="flex flex-col">
              <span class="text-[18px] font-bold text-gray-900 dark:text-gray-100 leading-tight">{{ lowQuotaCount }}</span>
              <span class="text-[11px] text-gray-500 mt-1">偏低</span>
            </div>
            <div class="flex flex-col">
              <span class="text-[18px] font-bold text-gray-900 dark:text-gray-100 leading-tight">{{ expiredCount }}</span>
              <span class="text-[11px] text-gray-500 mt-1">过期</span>
            </div>
          </div>
        </div>

        <!-- Plan Breakdown Card -->
        <div class="ios-glass bg-white/60 dark:bg-[#1C1C1E]/60 rounded-[28px] p-6 border border-black/5 dark:border-white/5 flex-1 flex flex-col">
          <h3 class="text-[14px] font-bold text-gray-400 dark:text-gray-500 tracking-wide mb-6">计划分布</h3>

          <div class="flex flex-col gap-5 sm:flex-row sm:items-start sm:gap-6">
            <PlanDistributionDonut :counts="planToneCounts" compact />

            <div class="flex-1 space-y-3">
              <div
                v-for="row in planRows"
                :key="row.tone"
                class="flex items-center justify-between gap-3 text-[13px] font-medium"
              >
                <div class="flex min-w-0 items-center gap-2.5">
                  <span
                    class="h-2.5 w-2.5 shrink-0 rounded-full ring-1 ring-black/10 dark:ring-white/15"
                    :style="{ backgroundColor: row.color }"
                  />
                  <span class="truncate text-gray-900 dark:text-gray-100 font-bold">{{ row.label }}</span>
                </div>
                <div class="flex shrink-0 items-center gap-3">
                  <span class="text-[11px] text-gray-400 dark:text-gray-500 tabular-nums">{{ Math.round(row.pct) }}%</span>
                  <span class="font-bold text-[16px] tabular-nums">{{ row.count }}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="mt-auto pt-8">
            <div class="flex w-full h-[6px] rounded-full overflow-hidden shrink-0 bg-gray-100 dark:bg-white/10" style="gap: 2px;">
              <div
                v-for="row in planRows"
                :key="`${row.tone}-bar`"
                class="h-full rounded-full flex-none transition-all duration-500"
                :style="{ width: `${row.pct}%`, backgroundColor: row.color }"
              />
            </div>
          </div>
        </div>

      </div>

    </div>
  </div>
</template>
