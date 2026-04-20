<script setup lang="ts">
import {
  computed,
  nextTick,
  onMounted,
  onUnmounted,
  reactive,
  ref,
  watch,
} from "vue";
import { useSettingsStore } from "../stores/useSettingsStore";
import IToggle from "../components/ios/IToggle.vue";
import {
  clampHotPollSeconds,
  clampQuotaMinutes,
  createDefaultSettings,
  formToSettings,
  normalizeSwitchPlanFilter,
  quotaPolicyOptions,
  settingsToForm,
  switchPlanFilterToneOptions,
  type SettingsForm,
} from "../utils/settingsModel";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton.vue";
import {
  CheckCircle2,
  Loader2,
  RefreshCcw,
  RotateCcw,
  Save,
  Radio,
} from "lucide-vue-next";
import { confirmDialog, showToast } from "../utils/toast";
import { APIInfo } from "../api/wails";

const settingsStore = useSettingsStore();
let autoSaveDebounceTimer: ReturnType<typeof setTimeout> | null = null;
let saveStateResetTimer: ReturnType<typeof setTimeout> | null = null;

const isSaving = ref(false);
const showSaved = ref(false);
const isSyncingLocal = ref(true);
const saveState = ref<"idle" | "saving" | "saved" | "error">("idle");
const lastSavedFingerprint = ref("");
const relayStatusLoaded = ref(false);
const local = reactive<SettingsForm>(settingsToForm(createDefaultSettings()));

// ── 套餐多选 checkbox helpers ──
const planFilterSet = computed(() => {
  const v = local.auto_switch_plan_filter;
  if (!v || v === 'all') return new Set<string>();
  return new Set(v.split(',').map((s) => s.trim()).filter(Boolean));
});
const planFilterActive = (tone: string) => {
  const s = planFilterSet.value;
  return s.size === 0 || s.has(tone);
};
const togglePlanFilter = (tone: string) => {
  const current = planFilterSet.value;
  const allTones = switchPlanFilterToneOptions.map((o) => o.value);
  if (current.size === 0) {
    // currently "all" → uncheck this one = select everything except this
    const next = allTones.filter((t) => t !== tone);
    local.auto_switch_plan_filter = normalizeSwitchPlanFilter(next.join(','));
  } else if (current.has(tone)) {
    current.delete(tone);
    local.auto_switch_plan_filter = normalizeSwitchPlanFilter([...current].join(',') || 'all');
  } else {
    current.add(tone);
    // if all selected → normalize to "all"
    local.auto_switch_plan_filter = normalizeSwitchPlanFilter([...current].join(','));
  }
};

onMounted(() => {
  void settingsStore.fetchSettings();
  void fetchRelayStatus();
});

watch(
  () => settingsStore.settings,
  (s) => {
    if (s) {
      isSyncingLocal.value = true;
      Object.assign(local, settingsToForm(s));
      lastSavedFingerprint.value = buildSettingsFingerprint();
      nextTick(() => {
        isSyncingLocal.value = false;
      });
    }
  },
  { immediate: true },
);

watch(
  () => ({
    ...local,
    quota_custom_interval_minutes: local.quota_custom_interval_minutes,
    quota_hot_poll_seconds: local.quota_hot_poll_seconds,
    concurrent_limit: local.concurrent_limit,
  }),
  () => {
    if (isSyncingLocal.value) {
      return;
    }
    scheduleAutoSave();
  },
  { deep: true },
);

const buildSettingsPayload = () => formToSettings(local);

const buildSettingsFingerprint = () => JSON.stringify(buildSettingsPayload());

const resetSavedStateLater = () => {
  if (saveStateResetTimer) {
    clearTimeout(saveStateResetTimer);
  }
  saveStateResetTimer = setTimeout(() => {
    if (saveState.value === "saved") {
      saveState.value = "idle";
      showSaved.value = false;
    }
  }, 1600);
};

const persistLocalSettings = async () => {
  const fingerprint = buildSettingsFingerprint();
  if (fingerprint === lastSavedFingerprint.value) {
    return;
  }
  isSaving.value = true;
  saveState.value = "saving";
  try {
    const payload = buildSettingsPayload();
    await settingsStore.updateSettings(payload);
    lastSavedFingerprint.value = fingerprint;
    saveState.value = "saved";
    showSaved.value = true;
    resetSavedStateLater();
  } catch (e) {
    saveState.value = "error";
    showToast(`自动保存失败: ${String(e)}`, "error");
  } finally {
    isSaving.value = false;
  }
};

