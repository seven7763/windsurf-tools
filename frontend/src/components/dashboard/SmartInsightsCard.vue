<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  Sparkles,
  X,
  ChevronRight,
  AlertTriangle,
  Info,
  Lightbulb,
  CheckCircle,
} from 'lucide-vue-next'
import type { DashboardInsight } from '../../utils/dashboardInsights'
import { useMainViewStore } from '../../stores/useMainViewStore'

const STORAGE_KEY = 'windsurf-tools.dashboard.insights.dismissed.v1'

const props = defineProps<{
  insights: DashboardInsight[]
}>()

const mainView = useMainViewStore()

function loadDismissed(): Set<string> {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return new Set<string>()
    const arr = JSON.parse(raw) as unknown
    return new Set(Array.isArray(arr) ? arr.filter((x) => typeof x === 'string') : [])
  } catch {
    return new Set<string>()
  }
}

const dismissed = ref<Set<string>>(loadDismissed())

type ToneMeta = {
  label: string
  icon: typeof AlertTriangle
  bar: string
  iconWrap: string
  iconColor: string
  cardBg: string
  cardRing: string
}

function toneMeta(tone: DashboardInsight['tone']): ToneMeta {
  switch (tone) {
    case 'warning':
      return {
        label: '需注意',
        icon: AlertTriangle,
        bar: 'bg-amber-500 dark:bg-amber-400',
        iconWrap: 'bg-amber-500/12 dark:bg-amber-400/15',
        iconColor: 'text-amber-600 dark:text-amber-400',
        cardBg: 'bg-amber-500/[0.04] dark:bg-amber-400/[0.07]',
        cardRing: 'ring-amber-500/12 dark:ring-amber-400/20',
      }
    case 'info':
      return {
        label: '提示',
        icon: Info,
        bar: 'bg-ios-blue',
        iconWrap: 'bg-ios-blue/12 dark:bg-ios-blue/18',
        iconColor: 'text-ios-blue dark:text-blue-400',
        cardBg: 'bg-ios-blue/[0.04] dark:bg-ios-blue/[0.08]',
        cardRing: 'ring-ios-blue/15 dark:ring-blue-400/20',
      }
    case 'success':
      return {
        label: '就绪',
        icon: CheckCircle,
        bar: 'bg-emerald-500',
        iconWrap: 'bg-emerald-500/12 dark:bg-emerald-400/15',
        iconColor: 'text-emerald-600 dark:text-emerald-400',
        cardBg: 'bg-emerald-500/[0.04] dark:bg-emerald-400/[0.06]',
        cardRing: 'ring-emerald-500/12 dark:ring-emerald-400/18',
      }
    case 'tip':
    default:
      return {
        label: '建议',
        icon: Lightbulb,
        bar: 'bg-violet-500 dark:bg-violet-400',
        iconWrap: 'bg-violet-500/10 dark:bg-violet-400/15',
        iconColor: 'text-violet-600 dark:text-violet-400',
        cardBg: 'bg-violet-500/[0.04] dark:bg-violet-400/[0.07]',
        cardRing: 'ring-violet-500/10 dark:ring-violet-400/18',
      }
  }
}

const visible = computed(() => props.insights.filter((i) => !dismissed.value.has(i.id)))

const visibleEnriched = computed(() =>
  visible.value.map((item) => ({
    ...item,
    meta: toneMeta(item.tone),
  })),
)

const dismiss = (id: string) => {
  dismissed.value = new Set([...dismissed.value, id])
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify([...dismissed.value]))
  } catch {
    /* ignore */
  }
}

const dismissAll = () => {
  for (const i of visible.value) {
    dismissed.value = new Set([...dismissed.value, i.id])
  }
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify([...dismissed.value]))
  } catch {
    /* ignore */
  }
}

const goSettings = () => {
  mainView.activeTab = 'Settings'
}

const showSettingsCta = (id: string) =>
  id === 'patch_missing' || id === 'switch_without_quota_sync' || id === 'multi_account_tokens'
</script>

