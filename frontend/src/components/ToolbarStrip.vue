<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useAccountStore } from '../stores/useAccountStore'
import { useSystemStore } from '../stores/useSystemStore'
import { APIInfo } from '../api/wails'
import { WindowShow } from '../../wailsjs/runtime/runtime'
import { Maximize2 } from 'lucide-vue-next'

const accountStore = useAccountStore()
const systemStore = useSystemStore()

let pollTimer: ReturnType<typeof setInterval> | null = null

const currentAccount = computed(() => {
  const e = (systemStore.currentAuthEmail || '').trim().toLowerCase()
  if (!e) {
    return null
  }
  return accountStore.accounts.find((a) => (a.email || '').trim().toLowerCase() === e) ?? null
})

const dailyPct = computed(() => currentAccount.value?.daily_remaining || '—')

const emailShort = computed(() => {
  const em = currentAccount.value?.email || systemStore.currentAuthEmail || '未识别会话'
  return em.length > 30 ? `${em.slice(0, 28)}…` : em
})

const ringPct = computed(() => {
  const s = String(dailyPct.value || '').replace('%', '').trim()
  const n = parseFloat(s)
  if (!Number.isFinite(n)) {
    return 0
  }
  return Math.max(0, Math.min(100, n))
})

const ringRadius = 12.5
const ringCirc = 2 * Math.PI * ringRadius

const dashOffset = computed(() => ringCirc * (1 - ringPct.value / 100))

async function refreshOne() {
  const acc = currentAccount.value
  if (!acc?.id) {
    return
  }
  try {
    await APIInfo.refreshAccountQuota(acc.id)
    await accountStore.fetchAccounts()
  } catch {
    /* 静默失败，避免小窗弹错 */
  }
}

async function openMain() {
  await APIInfo.restoreMainWindowLayout()
  WindowShow()
}

const pollTick = () => {
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
    return
  }
  void refreshOne()
}

const onVisibilityChange = () => {
  if (typeof document !== 'undefined' && document.visibilityState === 'visible') {
    void refreshOne()
  }
}

onMounted(() => {
  void refreshOne()
  pollTimer = setInterval(pollTick, 60_000)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>

<template>
  <div
    class="flex items-center gap-2.5 px-3 py-1 h-full min-h-0 bg-white/92 dark:bg-[#1c1c1e]/92 backdrop-blur-md border border-black/[0.08] dark:border-white/[0.08] rounded-2xl shadow-lg select-none"
  >
    <div class="relative w-10 h-10 shrink-0">
      <svg class="w-10 h-10 -rotate-90" viewBox="0 0 36 36" aria-hidden="true">
        <circle
          cx="18"
          cy="18"
          :r="ringRadius"
          fill="none"
          stroke="currentColor"
          class="text-black/[0.08] dark:text-white/10"
          stroke-width="3"
        />
        <circle
          cx="18"
          cy="18"
          :r="ringRadius"
          fill="none"
          stroke="currentColor"
          class="text-ios-blue transition-[stroke-dashoffset] duration-300"
          stroke-width="3"
          stroke-linecap="round"
          :stroke-dasharray="`${ringCirc}`"
          :stroke-dashoffset="dashOffset"
        />
      </svg>
      <span
        class="absolute inset-0 flex items-center justify-center text-[9px] font-bold tabular-nums text-ios-text dark:text-ios-textDark"
      >
        {{ Math.round(ringPct) }}%
      </span>
    </div>
    <div class="flex-1 min-w-0 overflow-hidden pr-1">
      <div class="text-[10px] uppercase tracking-wide text-ios-textSecondary dark:text-ios-textSecondaryDark">
        日剩余 · 当前会话
      </div>
      <div class="text-[13px] font-semibold font-mono leading-tight">{{ dailyPct }}</div>
      <div
        class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark truncate mt-0.5"
        :title="currentAccount?.email || systemStore.currentAuthEmail || ''"
      >
        {{ emailShort }}
      </div>
    </div>
    <button
      type="button"
      class="no-drag-region p-2 rounded-xl bg-ios-blue/12 text-ios-blue hover:bg-ios-blue/20 transition-colors"
      title="打开主窗口"
      @click="openMain"
    >
      <Maximize2 class="w-4 h-4" stroke-width="2.2" />
    </button>
  </div>
</template>
