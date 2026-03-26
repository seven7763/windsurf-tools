import { defineStore } from 'pinia'
import { ref } from 'vue'
import { DEFAULT_MAIN_VIEW, type ShellViewTab } from '../utils/appMode'

/** 主界面当前标签（纯 MITM 模式下保留总览 / 号池 / 中转 / 设置） */
export const useMainViewStore = defineStore('mainView', () => {
  const activeTab = ref<ShellViewTab>(DEFAULT_MAIN_VIEW)
  return { activeTab }
})
