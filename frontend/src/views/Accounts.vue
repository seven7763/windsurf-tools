<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useAccountStore } from '../stores/useAccountStore'
import { useSettingsStore } from '../stores/useSettingsStore'
import { useSystemStore } from '../stores/useSystemStore'
import ImportModal from '../components/accounts/ImportModal.vue'
import {
  Plus,
  Trash2,
  Power,
  RefreshCcw,
  Users,
  ChevronRight,
  KeyRound,
  BarChart3,
  UserX,
  Search,
  X,
  CalendarDays,
  Clock,
} from 'lucide-vue-next'
import { APIInfo } from '../api/wails'
import { getPlanTone } from '../utils/account'
import {
  SWITCH_PLAN_FILTER_TONES,
  type SwitchPlanTone,
  formatSwitchPlanFilterSummary,
  normalizeSwitchPlanFilter,
} from '../utils/settingsModel'
import { models } from '../../wailsjs/go/models'
import SwitchPlanFilterControl from '../components/settings/SwitchPlanFilterControl.vue'
import PageLoadingSkeleton from '../components/common/PageLoadingSkeleton.vue'
import { confirmDialog, showToast } from '../utils/toast'
import {
  formatDateTimeAsiaShanghai,
  formatResetCountdownZH,
  formatSyncTimeLine,
} from '../utils/datetimeAsia'

const accountStore = useAccountStore()
const settingsStore = useSettingsStore()
const systemStore = useSystemStore()
const showImportModal = ref(false)
const quotaRefreshingId = ref<string | null>(null)

const switchPlanFilter = ref('all')

const mitmOnly = computed(() => settingsStore.settings?.mitm_only === true)

const poolPlanCounts = computed<Partial<Record<SwitchPlanTone, number>>>(() => {
  const m: Partial<Record<SwitchPlanTone, number>> = {}
  for (const t of SWITCH_PLAN_FILTER_TONES) {
    m[t] = 0
  }
  for (const a of accountStore.accounts) {
    const tone = getPlanTone(a.plan_name) as SwitchPlanTone
    m[tone] = (m[tone] ?? 0) + 1
  }
  return m
})

watch(
  () => settingsStore.settings,
  (s) => {
    switchPlanFilter.value = normalizeSwitchPlanFilter(s?.auto_switch_plan_filter)
  },
  { immediate: true },
)

const planSectionOrder = ['pro', 'max', 'team', 'enterprise', 'trial', 'free', 'unknown'] as const

const planSectionLabels: Record<string, string> = {
  pro: 'Pro',
  max: 'Max / Ultimate',
  team: 'Teams',
  enterprise: 'Enterprise',
  trial: 'Trial',
  free: 'Free',
  unknown: '未识别',
}

const searchQuery = ref('')
const activeTab = ref<string>('all')

const filteredAccounts = computed(() => {
  let list = accountStore.accounts
  
  if (activeTab.value !== 'all') {
    list = list.filter(a => getPlanTone(a.plan_name) === activeTab.value)
  }

  const q = searchQuery.value.trim().toLowerCase()
  if (!q) {
    return list
  }
  return list.filter(
    (a) =>
      (a.email?.toLowerCase().includes(q) ?? false) ||
      (a.nickname?.toLowerCase().includes(q) ?? false) ||
      (a.remark?.toLowerCase().includes(q) ?? false) ||
      (a.plan_name?.toLowerCase().includes(q) ?? false),
  )
})

const tabsList = computed(() => {
  const groups = new Map<string, number>()
  for (const k of planSectionOrder) {
    groups.set(k, 0)
  }
  for (const acc of accountStore.accounts) {
    const tone = getPlanTone(acc.plan_name)
    groups.set(tone, (groups.get(tone) || 0) + 1)
  }
  
  const tabs = planSectionOrder
    .filter(k => (groups.get(k) || 0) > 0)
    .map(key => ({
      key,
      label: planSectionLabels[key] || key,
      count: groups.get(key) || 0
    }))
    
  return [{ key: 'all', label: '全部', count: accountStore.accounts.length }, ...tabs]
})

const freePlanAccountCount = computed(
  () => accountStore.accounts.filter((a) => getPlanTone(a.plan_name) === 'free').length,
)

