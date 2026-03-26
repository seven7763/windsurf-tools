import { defineStore } from "pinia";
import { ref } from "vue";
import { APIInfo } from "../api/wails";
import type { services } from "../../wailsjs/go/models";

/** 纯 MITM 壳层与工具栏共用状态，由 App 统一 start/stop 轮询 */
export const useMitmStatusStore = defineStore("mitmStatus", () => {
  const status = ref<services.MitmProxyStatus | null>(null);
  const isLoading = ref(false);
  const isRefreshing = ref(false);
  const hasLoadedOnce = ref(false);
  const switchLoading = ref(false);
  const switchTargetAccountId = ref("");
  let pollTimer: ReturnType<typeof setTimeout> | null = null;
  let fetchInFlight: Promise<void> | null = null;
  let lastFetchedAt = 0;

  const fetchStatus = async (force = false) => {
    const now = Date.now();
    if (fetchInFlight) return fetchInFlight;
    if (!force && status.value && now - lastFetchedAt < 1200) {
      return;
    }
    const blocking = !hasLoadedOnce.value;
    if (blocking) {
      isLoading.value = true;
    } else {
      isRefreshing.value = true;
    }
    fetchInFlight = (async () => {
      try {
        status.value = await APIInfo.getMitmProxyStatus();
      } catch (e) {
        console.error("GetMitmProxyStatus error:", e);
      } finally {
        lastFetchedAt = Date.now();
        hasLoadedOnce.value = true;
        if (blocking) {
          isLoading.value = false;
        } else {
          isRefreshing.value = false;
        }
        fetchInFlight = null;
      }
    })();
    return fetchInFlight;
  };

  const ensureStatusLoaded = async (maxAgeMs = 10_000) => {
    const now = Date.now();
    if (hasLoadedOnce.value && now - lastFetchedAt < maxAgeMs) {
      return;
    }
    return fetchStatus();
  };

  const nextPollDelay = () => (status.value?.running ? 8000 : 15000);

  const scheduleNextTick = () => {
    if (pollTimer) {
      clearTimeout(pollTimer);
    }
    pollTimer = setTimeout(() => {
      if (
        typeof document !== "undefined" &&
        document.visibilityState !== "visible"
      ) {
        scheduleNextTick();
        return;
      }
      void fetchStatus().finally(scheduleNextTick);
    }, nextPollDelay());
  };

  const onVisibilityChange = () => {
    if (
      typeof document !== "undefined" &&
      document.visibilityState === "visible"
    ) {
      void fetchStatus().finally(scheduleNextTick);
    }
  };

  const startPolling = () => {
    if (pollTimer) return;
    void fetchStatus().finally(scheduleNextTick);
    document.addEventListener("visibilitychange", onVisibilityChange);
  };

  const stopPolling = () => {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
    document.removeEventListener("visibilitychange", onVisibilityChange);
  };

  const switchToNext = async () => {
    switchLoading.value = true;
    switchTargetAccountId.value = "";
    try {
      const result = await APIInfo.switchMitmToNext();
      await fetchStatus(true);
      return result;
    } finally {
      switchLoading.value = false;
    }
  };

  const switchToAccount = async (accountID: string) => {
    switchLoading.value = true;
    switchTargetAccountId.value = accountID;
    try {
      const result = await APIInfo.switchMitmToAccount(accountID);
      await fetchStatus(true);
      return result;
    } finally {
      switchLoading.value = false;
      switchTargetAccountId.value = "";
    }
  };

  return {
    status,
    isLoading,
    isRefreshing,
    hasLoadedOnce,
    switchLoading,
    switchTargetAccountId,
    fetchStatus,
    ensureStatusLoaded,
    startPolling,
    stopPolling,
    switchToNext,
    switchToAccount,
  };
});
