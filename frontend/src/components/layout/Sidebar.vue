<script setup lang="ts">
import { computed } from 'vue'
import { Activity, Globe, HardDriveDownload, Hash, LayoutDashboard, MessageSquare, Settings, Shield, User, Users } from 'lucide-vue-next'
import { useAccountStore } from '../../stores/useAccountStore'
import { useMitmStatusStore } from '../../stores/useMitmStatusStore'
import { PRIMARY_POOL_LABEL, type ShellViewTab } from '../../utils/appMode'

const props = defineProps<{ activeTab: ShellViewTab }>()
const emit = defineEmits<{ (e: 'update:activeTab', tab: ShellViewTab): void }>()

const accountStore = useAccountStore()
const mitmStore = useMitmStatusStore()

const menuItems = [
  { id: 'Dashboard', icon: LayoutDashboard, label: '总览' },
  { id: 'Accounts', icon: Users, label: PRIMARY_POOL_LABEL },
  { id: 'Usage', icon: Activity, label: '用量统计' },
  { id: 'Relay', icon: Globe, label: 'OpenAI Relay' },
  { id: 'Cleanup', icon: HardDriveDownload, label: '清理优化' },
  { id: 'Settings', icon: Settings, label: 'MITM 设置' },
] satisfies Array<{ id: ShellViewTab; icon: typeof Users; label: string }>

const footerModeLabel = computed(() => 'Pure MITM')

const activeKey = computed(() => mitmStore.status?.pool_status?.find((item) => item.is_current) ?? null)

const activeSummary = computed(() => {
  const key = String(activeKey.value?.key_short || '').trim()
  if (!key) {
    return '等待活跃 Key'
  }
  return key
})

const activeAccountLabel = computed(() => {
  const k = activeKey.value
  if (!k) return ''
  const nick = String(k.nickname || '').trim()
  const email = String(k.email || '').trim()
  if (nick && email) return `${nick} (${email})`
  return email || nick || ''
})

const boundSessions = computed(() => {
  const sessions = mitmStore.status?.active_sessions ?? []
  const currentKeyShort = activeKey.value?.key_short ?? ''
  if (!currentKeyShort) return []
  return sessions.filter((s) => s.pool_key_short === currentKeyShort)
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
        MITM 概况
      </div>
      <div class="mt-3 flex items-center justify-between">
        <div>
          <div class="text-[20px] font-extrabold leading-none text-ios-text dark:text-ios-textDark">
            {{ accountStore.accounts.length }}
          </div>
          <div class="mt-1 text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark">
            号池总数
          </div>
        </div>
        <span class="rounded-full bg-ios-blue/10 px-2.5 py-1 text-[10px] font-bold tracking-wide text-ios-blue">
          {{ footerModeLabel }}
        </span>
      </div>

      <!-- 当前活跃 Key -->
      <div class="mt-3 rounded-[14px] bg-black/[0.03] px-3 py-2 text-[11px] font-medium text-ios-textSecondary dark:bg-white/[0.05] dark:text-ios-textSecondaryDark">
        当前活跃 Key
        <div class="mt-1 truncate text-[12px] font-semibold text-ios-text dark:text-ios-textDark" :title="activeSummary">
          {{ activeSummary }}
        </div>
      </div>

      <!-- 当前活跃账号 -->
      <div v-if="activeAccountLabel" class="mt-2 flex items-center gap-1.5 rounded-[14px] bg-ios-blue/[0.06] px-3 py-2 text-[11px] font-medium text-ios-blue">
        <User class="h-3.5 w-3.5 shrink-0" stroke-width="2.4" />
        <span class="truncate" :title="activeAccountLabel">{{ activeAccountLabel }}</span>
      </div>

      <!-- 绑定的对话 -->
      <div v-if="boundSessions.length > 0" class="mt-2 rounded-[14px] bg-black/[0.02] px-3 py-2 dark:bg-white/[0.03]">
        <div class="flex items-center gap-1 text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark mb-1.5">
          <MessageSquare class="h-3 w-3 shrink-0" stroke-width="2.2" />
          绑定对话 ({{ boundSessions.length }})
        </div>
        <ul class="space-y-1">
          <li
            v-for="session in boundSessions"
            :key="session.conv_id_short"
            class="flex items-center gap-1.5 text-[10px] text-ios-text dark:text-ios-textDark"
          >
            <Hash class="h-3 w-3 shrink-0 opacity-40" stroke-width="2" />
            <span class="truncate font-mono" :title="session.conv_id_short">{{ session.conv_id_short }}</span>
            <span class="ml-auto shrink-0 text-[9px] text-ios-textSecondary dark:text-ios-textSecondaryDark">{{ session.request_count }}次</span>
          </li>
        </ul>
      </div>

      <div class="mt-3 flex items-center gap-2 rounded-[14px] border border-emerald-500/12 bg-emerald-500/[0.06] px-3 py-2 text-[11px] font-medium text-emerald-700 dark:text-emerald-300">
        <Shield class="h-3.5 w-3.5 shrink-0" stroke-width="2.4" />
        健康 {{ mitmStore.status?.pool_status?.filter((item) => item.healthy).length ?? 0 }} / {{ mitmStore.status?.pool_status?.length ?? 0 }}
      </div>
    </div>
  </nav>
</template>
