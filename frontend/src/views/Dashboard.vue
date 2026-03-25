<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useAccountStore } from '../stores/useAccountStore'
import { useSystemStore } from '../stores/useSystemStore'
import { useSettingsStore } from '../stores/useSettingsStore'
import { useMitmStatusStore } from '../stores/useMitmStatusStore'
import { useMainViewStore } from '../stores/useMainViewStore'
import SmartInsightsCard from '../components/dashboard/SmartInsightsCard.vue'
import MitmPanel from '../components/MitmPanel.vue'
import PlanDistributionDonut from '../components/settings/PlanDistributionDonut.vue'
import { computeDashboardInsights } from '../utils/dashboardInsights'
import {
  KeyRound,
  Users,
  CheckCircle,
  AlertTriangle,
  Activity,
  BarChart3,
  Wifi,
  Sparkles,
  ChevronRight,
  RefreshCcw,
} from 'lucide-vue-next'
import { showToast } from '../utils/toast'
import { getPlanTone } from '../utils/account'
import { SWITCH_PLAN_FILTER_TONES, switchPlanFilterToneOptions, type SwitchPlanTone } from '../utils/settingsModel'
import { PLAN_TONE_CHART_COLORS } from '../utils/planToneChart'

const accountStore = useAccountStore()
const systemStore = useSystemStore()
const settingsStore = useSettingsStore()
const mitmStore = useMitmStatusStore()
const mainView = useMainViewStore()

type StartupGuideAction = 'refresh_session' | 'open_accounts' | 'open_settings'
type StartupGuideTone = 'sky' | 'emerald' | 'amber' | 'violet'
type StartupGuide = {
  id: string
  badge: string
  title: string
  body: string
  tone: StartupGuideTone
  steps: string[]
  primaryAction: StartupGuideAction
  primaryLabel: string
  secondaryAction?: StartupGuideAction
  secondaryLabel?: string
}

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

const currentAuthEmail = computed(() => (systemStore.currentAuthEmail || '').trim())

const startupGuide = computed<StartupGuide | null>(() => {
  const email = currentAuthEmail.value

  if (!email && totalAccounts.value === 0) {
    return {
      id: 'first_login_required',
      badge: '首次启动',
      title: '先在 Windsurf 完成一次官方登录',
      body: '当前这台机器还没检测到 Windsurf 登录态，账号池也还是空的。先让官方客户端在本机生成登录信息，后面才能导入、同步额度和自动切号。',
      tone: 'sky',
      steps: [
        '打开 Windsurf，使用你的账号完成一次正常登录。',
        '回到这里点击“刷新当前会话”，确认首页能检测到在线邮箱。',
        '再进入账号池导入账号；如果准备用本地 Auth 切号，后续去设置里确认安装路径。',
      ],
      primaryAction: 'refresh_session',
      primaryLabel: '我已登录，刷新检测',
      secondaryAction: 'open_accounts',
      secondaryLabel: '打开账号池',
    }
  }

  if (email && totalAccounts.value === 0) {
    return {
      id: 'login_detected_pool_empty',
      badge: '已检测到登录',
      title: '把当前 Windsurf 会话接入号池',
      body: `当前已检测到本机登录账号 ${email}，但账号池还是空的。本软件还无法为这个账号同步额度、自动切号或接入 MITM 轮换。`,
      tone: 'emerald',
      steps: [
        '进入账号池，导入当前账号或批量导入你的账号列表。',
        '导入后刷新凭证与额度，让首页开始显示健康状态。',
        mitmOnly.value
          ? '如果你准备只走 MITM 轮换，优先补齐 API Key / JWT。'
          : '如果你准备走本地 Auth 切号，再去设置里确认路径与运行模式。',
      ],
      primaryAction: 'open_accounts',
      primaryLabel: '去导入账号',
      secondaryAction: 'open_settings',
      secondaryLabel: '打开设置',
    }
  }

  if (email && totalAccounts.value > 0 && !currentOnlineAccount.value) {
    return {
      id: 'current_session_not_managed',
      badge: '尚未接管',
      title: '当前在线账号还不在号池里',
      body: `检测到 ${email} 已在 Windsurf 中登录，但它不在当前账号池里。本软件现在还无法对这个会话做额度同步、切号或 MITM 接管。`,
      tone: 'amber',
      steps: [
        '去账号池导入当前在线账号，或者切换到一个已经导入的账号。',
        mitmOnly.value
          ? '如果你想完全走 MITM 轮换，请确认账号池里已有可用 API Key / JWT。'
          : '如果你想继续用 Auth 切号，请顺手确认设置里的 Windsurf 路径与补丁状态。',
        '接管完成后，再回来刷新当前会话，首页会开始显示对应在线状态。',
      ],
      primaryAction: 'open_accounts',
      primaryLabel: '去账号池处理',
      secondaryAction: 'refresh_session',
      secondaryLabel: '刷新当前会话',
    }
  }

  return null
})

