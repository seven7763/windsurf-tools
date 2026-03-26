import { defineStore } from "pinia";
import { ref } from "vue";
import { APIInfo } from "../api/wails";
import { models } from "../../wailsjs/go/models";

export const useAccountStore = defineStore("account", () => {
  const accounts = ref<models.Account[]>([]);
  const isLoading = ref(false);
  const isRefreshing = ref(false);
  const hasLoadedOnce = ref(false);
  const actionLoading = ref(false);
  let fetchInFlight: Promise<void> | null = null;
  let lastFetchedAt = 0;

  const patchAccount = (account: models.Account | null | undefined) => {
    if (!account?.id) {
      return null;
    }
    const next = [...accounts.value];
    const idx = next.findIndex((item) => item.id === account.id);
    if (idx >= 0) {
      next[idx] = account;
    } else {
      next.unshift(account);
    }
    accounts.value = next;
    return account;
  };

  const fetchAccounts = async (force = false) => {
    const now = Date.now();
    if (fetchInFlight) {
      return fetchInFlight;
    }
    if (!force && now - lastFetchedAt < 1500) {
      return;
    }
    const blocking = !hasLoadedOnce.value && accounts.value.length === 0;
    if (blocking) {
      isLoading.value = true;
    } else {
      isRefreshing.value = true;
    }
    fetchInFlight = (async () => {
      try {
        const data = await APIInfo.getAllAccounts();
        // 让出主线程一帧，减轻大列表回填时的界面卡顿
        await new Promise<void>((resolve) =>
          requestAnimationFrame(() => resolve()),
        );
        accounts.value = data || [];
        lastFetchedAt = Date.now();
        hasLoadedOnce.value = true;
      } catch (e) {
        console.error("Failed to fetch accounts:", e);
      } finally {
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

  const ensureAccountsLoaded = async (maxAgeMs = 20_000) => {
    const now = Date.now();
    if (hasLoadedOnce.value && now - lastFetchedAt < maxAgeMs) {
      return;
    }
    return fetchAccounts();
  };

  const deleteAccount = async (id: string) => {
    await APIInfo.deleteAccount(id);
    await fetchAccounts(true);
  };

  /** 返回删除条数，失败抛错由调用方处理 */
  const cleanExpiredAccounts = async (): Promise<number> => {
    const n = await APIInfo.deleteExpiredAccounts();
    await fetchAccounts(true);
    return n;
  };

  /** 删除 plan 归类为 Free/Basic 的账号（与 getPlanTone === 'free' 一致） */
  const deleteFreePlanAccounts = async (): Promise<number> => {
    const n = await APIInfo.deleteFreePlanAccounts();
    await fetchAccounts(true);
    return n;
  };

  const refreshAllTokens = async (): Promise<Record<string, string>> => {
    actionLoading.value = true;
    try {
      const result = await APIInfo.refreshAllTokens();
      await fetchAccounts(true);
      return result || {};
    } finally {
      actionLoading.value = false;
    }
  };

  const refreshAllQuotas = async (): Promise<Record<string, string>> => {
    actionLoading.value = true;
    try {
      const result = await APIInfo.refreshAllQuotas();
      await fetchAccounts(true);
      return result || {};
    } finally {
      actionLoading.value = false;
    }
  };

  const refreshAccountQuota = async (id: string) => {
    await APIInfo.refreshAccountQuota(id);
    const updated = await APIInfo.getAccount(id);
    return patchAccount(updated);
  };

  return {
    accounts,
    isLoading,
    isRefreshing,
    hasLoadedOnce,
    actionLoading,
    patchAccount,
    fetchAccounts,
    ensureAccountsLoaded,
    deleteAccount,
    cleanExpiredAccounts,
    deleteFreePlanAccounts,
    refreshAllTokens,
    refreshAllQuotas,
    refreshAccountQuota,
  };
});
