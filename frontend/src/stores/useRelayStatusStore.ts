import { defineStore } from "pinia";
import { ref } from "vue";
import { APIInfo } from "../api/wails";

type RelayStatus = {
  running?: boolean;
  port?: number;
  url?: string;
};

export const useRelayStatusStore = defineStore("relayStatus", () => {
  const status = ref<RelayStatus | null>(null);
  const isLoading = ref(false);
  const isRefreshing = ref(false);
  const hasLoadedOnce = ref(false);
  let fetchInFlight: Promise<void> | null = null;
  let lastFetchedAt = 0;

  const fetchStatus = async (force = false) => {
    const now = Date.now();
    if (fetchInFlight) {
      return fetchInFlight;
    }
    if (!force && hasLoadedOnce.value && now - lastFetchedAt < 10_000) {
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
        status.value = await APIInfo.getOpenAIRelayStatus();
      } catch (error) {
        console.error("getOpenAIRelayStatus error:", error);
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

  return {
    status,
    isLoading,
    isRefreshing,
    hasLoadedOnce,
    fetchStatus,
    ensureStatusLoaded,
  };
});