const startupGuideToneClass = (tone: StartupGuideTone) => {
  switch (tone) {
    case 'emerald':
      return {
        wrap: 'border-emerald-500/14 bg-[radial-gradient(circle_at_top_left,rgba(16,185,129,0.16),transparent_36%),linear-gradient(180deg,rgba(255,255,255,0.9),rgba(255,255,255,0.76))] dark:bg-[radial-gradient(circle_at_top_left,rgba(16,185,129,0.18),transparent_36%),linear-gradient(180deg,rgba(28,28,30,0.96),rgba(28,28,30,0.88))]',
        icon: 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-300',
        badge: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
        step: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
        primary: 'bg-emerald-600 text-white hover:bg-emerald-500',
      }
    case 'amber':
      return {
        wrap: 'border-amber-500/14 bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.16),transparent_36%),linear-gradient(180deg,rgba(255,255,255,0.9),rgba(255,255,255,0.76))] dark:bg-[radial-gradient(circle_at_top_left,rgba(245,158,11,0.18),transparent_36%),linear-gradient(180deg,rgba(28,28,30,0.96),rgba(28,28,30,0.88))]',
        icon: 'bg-amber-500/12 text-amber-600 dark:text-amber-300',
        badge: 'bg-amber-500/10 text-amber-700 dark:text-amber-300',
        step: 'bg-amber-500/10 text-amber-700 dark:text-amber-300',
        primary: 'bg-amber-600 text-white hover:bg-amber-500',
      }
    case 'violet':
      return {
        wrap: 'border-violet-500/14 bg-[radial-gradient(circle_at_top_left,rgba(139,92,246,0.18),transparent_36%),linear-gradient(180deg,rgba(255,255,255,0.9),rgba(255,255,255,0.76))] dark:bg-[radial-gradient(circle_at_top_left,rgba(139,92,246,0.2),transparent_36%),linear-gradient(180deg,rgba(28,28,30,0.96),rgba(28,28,30,0.88))]',
        icon: 'bg-violet-500/12 text-violet-600 dark:text-violet-300',
        badge: 'bg-violet-500/10 text-violet-700 dark:text-violet-300',
        step: 'bg-violet-500/10 text-violet-700 dark:text-violet-300',
        primary: 'bg-violet-600 text-white hover:bg-violet-500',
      }
    case 'sky':
    default:
      return {
        wrap: 'border-ios-blue/14 bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.16),transparent_36%),linear-gradient(180deg,rgba(255,255,255,0.9),rgba(255,255,255,0.76))] dark:bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.2),transparent_36%),linear-gradient(180deg,rgba(28,28,30,0.96),rgba(28,28,30,0.88))]',
        icon: 'bg-ios-blue/12 text-ios-blue dark:text-blue-300',
        badge: 'bg-ios-blue/10 text-ios-blue dark:text-blue-300',
        step: 'bg-ios-blue/10 text-ios-blue dark:text-blue-300',
        primary: 'bg-ios-blue text-white hover:bg-blue-500',
      }
  }
}

const runStartupGuideAction = async (action: StartupGuideAction) => {
  switch (action) {
    case 'open_accounts':
      mainView.activeTab = 'Accounts'
      return
    case 'open_settings':
      mainView.activeTab = 'Settings'
      return
    case 'refresh_session':
      try {
        await Promise.all([systemStore.fetchCurrentAuth(), accountStore.fetchAccounts(true)])
        if (systemStore.currentAuthEmail?.trim()) {
          showToast(`已检测到当前会话：${systemStore.currentAuthEmail}`, 'success')
        } else {
          showToast('还没检测到 Windsurf 登录，请先在官方客户端完成一次登录后再刷新。', 'info')
        }
      } catch (e: unknown) {
        showToast(`刷新当前会话失败: ${String(e)}`, 'error')
      }
      return
  }
}

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

const isQuotaDepletedAccount = (account: { daily_remaining?: string | null; weekly_remaining?: string | null }) => {
  const daily = parseQuota(account.daily_remaining)
  const weekly = parseQuota(account.weekly_remaining)
  const dailyKnown = daily !== null
  const weeklyKnown = weekly !== null
  return Boolean(dailyKnown && daily <= 0 && (!weeklyKnown || (weekly ?? 0) <= 0))
}

const isLowQuotaAccount = (account: { daily_remaining?: string | null; weekly_remaining?: string | null }) => {
  if (isQuotaDepletedAccount(account)) {
    return false
  }
  const daily = parseQuota(account.daily_remaining)
  const weekly = parseQuota(account.weekly_remaining)
  return Boolean(
    (daily !== null && daily > 0 && daily < 20) ||
      (weekly !== null && weekly > 0 && weekly < 20),
  )
}

