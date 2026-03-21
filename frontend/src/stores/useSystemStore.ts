import { defineStore } from 'pinia'
import { ref } from 'vue'
import { APIInfo } from '../api/wails'

export const useSystemStore = defineStore('system', () => {
  const windsurfPath = ref('')
  /** 号池 accounts.json / settings.json 所在目录（跨平台 WindsurfTools） */
  const appStoragePath = ref('')
  const patchStatus = ref(false)
  const isGlobalLoading = ref(false)
  /** 当前 windsurf_auth.json 中的邮箱（与号池比对用于「在线」高亮） */
  const currentAuthEmail = ref('')
  const currentAuthToken = ref('')

  const fetchCurrentAuth = async () => {
    try {
      const auth = await APIInfo.getCurrentWindsurfAuth()
      currentAuthEmail.value = auth?.email ?? ''
      currentAuthToken.value = auth?.token ?? ''
    } catch {
      currentAuthEmail.value = ''
      currentAuthToken.value = ''
    }
  }

  const initSystemEnvironment = async () => {
    try {
      try {
        appStoragePath.value = (await APIInfo.getAppStoragePath()) || ''
      } catch {
        appStoragePath.value = ''
      }
      windsurfPath.value = await APIInfo.findWindsurfPath()
      if (windsurfPath.value) {
        patchStatus.value = await APIInfo.checkPatchStatus(windsurfPath.value)
      }
      await fetchCurrentAuth()
    } catch (e) {
      console.error('Error init system store:', e)
    }
  }

  const detectWindsurfPath = async () => {
    try {
      windsurfPath.value = await APIInfo.findWindsurfPath()
      if (windsurfPath.value) {
        patchStatus.value = await APIInfo.checkPatchStatus(windsurfPath.value)
      }
      return windsurfPath.value
    } catch (e) {
      console.error('detectWindsurfPath:', e)
      return ''
    }
  }

  const applySeamlessPatch = async () => {
    if (!windsurfPath.value) return false
    try {
      isGlobalLoading.value = true
      await APIInfo.applySeamlessPatch(windsurfPath.value)
      patchStatus.value = await APIInfo.checkPatchStatus(windsurfPath.value)
      return true
    } catch (e) {
      console.error('Patch failed:', e)
      return false
    } finally {
      isGlobalLoading.value = false
    }
  }

  const restoreSeamlessPatch = async () => {
    if (!windsurfPath.value) return false
    try {
      isGlobalLoading.value = true
      await APIInfo.restoreSeamlessPatch(windsurfPath.value)
      patchStatus.value = await APIInfo.checkPatchStatus(windsurfPath.value)
      return true
    } catch (e) {
      console.error('Restore failed:', e)
      return false
    } finally {
      isGlobalLoading.value = false
    }
  }

  return {
    windsurfPath,
    appStoragePath,
    patchStatus,
    isGlobalLoading,
    currentAuthEmail,
    currentAuthToken,
    fetchCurrentAuth,
    initSystemEnvironment,
    detectWindsurfPath,
    applySeamlessPatch,
    restoreSeamlessPatch,
  }
})
