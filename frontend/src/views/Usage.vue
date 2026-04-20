<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { APIInfo } from "../api/wails";
import { confirmDialog, showToast } from "../utils/toast";
import { useMainViewStore } from "../stores/useMainViewStore";
import {
  Activity,
  ArrowRightLeft,
  Box,
  CheckCircle2,
  Clock,
  Layers,
  RefreshCw,
  Search,
  Trash2,
  XCircle,
} from "lucide-vue-next";
import type { Models } from "../api/wails";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton.vue";
import ISegmented from "../components/ios/ISegmented.vue";

const POLL_INTERVAL_MS = 5000;
const RECORDS_REFRESH_MS = 20000;
const RECORD_LIMIT = 5000;
const MODEL_BREAKDOWN_LIMIT = 6;
const STATUS_FILTER_OPTIONS = [
  { label: "全部", value: "all" },
  { label: "成功", value: "ok" },
  { label: "错误", value: "error" },
];

const mainView = useMainViewStore();
const loading = ref(true);
const refreshing = ref(false);
const summary = ref<Models.services.UsageSummary | null>(null);
const records = ref<Models.services.UsageRecord[]>([]);
const selectedDate = ref<string | null>(null);
const statusFilter = ref("all");
const modelFilter = ref("all");
const searchQuery = ref("");
const currentPage = ref(1);
const pageSize = ref(100);

let pollTimer: ReturnType<typeof setTimeout> | null = null;
let summaryFetchInFlight: Promise<void> | null = null;
let recordsFetchInFlight: Promise<void> | null = null;
let lastSummaryFetchedAt = 0;
let lastRecordsFetchedAt = 0;

const estimatedCost = computed(() =>
  (summary.value?.estimated_cost_usd || 0).toFixed(2),
);
const totalRecordCount = computed(() => summary.value?.total_requests || 0);
const successCount = computed(() =>
  Math.max(0, totalRecordCount.value - (summary.value?.error_count || 0)),
);
const successRate = computed(() => {
  if (!totalRecordCount.value) {
    return 0;
  }
  return (successCount.value / totalRecordCount.value) * 100;
});
const errorRate = computed(() => {
  if (!totalRecordCount.value) {
    return 0;
  }
  return ((summary.value?.error_count || 0) / totalRecordCount.value) * 100;
});

const modelOptions = computed(() => {
  const byModel = summary.value?.by_model || {};
  const byTokens = summary.value?.by_model_tokens || {};
  return Object.keys(byModel).sort((left, right) => {
    const tokenDelta = (byTokens[right] || 0) - (byTokens[left] || 0);
    if (tokenDelta !== 0) {
      return tokenDelta;
    }
    return (byModel[right] || 0) - (byModel[left] || 0);
  });
});

const topModels = computed(() => {
  const byModel = summary.value?.by_model || {};
  const byTokens = summary.value?.by_model_tokens || {};
  const total = totalRecordCount.value || 1;
  return modelOptions.value.slice(0, MODEL_BREAKDOWN_LIMIT).map((model) => ({
    model,
    requests: byModel[model] || 0,
    tokens: byTokens[model] || 0,
    share: ((byModel[model] || 0) / total) * 100,
  }));
});

const dailyDates = computed(() =>
  Object.keys(summary.value?.by_date || {}).sort((left, right) =>
    right.localeCompare(left),
  ),
);

const activeFilterLabel = computed(() => {
  const parts: string[] = [];
  if (selectedDate.value) {
    parts.push(`日期 ${selectedDate.value}`);
  }
  if (statusFilter.value !== "all") {
    parts.push(statusFilter.value === "ok" ? "仅成功" : "仅错误");
  }
  if (modelFilter.value !== "all") {
    parts.push(modelFilter.value);
  }
  if (searchQuery.value.trim()) {
    parts.push(`搜索 "${searchQuery.value.trim()}"`);
  }
  return parts.length ? parts.join(" · ") : "全部记录";
});