const lowQuotaCount = computed(() => accountStore.accounts.filter(isLowQuotaAccount).length)

const depletedCount = computed(() => accountStore.accounts.filter(isQuotaDepletedAccount).length)

const normalCount = computed(() => {
  const c = totalAccounts.value - lowQuotaCount.value - depletedCount.value
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

    <div
      v-if="startupGuide"
      class="mb-6 overflow-hidden rounded-[26px] border shadow-[0_14px_42px_-18px_rgba(15,23,42,0.2)] dark:shadow-[0_14px_42px_-18px_rgba(0,0,0,0.45)]"
      :class="startupGuideToneClass(startupGuide.tone).wrap"
    >
      <div class="flex items-start gap-4 border-b border-black/[0.05] px-5 py-5 dark:border-white/[0.06]">
        <div
          class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ring-1 ring-black/[0.04] dark:ring-white/[0.06]"
          :class="startupGuideToneClass(startupGuide.tone).icon"
        >
          <Sparkles class="h-5 w-5" stroke-width="2.4" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2 flex-wrap">
            <span
              class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.18em]"
              :class="startupGuideToneClass(startupGuide.tone).badge"
            >
              {{ startupGuide.badge }}
            </span>
            <span class="text-[11px] font-semibold text-gray-400 dark:text-gray-500">首次接入引导</span>
          </div>
          <h2 class="mt-2 text-[20px] font-[800] tracking-tight text-gray-900 dark:text-gray-100">{{ startupGuide.title }}</h2>
          <p class="mt-2 max-w-3xl text-[13px] leading-relaxed text-gray-600 dark:text-gray-300">
            {{ startupGuide.body }}
          </p>
        </div>
      </div>

      <div class="grid gap-5 px-5 py-5 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
        <div class="space-y-2.5">
          <div
            v-for="(step, index) in startupGuide.steps"
            :key="`${startupGuide.id}-${index}`"
            class="flex items-start gap-3 rounded-[18px] border border-black/[0.04] bg-white/75 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
          >
            <span
              class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-black"
              :class="startupGuideToneClass(startupGuide.tone).step"
            >
              {{ index + 1 }}
            </span>
            <span class="text-[13px] font-medium leading-relaxed text-gray-700 dark:text-gray-200">{{ step }}</span>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-2 lg:w-[220px] lg:flex-col lg:items-stretch">
          <button
            type="button"
            class="no-drag-region inline-flex items-center justify-center gap-2 rounded-[14px] px-4 py-3 text-[13px] font-bold transition-colors ios-btn"
            :class="startupGuideToneClass(startupGuide.tone).primary"
            @click="runStartupGuideAction(startupGuide.primaryAction)"
          >
            <RefreshCcw
              v-if="startupGuide.primaryAction === 'refresh_session'"
              class="h-4 w-4"
              stroke-width="2.4"
            />
            <Users
              v-else-if="startupGuide.primaryAction === 'open_accounts'"
              class="h-4 w-4"
              stroke-width="2.4"
            />
            <BarChart3
              v-else
              class="h-4 w-4"
              stroke-width="2.4"
            />
            {{ startupGuide.primaryLabel }}
          </button>
          <button
            v-if="startupGuide.secondaryAction && startupGuide.secondaryLabel"
            type="button"
            class="no-drag-region inline-flex items-center justify-center gap-2 rounded-[14px] border border-black/[0.06] bg-white/80 px-4 py-3 text-[13px] font-bold text-gray-700 transition-colors hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-gray-200 dark:hover:bg-white/[0.08] ios-btn"
            @click="runStartupGuideAction(startupGuide.secondaryAction)"
          >
            {{ startupGuide.secondaryLabel }}
            <ChevronRight class="h-4 w-4" stroke-width="2.4" />
          </button>
        </div>
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
        <div class="text-[32px] font-extrabold leading-none mb-3 tracking-tight" :class="lowQuotaCount > 0 ? 'text-amber-500' : 'text-gray-900 dark:text-gray-100'">{{ lowQuotaCount }}</div>
        <div class="flex items-center text-[12px] text-gray-500 dark:text-gray-400 font-medium">
          <AlertTriangle class="w-4 h-4 mr-1.5 text-amber-500 opacity-80" stroke-width="2.5" /> 额度偏低
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
              <span class="text-[18px] font-bold leading-tight" :class="lowQuotaCount > 0 ? 'text-amber-500' : 'text-gray-900 dark:text-gray-100'">{{ lowQuotaCount }}</span>
              <span class="text-[11px] text-gray-500 mt-1">偏低</span>
            </div>
            <div class="flex flex-col">
              <span class="text-[18px] font-bold leading-tight" :class="depletedCount > 0 ? 'text-rose-500' : 'text-gray-900 dark:text-gray-100'">{{ depletedCount }}</span>
              <span class="text-[11px] text-gray-500 mt-1">见底</span>
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
