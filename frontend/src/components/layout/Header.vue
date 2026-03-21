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
</script>

<template>
  <header
    class="drag-region grid h-[56px] w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-4 md:px-5 bg-white/80 dark:bg-[#1C1C1E]/85 backdrop-blur-2xl border-b border-ios-divider dark:border-ios-dividerDark select-none z-50 shrink-0"
  >
    <div class="flex min-w-0 items-center gap-3">
      <div class="flex h-9 w-9 items-center justify-center rounded-2xl bg-gradient-to-br from-ios-blue to-sky-400 text-white shadow-[0_10px_20px_rgba(37,99,235,0.22)]">
        <span class="text-[15px] font-black tracking-tight">W</span>
      </div>
      <div class="min-w-0">
        <div class="flex min-w-0 items-center gap-2">
          <span class="truncate text-[15px] font-semibold tracking-tight text-ios-text dark:text-ios-textDark">
            Windsurf Tools
          </span>
          <span class="hidden rounded-full bg-ios-blue/10 px-2 py-0.5 text-[10px] font-bold tracking-wide text-ios-blue md:inline-flex">
            {{ modeLabel }}
          </span>
        </div>
        <div class="mt-0.5 flex min-w-0 flex-col text-ios-textSecondary dark:text-ios-textSecondaryDark">
          <span class="text-[10px] font-medium tracking-wide tabular-nums">
            Control Deck · v{{ APP_VERSION }}
          </span>
          <span class="hidden truncate text-[11px] font-semibold md:block" :title="systemStore.currentAuthEmail || ''">
            {{ onlineEmail }}
          </span>
        </div>
      </div>
    </div>

    <div class="no-drag-region flex items-center justify-end gap-2">
      <div class="hidden min-w-[220px] rounded-[16px] border border-black/[0.06] bg-black/[0.03] px-3 py-2 text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.06] dark:text-ios-textSecondaryDark lg:flex lg:flex-col">
        <span class="text-[10px] font-bold uppercase tracking-[0.18em]">
          当前会话
        </span>
        <span class="mt-1 truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark" :title="systemStore.currentAuthEmail || ''">
          {{ onlineEmail }}
        </span>
      </div>
      <button
        type="button"
        class="w-8 h-8 rounded-full flex items-center justify-center hover:bg-black/5 dark:hover:bg-white/10 transition-colors text-ios-text dark:text-ios-textDark"
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
