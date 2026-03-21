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
import { useSystemStore } from './useSystemStore'

export const useSettingsStore = defineStore('settings', () => {
  const settings = ref<models.Settings | null>(null)
  const isLoading = ref(true)

  const fetchSettings = async () => {
    isLoading.value = true
    try {
      const data = await APIInfo.getSettings()
      settings.value = normalizeSettings(data)
    } catch (e) {
      console.error('Failed to fetch settings:', e)
      settings.value = createDefaultSettings()
    } finally {
      isLoading.value = false
    }
  }

  const updateSettings = async (payload: models.Settings) => {
    await APIInfo.updateSettings(payload)
    settings.value = normalizeSettings(payload)
  }

  /** 仅更新「无感下一席位」计划筛选并写回设置文件 */
  const saveAutoSwitchPlanFilter = async (filter: string) => {
    const sys = useSystemStore()
    const base = normalizeSettings(settings.value ?? createDefaultSettings())
    const form = settingsToForm(base)
    form.auto_switch_plan_filter = normalizeSwitchPlanFilter(filter)
    await updateSettings(formToSettings(form, sys.patchStatus, base))
  }

  return {
    settings,
    isLoading,
    fetchSettings,
    updateSettings,
    saveAutoSwitchPlanFilter,
  }
})
