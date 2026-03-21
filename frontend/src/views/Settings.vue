<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useSettingsStore } from '../stores/useSettingsStore'
import { useSystemStore } from '../stores/useSystemStore'
import { useAccountStore } from '../stores/useAccountStore'
import IToggle from '../components/ios/IToggle.vue'
import {
  createDefaultSettings,
  formToSettings,
  quotaPolicyOptions,
  settingsToForm,
  type SettingsForm,
  SWITCH_PLAN_FILTER_TONES,
  type SwitchPlanTone,
} from '../utils/settingsModel'
import { getPlanTone } from '../utils/account'
import SwitchPlanFilterControl from '../components/settings/SwitchPlanFilterControl.vue'
import PageLoadingSkeleton from '../components/common/PageLoadingSkeleton.vue'
import {
  CheckCircle2,
  FolderOpen,
  Loader2,
  Minimize2,
  Monitor,
  MoonStar,
  RefreshCcw,
  Save,
} from 'lucide-vue-next'
import { showToast } from '../utils/toast'
import { APIInfo } from '../api/wails'

const settingsStore = useSettingsStore()
const systemStore = useSystemStore()
const accountStore = useAccountStore()
let autoSaveDebounceTimer: ReturnType<typeof setTimeout> | null = null
let saveStateResetTimer: ReturnType<typeof setTimeout> | null = null

const poolPlanCounts = computed<Partial<Record<SwitchPlanTone, number>>>(() => {
  const m: Partial<Record<SwitchPlanTone, number>> = {}
  for (const t of SWITCH_PLAN_FILTER_TONES) {
    m[t] = 0
  }
  for (const a of accountStore.accounts) {
    const tone = getPlanTone(a.plan_name) as SwitchPlanTone
    m[tone] = (m[tone] ?? 0) + 1
  }
  return m
})
const isSaving = ref(false)
const isToolbarSaving = ref(false)
const showSaved = ref(false)
const isSyncingLocal = ref(true)
const saveState = ref<'idle' | 'saving' | 'saved' | 'error'>('idle')
const lastSavedFingerprint = ref('')

const local = reactive<SettingsForm>(settingsToForm(createDefaultSettings()))

onMounted(() => {
  void settingsStore.fetchSettings()
  void accountStore.fetchAccounts()
})

watch(
  () => settingsStore.settings,
  (s) => {
    if (s) {
      isSyncingLocal.value = true
      Object.assign(local, settingsToForm(s))
      if (!local.windsurf_path.trim() && systemStore.windsurfPath) {
        local.windsurf_path = systemStore.windsurfPath
      }
      lastSavedFingerprint.value = buildSettingsFingerprint()
      nextTick(() => {
        isSyncingLocal.value = false
      })
    }
  },
  { immediate: true },
)

watch(
  () => ({
    ...local,
    windsurf_path: local.windsurf_path,
    proxy_url: local.proxy_url,
    quota_custom_interval_minutes: local.quota_custom_interval_minutes,
    quota_hot_poll_seconds: local.quota_hot_poll_seconds,
    concurrent_limit: local.concurrent_limit,
  }),
  () => {
    if (isSyncingLocal.value || isToolbarSaving.value) {
      return
    }
    scheduleAutoSave()
  },
  { deep: true },
)

const buildSettingsPayload = () =>
  formToSettings(local, systemStore.patchStatus, settingsStore.settings ?? undefined)

const buildSettingsFingerprint = () => JSON.stringify(buildSettingsPayload())

const resetSavedStateLater = () => {
  if (saveStateResetTimer) {
    clearTimeout(saveStateResetTimer)
  }
  saveStateResetTimer = setTimeout(() => {
    if (saveState.value === 'saved') {
      saveState.value = 'idle'
      showSaved.value = false
    }
  }, 1600)
}

const persistLocalSettings = async () => {
  const fingerprint = buildSettingsFingerprint()
  if (fingerprint === lastSavedFingerprint.value) {
    return
  }
  isSaving.value = true
  saveState.value = 'saving'
  try {
    await settingsStore.updateSettings(buildSettingsPayload())
    lastSavedFingerprint.value = fingerprint
    saveState.value = 'saved'
    showSaved.value = true
    resetSavedStateLater()
  } catch (e) {
    saveState.value = 'error'
    showToast(`自动保存失败: ${String(e)}`, 'error')
  } finally {
    isSaving.value = false
  }
}

