import { defineStore } from 'pinia'
import { ref } from 'vue'
import { APIInfo } from '../api/wails'
import { models } from '../../wailsjs/go/models'
import {
  createDefaultSettings,
  formToSettings,
  normalizeSettings,
  normalizeSwitchPlanFilter,
  settingsToForm,
} from '../utils/settingsModel'
import { PURE_MITM_ONLY } from '../utils/appMode'

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<models.Settings | null>(null)
  const isLoading = ref(true)
  const isRefreshing = ref(false)
  const hasLoadedOnce = ref(false)
  let fetchInFlight: Promise<void> | null = null
  let lastFetchedAt = 0

  const fetchSettings = async (force = false) => {
    const now = Date.now()
    if (fetchInFlight) {
      return fetchInFlight
    }
    if (!force && settings.value && now-lastFetchedAt < 2500) {
      return
    }
    const blocking = !hasLoadedOnce.value || settings.value == null
    if (blocking) {
      isLoading.value = true
    } else {
      isRefreshing.value = true
    }
    fetchInFlight = (async () => {
      try {
        const data = await APIInfo.getSettings()
        settings.value = normalizeSettings(data)
      } catch (e) {
        console.error('Failed to fetch settings:', e)
        settings.value = createDefaultSettings()
      } finally {
        lastFetchedAt = Date.now()
        hasLoadedOnce.value = true
        if (blocking) {
          isLoading.value = false
        } else {
          isRefreshing.value = false
        }
        fetchInFlight = null
      }
    })()
    return fetchInFlight
  }

  const updateSettings = async (payload: models.Settings) => {
    await APIInfo.updateSettings(payload)
    settings.value = normalizeSettings(payload)
  }

  const ensurePureMitm = async () => {
    if (!PURE_MITM_ONLY) {
      return
    }
    const current = normalizeSettings(settings.value ?? createDefaultSettings())
    if (current.mitm_only === true) {
      settings.value = current
      return
    }
    const next = new models.Settings({
      ...current,
      mitm_only: true,
    })
    await updateSettings(next)
  }

  /** 仅更新「无感下一席位」计划筛选并写回设置文件 */
  const saveAutoSwitchPlanFilter = async (filter: string) => {
    const base = normalizeSettings(settings.value ?? createDefaultSettings())
    const form = settingsToForm(base)
    form.auto_switch_plan_filter = normalizeSwitchPlanFilter(filter)
    await updateSettings(formToSettings(form))
  }

  return {
    settings,
    isLoading,
    isRefreshing,
    hasLoadedOnce,
    fetchSettings,
    updateSettings,
    ensurePureMitm,
    saveAutoSwitchPlanFilter,
  }
})
