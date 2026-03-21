import { defineStore } from 'pinia'
import { ref } from 'vue'
import { APIInfo } from '../api/wails'
import type { services } from '../../wailsjs/go/models'

/** 控制台与 MITM 面板共用状态，由 Dashboard 统一 start/stop 轮询 */
export const useMitmStatusStore = defineStore('mitmStatus', () => {
  const status = ref<services.MitmProxyStatus | null>(null)
  let pollTimer: ReturnType<typeof setInterval> | null = null

  const fetchStatus = async () => {
    try {
      status.value = await APIInfo.getMitmProxyStatus()
    } catch (e) {
      console.error('GetMitmProxyStatus error:', e)
    }
  }

  const pollTick = () => {
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
    void fetchStatus()
  }

  const onVisibilityChange = () => {
    if (typeof document !== 'undefined' && document.visibilityState === 'visible') void fetchStatus()
  }

  const startPolling = () => {
    if (pollTimer) return
    void fetchStatus()
    pollTimer = setInterval(pollTick, 5000)
    document.addEventListener('visibilitychange', onVisibilityChange)
  }

  const stopPolling = () => {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
    document.removeEventListener('visibilitychange', onVisibilityChange)
  }

  return { status, fetchStatus, startPolling, stopPolling }
})