const scheduleAutoSave = () => {
  if (autoSaveDebounceTimer) {
    clearTimeout(autoSaveDebounceTimer)
  }
  autoSaveDebounceTimer = setTimeout(() => {
    void persistLocalSettings()
  }, 420)
}

const handleDetectPath = async () => {
  const p = await systemStore.detectWindsurfPath()
  if (p) {
    local.windsurf_path = p
  }
}

const copyAppStoragePath = async () => {
  const p = systemStore.appStoragePath
  if (!p) return
  try {
    await navigator.clipboard.writeText(p)
    showToast('路径已复制', 'success')
  } catch {
    showToast('复制失败', 'error')
  }
}

const handleDesktopToolbarToggle = async (enabled: boolean) => {
  if (isToolbarSaving.value) {
    return
  }
  isToolbarSaving.value = true
  local.show_desktop_toolbar = enabled
  try {
    await persistLocalSettings()
    await nextTick()
    if (enabled) {
      await APIInfo.applyToolbarLayout(true)
    } else {
      await APIInfo.restoreMainWindowLayout()
    }
  } catch (e) {
    local.show_desktop_toolbar = Boolean(settingsStore.settings?.show_desktop_toolbar)
    console.error(e)
    showToast(`桌面工具栏切换失败: ${String(e)}`, 'error')
  } finally {
    isToolbarSaving.value = false
  }
}

onUnmounted(() => {
  if (autoSaveDebounceTimer) {
    clearTimeout(autoSaveDebounceTimer)
    autoSaveDebounceTimer = null
  }
  if (saveStateResetTimer) {
    clearTimeout(saveStateResetTimer)
    saveStateResetTimer = null
  }
})
</script>

