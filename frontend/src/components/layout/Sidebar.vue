<script setup lang="ts">
import { computed } from 'vue'
import { Globe, LayoutDashboard, Users, Settings } from 'lucide-vue-next'
import { useAccountStore } from '../../stores/useAccountStore'
import { useSettingsStore } from '../../stores/useSettingsStore'
import { useSystemStore } from '../../stores/useSystemStore'

const props = defineProps<{ activeTab: string }>()
const emit = defineEmits<{ (e: 'update:activeTab', tab: string): void }>()

const accountStore = useAccountStore()
const settingsStore = useSettingsStore()
const systemStore = useSystemStore()

const menuItems = computed(() => {
  const mitm = settingsStore.settings?.mitm_only === true
  return [
    { id: 'Dashboard', icon: LayoutDashboard, label: '总览' },
    { id: 'Accounts', icon: Users, label: mitm ? '号池 (MITM)' : '账号池' },
    { id: 'Relay', icon: Globe, label: 'API 中转' },
    { id: 'Settings', icon: Settings, label: '设置' },
  ]
})

const footerModeLabel = computed(() =>
  settingsStore.settings?.mitm_only === true ? 'MITM 多号轮换' : '本地 Auth 切号',
)

const onlineSummary = computed(() => {
  const email = (systemStore.currentAuthEmail || '').trim()
  if (!email) {
    return '未检测到在线账号'
  }
  return email.length > 22 ? `${email.slice(0, 20)}…` : email
})
</script>

<template>
  <nav class="w-60 h-full ios-glass border-r flex flex-col pt-6 pb-6 z-40 shrink-0">
    <div class="px-5 pb-2 mb-2 text-xs font-semibold uppercase text-ios-textSecondary dark:text-ios-textSecondaryDark tracking-wider">
      导航
    </div>
    <ul class="flex-1 space-y-1.5 px-3">
      <li v-for="item in menuItems" :key="item.id">
        <button
          type="button"
          class="no-drag-region"
          @click="emit('update:activeTab', item.id)"
          :class="[
            'w-full flex items-center px-4 py-2.5 rounded-[14px] text-[14px] transition-all duration-[250ms] font-medium ios-btn',
            activeTab === item.id 
              ? 'bg-gradient-to-b from-[#3b82f6] to-ios-blue text-white shadow-md shadow-ios-blue/25 ring-1 ring-black/5 dark:ring-white/10 ring-inset' 
              : 'text-ios-text dark:text-ios-textDark hover:bg-black/5 dark:hover:bg-white/10'
          ]"
        >
          <component :is="item.icon" class="w-5 h-5 mr-3 transition-opacity duration-300" :class="activeTab === item.id ? 'opacity-100' : 'opacity-70'" stroke-width="2.2" />
          {{ item.label }}
        </button>
      </li>
    </ul>

    <div class="mx-3 mt-4 rounded-[18px] border border-black/[0.05] bg-white/60 px-4 py-4 shadow-[0_8px_24px_rgba(15,23,42,0.06)] dark:border-white/[0.06] dark:bg-white/[0.04]">
      <div class="text-[11px] font-bold uppercase tracking-[0.22em] text-ios-textSecondary dark:text-ios-textSecondaryDark">
        实时概况
      </div>
      <div class="mt-3 flex items-center justify-between">
        <div>
          <div class="text-[20px] font-extrabold leading-none text-ios-text dark:text-ios-textDark">
            {{ accountStore.accounts.length }}
          </div>
          <div class="mt-1 text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark">
            账号总数
          </div>
        </div>
        <span class="rounded-full bg-ios-blue/10 px-2.5 py-1 text-[10px] font-bold tracking-wide text-ios-blue">
          {{ footerModeLabel }}
        </span>
      </div>
      <div class="mt-3 rounded-[14px] bg-black/[0.03] px-3 py-2 text-[11px] font-medium text-ios-textSecondary dark:bg-white/[0.05] dark:text-ios-textSecondaryDark">
        当前会话
        <div class="mt-1 truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark" :title="systemStore.currentAuthEmail || ''">
          {{ onlineSummary }}
        </div>
      </div>
    </div>
  </nav>
</template>
