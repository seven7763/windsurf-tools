<script setup lang="ts">
import { computed } from 'vue'
import { Moon, Monitor, Sun } from 'lucide-vue-next'
import { useSettingsStore } from '../../stores/useSettingsStore'
import { useSystemStore } from '../../stores/useSystemStore'
import { APP_VERSION } from '../../utils/appMeta'
import { cycleTheme, themeLabel, themeMode } from '../../utils/theme'

const settingsStore = useSettingsStore()
const systemStore = useSystemStore()

const modeLabel = computed(() =>
  settingsStore.settings?.mitm_only === true ? 'MITM 轮换' : 'Auth 切号',
)

const onlineEmail = computed(() => {
  const email = (systemStore.currentAuthEmail || '').trim()
  if (!email) {
    return '未检测到在线账号'
  }
  return email.length > 28 ? `${email.slice(0, 26)}…` : email
})

const onlineEmailFull = computed(() => (systemStore.currentAuthEmail || '').trim())

const onlineSummary = computed(() => {
  const email = onlineEmailFull.value
  if (!email) {
    return '等待号池或当前会话同步'
  }
  const [local, domain] = email.split('@')
  if (!local || !domain) {
    return '当前会话已连接'
  }
  return `${local.length > 16 ? `${local.slice(0, 14)}…` : local} @ ${domain}`
})

const sessionStateLabel = computed(() =>
  onlineEmailFull.value ? '当前在线账号' : '会话未检测',
)

const sessionStateTone = computed(() =>
  onlineEmailFull.value
    ? 'border-emerald-500/18 bg-emerald-500/[0.08] text-emerald-700 dark:text-emerald-300'
    : 'border-black/[0.06] bg-black/[0.03] text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark',
)
</script>

<template>
  <header
    class="drag-region grid h-[64px] w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-4 px-4 md:px-5 bg-white/82 dark:bg-[#1C1C1E]/88 backdrop-blur-2xl border-b border-ios-divider dark:border-ios-dividerDark select-none z-50 shrink-0"
  >
    <div class="flex min-w-0 items-center gap-3">
      <div class="flex h-10 w-10 items-center justify-center rounded-2xl bg-gradient-to-br from-ios-blue to-sky-400 text-white shadow-[0_10px_22px_rgba(37,99,235,0.24)]">
        <span class="text-[15px] font-black tracking-tight">W</span>
      </div>
      <div class="min-w-0">
        <div class="flex min-w-0 items-center gap-2">
          <span class="truncate text-[16px] font-semibold tracking-tight text-ios-text dark:text-ios-textDark">
            Windsurf Tools
          </span>
          <span class="hidden rounded-full bg-ios-blue/10 px-2.5 py-0.5 text-[10px] font-bold tracking-wide text-ios-blue md:inline-flex">
            {{ modeLabel }}
          </span>
        </div>
        <div class="mt-0.5 flex min-w-0 items-center gap-2 text-ios-textSecondary dark:text-ios-textSecondaryDark">
          <span class="text-[10px] font-medium tracking-wide tabular-nums">
            Control Deck · v{{ APP_VERSION }}
          </span>
          <span class="hidden h-1 w-1 rounded-full bg-black/20 dark:bg-white/20 md:block" />
          <span class="hidden truncate text-[11px] font-medium md:block">
            多账号额度同步与切号控制台
          </span>
        </div>
      </div>
    </div>

    <div class="no-drag-region flex min-w-0 items-center justify-end gap-2">
      <div
        class="hidden min-w-[240px] max-w-[360px] items-center gap-3 rounded-[18px] border px-3.5 py-2 shadow-[0_8px_22px_rgba(15,23,42,0.05)] lg:flex"
        :class="sessionStateTone"
      >
        <div
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl"
          :class="onlineEmailFull ? 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-300' : 'bg-black/[0.05] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark'"
        >
          <span
            class="h-2.5 w-2.5 rounded-full shadow-[0_0_10px_rgba(52,211,153,0.35)]"
            :class="onlineEmailFull ? 'bg-emerald-500' : 'bg-slate-400 dark:bg-slate-500'"
          />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="truncate text-[10px] font-bold uppercase tracking-[0.16em]">
              {{ sessionStateLabel }}
            </span>
          </div>
          <div class="mt-1 truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark" :title="onlineEmailFull || ''">
            {{ onlineEmail }}
          </div>
        </div>
        <span
          class="hidden shrink-0 rounded-full px-2 py-1 text-[10px] font-bold tracking-wide xl:inline-flex"
          :class="onlineEmailFull ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-black/[0.05] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark'"
          :title="onlineEmailFull || onlineSummary"
        >
          {{ onlineSummary }}
        </span>
      </div>

      <div
        class="flex min-w-0 max-w-[220px] items-center gap-2 rounded-full border border-black/[0.06] bg-black/[0.03] px-3 py-1.5 text-[11px] font-medium text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark lg:hidden"
        :title="onlineEmailFull || ''"
      >
        <span
          class="h-2 w-2 shrink-0 rounded-full"
          :class="onlineEmailFull ? 'bg-emerald-500' : 'bg-slate-400 dark:bg-slate-500'"
        />
        <span class="truncate">{{ onlineEmail }}</span>
      </div>

      <button
        type="button"
        class="flex h-9 w-9 items-center justify-center rounded-full border border-black/[0.06] bg-white/70 text-ios-text shadow-sm transition-colors hover:bg-black/5 dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textDark dark:hover:bg-white/10"
        :title="themeLabel(themeMode)"
        :aria-label="`主题：${themeLabel(themeMode)}，点击切换`"
        @click="cycleTheme()"
      >
        <Sun v-if="themeMode === 'light'" class="w-[18px] h-[18px]" stroke-width="2.5" />
        <Moon v-else-if="themeMode === 'dark'" class="w-[18px] h-[18px]" stroke-width="2.5" />
        <Monitor v-else class="w-[18px] h-[18px]" stroke-width="2.5" />
      </button>
    </div>
  </header>
</template>