const scheduleAutoSave = () => {
  if (autoSaveDebounceTimer) {
    clearTimeout(autoSaveDebounceTimer);
  }
  autoSaveDebounceTimer = setTimeout(() => {
    void persistLocalSettings();
  }, 420);
};

// ── OpenAI 中转 ──
const relayRunning = ref(false);
const relayLoading = ref(false);
const relayAddress = ref("");

const fetchRelayStatus = async () => {
  try {
    const st = await APIInfo.getOpenAIRelayStatus();
    relayRunning.value = Boolean(st.running);
    relayAddress.value = String(st.url || "");
  } catch {
    /* ignore */
  } finally {
    relayStatusLoaded.value = true;
  }
};

const handleRelayToggle = async (enabled: boolean) => {
  relayLoading.value = true;
  try {
    if (enabled) {
      await APIInfo.startOpenAIRelay(
        local.openai_relay_port || 8787,
        local.openai_relay_secret || "",
      );
      showToast("OpenAI 中转已启动", "success");
    } else {
      await APIInfo.stopOpenAIRelay();
      showToast("OpenAI 中转已停止", "success");
    }
    await fetchRelayStatus();
  } catch (e) {
    showToast(`中转操作失败: ${String(e)}`, "error");
  } finally {
    relayLoading.value = false;
  }
};

const copyRelayAddress = async () => {
  const addr =
    relayAddress.value || `http://127.0.0.1:${local.openai_relay_port || 8787}`;
  try {
    await navigator.clipboard.writeText(addr);
    showToast("地址已复制", "success");
  } catch {
    showToast("复制失败", "error");
  }
};

const relaySectionBooting = computed(() => !relayStatusLoaded.value);
const relaySectionRefreshing = computed(
  () => !relaySectionBooting.value && relayLoading.value,
);

onUnmounted(() => {
  if (autoSaveDebounceTimer) {
    clearTimeout(autoSaveDebounceTimer);
    autoSaveDebounceTimer = null;
    void persistLocalSettings();
  }
  if (saveStateResetTimer) {
    clearTimeout(saveStateResetTimer);
    saveStateResetTimer = null;
  }
});
</script>