<template>
  <div
    v-if="visibleEnriched.length"
    class="mb-6 rounded-[22px] overflow-hidden ring-1 ring-black/[0.06] dark:ring-white/[0.08] shadow-[0_12px_40px_-12px_rgba(0,0,0,0.12)] dark:shadow-[0_12px_40px_-12px_rgba(0,0,0,0.45)]"
  >
    <!-- 顶部：渐变条 + 玻璃 -->
    <div
      class="relative px-4 py-3.5 flex items-center gap-3 bg-gradient-to-br from-white/90 via-white/80 to-violet-50/40 dark:from-[#2C2C2E]/95 dark:via-[#1C1C1E]/90 dark:to-violet-950/30 backdrop-blur-xl border-b border-black/[0.05] dark:border-white/[0.06]"
    >
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-violet-500/20 to-fuchsia-500/10 dark:from-violet-400/25 dark:to-fuchsia-500/15 ring-1 ring-violet-500/20 dark:ring-violet-400/25 shadow-inner"
      >
        <Sparkles class="w-[18px] h-[18px] text-violet-600 dark:text-violet-300" stroke-width="2.25" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2 flex-wrap">
          <h3 class="text-[15px] font-bold tracking-tight text-ios-text dark:text-ios-textDark">
            智能提示
          </h3>
          <span
            class="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider bg-black/[0.06] dark:bg-white/[0.08] text-ios-textSecondary dark:text-ios-textSecondaryDark tabular-nums"
          >
            {{ visibleEnriched.length }} 条
          </span>
        </div>
        <p class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-0.5 leading-snug">
          结合设置、号池与 MITM 状态 · 可单条关闭
        </p>
      </div>
      <button
        v-if="visibleEnriched.length > 1"
        type="button"
        class="no-drag-region shrink-0 text-[11px] font-semibold text-ios-textSecondary hover:text-ios-text dark:hover:text-ios-textDark px-2.5 py-1.5 rounded-full ios-btn hover:bg-black/[0.05] dark:hover:bg-white/[0.06] transition-colors"
        title="本次会话隐藏全部提示"
        @click="dismissAll"
      >
        全部忽略
      </button>
    </div>

    <!-- 列表：卡片组 -->
    <div
      class="px-3 pb-3 pt-2 space-y-2 bg-white/55 dark:bg-[#1C1C1E]/55 backdrop-blur-[20px]"
    >
      <article
        v-for="(item, idx) in visibleEnriched"
        :key="item.id"
        class="group relative flex gap-0 rounded-[18px] overflow-hidden ring-1 transition-shadow duration-300 hover:shadow-md hover:shadow-black/[0.04] dark:hover:shadow-black/30"
        :class="[item.meta.cardBg, item.meta.cardRing]"
        :style="{ animationDelay: `${idx * 45}ms` }"
      >
        <!-- 左侧色条 -->
        <div class="w-1 shrink-0 self-stretch" :class="item.meta.bar" aria-hidden="true" />

        <div class="flex gap-3 min-w-0 flex-1 py-3 pl-2 pr-2 sm:pr-3">
          <div
            class="shrink-0 h-9 w-9 flex items-center justify-center rounded-xl"
            :class="item.meta.iconWrap"
          >
            <component
              :is="item.meta.icon"
              class="w-[18px] h-[18px]"
              :class="item.meta.iconColor"
              stroke-width="2.25"
            />
          </div>

          <div class="min-w-0 flex-1 pt-0.5">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <span
                  class="inline-block mb-1 text-[10px] font-bold uppercase tracking-wider text-ios-textSecondary/90 dark:text-ios-textSecondaryDark/90"
                >
                  {{ item.meta.label }}
                </span>
                <p class="text-[13.5px] font-semibold text-ios-text dark:text-ios-textDark leading-snug">
                  {{ item.title }}
                </p>
              </div>
              <button
                type="button"
                class="no-drag-region shrink-0 -mr-1 -mt-1 p-2 rounded-xl text-ios-textSecondary/70 hover:text-ios-text dark:hover:text-ios-textDark hover:bg-black/[0.06] dark:hover:bg-white/[0.08] opacity-70 hover:opacity-100 transition-all ios-btn"
                title="本次会话不再显示本条"
                @click="dismiss(item.id)"
              >
                <X class="w-4 h-4" stroke-width="2.25" />
              </button>
            </div>
            <p
              v-if="item.body"
              class="text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-1.5 leading-relaxed pr-1"
            >
              {{ item.body }}
            </p>
            <button
              v-if="showSettingsCta(item.id)"
              type="button"
              class="no-drag-region mt-2.5 inline-flex items-center gap-1 px-3 py-1.5 rounded-full text-[12px] font-semibold bg-ios-blue/12 text-ios-blue dark:text-blue-400 dark:bg-blue-500/15 ring-1 ring-ios-blue/20 dark:ring-blue-400/25 ios-btn hover:bg-ios-blue/18 dark:hover:bg-blue-500/25"
              @click="goSettings"
            >
              打开设置
              <ChevronRight class="w-3.5 h-3.5 opacity-90" stroke-width="2.5" />
            </button>
          </div>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
article {
  animation: insight-in 0.42s cubic-bezier(0.2, 0.8, 0.2, 1) both;
}

@keyframes insight-in {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  article {
    animation: none;
    opacity: 1;
    transform: none;
  }
}
</style>
