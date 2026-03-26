<script setup lang="ts">
import {
  computed,
  defineAsyncComponent,
  nextTick,
  onMounted,
  onUnmounted,
  ref,
  watch,
} from "vue";
import Header from "./components/layout/Header.vue";
import Sidebar from "./components/layout/Sidebar.vue";
import AppFooter from "./components/layout/AppFooter.vue";
import IConfirm from "./components/ios/IConfirm.vue";
import IToast from "./components/ios/IToast.vue";
import ToolbarStrip from "./components/ToolbarStrip.vue";
import PageLoadingSkeleton from "./components/common/PageLoadingSkeleton.vue";
import { useAccountStore } from "./stores/useAccountStore";
import { useSettingsStore } from "./stores/useSettingsStore";
import { useMitmStatusStore } from "./stores/useMitmStatusStore";
import { useMainViewStore } from "./stores/useMainViewStore";
import {
  DEFAULT_MAIN_VIEW,
  PURE_MITM_ONLY,
  type ShellViewTab,
} from "./utils/appMode";
import { APIInfo } from "./api/wails";
import { EventsOn, WindowShow } from "../wailsjs/runtime/runtime";

const mainView = useMainViewStore();
const settings = useSettingsStore();
const mitmStore = useMitmStatusStore();
const toolbarMode = ref(false);
const shellReady = ref(false);
let unToolbarEvent: (() => void) | undefined;
let unVisibilityRefresh: (() => void) | undefined;

const viewRegistry = {
  Dashboard: {
    component: defineAsyncComponent(() => import("./views/Dashboard.vue")),
    skeleton: "dashboard",
  },
  Accounts: {
    component: defineAsyncComponent(() => import("./views/Accounts.vue")),
    skeleton: "accounts",
  },
  Relay: {
    component: defineAsyncComponent(() => import("./views/Relay.vue")),
    skeleton: "relay",
  },
  Settings: {
    component: defineAsyncComponent(() => import("./views/Settings.vue")),
    skeleton: "settings",
  },
} as const;

const activeViewComponent = computed(() => {
  const current = mainView.activeTab as ShellViewTab;
  return (
    viewRegistry[current]?.component ??
    viewRegistry[DEFAULT_MAIN_VIEW].component
  );
});

const activeViewSkeleton = computed(() => {
  const current = mainView.activeTab as ShellViewTab;
  return (
    viewRegistry[current]?.skeleton ?? viewRegistry[DEFAULT_MAIN_VIEW].skeleton
  );
});

watch(
  () => settings.settings?.show_desktop_toolbar,
  (enabled) => {
    if (typeof enabled === "boolean") {
      toolbarMode.value = enabled;
    }
  },
  { immediate: true },
);

onMounted(async () => {
  const accounts = useAccountStore();
  await settings.fetchSettings();
  if (PURE_MITM_ONLY) {
    await settings.ensurePureMitm();
    if (!(mainView.activeTab in viewRegistry)) {
      mainView.activeTab = DEFAULT_MAIN_VIEW;
    }
  }
  toolbarMode.value = settings.settings?.show_desktop_toolbar === true;

  unToolbarEvent = EventsOn("toolbar:set", (...data: unknown[]) => {
    toolbarMode.value = Boolean(data[0]);
  });

  // 必须先切 toolbarMode 再 Resize，否则小窗里仍是完整主界面 DOM（错乱）
  if (settings.settings?.show_desktop_toolbar) {
    toolbarMode.value = true;
    await nextTick();
    await APIInfo.applyToolbarLayout(true);
    // 静默启动时 Go 会先 WindowHide；小窗就绪后必须再 Show，否则只见托盘不见小条
    WindowShow();
  }

  shellReady.value = true;
  mitmStore.startPolling();
  if (!toolbarMode.value) {
    void accounts.ensureAccountsLoaded().catch((error) => {
      console.error("App bootstrap accounts fetch failed:", error);
    });
  }

  // 从后台切回前台时仅刷新 MITM 相关数据，避免旧 Auth 路径带来的额外抖动
  let lastFocusRefresh = 0;
  const onVisibilityChange = () => {
    if (
      typeof document === "undefined" ||
      document.visibilityState !== "visible"
    ) {
      return;
    }
    const now = Date.now();
    if (now - lastFocusRefresh < 2500) {
      return;
    }
    lastFocusRefresh = now;
    if (toolbarMode.value) {
      return;
    }
    void accounts.fetchAccounts();
    void mitmStore.fetchStatus();
  };
  document.addEventListener("visibilitychange", onVisibilityChange);
  unVisibilityRefresh = () =>
    document.removeEventListener("visibilitychange", onVisibilityChange);
});

onUnmounted(() => {
  mitmStore.stopPolling();
  unToolbarEvent?.();
  unVisibilityRefresh?.();
});
</script>

<template>
  <div
    class="flex flex-col h-full text-ios-text dark:text-ios-textDark overflow-hidden antialiased app-root"
    :class="toolbarMode ? 'bg-transparent' : ''"
  >
    <template v-if="!shellReady">
      <div class="flex-1 min-h-0 p-4">
        <div
          class="h-full rounded-[28px] backdrop-blur-2xl"
          :class="
            toolbarMode
              ? 'border border-transparent bg-transparent shadow-none'
              : 'border border-black/[0.05] bg-white/72 dark:border-white/[0.08] dark:bg-[#1C1C1E]/82'
          "
        />
      </div>
    </template>
    <template v-else-if="toolbarMode">
      <ToolbarStrip class="flex-1 min-h-0 flex flex-col justify-center" />
    </template>
    <template v-else>
      <Header />
      <div class="flex flex-1 overflow-hidden relative">
        <Sidebar
          :activeTab="mainView.activeTab"
          @update:activeTab="mainView.activeTab = $event"
        />
        <main
          class="flex-1 flex flex-col min-h-0 overflow-hidden relative bg-black/[0.01] dark:bg-white/[0.01]"
        >
          <div
            class="flex-1 overflow-y-auto overflow-x-hidden relative scroll-smooth min-h-0 flex flex-col"
          >
            <div class="flex-1 shrink-0 flex flex-col relative">
              <Transition name="fade">
                <Suspense :key="mainView.activeTab">
                  <component :is="activeViewComponent" />
                  <template #fallback>
                    <PageLoadingSkeleton
                      :variant="activeViewSkeleton"
                      class="flex-1"
                    />
                  </template>
                </Suspense>
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
  transition:
    opacity 0.24s cubic-bezier(0.2, 0.8, 0.2, 1),
    transform 0.24s cubic-bezier(0.2, 0.8, 0.2, 1);
}
.fade-leave-active {
  position: absolute;
  inset: 0;
  width: 100%;
  pointer-events: none;
}
.fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.fade-leave-to {
  opacity: 0;
  transform: translateY(-2px);
}
</style>