const accountSort = ref<'group' | 'name' | 'quota'>('group')

const displayAccounts = computed(() => {
  const items = [...filteredAccounts.value]
  
  if (accountSort.value === 'group') {
    items.sort((a, b) => {
      const ia = planSectionOrder.indexOf(getPlanTone(a.plan_name) as any)
      const ib = planSectionOrder.indexOf(getPlanTone(b.plan_name) as any)
      if (ia !== ib) return ia - ib
      return (a.email || '').localeCompare(b.email || '', 'zh-CN')
    })
  } else if (accountSort.value === 'name') {
    items.sort((a, b) => (a.email || '').localeCompare(b.email || '', 'zh-CN'))
  } else if (accountSort.value === 'quota') {
    items.sort((a, b) => {
      const pa = parseFloat(String(a.daily_remaining || '').replace('%', '')) || 0
      const pb = parseFloat(String(b.daily_remaining || '').replace('%', '')) || 0
      return pa - pb
    })
  }
  return items
})

const isCurrentOnline = (acc: models.Account) => {
  const cur = (systemStore.currentAuthEmail || '').trim().toLowerCase()
  if (!cur) {
    return false
  }
  return (acc.email || '').trim().toLowerCase() === cur
}

const switchPoolLabel = computed(() => formatSwitchPlanFilterSummary(switchPlanFilter.value))

onMounted(() => {
  void accountStore.fetchAccounts()
})

const persistSwitchPool = async () => {
  try {
    await settingsStore.saveAutoSwitchPlanFilter(switchPlanFilter.value)
  } catch (e: unknown) {
    showToast(`保存切号范围失败: ${String(e)}`, 'error')
  }
}

const onSwitchPlanFilterUpdate = (v: string) => {
  switchPlanFilter.value = v
  void persistSwitchPool()
}

const switchFollowUpHint = () => {
  const on = settingsStore.settings?.restart_windsurf_after_switch !== false
  return on
    ? '已尝试重启 Windsurf 以加载新账号。若仍显示旧账号，请检查设置里的安装路径是否正确。'
    : '已写入 windsurf_auth.json。因未开启「自动重启」，请手动重开 IDE。'
}

const handleSwitch = async (id: string) => {
  try {
    await APIInfo.switchAccount(id)
    await systemStore.fetchCurrentAuth()
    showToast(`切换成功。\n${switchFollowUpHint()}`, 'success', 6500)
  } catch (e: unknown) {
    showToast(`切换失败: ${String(e)}`, 'error')
  }
}

const handleAutoNext = async (id: string) => {
  try {
    const email = await accountStore.autoSwitchToNext(id, switchPlanFilter.value)
    await systemStore.fetchCurrentAuth()
    showToast(`已切换到：${email}\n范围：${switchPoolLabel.value}\n\n${switchFollowUpHint()}`, 'success', 7000)
  } catch (e: unknown) {
    showToast(`自动切号失败: ${String(e)}`, 'error')
  }
}

const handleDelete = async (id: string) => {
  const ok = await confirmDialog('是否确认移除该账号？', {
    confirmText: '移除',
    cancelText: '取消',
    destructive: true,
  })
  if (ok) {
    await accountStore.deleteAccount(id)
  }
}

const handleCleanExpired = async () => {
  try {
    const n = await accountStore.cleanExpiredAccounts()
    showToast(`已清理 ${n} 个过期账号`, 'success')
  } catch (e: unknown) {
    showToast(`清理失败: ${String(e)}`, 'error')
  }
}

const handleDeleteFreePlans = async () => {
  const n = freePlanAccountCount.value
  if (n === 0) {
    showToast('当前没有 Free 计划的账号', 'info')
    return
  }
  const ok = await confirmDialog(
    `将永久删除 ${n} 个免费计划账号，不可恢复。`,
    { confirmText: '删除', cancelText: '取消', destructive: true },
  )
  if (!ok) return
  
  try {
    const deleted = await accountStore.deleteFreePlanAccounts()
    showToast(`已删除 ${deleted} 个免费账号`, 'success')
  } catch (e: unknown) {
    showToast(`删除失败: ${String(e)}`, 'error')
  }
}

