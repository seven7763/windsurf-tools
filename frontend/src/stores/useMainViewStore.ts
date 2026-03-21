import { defineStore } from 'pinia'
import { ref } from 'vue'

/** 主界面当前标签（总览 / 账号池 / 设置），供智能提示等跨组件跳转 */
export const useMainViewStore = defineStore('mainView', () => {
  const activeTab = ref('Dashboard')
  return { activeTab }
})
