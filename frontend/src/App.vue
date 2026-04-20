<script setup lang="ts">
import {
  type Component,
  computed,
  defineAsyncComponent,
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
import PageLoadingSkeleton from "./components/common/PageLoadingSkeleton.vue";
import { useAccountStore } from "./stores/useAccountStore";
import { useSettingsStore } from "./stores/useSettingsStore";
import { useMitmStatusStore } from "./stores/useMitmStatusStore";
import { useMainViewStore } from "./stores/useMainViewStore";
import {
  DEFAULT_MAIN_VIEW,
  type ShellViewTab,
} from "./utils/appMode";

const mainView = useMainViewStore();
const settings = useSettingsStore();
const mitmStore = useMitmStatusStore();
const shellReady = ref(false);
const mountedViews = ref<ShellViewTab[]>([]);
let unVisibilityRefresh: (() => void) | undefined;
let viewPreloadTimer: ReturnType<typeof setTimeout> | undefined;

type AsyncViewModule = { default: Component };

const viewLoaders: Record<ShellViewTab, () => Promise<AsyncViewModule>> = {
  Dashboard: () => import("./views/Dashboard.vue"),
  Accounts: () => import("./views/Accounts.vue"),
  Usage: () => import("./views/Usage.vue"),
  Relay: () => import("./views/Relay.vue"),
  Cleanup: () => import("./views/Cleanup.vue"),
  Settings: () => import("./views/Settings.vue"),
};

const preloadedViews = new Set<ShellViewTab>();

const viewRegistry = {
  Dashboard: {
    component: defineAsyncComponent(viewLoaders.Dashboard),
    skeleton: "dashboard",
  },
  Accounts: {
    component: defineAsyncComponent(viewLoaders.Accounts),
    skeleton: "accounts",
  },
  Usage: {
    component: defineAsyncComponent(viewLoaders.Usage),
    skeleton: "usage",
  },
  Relay: {
    component: defineAsyncComponent(viewLoaders.Relay),
    skeleton: "relay",
  },
  Cleanup: {
    component: defineAsyncComponent(viewLoaders.Cleanup),
    skeleton: "settings",
  },
  Settings: {
    component: defineAsyncComponent(viewLoaders.Settings),
    skeleton: "settings",
  },
} as const;

const shellTabs = Object.keys(viewRegistry) as ShellViewTab[];

const resolveShellViewTab = (value: string | null | undefined): ShellViewTab =>
  shellTabs.includes(value as ShellViewTab)
    ? (value as ShellViewTab)
    : DEFAULT_MAIN_VIEW;

const ensureViewMounted = (tab: ShellViewTab) => {
  if (!mountedViews.value.includes(tab)) {
    mountedViews.value = [...mountedViews.value, tab];
  }
};

const preloadView = async (tab: ShellViewTab) => {
  if (preloadedViews.has(tab)) {
    return;
  }
  preloadedViews.add(tab);
  try {
    await viewLoaders[tab]();
  } catch (error) {
    preloadedViews.delete(tab);
    console.error(`Failed to preload ${tab} view:`, error);
  }
};

const scheduleBackgroundViewPreload = (activeTab?: ShellViewTab) => {
  if (viewPreloadTimer) {
    clearTimeout(viewPreloadTimer);
  }
  viewPreloadTimer = window.setTimeout(() => {
    for (const tab of shellTabs) {
      if (tab !== activeTab) {
        void preloadView(tab);
      }
    }
  }, 160);
};

const renderedViews = computed(() =>
  mountedViews.value.map((tab) => ({
    key: tab,
    component: viewRegistry[tab].component,
    skeleton: viewRegistry[tab].skeleton,
  })),
);

watch(
  () => mainView.activeTab,
  (value) => {
    const resolved = resolveShellViewTab(value);
    if (mainView.activeTab !== resolved) {
      mainView.activeTab = resolved;
    }
    ensureViewMounted(resolved);
    void preloadView(resolved);
    scheduleBackgroundViewPreload(resolved);
  },
  { immediate: true },
);

onMounted(async () => {
  const accounts = useAccountStore();
  await settings.fetchSettings();
  if (!(mainView.activeTab in viewRegistry)) {
    mainView.activeTab = DEFAULT_MAIN_VIEW;
  }
  shellReady.value = true;
  const currentTab = resolveShellViewTab(mainView.activeTab);
  ensureViewMounted(currentTab);
  void preloadView(currentTab);
  scheduleBackgroundViewPreload(currentTab);
  mitmStore.startPolling();
  void accounts.ensureAccountsLoaded().catch((error) => {
    console.error("App bootstrap accounts fetch failed:", error);
  });

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
    void accounts.fetchAccounts();
    void mitmStore.fetchStatus();
  };
  document.addEventListener("visibilitychange", onVisibilityChange);
  unVisibilityRefresh = () =>
    document.removeEventListener("visibilitychange", onVisibilityChange);
});

onUnmounted(() => {
  mitmStore.stopPolling();
  unVisibilityRefresh?.();
  if (viewPreloadTimer) {
    clearTimeout(viewPreloadTimer);
    viewPreloadTimer = undefined;
  }
});
</script>

<template>
  <div
    class="flex flex-col h-full text-ios-text dark:text-ios-textDark overflow-hidden antialiased app-root"
  >
    <template v-if="!shellReady">
      <div class="flex-1 min-h-0 p-4">
        <div
          class="h-full rounded-[28px] backdrop-blur-2xl border border-black/[0.05] bg-white/72 dark:border-white/[0.08] dark:bg-[#1C1C1E]/82"
        />
      </div>
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
              <section
                v-for="view in renderedViews"
                :key="view.key"
                v-show="mainView.activeTab === view.key"
                class="flex-1 min-h-0 flex flex-col ios-view-surface"
                :aria-hidden="mainView.activeTab === view.key ? 'false' : 'true'"
              >
                <Suspense>
                  <component :is="view.component" />
                  <template #fallback>
                    <PageLoadingSkeleton :variant="view.skeleton" class="flex-1" />
                  </template>
                </Suspense>
              </section>
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
.ios-view-surface {
  contain: layout paint;
}
</style>
