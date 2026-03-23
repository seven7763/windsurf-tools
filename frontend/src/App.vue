<script setup lang="ts">
import { ref, onMounted, onUnmounted, nextTick, watch } from 'vue'
import Header from './components/layout/Header.vue'
import Sidebar from './components/layout/Sidebar.vue'
import AppFooter from './components/layout/AppFooter.vue'
import IConfirm from './components/ios/IConfirm.vue'
import IToast from './components/ios/IToast.vue'
import ToolbarStrip from './components/ToolbarStrip.vue'
import Dashboard from './views/Dashboard.vue'
import Accounts from './views/Accounts.vue'
import Relay from './views/Relay.vue'
import Settings from './views/Settings.vue'
import { useAccountStore } from './stores/useAccountStore'
import { useSettingsStore } from './stores/useSettingsStore'
import { useSystemStore } from './stores/useSystemStore'
import { useMainViewStore } from './stores/useMainViewStore'
import { APIInfo } from './api/wails'
import { EventsOn, WindowShow } from '../wailsjs/runtime/runtime'

const mainView = useMainViewStore()
const settings = useSettingsStore()
const toolbarMode = ref(false)
let unToolbarEvent: (() => void) | undefined
let unVisibilityRefresh: (() => void) | undefined

watch(
  () => settings.settings?.show_desktop_toolbar,
  (enabled) => {
    if (typeof enabled === 'boolean') {
      toolbarMode.value = enabled
    }
  },
  { immediate: true },
)

onMounted(async () => {
  const accounts = useAccountStore()
  const system = useSystemStore()
  await settings.fetchSettings()

  unToolbarEvent = EventsOn('toolbar:set', (...data: unknown[]) => {
    toolbarMode.value = Boolean(data[0])
  })

  // 必须先切 toolbarMode 再 Resize，否则小窗里仍是完整主界面 DOM（错乱）
  if (settings.settings?.show_desktop_toolbar) {
    toolbarMode.value = true
    await nextTick()
    await APIInfo.applyToolbarLayout(true)
    // 静默启动时 Go 会先 WindowHide；小窗就绪后必须再 Show，否则只见托盘不见小条
    WindowShow()
  }

  await Promise.all([accounts.fetchAccounts(), system.initSystemEnvironment()])

  // 从后台切回前台时刷新当前会话与号池（节流，避免频繁触发）
  let lastFocusRefresh = 0
  const onVisibilityChange = () => {
    if (typeof document === 'undefined' || document.visibilityState !== 'visible') {
      return
    }
    const now = Date.now()
    if (now - lastFocusRefresh < 2500) {
      return
    }
    lastFocusRefresh = now
    void system.fetchCurrentAuth()
    void accounts.fetchAccounts()
  }
  document.addEventListener('visibilitychange', onVisibilityChange)
  unVisibilityRefresh = () => document.removeEventListener('visibilitychange', onVisibilityChange)
})

onUnmounted(() => {
  unToolbarEvent?.()
  unVisibilityRefresh?.()
})
</script>

<template>
  <div
    class="flex flex-col h-full text-ios-text dark:text-ios-textDark overflow-hidden antialiased app-root"
    :class="toolbarMode ? 'bg-transparent' : ''"
  >
    <template v-if="toolbarMode">
      <ToolbarStrip class="flex-1 min-h-0 flex flex-col justify-center" />
    </template>
    <template v-else>
      <Header />
      <div class="flex flex-1 overflow-hidden relative">
        <Sidebar :activeTab="mainView.activeTab" @update:activeTab="mainView.activeTab = $event" />
        <main class="flex-1 flex flex-col min-h-0 overflow-hidden relative bg-black/[0.01] dark:bg-white/[0.01]">
          <div class="flex-1 overflow-y-auto overflow-x-hidden relative scroll-smooth min-h-0 flex flex-col">
            <div class="flex-1 shrink-0 flex flex-col">
              <Transition name="fade" mode="out-in">
                <Dashboard v-if="mainView.activeTab === 'Dashboard'" />
                <Accounts v-else-if="mainView.activeTab === 'Accounts'" />
                <Relay v-else-if="mainView.activeTab === 'Relay'" />
                <Settings v-else />
              </Transition>
            </div>
            <AppFooter class="mt-auto" />
          </div>
        </main>
      </div>
    </template>
    <IConfirm />
    <IToast />
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.28s cubic-bezier(0.2, 0.8, 0.2, 1), transform 0.28s cubic-bezier(0.2, 0.8, 0.2, 1);
}
.fade-enter-from {
  opacity: 0;
  transform: translateY(8px);
}
.fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