const handleRefreshTokens = async () => {
  try {
    const map = await accountStore.refreshAllTokens()
    const entries = Object.entries(map || {})
    const ok = entries.filter(([, v]) => String(v).includes('成功')).length
    showToast(`刷新完成：${ok} / ${entries.length}`, 'success')
  } catch (e: unknown) {
    showToast(`刷新失败: ${String(e)}`, 'error')
  }
}

const handleRefreshAllQuotas = async () => {
  try {
    const map = await accountStore.refreshAllQuotas()
    const entries = Object.entries(map || {})
    const synced = entries.filter(([, v]) => String(v).includes('已同步')).length
    showToast(`额度同步完成：已更新 ${synced} 共 ${entries.length} 条`, 'success')
  } catch (e: unknown) {
    showToast(`同步额度失败: ${String(e)}`, 'error')
  }
}

const handleRefreshOneQuota = async (id: string, email: string) => {
  quotaRefreshingId.value = id
  try {
    await accountStore.refreshAccountQuota(id)
    showToast(`${email} 额度已更新`, 'success')
  } catch (e: unknown) {
    showToast(`刷新额度失败: ${String(e)}`, 'error')
  } finally {
    quotaRefreshingId.value = null
  }
}

const parseQuotaWidth = (str: string) => {
  if (!str) return '0%'
  const n = parseFloat(String(str).replace('%', '').trim())
  if (!Number.isFinite(n)) return '0%'
  return `${Math.max(0, Math.min(100, n))}%`
}

const looksLikeDateish = (value: string) =>
  /^\d{4}[./-]\d{1,2}[./-]\d{1,2}(?:\s+\d{1,2}:\d{2})?$/.test(value.trim())

const getEmailUsername = (email?: string) => {
  const clean = String(email || '').trim()
  if (!clean) {
    return ''
  }
  const [local] = clean.split('@')
  return local || clean
}

const getAccountUsername = (acc: models.Account) => {
  const emailUsername = getEmailUsername(acc.email)
  if (emailUsername) {
    return emailUsername
  }
  const nickname = String(acc.nickname || '').trim()
  return nickname || '未命名账号'
}

const getAccountNickname = (acc: models.Account) => {
  const nickname = String(acc.nickname || '').trim()
  if (!nickname || looksLikeDateish(nickname)) {
    return ''
  }
  if (nickname.toLowerCase() === getAccountUsername(acc).toLowerCase()) {
    return ''
  }
  return nickname
}

const getAccountDisplayName = (acc: models.Account) => getAccountNickname(acc) || getAccountUsername(acc)

const shouldShowUsernameMeta = (acc: models.Account) => {
  const nickname = getAccountNickname(acc)
  const username = getAccountUsername(acc)
  if (!nickname || !username) {
    return false
  }
  return nickname.toLowerCase() !== username.toLowerCase()
}

const getAccountRemark = (acc: models.Account) => String(acc.remark || '').trim()

const getQuotaColor = (str: string) => {
  const n = parseFloat(String(str).replace('%', '').trim())
  if (!Number.isFinite(n)) return 'bg-gray-400'
  if (n > 50) return 'bg-ios-green'
  if (n > 20) return 'bg-yellow-500'
  return 'bg-ios-red'
}

const isExpiredAccount = (acc: models.Account) => {
  const status = String(acc.status || '').toLowerCase()
  if (status === 'disabled' || status === 'expired') {
    return true
  }
  if (!acc.subscription_expires_at) {
    return false
  }
  const ts = Date.parse(acc.subscription_expires_at)
  return Number.isFinite(ts) && ts < Date.now()
}

const isInSwitchPool = (acc: models.Account) => {
  if (mitmOnly.value) {
    return true
  }
  const filter = normalizeSwitchPlanFilter(switchPlanFilter.value)
  if (filter === 'all') {
    return true
  }
  const tone = getPlanTone(acc.plan_name)
  return filter.split(',').includes(tone)
}

type CardStateTone = 'online' | 'ready' | 'warning' | 'danger' | 'pending' | 'muted'

