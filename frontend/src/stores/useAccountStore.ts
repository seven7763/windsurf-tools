import { defineStore } from 'pinia'
import { ref } from 'vue'
import { APIInfo } from '../api/wails'
import { models } from '../../wailsjs/go/models'

export const useAccountStore = defineStore('account', () => {
  const accounts = ref<models.Account[]>([])
  const isLoading = ref(false)
  const actionLoading = ref(false)
  let fetchInFlight: Promise<void> | null = null
  let lastFetchedAt = 0

  const fetchAccounts = async (force = false) => {
    const now = Date.now()
    if (!force && fetchInFlight) {
      return fetchInFlight
    }
    if (!force && now-lastFetchedAt < 1500) {
      return
    }
    isLoading.value = true
    fetchInFlight = (async () => {
      try {
        const data = await APIInfo.getAllAccounts()
        // 让出主线程一帧，减轻大列表回填时的界面卡顿
        await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
        accounts.value = data || []
        lastFetchedAt = Date.now()
      } catch (e) {
        console.error('Failed to fetch accounts:', e)
      } finally {
        isLoading.value = false
        fetchInFlight = null
      }
    })()
    return fetchInFlight
  }

  const deleteAccount = async (id: string) => {
    await APIInfo.deleteAccount(id)
    await fetchAccounts(true)
  }

  /** 返回删除条数，失败抛错由调用方处理 */
  const cleanExpiredAccounts = async (): Promise<number> => {
    const n = await APIInfo.deleteExpiredAccounts()
    await fetchAccounts(true)
    return n
  }

  /** 删除 plan 归类为 Free/Basic 的账号（与 getPlanTone === 'free' 一致） */
  const deleteFreePlanAccounts = async (): Promise<number> => {
    const n = await APIInfo.deleteFreePlanAccounts()
    await fetchAccounts(true)
    return n
  }

  const refreshAllTokens = async (): Promise<Record<string, string>> => {
    actionLoading.value = true
    try {
      const result = await APIInfo.refreshAllTokens()
      await fetchAccounts(true)
      return result || {}
    } finally {
      actionLoading.value = false
    }
  }

  const refreshAllQuotas = async (): Promise<Record<string, string>> => {
    actionLoading.value = true
    try {
      const result = await APIInfo.refreshAllQuotas()
      await fetchAccounts(true)
      return result || {}
    } finally {
      actionLoading.value = false
    }
  }

  const refreshAccountQuota = async (id: string) => {
    await APIInfo.refreshAccountQuota(id)
    await fetchAccounts(true)
  }

  const autoSwitchToNext = async (currentId: string, planFilter: string): Promise<string> => {
    return APIInfo.autoSwitchToNext(currentId, planFilter)
  }

  return {
    accounts,
    isLoading,
    actionLoading,
    fetchAccounts,
    deleteAccount,
    cleanExpiredAccounts,
    deleteFreePlanAccounts,
    refreshAllTokens,
    refreshAllQuotas,
    refreshAccountQuota,
    autoSwitchToNext,
  }
})