const visibleRecordHint = computed(() => {
  const loaded = formatNumber(records.value.length);
  const total = formatNumber(totalRecordCount.value);
  if (!records.value.length) {
    return "尚未加载到调用记录";
  }
  if (totalRecordCount.value > records.value.length) {
    return `已加载最近 ${loaded} 条，累计 ${total} 条`;
  }
  return `累计 ${total} 条记录`;
});

const filteredRecords = computed(() => {
  const dateFilter = selectedDate.value;
  const status = statusFilter.value;
  const model = modelFilter.value;
  const query = searchQuery.value.trim().toLowerCase();

  return records.value.filter((rec) => {
    if (dateFilter && (!rec.at || !rec.at.startsWith(dateFilter))) {
      return false;
    }
    if (status !== "all" && rec.status !== status) {
      return false;
    }
    const recordModel = rec.model || rec.request_model || "unknown";
    if (model !== "all" && recordModel !== model) {
      return false;
    }
    if (!query) {
      return true;
    }
    const haystack = [
      rec.model,
      rec.request_model,
      rec.api_key_short,
      rec.status,
      rec.error_detail,
      rec.format,
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return haystack.includes(query);
  });
});

const totalPages = computed(() =>
  Math.max(1, Math.ceil(filteredRecords.value.length / pageSize.value)),
);

const paginatedRecords = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  return filteredRecords.value.slice(start, start + pageSize.value);
});

const emptyStateMessage = computed(() => {
  if (!records.value.length) {
    return "暂无调用记录，开始对话后将在此实时展示。";
  }
  return "当前筛选条件下暂无记录。";
});

watch([selectedDate, statusFilter, modelFilter, searchQuery], () => {
  currentPage.value = 1;
});

watch(totalPages, (nextPageCount) => {
  if (currentPage.value > nextPageCount) {
    currentPage.value = nextPageCount;
  }
});

const clearPollTimer = () => {
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }
};

const shouldPoll = () => {
  if (mainView.activeTab !== "Usage") {
    return false;
  }
  if (typeof document !== "undefined" && document.visibilityState !== "visible") {
    return false;
  }
  return true;
};

const shouldRefreshRecords = (force = false) =>
  force ||
  records.value.length === 0 ||
  Date.now() - lastRecordsFetchedAt >= RECORDS_REFRESH_MS;

const fetchSummary = async (force = false) => {
  if (summaryFetchInFlight) {
    return summaryFetchInFlight;
  }
  if (!force && Date.now() - lastSummaryFetchedAt < 2500) {
    return;
  }
  summaryFetchInFlight = (async () => {
    summary.value = await APIInfo.getUsageSummary();
    lastSummaryFetchedAt = Date.now();
  })();
  try {
    await summaryFetchInFlight;
  } finally {
    summaryFetchInFlight = null;
  }
};

const fetchRecords = async (force = false) => {
  if (recordsFetchInFlight) {
    return recordsFetchInFlight;
  }
  if (!shouldRefreshRecords(force)) {
    return;
  }
  recordsFetchInFlight = (async () => {
    records.value = (await APIInfo.getUsageRecords(RECORD_LIMIT)) || [];
    lastRecordsFetchedAt = Date.now();
  })();
  try {
    await recordsFetchInFlight;
  } finally {
    recordsFetchInFlight = null;
  }
};

const scheduleNextPoll = () => {
  clearPollTimer();
  if (!shouldPoll()) {
    return;
  }
  pollTimer = setTimeout(() => {
    void fetchUsageData({ silent: true }).finally(scheduleNextPoll);
  }, POLL_INTERVAL_MS);
};