const getCardStateMeta = (acc: models.Account): { tone: CardStateTone; label: string } => {
  if (isCurrentOnline(acc)) {
    return {
      tone: 'online',
      label: '当前在线',
    }
  }

  if (mitmOnly.value) {
    if (acc.windsurf_api_key) {
      return {
        tone: 'ready',
        label: 'MITM 可用',
      }
    }
    return {
      tone: 'pending',
      label: '待补 API Key',
    }
  }

  if (isExpiredAccount(acc)) {
    return {
      tone: 'danger',
      label: '已过期',
    }
  }

  const daily = parseFloat(String(acc.daily_remaining || '').replace('%', '').trim())
  const weekly = parseFloat(String(acc.weekly_remaining || '').replace('%', '').trim())
  const dailyKnown = Number.isFinite(daily)
  const weeklyKnown = Number.isFinite(weekly)
  const exhausted =
    (dailyKnown && daily <= 0) &&
    (!weeklyKnown || weekly <= 0)
  const lowQuota =
    (dailyKnown && daily > 0 && daily < 20) ||
    (weeklyKnown && weekly > 0 && weekly < 20)

  if (!acc.subscription_expires_at && !dailyKnown && !weeklyKnown && !acc.last_quota_update) {
    return {
      tone: 'pending',
      label: '待同步',
    }
  }

  if (exhausted) {
    return {
      tone: 'danger',
      label: '额度见底',
    }
  }

  if (lowQuota) {
    return {
      tone: 'warning',
      label: '额度偏低',
    }
  }

  if (!isInSwitchPool(acc)) {
    return {
      tone: 'muted',
      label: '切号池外',
    }
  }

  return {
    tone: 'ready',
    label: '可参与下一席',
  }
}

const getCardStatePanelClass = (tone: CardStateTone) => {
  switch (tone) {
    case 'online':
      return 'border-emerald-500/15 bg-emerald-500/[0.07]'
    case 'ready':
      return 'border-ios-blue/15 bg-ios-blue/[0.06]'
    case 'warning':
      return 'border-amber-500/15 bg-amber-500/[0.07]'
    case 'danger':
      return 'border-rose-500/15 bg-rose-500/[0.07]'
    case 'pending':
      return 'border-black/[0.08] bg-black/[0.03] dark:border-white/[0.08] dark:bg-white/[0.04]'
    case 'muted':
    default:
      return 'border-slate-500/12 bg-slate-500/[0.06]'
  }
}

const getPlanAccentClass = (acc: models.Account) => {
  switch (getPlanTone(acc.plan_name)) {
    case 'pro':
      return 'from-ios-blue via-sky-400 to-cyan-300'
    case 'max':
      return 'from-violet-500 via-fuchsia-400 to-rose-300'
    case 'team':
      return 'from-indigo-500 via-blue-400 to-cyan-300'
    case 'enterprise':
      return 'from-slate-600 via-slate-500 to-slate-300'
    case 'trial':
      return 'from-amber-500 via-orange-400 to-yellow-300'
    case 'free':
      return 'from-slate-400 via-slate-300 to-slate-200'
    default:
      return 'from-gray-400 via-gray-300 to-gray-200'
  }
}

</script>