<template>
  <div class="p-6 md:p-8 max-w-4xl mx-auto w-full pb-10">
    <div class="flex items-start justify-between mb-8 shrink-0 flex-wrap gap-4">
      <div>
        <h1 class="text-[32px] font-[800] text-gray-900 dark:text-gray-100 tracking-tight leading-none">高级设置</h1>
        <p class="text-[13px] text-gray-500 font-medium mt-3">
          全部设置自动保存；开关点击后立即生效，输入框在你停下后自动落盘
        </p>
      </div>
      <div
        class="inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold shadow-sm dark:border-white/[0.08] dark:bg-white/[0.05]"
        :class="{
          'text-ios-textSecondary dark:text-ios-textSecondaryDark': saveState === 'idle',
          'text-ios-blue': saveState === 'saving',
          'text-emerald-600 dark:text-emerald-300': saveState === 'saved',
          'text-rose-600 dark:text-rose-300': saveState === 'error',
        }"
      >
        <Loader2 v-if="saveState === 'saving'" class="w-4 h-4 ios-spinner" stroke-width="2.4" />
        <CheckCircle2 v-else-if="showSaved || saveState === 'saved'" class="w-4 h-4" stroke-width="2.4" />
        <Save v-else class="w-4 h-4" stroke-width="2.4" />
        <span>
          {{
            saveState === 'saving'
              ? '自动保存中'
              : showSaved || saveState === 'saved'
                ? '已自动保存'
                : saveState === 'error'
                  ? '保存失败'
                  : '自动保存'
          }}
        </span>
      </div>
    </div>

    <Transition name="fade" mode="out-in">
      <div
        v-if="settingsStore.isLoading"
        key="settings-loading"
        class="space-y-8 w-full"
      >
        <PageLoadingSkeleton variant="settings" />
      </div>

      <div v-else key="settings-content" class="space-y-8">
        
        <!-- 使用模式 -->
        <section>
          <h2 class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2">使用模式</h2>
          <div class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden">
            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex-1 pr-4">
                <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">仅 MITM 模式</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  日常无感换号以 MITM 为主；开启本项后关闭本机 windsurf_auth 与「用尽切号」，多号完全由 MITM 与号池轮换。号池仍用于导入密钥和额度同步。
                </div>
              </div>
              <IToggle v-model="local.mitm_only" class="shrink-0" />
            </div>
            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="flex-1 pr-4">
                <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">TUN / 全局代理说明</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  开启后在「MITM」面板显示与 TUN 模式（如 Clash / sing-box）并存时的注意事项提示。
                </div>
              </div>
              <IToggle v-model="local.mitm_tun_mode" class="shrink-0" />
            </div>
          </div>
        </section>

        <!-- 目录与界面行为 -->
        <section>
          <h2 class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2">环境与界面行为</h2>
          <div class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden flex flex-col">
            
            <div class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="mb-3 flex items-start gap-3">
                <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-ios-blue/10 text-ios-blue">
                  <FolderOpen class="h-5 w-5" stroke-width="2.4" />
                </div>
                <div class="min-w-0 flex-1">
                  <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">Windsurf 安装路径</div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                    用于检测与写入无感切号补丁。可自动探测或粘贴本机路径。
                  </div>
                </div>
              </div>
              <div class="flex gap-2">
                <input
                  v-model="local.windsurf_path"
                  type="text"
                  class="no-drag-region flex-1 bg-gray-50 dark:bg-black/20 border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[13px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                  placeholder="自动探测中..."
                />
                <button
                  type="button"
                  class="no-drag-region shrink-0 px-4 py-2.5 rounded-[12px] bg-ios-blue/10 text-ios-blue font-bold text-[13px] ios-btn hover:bg-ios-blue/15 transition-colors"
                  :disabled="systemStore.isGlobalLoading"
                  @click="handleDetectPath"
                >
                  自动检测
                </button>
              </div>
              <div
                v-if="systemStore.appStoragePath"
                class="mt-4 rounded-[14px] bg-gray-50 dark:bg-black/20 border border-black/[0.03] dark:border-white/[0.03] p-4 flex flex-col gap-2"
              >
                <div class="flex items-center justify-between">
                  <div class="text-[11px] font-bold text-gray-400 dark:text-gray-500 uppercase tracking-widest">号池配置目录</div>
                  <button
                    type="button"
                    class="no-drag-region text-[11px] font-bold text-ios-blue bg-ios-blue/10 px-2 py-1.5 rounded-md hover:bg-ios-blue/20 transition-colors"
                    @click="copyAppStoragePath"
                  >
                    复制路径
                  </button>
                </div>
                <div class="font-mono text-[12px] text-gray-700 dark:text-gray-300 break-all select-text bg-white dark:bg-[#1C1C1E] rounded-lg p-2 border border-black/5 dark:border-white/5">
                  {{ systemStore.appStoragePath }}
                </div>
              </div>
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex flex-1 items-start gap-3 pr-4">
                <div class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                  <RefreshCcw class="h-5 w-5" stroke-width="2.4" />
                </div>
                <div class="flex-1">
                  <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">写文件切号后自动重启IDE</div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                    仅针对写入本地 <code>windsurf_auth</code> 的切号动作。自动重启以让 IDE 读取最鲜 Auth 文件，确保额度更新（MITM 切号本来就免重启）。
                  </div>
                </div>
              </div>
              <IToggle v-model="local.restart_windsurf_after_switch" class="shrink-0" />
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex flex-1 items-start gap-3 pr-4">
                <div class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-slate-500/10 text-slate-600 dark:text-slate-300">
                  <Minimize2 class="h-5 w-5" stroke-width="2.4" />
                </div>
                <div class="flex-1">
                  <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">关闭时隐藏至系统托盘</div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                    开启后点击右上角关闭只会隐藏到托盘；关闭本项后点击关闭会真正退出，并自动恢复 MITM 的 hosts / ProxyOverride / Codeium 配置 / CA 环境。
                  </div>
                </div>
              </div>
              <IToggle v-model="local.minimize_to_tray" class="shrink-0" />
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex flex-1 items-start gap-3 pr-4">
                <div class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-violet-500/10 text-violet-600 dark:text-violet-300">
                  <Monitor class="h-5 w-5" stroke-width="2.4" />
                </div>
                <div class="flex-1">
                  <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">在桌面展示工具栏</div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                    将窗口缩小至屏幕右下角的小监控条（类似桌面小组件），展示存活与额度。切换后立即生效，无需点保存。
                  </div>
                </div>
              </div>
              <IToggle
                :model-value="local.show_desktop_toolbar"
                class="shrink-0"
                @update:model-value="handleDesktopToolbarToggle"
              />
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
              <div class="flex flex-1 items-start gap-3 pr-4">
                <div class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-amber-500/10 text-amber-600 dark:text-amber-300">
                  <MoonStar class="h-5 w-5" stroke-width="2.4" />
                </div>
                <div class="flex-1">
                  <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">静默启动</div>
                  <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                    启动时不弹主窗口直接挂载托盘后台。
                  </div>
                </div>
              </div>
              <IToggle v-model="local.silent_start" class="shrink-0" />
            </div>

          </div>
        </section>

        <!-- 网络代理 -->
        <section>
          <h2 class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2">网络代理</h2>
          <div class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden">
            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4" :class="{ 'border-b border-black/[0.04] dark:border-white/[0.04]': local.proxy_enabled }">
              <div class="flex-1 pr-4">
                <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">启用 HTTP 代理</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  登录、凭证刷新、额度同步请求通过此代理转发。
                </div>
              </div>
              <IToggle v-model="local.proxy_enabled" class="shrink-0" />
            </div>
            <div v-if="local.proxy_enabled" class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10">
              <input
                v-model="local.proxy_url"
                type="text"
                class="no-drag-region w-full bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-3 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                placeholder="http://127.0.0.1:7890"
              />
            </div>
          </div>
        </section>

        <!-- 保活与额度同步 -->
        <section>
          <h2 class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2">后台保活与额度同步</h2>
          <div class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden">
            
            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex-1 pr-4">
                <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">自动刷新 Token</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  后台定时为账号池自动续期 JWT。
                </div>
              </div>
              <IToggle v-model="local.auto_refresh_tokens" class="shrink-0" />
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex-1 pr-4">
                <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">定期同步额度</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  在后台定时从服务端核验最新可用配额，用于展示最新健康度。
                </div>
              </div>
              <IToggle v-model="local.auto_refresh_quotas" class="shrink-0" />
            </div>

            <div class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04] bg-gray-50/50 dark:bg-black/10" v-if="local.auto_refresh_quotas">
              <div class="flex flex-col gap-2 max-w-sm">
                <label class="text-[13px] font-bold text-gray-700 dark:text-gray-300">全局额度同步策略</label>
                <select
                  v-model="local.quota_refresh_policy"
                  class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/10 dark:border-white/10 rounded-[12px] px-3 py-2.5 text-[14px] outline-none focus:ring-2 focus:ring-ios-blue/30 font-medium"
                >
                  <option v-for="opt in quotaPolicyOptions" :key="opt.value" :value="opt.value">
                    {{ opt.label }}
                  </option>
                </select>
                <div v-if="local.quota_refresh_policy === 'custom'" class="pt-2">
                  <label class="text-[12px] text-gray-500 font-bold mb-1 block">自定义分钟（5~10080）</label>
                  <input
                    v-model.number="local.quota_custom_interval_minutes"
                    type="number" min="5" max="10080"
                    class="no-drag-region w-full bg-white dark:bg-[#1C1C1E] border border-black/10 dark:border-white/10 rounded-[12px] px-3 py-2.5 text-[14px] outline-none focus:ring-2"
                  />
                </div>
              </div>
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex-1 pr-4">
                <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">额度用尽自动切下席位</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  单独运行监控，仅紧盯正在使用的高频号。
                </div>
              </div>
              <IToggle v-model="local.auto_switch_on_quota_exhausted" :disabled="!local.auto_refresh_quotas" class="shrink-0" />
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]" v-if="local.auto_refresh_quotas && local.auto_switch_on_quota_exhausted">
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">当前存活席位监控频率</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  最小 5 秒。建议 15-30。越低越容易察觉到额度耗尽，发包压力越高。
                </div>
              </div>
              <div class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5">
                <input
                  v-model.number="local.quota_hot_poll_seconds"
                  type="number" min="5" max="60"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
                <span class="text-[13px] font-bold text-gray-400 ml-1">sec</span>
              </div>
            </div>

            <div class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]">
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">并发更新上限</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 flex items-center gap-2">
                  刷新配置时的请求并行度限制。
                </div>
              </div>
              <div class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5">
                <input
                  v-model.number="local.concurrent_limit"
                  type="number" min="1" max="50"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
              </div>
            </div>

            <div class="p-5 sm:p-6 bg-gray-50/30 dark:bg-black/10">
              <div class="mb-4">
                <div class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1">无感「下一席位」计划范围</div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium">
                  可勾选多个计划（例如同时 Trial + Pro），仅在所选池内轮换下一席位；与账号池分组一致。（AUTO_SWITCH_PLAN_FILTER）
                </div>
              </div>
              <div class="bg-white dark:bg-[#1C1C1E] rounded-2xl border border-black/[0.04] dark:border-white/[0.04] shadow-sm">
                <SwitchPlanFilterControl variant="default" v-model="local.auto_switch_plan_filter" :pool-counts="poolPlanCounts" />
              </div>
            </div>

          </div>
        </section>

      </div>
    </Transition>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.28s cubic-bezier(0.2, 0.8, 0.2, 1), transform 0.28s cubic-bezier(0.2, 0.8, 0.2, 1);
}
.fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}
.fade-leave-to {
  opacity: 0;
  transform: translateY(-3px);
}
</style>