<template>
  <div class="p-6 md:p-8 max-w-4xl mx-auto w-full pb-10">
    <div class="flex items-start justify-between mb-8 shrink-0 flex-wrap gap-4">
      <div>
        <h1
          class="text-[32px] font-[800] text-gray-900 dark:text-gray-100 tracking-tight leading-none"
        >
          MITM 设置
        </h1>
        <p class="text-[13px] text-gray-500 font-medium mt-3">
          纯 MITM 模式：号池轮换、MITM 代理与 OpenAI Relay；全部设置自动保存
        </p>
      </div>
      <div
        class="inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold shadow-sm dark:border-white/[0.08] dark:bg-white/[0.05]"
        :class="{
          'text-ios-textSecondary dark:text-ios-textSecondaryDark':
            saveState === 'idle',
          'text-ios-blue': saveState === 'saving',
          'text-emerald-600 dark:text-emerald-300': saveState === 'saved',
          'text-rose-600 dark:text-rose-300': saveState === 'error',
        }"
      >
        <Loader2
          v-if="saveState === 'saving'"
          class="w-4 h-4 ios-spinner"
          stroke-width="2.4"
        />
        <CheckCircle2
          v-else-if="showSaved || saveState === 'saved'"
          class="w-4 h-4"
          stroke-width="2.4"
        />
        <Save v-else class="w-4 h-4" stroke-width="2.4" />
        <span>
          {{
            saveState === "saving"
              ? "自动保存中"
              : showSaved || saveState === "saved"
                ? "已自动保存"
                : saveState === "error"
                  ? "保存失败"
                  : "自动保存"
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
          <h2
            class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2"
          >
            使用模式
          </h2>
          <div
            class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
          >
            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  纯 MITM 模式
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  当前版本已经固定为纯 MITM 工作流：所有轮换都从号池、MITM
                  代理和 Relay 走，界面只保留这条主链路相关设置。
                </div>
              </div>
              <IToggle :model-value="true" :disabled="true" class="shrink-0" />
            </div>
            <div
              class="p-5 sm:p-6 bg-ios-blue/[0.05] dark:bg-ios-blue/[0.1] border-t border-black/[0.04] dark:border-white/[0.04]"
            >
              <div
                class="text-[14px] font-bold text-gray-900 dark:text-gray-100 mb-1"
              >
                Windows 默认以管理员权限启动
              </div>
              <div
                class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
              >
                Windows 版桌面包会在启动时直接申请管理员权限，这样 Hosts、CA
                证书、系统服务和代理相关动作都能一次完成，不需要进程起来后再补提权。
              </div>
            </div>
          </div>
        </section>

        <!-- OpenAI 中转 -->
        <section>
          <h2
            class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2"
          >
            OpenAI 协议中转
          </h2>
          <div
            v-if="relaySectionBooting"
            class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
            aria-busy="true"
            aria-label="Relay 状态加载中"
          >
            <div
              class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div
                class="flex flex-col sm:flex-row sm:items-center justify-between gap-4"
              >
                <div class="min-w-0 flex-1 space-y-3">
                  <SkeletonBlock class="h-5 w-40 rounded-lg" />
                  <SkeletonBlock class="h-4 w-[74%] rounded-lg" />
                </div>
                <SkeletonBlock class="h-10 w-24 rounded-[12px] shrink-0" />
              </div>
            </div>
            <div class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10 space-y-4">
              <div class="flex flex-col sm:flex-row gap-4">
                <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
                <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
              </div>
              <SkeletonBlock class="h-14 w-full rounded-[14px]" />
              <SkeletonBlock class="h-4 w-[70%] rounded-md" />
            </div>
          </div>

          <SkeletonOverlay
            v-else
            :active="relaySectionRefreshing"
            label="Relay 配置刷新中"
            overlayClass="rounded-[24px] bg-white/45 backdrop-blur-[2px] dark:bg-[#1C1C1E]/45"
          >
            <div
              class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
            >
              <div
                class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
              >
                <div class="flex-1 pr-4">
                  <div class="flex items-center gap-2">
                    <div
                      class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                    >
                      启用中转服务器
                    </div>
                    <span
                      class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                      :class="
                        relayRunning
                          ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                          : 'bg-slate-500/10 text-slate-700 dark:text-slate-300'
                      "
                    >
                      {{ relayRunning ? "运行中" : "已停止" }}
                    </span>
                  </div>
                  <div
                    class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                  >
                    在本地启动 OpenAI 兼容的 HTTP API，将
                    <code>/v1/chat/completions</code> 请求转发到 Windsurf
                    Cascade，自动从号池轮换账号。
                  </div>
                </div>
                <button
                  type="button"
                  class="no-drag-region shrink-0 px-5 py-2.5 rounded-[12px] font-bold text-[13px] ios-btn transition-colors disabled:opacity-50"
                  :class="
                    relayRunning
                      ? 'bg-rose-500/10 text-rose-700 dark:text-rose-300 hover:bg-rose-500/15'
                      : 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-500/15'
                  "
                  :disabled="relayLoading"
                  @click="handleRelayToggle(!relayRunning)"
                >
                  <span class="inline-flex items-center gap-2">
                    <Radio class="w-4 h-4" stroke-width="2.4" />
                    {{ relayRunning ? "停止" : "启动" }}
                  </span>
                </button>
              </div>
              <div class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10 space-y-4">
                <div class="flex flex-col sm:flex-row gap-4">
                  <div class="flex-1 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >监听端口</label
                    >
                    <input
                      v-model.number="local.openai_relay_port"
                      type="number"
                      min="1"
                      max="65535"
                      class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="8787"
                    />
                  </div>
                  <div class="flex-1 flex flex-col gap-1.5">
                    <label
                      class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                      >Bearer 密钥（留空不鉴权）</label
                    >
                    <input
                      v-model="local.openai_relay_secret"
                      type="text"
                      class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/5 dark:border-white/5 px-4 py-2.5 rounded-[12px] font-mono text-[14px] focus:ring-2 focus:ring-ios-blue/30 outline-none transition-shadow"
                      placeholder="sk-your-secret"
                    />
                  </div>
                </div>
                <div
                  v-if="relayRunning"
                  class="flex items-center gap-3 rounded-[14px] border border-emerald-500/20 bg-emerald-500/10 px-3.5 py-3"
                >
                  <div
                    class="text-[12px] font-medium text-emerald-700 dark:text-emerald-300 flex-1"
                  >
                    API 地址：<code class="font-mono">{{
                      relayAddress ||
                      `http://127.0.0.1:${local.openai_relay_port || 8787}`
                    }}</code>
                  </div>
                  <button
                    type="button"
                    class="no-drag-region shrink-0 rounded-full bg-emerald-600/20 px-2.5 py-1 text-[10px] font-bold text-emerald-700 dark:text-emerald-300 hover:bg-emerald-600/30 transition-colors"
                    @click="copyRelayAddress"
                  >
                    复制
                  </button>
                </div>
                <div
                  class="text-[12px] text-gray-400 dark:text-gray-500 leading-relaxed"
                >
                  兼容所有 OpenAI SDK / ChatGPT 客户端。设置
                  <code>base_url</code> 为上面的地址即可。流式和非流式均支持。
                </div>
              </div>
            </div>
            <template #skeleton>
              <div
                class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
              >
                <div
                  class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04]"
                >
                  <div
                    class="flex flex-col sm:flex-row sm:items-center justify-between gap-4"
                  >
                    <div class="min-w-0 flex-1 space-y-3">
                      <SkeletonBlock class="h-5 w-40 rounded-lg" />
                      <SkeletonBlock class="h-4 w-[74%] rounded-lg" />
                    </div>
                    <SkeletonBlock class="h-10 w-24 rounded-[12px] shrink-0" />
                  </div>
                </div>
                <div
                  class="p-5 sm:p-6 bg-gray-50/50 dark:bg-black/10 space-y-4"
                >
                  <div class="flex flex-col sm:flex-row gap-4">
                    <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
                    <SkeletonBlock class="h-11 flex-1 rounded-[12px]" />
                  </div>
                  <SkeletonBlock class="h-14 w-full rounded-[14px]" />
                  <SkeletonBlock class="h-4 w-[70%] rounded-md" />
                </div>
              </div>
            </template>
          </SkeletonOverlay>
        </section>

        <!-- 保活与额度同步 -->
        <section>
          <h2
            class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2"
          >
            后台保活与额度同步
          </h2>
          <div
            class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden"
          >
            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  自动刷新 Token
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  后台定时为账号池自动续期 JWT。
                </div>
              </div>
              <IToggle v-model="local.auto_refresh_tokens" class="shrink-0" />
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  定期同步额度
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  在后台定时从服务端核验最新可用配额，用于展示最新健康度。
                </div>
              </div>
              <IToggle v-model="local.auto_refresh_quotas" class="shrink-0" />
            </div>

            <div
              class="p-5 sm:p-6 border-b border-black/[0.04] dark:border-white/[0.04] bg-gray-50/50 dark:bg-black/10"
              v-if="local.auto_refresh_quotas"
            >
              <div class="flex flex-col gap-2 max-w-sm">
                <label
                  class="text-[13px] font-bold text-gray-700 dark:text-gray-300"
                  >全局额度同步策略</label
                >
                <select
                  v-model="local.quota_refresh_policy"
                  class="no-drag-region bg-white dark:bg-[#1C1C1E] border border-black/10 dark:border-white/10 rounded-[12px] px-3 py-2.5 text-[14px] outline-none focus:ring-2 focus:ring-ios-blue/30 font-medium"
                >
                  <option
                    v-for="opt in quotaPolicyOptions"
                    :key="opt.value"
                    :value="opt.value"
                  >
                    {{ opt.label }}
                  </option>
                </select>
                <div
                  v-if="local.quota_refresh_policy === 'custom'"
                  class="pt-2"
                >
                  <label class="text-[12px] text-gray-500 font-bold mb-1 block"
                    >自定义分钟（5~10080）</label
                  >
                  <input
                    v-model.number="local.quota_custom_interval_minutes"
                    type="number"
                    min="5"
                    max="10080"
                    class="no-drag-region w-full bg-white dark:bg-[#1C1C1E] border border-black/10 dark:border-white/10 rounded-[12px] px-3 py-2.5 text-[14px] outline-none focus:ring-2"
                  />
                </div>
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[16px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  额度用尽自动切下席位
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  单独运行监控，仅紧盯正在使用的高频号。
                </div>
              </div>
              <IToggle
                v-model="local.auto_switch_on_quota_exhausted"
                :disabled="!local.auto_refresh_quotas"
                class="shrink-0"
              />
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
              v-if="
                local.auto_refresh_quotas &&
                local.auto_switch_on_quota_exhausted
              "
            >
              <div>
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  自动切号套餐范围
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  勾选允许自动轮换到哪些套餐类型，全选或不选等同于「不限制」。
                </div>
              </div>
              <div class="flex flex-wrap gap-2">
                <label
                  v-for="opt in switchPlanFilterToneOptions"
                  :key="opt.value"
                  @click.prevent="togglePlanFilter(opt.value)"
                  class="no-drag-region inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border text-[13px] font-semibold cursor-pointer select-none transition-all duration-150"
                  :class="planFilterActive(opt.value)
                    ? 'bg-ios-blue/10 dark:bg-ios-blue/20 border-ios-blue/40 text-ios-blue shadow-sm'
                    : 'bg-gray-100 dark:bg-white/5 border-black/5 dark:border-white/10 text-gray-500 dark:text-gray-400 hover:bg-gray-200/70 dark:hover:bg-white/10'"
                >
                  <span
                    class="w-3.5 h-3.5 rounded-[4px] border-2 flex items-center justify-center transition-colors"
                    :class="planFilterActive(opt.value)
                      ? 'border-ios-blue bg-ios-blue'
                      : 'border-gray-300 dark:border-gray-600'"
                  >
                    <svg v-if="planFilterActive(opt.value)" class="w-2.5 h-2.5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="3.5"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>
                  </span>
                  {{ opt.label }}
                </label>
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
              v-if="
                local.auto_refresh_quotas &&
                local.auto_switch_on_quota_exhausted
              "
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  当前存活席位监控频率
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 leading-relaxed font-medium"
                >
                  最小 5 秒。建议
                  15-30。越低越容易察觉到额度耗尽，发包压力越高。
                </div>
              </div>
              <div
                class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5"
              >
                <input
                  v-model.number="local.quota_hot_poll_seconds"
                  type="number"
                  min="5"
                  max="60"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
                <span class="text-[13px] font-bold text-gray-400 ml-1"
                  >sec</span
                >
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  并发更新上限
                </div>
                <div
                  class="text-[13px] text-gray-500 dark:text-gray-400 flex items-center gap-2"
                >
                  JWT
                  与额度同步会按批次推进，这里控制每一批的并发上限，避免一次性把整个号池打满。
                </div>
              </div>
              <div
                class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5"
              >
                <input
                  v-model.number="local.concurrent_limit"
                  type="number"
                  min="1"
                  max="50"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  导入并发数
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  批量导入账号时的最大并发数（1～20），值越大导入越快但更容易触发上游限流。
                </div>
              </div>
              <div
                class="relative shrink-0 flex items-center bg-gray-100 dark:bg-black/20 rounded-[12px] px-3 py-1.5 focus-within:ring-2 focus-within:ring-ios-blue/30 border border-black/5 dark:border-white/5"
              >
                <input
                  v-model.number="local.import_concurrency"
                  type="number"
                  min="1"
                  max="20"
                  class="no-drag-region w-14 text-center bg-transparent border-none text-[15px] font-bold text-gray-900 dark:text-gray-100 outline-none p-0"
                />
              </div>
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div
                  class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1"
                >
                  调试日志
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  开启后将代理、轮换、额度判定等关键操作写入 debug.log 文件。
                </div>
              </div>
              <IToggle v-model="local.debug_log" />
            </div>
          </div>
        </section>

        <!-- 高级抓包与伪造专区 -->
        <section>
          <h2 class="text-[13px] font-bold text-gray-500 dark:text-gray-400 uppercase tracking-widest mb-3 px-2">
            高级抓包与诊断配置
          </h2>
          <div class="bg-white/70 dark:bg-[#1C1C1E]/70 ios-glass rounded-[24px] border border-black/[0.04] dark:border-white/[0.04] shadow-[0_2px_12px_rgba(0,0,0,0.02)] overflow-hidden">
            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                  全量离线抓包 (Full Capture)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  记录代理过程中所有会话日志并落盘存入 <code>capture/</code> 目录下（JSONL 序列化）。
                </div>
              </div>
              <IToggle v-model="local.mitm_full_capture" />
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                  Protobuf 深度解包 (Debug Dump)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  开启后将在底层将特权结构体与未知节点 dump 至 <code>proto_dumps/</code> 以供二次逆向研究。
                </div>
              </div>
              <IToggle v-model="local.mitm_debug_dump" />
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 border-b border-black/[0.04] dark:border-white/[0.04]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1">
                  静态资源高速缓存拦截 (Cache Intercept)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  内置直返 Codeium Bin 预构建离线缓存，减少跨域拉取耗时。
                </div>
              </div>
              <IToggle v-model="local.static_cache_intercept" />
            </div>

            <div
              class="p-5 sm:p-6 flex items-center justify-between gap-4 bg-amber-500/[0.03]"
            >
              <div class="flex-1 pr-4">
                <div class="text-[15px] font-bold text-gray-900 dark:text-gray-100 mb-1 flex items-center gap-2">
                  <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span> GetUserStatus伪装 (Forge)
                </div>
                <div class="text-[13px] text-gray-500 dark:text-gray-400">
                  强制劫盖响应，伪造为企业版无限额度状态（可能导致服务端反爬锁号，谨慎使用）。
                </div>
              </div>
              <IToggle v-model="local.forge_enabled" />
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
  transition:
    opacity 0.28s cubic-bezier(0.2, 0.8, 0.2, 1),
    transform 0.28s cubic-bezier(0.2, 0.8, 0.2, 1);
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