<template>
  <div class="p-6 md:p-8 flex flex-col max-w-6xl mx-auto w-full min-h-0">
    <div class="flex flex-wrap items-center justify-between gap-4 mb-4 shrink-0">
      <div>
        <h1 class="text-[32px] font-bold tracking-tight">{{ mitmOnly ? '号池 (MITM)' : '账号池' }}</h1>
        <p class="text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-1">
          <template v-if="mitmOnly">
            维护 API Key / 凭证供 MITM 代理轮换。
          </template>
          <template v-else>
            一眼查看账号、到期时间和额度状态。
          </template>
        </p>
      </div>
      <div class="flex flex-wrap gap-2 justify-end items-center">
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-black/5 dark:bg-white/10 text-ios-text dark:text-ios-textDark rounded-full font-semibold text-[14px] ios-btn hover:bg-black/10 dark:hover:bg-white/15 transition-colors disabled:opacity-50"
          :disabled="accountStore.actionLoading"
          @click="handleRefreshTokens"
        >
          <KeyRound class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          刷新凭证
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 rounded-full font-semibold text-[14px] ios-btn hover:bg-emerald-500/15 transition-colors disabled:opacity-50"
          :disabled="accountStore.actionLoading"
          @click="handleRefreshAllQuotas"
        >
          <BarChart3 class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          同步额度
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-ios-red/10 text-ios-red dark:text-ios-redDark rounded-full font-semibold text-[14px] ios-btn hover:bg-ios-red/20 transition-colors"
          @click="handleCleanExpired"
        >
          <Trash2 class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          清理过期
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-amber-500/12 text-amber-900 dark:text-amber-300 rounded-full font-semibold text-[14px] ios-btn hover:bg-amber-500/18 transition-colors disabled:opacity-50"
          @click="handleDeleteFreePlans"
        >
          <UserX class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          删除免费
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-5 py-2.5 bg-gradient-to-b from-[#3b82f6] to-ios-blue text-white rounded-full font-semibold text-[14px] ios-btn shadow-md ring-1 ring-black/5"
          @click="showImportModal = true"
        >
          <Plus class="w-[18px] h-[18px] mr-1" stroke-width="2.5" />
          批量导入
        </button>
      </div>
    </div>

    <!-- 顶部计划类型导航条 -->
    <div class="flex items-center gap-2 mb-6 overflow-x-auto no-scrollbar shrink-0 pb-1">
      <button
        v-for="tab in tabsList"
        :key="tab.key"
        type="button"
        class="no-drag-region flex items-center gap-2 px-4 py-2 rounded-full font-bold text-[14px] transition-all whitespace-nowrap"
        :class="activeTab === tab.key ? 'bg-ios-text text-white dark:bg-white dark:text-black shadow-md' : 'bg-black/5 dark:bg-white/5 text-ios-textSecondary hover:bg-black/10 dark:hover:bg-white/10'"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
        <span
          class="px-2 py-0.5 rounded-full text-[11px] font-bold"
          :class="activeTab === tab.key ? 'bg-white/20 dark:bg-black/10' : 'bg-black/5 dark:bg-white/10'"
        >
          {{ tab.count }}
        </span>
      </button>
    </div>

    <SwitchPlanFilterControl
      v-if="!mitmOnly"
      class="mb-6 w-full shrink-0"
      variant="compact"
      :pool-counts="poolPlanCounts"
      :model-value="switchPlanFilter"
      @update:model-value="onSwitchPlanFilterUpdate"
    />

    <div
      v-if="accountStore.accounts.length > 0"
      class="flex flex-col sm:flex-row gap-3 mb-6 shrink-0 max-w-6xl"
    >
      <div class="relative flex-1 min-w-0">
        <Search class="absolute left-3.5 top-1/2 -translate-y-1/2 w-[18px] h-[18px] text-ios-textSecondary opacity-60 pointer-events-none" />
        <input
          v-model="searchQuery"
          type="search"
          placeholder="搜索邮箱、昵称、备注、计划…"
          class="no-drag-region w-full pl-11 pr-10 py-2.5 rounded-[14px] bg-black/[0.04] border border-black/[0.06] text-[14px] outline-none focus:ring-2 focus:ring-ios-blue/25"
        />
        <button
          v-if="searchQuery"
          type="button"
          class="no-drag-region absolute right-2 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full hover:bg-black/10 text-ios-textSecondary"
          @click="searchQuery = ''"
        >
          <X class="w-4 h-4 mx-auto" stroke-width="2.5" />
        </button>
      </div>
      <select
        v-model="accountSort"
        class="no-drag-region shrink-0 px-4 py-2.5 rounded-[14px] bg-black/[0.04] border border-black/[0.06] text-[13px] font-medium outline-none focus:ring-2 focus:ring-ios-blue/25"
      >
        <option value="group">按分组（默认）</option>
        <option value="name">按邮箱 A→Z</option>
        <option value="quota">按日剩余额度 ↑</option>
      </select>
    </div>

    <PageLoadingSkeleton v-if="accountStore.isLoading" variant="accounts" class="flex-1" />

    <div
      v-else-if="accountStore.accounts.length === 0"
      class="flex flex-col items-center justify-center flex-1 text-ios-textSecondary"
    >
      <div class="w-24 h-24 mb-6 rounded-3xl bg-black/5 flex items-center justify-center">
        <Users class="w-12 h-12 opacity-50" />
      </div>
      <p class="text-[17px] font-medium">账号簿为空</p>
    </div>

    <div
      v-else-if="accountStore.accounts.length > 0 && searchQuery.trim() && displayAccounts.length === 0"
      class="flex flex-col items-center justify-center flex-1 py-16 text-ios-textSecondary"
    >
      <Search class="w-12 h-12 opacity-50 mb-4" />
      <p class="text-[17px] font-medium">未找到匹配的账号</p>
      <button class="mt-3 text-[14px] font-semibold text-ios-blue ios-btn" @click="searchQuery = ''">清除搜索</button>
    </div>

    <div v-else class="pb-10 min-h-0">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5 auto-rows-max">
        <div
          v-for="acc in displayAccounts"
          :key="acc.id"
          :class="[
            'bg-white dark:bg-[#1C1C1E] rounded-[22px] flex flex-col relative overflow-hidden transition-all duration-300 ease-out hover:shadow-lg hover:-translate-y-0.5',
            isCurrentOnline(acc)
              ? 'border-2 border-ios-green/40 dark:border-ios-greenDark/40 shadow-[0_0_0_1px_rgba(52,199,89,0.12)]'
              : 'border border-black/[0.05] dark:border-white/[0.08] shadow-sm',
          ]"
        >
          <div class="absolute inset-x-0 top-0 h-1.5 bg-gradient-to-r opacity-95" :class="getPlanAccentClass(acc)" />
          <div class="relative z-10 flex h-full flex-col p-5">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0 flex flex-wrap items-center gap-2">
                <span class="rounded-full bg-[#F0F5FF] px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.18em] text-ios-blue dark:bg-ios-blue/20">
                  {{ acc.plan_name || 'unknown' }}
                </span>
                <span
                  class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] font-bold tracking-[0.14em] text-gray-800 dark:text-gray-100"
                  :class="getCardStatePanelClass(getCardStateMeta(acc).tone)"
                >
                  {{ getCardStateMeta(acc).label }}
                </span>
              </div>

              <div class="flex shrink-0 gap-1 rounded-full border border-black/5 bg-gray-50/95 p-1 shadow-sm dark:border-white/5 dark:bg-black/20">
                <button
                  type="button"
                  class="flex h-[30px] w-[30px] min-w-[30px] items-center justify-center rounded-full bg-white text-ios-blue shadow-sm transition hover:scale-105 dark:bg-black/40 ios-btn"
                  title="写入 windsurf_auth 并切换到这个账号"
                  @click="handleSwitch(acc.id)"
                >
                  <Power class="h-[15px] w-[15px]" stroke-width="2.5" />
                </button>
                <button
                  type="button"
                  class="flex h-[30px] w-[30px] min-w-[30px] items-center justify-center rounded-full bg-white text-emerald-600 shadow-sm transition hover:scale-105 dark:bg-black/40 ios-btn"
                  :disabled="quotaRefreshingId === acc.id"
                  title="只刷新这张卡的额度与订阅信息"
                  @click="handleRefreshOneQuota(acc.id, acc.email)"
                >
                  <RefreshCcw class="h-[15px] w-[15px]" :class="{ 'ios-spinner': quotaRefreshingId === acc.id }" stroke-width="2.5" />
                </button>
                <button
                  v-if="!mitmOnly"
                  type="button"
                  class="flex h-[30px] w-[30px] min-w-[30px] items-center justify-center rounded-full bg-white text-gray-600 shadow-sm transition hover:scale-105 dark:bg-black/40 dark:text-gray-300 ios-btn"
                  title="切到下一席可用账号"
                  @click="handleAutoNext(acc.id)"
                >
                  <ChevronRight class="h-[15px] w-[15px]" stroke-width="2.5" />
                </button>
                <button
                  type="button"
                  class="flex h-[30px] w-[30px] min-w-[30px] items-center justify-center rounded-full bg-white text-ios-red shadow-sm transition hover:scale-105 dark:bg-black/40 ios-btn"
                  title="移除账号"
                  @click="handleDelete(acc.id)"
                >
                  <Trash2 class="h-4 w-4" />
                </button>
              </div>
            </div>

            <div class="mt-4 min-w-0">
              <div
                class="truncate text-[24px] font-bold tracking-tight text-ios-text dark:text-ios-textDark"
                :title="getAccountDisplayName(acc)"
              >
                {{ getAccountDisplayName(acc) }}
              </div>
              <div class="mt-2 truncate text-[13px] font-medium text-gray-600 dark:text-gray-300" :title="acc.email || '未填写邮箱'">
                {{ acc.email || '未填写邮箱' }}
              </div>
              <div v-if="shouldShowUsernameMeta(acc) || getAccountRemark(acc)" class="mt-3 flex flex-wrap gap-2">
                <span
                  v-if="shouldShowUsernameMeta(acc)"
                  class="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-semibold text-ios-textSecondary dark:bg-white/[0.08] dark:text-ios-textSecondaryDark"
                  :title="`@${getAccountUsername(acc)}`"
                >
                  @{{ getAccountUsername(acc) }}
                </span>
                <span
                  v-if="getAccountRemark(acc)"
                  class="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-semibold text-ios-textSecondary dark:bg-white/[0.08] dark:text-ios-textSecondaryDark"
                  :title="getAccountRemark(acc)"
                >
                  {{ getAccountRemark(acc) }}
                </span>
              </div>
            </div>

            <div class="mt-4 grid grid-cols-2 gap-3">
              <div class="rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-3 dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div class="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  <CalendarDays class="h-3.5 w-3.5 opacity-70" />
                  到期时间
                </div>
                <div class="mt-2 text-[13px] font-semibold leading-snug text-ios-text dark:text-ios-textDark">
                  {{ acc.subscription_expires_at ? formatDateTimeAsiaShanghai(acc.subscription_expires_at) : '待同步' }}
                </div>
              </div>

              <div class="rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-3 dark:border-white/[0.06] dark:bg-white/[0.04]">
                <div class="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
                  <Clock class="h-3.5 w-3.5 opacity-70" />
                  额度同步
                </div>
                <div class="mt-2 text-[13px] font-semibold leading-snug text-ios-text dark:text-ios-textDark">
                  {{ acc.last_quota_update ? formatSyncTimeLine(acc.last_quota_update) : '未同步' }}
                </div>
                <div
                  v-if="acc.last_quota_update"
                  class="mt-1 truncate text-[10px] text-gray-500 dark:text-gray-400"
                  :title="formatDateTimeAsiaShanghai(acc.last_quota_update)"
                >
                  {{ formatDateTimeAsiaShanghai(acc.last_quota_update) }}
                </div>
              </div>
            </div>

            <div class="mt-4 rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-4 dark:border-white/[0.06] dark:bg-white/[0.04]">
              <div class="space-y-1.5">
                <div class="flex items-center justify-between text-[11px] font-bold text-gray-800 dark:text-gray-200">
                  <span>日额度</span>
                  <span>{{ acc.daily_remaining || '—' }}</span>
                </div>
                <div class="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
                  <div
                    class="h-full rounded-full transition-all duration-500 ease-out"
                    :class="getQuotaColor(acc.daily_remaining || '')"
                    :style="{ width: parseQuotaWidth(acc.daily_remaining || '') }"
                  />
                </div>
                <div
                  v-if="acc.daily_reset_at"
                  class="truncate pt-1 text-[10px] font-medium text-gray-500 dark:text-gray-400"
                  :title="formatDateTimeAsiaShanghai(acc.daily_reset_at)"
                >
                  {{ formatResetCountdownZH(acc.daily_reset_at) }}
                </div>
              </div>

              <div class="mt-4 space-y-1.5">
                <div class="flex items-center justify-between text-[11px] font-bold text-gray-800 dark:text-gray-200">
                  <span>周额度</span>
                  <span>{{ acc.weekly_remaining || '—' }}</span>
                </div>
                <div class="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-white/10">
                  <div
                    class="h-full rounded-full transition-all duration-500 ease-out"
                    :class="getQuotaColor(acc.weekly_remaining || '')"
                    :style="{ width: parseQuotaWidth(acc.weekly_remaining || '') }"
                  />
                </div>
                <div
                  v-if="acc.weekly_reset_at"
                  class="truncate pt-1 text-[10px] font-medium text-gray-500 dark:text-gray-400"
                  :title="formatDateTimeAsiaShanghai(acc.weekly_reset_at)"
                >
                  {{ formatResetCountdownZH(acc.weekly_reset_at) }}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <ImportModal :isOpen="showImportModal" @close="showImportModal = false" />
  </div>
</template>
