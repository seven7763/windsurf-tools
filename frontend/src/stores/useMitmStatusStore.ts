import { defineStore } from 'pinia'
import { ref } from 'vue'
import { APIInfo } from '../api/wails'
import type { services } from '../../wailsjs/go/models'

/** 控制台与 MITM 面板共用状态，由 Dashboard 统一 start/stop 轮询 */
export const useMitmStatusStore = defineStore('mitmStatus', () => {
  const status = ref<services.MitmProxyStatus | null>(null)
  let pollTimer: ReturnType<typeof setTimeout> | null = null
  let fetchInFlight = false

  const fetchStatus = async () => {
    if (fetchInFlight) return
    fetchInFlight = true
    try {
      status.value = await APIInfo.getMitmProxyStatus()
    } catch (e) {
      console.error('GetMitmProxyStatus error:', e)
    } finally {
      fetchInFlight = false
    }
  }

  const nextPollDelay = () => (status.value?.running ? 8000 : 15000)

  const scheduleNextTick = () => {
    if (pollTimer) {
      clearTimeout(pollTimer)
    }
    pollTimer = setTimeout(() => {
      if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
        scheduleNextTick()
        return
      }
      void fetchStatus().finally(scheduleNextTick)
    }, nextPollDelay())
  }

  const onVisibilityChange = () => {
    if (typeof document !== 'undefined' && document.visibilityState === 'visible') {
      void fetchStatus().finally(scheduleNextTick)
    }
  }

  const startPolling = () => {
    if (pollTimer) return
    void fetchStatus().finally(scheduleNextTick)
    document.addEventListener('visibilitychange', onVisibilityChange)
  }

  const stopPolling = () => {
    if (pollTimer) {
      clearTimeout(pollTimer)
      pollTimer = null
    }
    document.removeEventListener('visibilitychange', onVisibilityChange)
  }

  return { status, fetchStatus, startPolling, stopPolling }
})