const fetchUsageData = async (options?: {
  silent?: boolean;
  forceSummary?: boolean;
  forceRecords?: boolean;
}) => {
  const silent = options?.silent ?? false;
  const forceSummary = options?.forceSummary ?? false;
  const forceRecords = options?.forceRecords ?? false;

  if (!silent) {
    loading.value = true;
  } else {
    refreshing.value = true;
  }

  try {
    await Promise.all([
      fetchSummary(forceSummary),
      forceRecords || shouldRefreshRecords() ? fetchRecords(forceRecords) : Promise.resolve(),
    ]);
  } catch (e: any) {
    if (!silent) {
      showToast(`获取用量数据失败: ${String(e)}`, "error");
    } else {
      console.error("Silent usage refresh failed:", e);
    }
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
};

const handleRefresh = async () => {
  await fetchUsageData({
    silent: true,
    forceSummary: true,
    forceRecords: true,
  });
  showToast("用量统计已刷新", "success");
};

const handleClear = async () => {
  const ok = await confirmDialog("确认清空所有用量记录？", {
    confirmText: "清空",
    destructive: true,
  });
  if (!ok) {
    return;
  }

  try {
    const deletedCount = await APIInfo.deleteAllUsage();
    await fetchUsageData({
      forceSummary: true,
      forceRecords: true,
    });
    showToast(`已清空 ${deletedCount} 条用量记录`, "success");
  } catch (e: any) {
    showToast(`清空记录失败: ${String(e)}`, "error");
  }
};

const resumePolling = (forceRefresh = false) => {
  if (!shouldPoll()) {
    clearPollTimer();
    return;
  }
  void fetchUsageData({
    silent: true,
    forceSummary: forceRefresh,
    forceRecords: forceRefresh,
  }).finally(scheduleNextPoll);
};

const onVisibilityChange = () => {
  if (typeof document === "undefined") {
    return;
  }
  if (document.visibilityState === "visible") {
    resumePolling();
    return;
  }
  clearPollTimer();
};

watch(
  () => mainView.activeTab,
  (tab) => {
    if (tab === "Usage") {
      resumePolling();
      return;
    }
    clearPollTimer();
  },
);

onMounted(() => {
  void fetchUsageData({
    forceSummary: true,
    forceRecords: true,
  }).finally(scheduleNextPoll);
  document.addEventListener("visibilitychange", onVisibilityChange);
});

onUnmounted(() => {
  clearPollTimer();
  document.removeEventListener("visibilitychange", onVisibilityChange);
});

const formatDate = (dateStr: string) => {
  if (!dateStr) return "-";
  return new Date(dateStr).toLocaleString();
};

const formatNumber = (num: number) =>
  new Intl.NumberFormat("en-US").format(num || 0);

const formatCompactToken = (num: number) => {
  if (!num) return "0";
  if (num >= 1000000) {
    return new Intl.NumberFormat("en-US", {
      notation: "compact",
      maximumFractionDigits: 2,
    }).format(num);
  }
  return new Intl.NumberFormat("en-US").format(num);
};

const formatDuration = (ms: number) => {
  if (!ms) return "-";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
};

const formatPercent = (value: number) => `${value.toFixed(value >= 10 ? 1 : 2)}%`;
</script>

<template>
  <div class="p-6 md:p-8 max-w-5xl mx-auto w-full pb-10">
    <div class="flex items-start justify-between mb-8 shrink-0 flex-wrap gap-4">
      <div>
        <h1
          class="text-[32px] font-[800] text-gray-900 dark:text-gray-100 tracking-tight leading-none flex items-center gap-3"
        >
          <Activity class="w-8 h-8 text-ios-blue" stroke-width="2.4" />
          用量统计
        </h1>
        <p class="text-[13px] text-gray-500 font-medium mt-3">
          实时监控底层 MITM 代理与 OpenAI Relay 的全量请求日志与 Token 消耗流水。
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="no-drag-region flex items-center gap-1.5 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark"
          :disabled="refreshing || loading"
          @click="handleRefresh"
        >
          <RefreshCw
            class="h-4 w-4"
            :class="refreshing ? 'animate-spin' : ''"
            stroke-width="2.4"
          />
          刷新
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center gap-1.5 rounded-full border border-rose-500/15 bg-rose-500/[0.06] px-4 py-2 text-[12px] font-semibold text-rose-600 shadow-sm transition-all ios-btn hover:bg-rose-500/[0.1] dark:text-rose-400"
          @click="handleClear"
        >
          <Trash2 class="h-4 w-4" stroke-width="2.4" />
          清空数据
        </button>
      </div>
    </div>

    <PageLoadingSkeleton v-if="loading" variant="usage" class="w-full" />

    <div v-else class="space-y-6">
      <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <div class="rounded-[24px] border border-black/[0.04] bg-white/70 p-5 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-gray-500 dark:text-gray-400 mb-2">
            <ArrowRightLeft class="w-4 h-4" stroke-width="2.4" /> 请求总数
          </div>
          <div
            class="text-[28px] lg:text-[32px] font-extrabold text-gray-900 dark:text-gray-100 truncate"
            :title="formatNumber(summary?.total_requests || 0)"
          >
            {{ formatCompactToken(summary?.total_requests || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-gray-500 font-medium">包含出错与未完成</div>
        </div>

        <div class="rounded-[24px] border border-blue-500/15 bg-blue-500/[0.04] p-5 shadow-sm ios-glass">
          <div class="flex items-center justify-between mb-2">
            <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-blue-600 dark:text-blue-400">
              <Box class="w-4 h-4" stroke-width="2.4" /> 总计 Tokens
            </div>
            <div class="bg-blue-500/10 text-blue-700 dark:bg-blue-500/20 dark:text-blue-300 px-2 py-0.5 rounded text-[10px] font-bold">
              等效约 ${{ estimatedCost }}
            </div>
          </div>
          <div
            class="text-[28px] lg:text-[32px] font-extrabold text-blue-700 dark:text-blue-300 truncate"
            :title="formatNumber(summary?.total_tokens || 0)"
          >
            {{ formatCompactToken(summary?.total_tokens || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-blue-600/70 dark:text-blue-400/70 font-medium">
            Prompt + Completion
          </div>
        </div>

        <div class="rounded-[24px] border border-violet-500/15 bg-violet-500/[0.04] p-5 shadow-sm ios-glass relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-violet-600 dark:text-violet-400 mb-2">
            <Layers class="w-4 h-4" stroke-width="2.4" /> Prompt Tokens
          </div>
          <div
            class="text-[28px] lg:text-[32px] font-extrabold text-violet-700 dark:text-violet-300 truncate"
            :title="formatNumber(summary?.total_prompt_tokens || 0)"
          >
            {{ formatCompactToken(summary?.total_prompt_tokens || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-violet-600/70 dark:text-violet-400/70 font-medium">
            上行请求用量
          </div>
        </div>

        <div class="rounded-[24px] border border-emerald-500/15 bg-emerald-500/[0.04] p-5 shadow-sm ios-glass relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-emerald-600 dark:text-emerald-400 mb-2">
            <Layers class="w-4 h-4" stroke-width="2.4" /> Completion Tokens
          </div>
          <div
            class="text-[28px] lg:text-[32px] font-extrabold text-emerald-700 dark:text-emerald-300 truncate"
            :title="formatNumber(summary?.total_completion_tokens || 0)"
          >
            {{ formatCompactToken(summary?.total_completion_tokens || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-emerald-600/70 dark:text-emerald-400/70 font-medium">
            下行生成用量
          </div>
        </div>

        <div class="rounded-[24px] border border-emerald-500/15 bg-emerald-500/[0.04] p-5 shadow-sm ios-glass relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-emerald-600 dark:text-emerald-400 mb-2">
            <CheckCircle2 class="w-4 h-4" stroke-width="2.4" /> 成功率
          </div>
          <div class="text-[28px] lg:text-[32px] font-extrabold text-emerald-700 dark:text-emerald-300 truncate">
            {{ formatPercent(successRate) }}
          </div>
          <div class="mt-1 text-[12px] text-emerald-600/70 dark:text-emerald-400/70 font-medium">
            成功 {{ formatNumber(successCount) }} / 总计
            {{ formatNumber(totalRecordCount) }}
          </div>
        </div>

        <div class="rounded-[24px] border border-rose-500/15 bg-rose-500/[0.04] p-5 shadow-sm ios-glass relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-rose-600 dark:text-rose-400 mb-2">
            <XCircle class="w-4 h-4" stroke-width="2.4" /> 错误请求
          </div>
          <div class="text-[28px] lg:text-[32px] font-extrabold text-rose-700 dark:text-rose-300 truncate">
            {{ formatNumber(summary?.error_count || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-rose-600/70 dark:text-rose-400/70 font-medium">
            错误率 {{ formatPercent(errorRate) }}
          </div>
        </div>
      </div>

      <div class="grid grid-cols-1 xl:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)] gap-4">
        <div
          v-if="summary && summary.by_date && dailyDates.length > 0"
          class="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden"
        >
          <div class="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between">
            <h2 class="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
              <Clock class="w-4 h-4 text-ios-blue" stroke-width="2.4" /> 每日用量统计
            </h2>
            <span class="text-[12px] font-medium text-gray-500">点击行即可联动筛选明细</span>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-[13px]">
              <thead class="bg-blue-50/30 dark:bg-blue-900/10 text-gray-500 dark:text-gray-400 text-[11px] uppercase tracking-wider font-bold">
                <tr>
                  <th class="px-6 py-3">日期</th>
                  <th class="px-6 py-3 text-right">调用次数</th>
                  <th class="px-6 py-3 text-right">消耗 Tokens</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-black/[0.04] dark:divide-white/[0.04]">
                <tr
                  v-for="date in dailyDates"
                  :key="date"
                  class="transition-colors cursor-pointer"
                  :class="selectedDate === date ? 'bg-ios-blue/[0.08] dark:bg-ios-blue/[0.15]' : 'hover:bg-black/[0.02] dark:hover:bg-white/[0.02]'"
                  @click="selectedDate = selectedDate === date ? null : date"
                >
                  <td
                    class="px-6 py-3 whitespace-nowrap font-mono font-semibold"
                    :class="selectedDate === date ? 'text-ios-blue' : 'text-gray-800 dark:text-gray-200'"
                  >
                    {{ date }}
                    <span
                      v-if="selectedDate === date"
                      class="ml-2 text-[10px] bg-ios-blue text-white rounded px-1.5 py-0.5"
                    >
                      筛选中
                    </span>
                  </td>
                  <td
                    class="px-6 py-3 whitespace-nowrap text-right font-mono"
                    :class="selectedDate === date ? 'text-ios-blue' : 'text-gray-600 dark:text-gray-300'"
                  >
                    {{ formatNumber(summary.by_date[date]) }}
                  </td>
                  <td class="px-6 py-3 whitespace-nowrap text-right font-mono font-bold text-ios-blue">
                    {{ formatNumber(summary.by_date_tokens[date] || 0) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div class="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden">
          <div class="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between gap-3">
            <div>
              <h2 class="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
                <Box class="w-4 h-4 text-gray-500" stroke-width="2.4" /> 模型分布
              </h2>
              <p class="mt-1 text-[12px] font-medium text-gray-500">
                按请求量展示最常用模型与累计 Tokens。
              </p>
            </div>
            <span class="text-[12px] font-semibold text-gray-400">
              {{ formatNumber(modelOptions.length) }} 个模型
            </span>
          </div>
          <div v-if="topModels.length" class="px-6 py-5 space-y-4">
            <div
              v-for="entry in topModels"
              :key="entry.model"
              class="rounded-[20px] border border-black/[0.04] bg-black/[0.02] p-4 dark:border-white/[0.04] dark:bg-white/[0.03]"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="truncate font-mono text-[12px] font-bold text-gray-800 dark:text-gray-100">
                    {{ entry.model }}
                  </div>
                  <div class="mt-1 text-[12px] text-gray-500">
                    {{ formatNumber(entry.requests) }} 次请求 ·
                    {{ formatCompactToken(entry.tokens) }} Tokens
                  </div>
                </div>
                <div class="text-[12px] font-bold text-ios-blue">
                  {{ formatPercent(entry.share) }}
                </div>
              </div>
              <div class="mt-3 h-2 rounded-full bg-black/[0.06] dark:bg-white/[0.08] overflow-hidden">
                <div
                  class="h-full rounded-full bg-gradient-to-r from-ios-blue to-cyan-400"
                  :style="{ width: `${Math.min(100, Math.max(entry.share, 3))}%` }"
                />
              </div>
            </div>
          </div>
          <div v-else class="px-6 py-10 text-center text-[13px] text-gray-500">
            暂无模型分布数据。
          </div>
        </div>
      </div>

      <div class="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden">
        <div class="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04] flex items-start justify-between gap-4 flex-wrap">
          <div>
            <h2 class="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
              <Clock class="w-4 h-4 text-gray-500" stroke-width="2.4" />
              {{ selectedDate ? `${selectedDate} 调用流水` : "近期调用流水" }}
            </h2>
            <p class="mt-1 text-[12px] font-medium text-gray-500">
              {{ activeFilterLabel }} · {{ visibleRecordHint }}
            </p>
          </div>
          <div class="flex items-center gap-3">
            <button
              v-if="selectedDate"
              class="text-[12px] font-bold text-ios-blue hover:underline"
              @click="selectedDate = null"
            >
              清除筛选
            </button>
            <span class="text-[12px] font-medium text-gray-500">
              当前命中 {{ formatNumber(filteredRecords.length) }} 条
            </span>
          </div>
        </div>

        <div class="px-6 py-4 border-b border-black/[0.04] dark:border-white/[0.04] bg-black/[0.015] dark:bg-white/[0.015]">
          <div class="grid grid-cols-1 xl:grid-cols-[220px_minmax(0,1fr)_220px] gap-3">
            <div class="min-w-0">
              <div class="text-[11px] font-bold uppercase tracking-[0.1em] text-gray-400 mb-2">
                状态
              </div>
              <ISegmented v-model="statusFilter" :options="STATUS_FILTER_OPTIONS" />
            </div>
            <label class="min-w-0">
              <div class="text-[11px] font-bold uppercase tracking-[0.1em] text-gray-400 mb-2">
                搜索
              </div>
              <div class="flex items-center gap-2 rounded-[16px] border border-black/[0.06] bg-white/80 px-4 py-3 shadow-sm dark:border-white/[0.08] dark:bg-black/20">
                <Search class="h-4 w-4 shrink-0 text-gray-400" stroke-width="2.2" />
                <input
                  v-model.trim="searchQuery"
                  type="text"
                  class="w-full bg-transparent text-[13px] text-gray-800 outline-none placeholder:text-gray-400 dark:text-gray-100"
                  placeholder="模型 / 状态 / Key / 错误信息"
                />
              </div>
            </label>
            <label class="min-w-0">
              <div class="text-[11px] font-bold uppercase tracking-[0.1em] text-gray-400 mb-2">
                模型
              </div>
              <select
                v-model="modelFilter"
                class="no-drag-region w-full rounded-[16px] border border-black/[0.06] bg-white/80 px-4 py-3 text-[13px] font-medium text-gray-800 shadow-sm outline-none transition focus:border-ios-blue/40 dark:border-white/[0.08] dark:bg-black/20 dark:text-gray-100"
              >
                <option value="all">全部模型</option>
                <option v-for="model in modelOptions" :key="model" :value="model">
                  {{ model }}
                </option>
              </select>
            </label>
          </div>
        </div>

        <div v-if="filteredRecords.length === 0" class="p-12 text-center text-gray-500">
          {{ emptyStateMessage }}
        </div>

        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-[13px]">
            <thead class="bg-gray-50/50 dark:bg-black/10 text-gray-500 dark:text-gray-400 text-[11px] uppercase tracking-wider font-bold">
              <tr>
                <th class="px-6 py-3">时间</th>
                <th class="px-6 py-3">状态</th>
                <th class="px-6 py-3">模型</th>
                <th class="px-6 py-3 text-right">Prompt</th>
                <th class="px-6 py-3 text-right">Completion</th>
                <th class="px-6 py-3 text-right">Total Tokens</th>
                <th class="px-6 py-3 text-right">耗时</th>
                <th class="px-6 py-3">来源</th>
                <th class="px-6 py-3">来源 Key (短尾)</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-black/[0.04] dark:divide-white/[0.04]">
              <tr
                v-for="rec in paginatedRecords"
                :key="rec.id"
                class="hover:bg-black/[0.01] dark:hover:bg-white/[0.01] transition-colors"
              >
                <td class="px-6 py-3.5 whitespace-nowrap text-gray-500 dark:text-gray-400">
                  {{ formatDate(rec.at) }}
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap">
                  <div
                    class="flex items-center gap-1.5"
                    :class="rec.status === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'"
                  >
                    <CheckCircle2
                      v-if="rec.status === 'ok'"
                      class="w-4 h-4"
                      stroke-width="2.5"
                    />
                    <XCircle v-else class="w-4 h-4" stroke-width="2.5" />
                    <span class="font-bold text-[11px] uppercase">{{ rec.status }}</span>
                  </div>
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap">
                  <span class="bg-black/5 dark:bg-white/10 px-2 py-0.5 rounded font-mono text-[11px] text-gray-700 dark:text-gray-300 font-semibold shadow-sm">
                    {{ rec.model || rec.request_model || "unknown" }}
                  </span>
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap text-right font-mono text-gray-600 dark:text-gray-300">
                  {{ formatNumber(rec.prompt_tokens) }}
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap text-right font-mono text-gray-600 dark:text-gray-300">
                  {{ formatNumber(rec.completion_tokens) }}
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap text-right font-mono font-bold text-gray-900 dark:text-gray-100">
                  {{ formatNumber(rec.total_tokens) }}
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap text-right text-gray-500">
                  {{ formatDuration(rec.duration_ms) }}
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap">
                  <span class="bg-sky-500/10 text-sky-700 dark:bg-sky-500/20 dark:text-sky-300 px-2 py-0.5 rounded font-mono text-[11px] font-semibold uppercase">
                    {{ rec.format || "-" }}
                  </span>
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap">
                  <div v-if="rec.api_key_short || rec.error_detail" class="space-y-1">
                    <span
                      v-if="rec.api_key_short"
                      class="inline-flex font-mono text-[11px] text-gray-500 bg-gray-100 dark:bg-gray-800 px-1.5 py-0.5 rounded"
                    >
                      ...{{ rec.api_key_short }}
                    </span>
                    <div
                      v-if="rec.error_detail"
                      class="max-w-[280px] truncate text-[11px] text-rose-500"
                      :title="rec.error_detail"
                    >
                      {{ rec.error_detail }}
                    </div>
                  </div>
                  <span v-else class="text-gray-400">-</span>
                </td>
              </tr>
            </tbody>
          </table>

          <div
            v-if="totalPages > 1"
            class="px-6 py-4 border-t border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between"
          >
            <div class="text-[12px] text-gray-500">
              显示 {{ (currentPage - 1) * pageSize + 1 }} -
              {{ Math.min(currentPage * pageSize, filteredRecords.length) }} 条，共
              {{ filteredRecords.length }} 条记录
            </div>
            <div class="flex items-center gap-1.5">
              <button
                :disabled="currentPage === 1"
                class="px-2.5 py-1.5 rounded border border-black/[0.06] dark:border-white/[0.08] text-[12px] font-medium disabled:opacity-30 disabled:cursor-not-allowed hover:bg-black/[0.04] dark:hover:bg-white/[0.04] transition-colors"
                @click="currentPage--"
              >
                上一页
              </button>
              <div class="px-3 text-[12px] font-bold text-gray-700 dark:text-gray-300">
                {{ currentPage }} / {{ totalPages }}
              </div>
              <button
                :disabled="currentPage === totalPages"
                class="px-2.5 py-1.5 rounded border border-black/[0.06] dark:border-white/[0.08] text-[12px] font-medium disabled:opacity-30 disabled:cursor-not-allowed hover:bg-black/[0.04] dark:hover:bg-white/[0.04] transition-colors"
                @click="currentPage++"
              >
                下一页
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
