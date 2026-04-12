<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { APIInfo } from "../api/wails";
import { showToast, confirmDialog } from "../utils/toast";
import {
  Activity,
  Trash2,
  RefreshCw,
  Box,
  Layers,
  ArrowRightLeft,
  Clock,
  CheckCircle2,
  XCircle
} from "lucide-vue-next";
import type { Models } from "../api/wails";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton.vue";

const loading = ref(true);
const refreshing = ref(false);
const summary = ref<Models.UsageSummary | null>(null);
const records = ref<Models.UsageRecord[]>([]);
const selectedDate = ref<string | null>(null);
let pollTimer: ReturnType<typeof setInterval> | null = null;

const filteredRecords = computed(() => {
  if (!selectedDate.value) return records.value;
  return records.value.filter(rec => rec.at && rec.at.startsWith(selectedDate.value!));
});

// 分页逻辑
const currentPage = ref(1);
const pageSize = ref(100);

const totalPages = computed(() => Math.max(1, Math.ceil(filteredRecords.value.length / pageSize.value)));

const paginatedRecords = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  const end = start + pageSize.value;
  return filteredRecords.value.slice(start, end);
});

import { watch } from "vue";
watch(selectedDate, () => {
  currentPage.value = 1;
});


const estimatedCost = computed(() => {
  if (!summary.value) return '0.00';
  // 通过后端精准的基于单条模型（Opus / Sonnet / GPT 等）累加得出的成本
  return (summary.value.estimated_cost_usd || 0).toFixed(2);
});

const fetchUsageData = async (isSilent = false) => {
  if (!isSilent) loading.value = true;
  else refreshing.value = true;
  
  try {
    const [sumData, recData] = await Promise.all([
      APIInfo.getUsageSummary(),
      APIInfo.getUsageRecords(5000), // 获取最近 5000 条记录
    ]);
    summary.value = sumData;
    records.value = recData || [];
  } catch (e: any) {
    if (!isSilent) showToast(`获取用量数据失败: ${String(e)}`, "error");
  } finally {
    loading.value = false;
    refreshing.value = false;
  }
};

const handleRefresh = async () => {
  await fetchUsageData(true);
  showToast("数据已刷新", "success");
};

const handleClear = async () => {
  const ok = await confirmDialog("确认清空所有用量记录？", {
    confirmText: "清空",
    destructive: true
  });
  if (!ok) return;
  
  try {
    const deletedCount = await APIInfo.deleteAllUsage();
    showToast(`已清空 ${deletedCount} 条用量记录`, "success");
    await fetchUsageData();
  } catch (e: any) {
    showToast(`清空记录失败: ${String(e)}`, "error");
  }
};

onMounted(() => {
  void fetchUsageData();
  // Auto refresh every 5 seconds
  pollTimer = setInterval(() => {
    void fetchUsageData(true);
  }, 5000);
});

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
});

// Utilities
const formatDate = (dateStr: string) => {
  if (!dateStr) return "-";
  const date = new Date(dateStr);
  return date.toLocaleString();
};

const formatNumber = (num: number) => {
  return new Intl.NumberFormat('en-US').format(num || 0);
};

const formatCompactToken = (num: number) => {
  if (!num) return "0";
  if (num >= 1000000) {
    return new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 2 }).format(num);
  }
  return new Intl.NumberFormat('en-US').format(num);
};

const formatDuration = (ms: number) => {
  if (!ms) return "-";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
};
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

    <PageLoadingSkeleton v-if="loading" variant="relay" class="w-full" />

    <div v-else class="space-y-6">
      <!-- 汇总卡片 -->
      <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div class="rounded-[24px] border border-black/[0.04] bg-white/70 p-5 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-gray-500 dark:text-gray-400 mb-2">
            <ArrowRightLeft class="w-4 h-4" stroke-width="2.4" /> 请求总数
          </div>
          <div class="text-[28px] lg:text-[32px] font-extrabold text-gray-900 dark:text-gray-100 truncate" :title="formatNumber(summary?.total_requests || 0)">
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
          <div class="text-[28px] lg:text-[32px] font-extrabold text-blue-700 dark:text-blue-300 truncate" :title="formatNumber(summary?.total_tokens || 0)">
            {{ formatCompactToken(summary?.total_tokens || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-blue-600/70 dark:text-blue-400/70 font-medium">Prompt + Completion</div>
        </div>

        <div class="rounded-[24px] border border-violet-500/15 bg-violet-500/[0.04] p-5 shadow-sm ios-glass relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-violet-600 dark:text-violet-400 mb-2">
            <Layers class="w-4 h-4" stroke-width="2.4" /> Prompt Tokens
          </div>
          <div class="text-[28px] lg:text-[32px] font-extrabold text-violet-700 dark:text-violet-300 truncate" :title="formatNumber(summary?.total_prompt_tokens || 0)">
            {{ formatCompactToken(summary?.total_prompt_tokens || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-violet-600/70 dark:text-violet-400/70 font-medium">上行请求用量</div>
        </div>

        <div class="rounded-[24px] border border-emerald-500/15 bg-emerald-500/[0.04] p-5 shadow-sm ios-glass relative min-w-0">
          <div class="flex items-center gap-2 text-[11px] font-bold uppercase tracking-[0.1em] text-emerald-600 dark:text-emerald-400 mb-2">
            <Layers class="w-4 h-4" stroke-width="2.4" /> Completion Tokens
          </div>
          <div class="text-[28px] lg:text-[32px] font-extrabold text-emerald-700 dark:text-emerald-300 truncate" :title="formatNumber(summary?.total_completion_tokens || 0)">
            {{ formatCompactToken(summary?.total_completion_tokens || 0) }}
          </div>
          <div class="mt-1 text-[12px] text-emerald-600/70 dark:text-emerald-400/70 font-medium">下行生成用量</div>
        </div>
      </div>

      <!-- 按天汇总 -->
      <div v-if="summary && summary.by_date && Object.keys(summary.by_date).length > 0" class="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden">
        <div class="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between">
          <h2 class="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <Clock class="w-4 h-4 text-ios-blue" stroke-width="2.4" /> 每日用量统计
          </h2>
          <span class="text-[12px] font-medium text-gray-500">点击选中行以筛选下方流水</span>
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
                v-for="date in Object.keys(summary.by_date).sort((a,b)=>b.localeCompare(a))" 
                :key="date"
                class="transition-colors cursor-pointer"
                :class="selectedDate === date ? 'bg-ios-blue/[0.08] dark:bg-ios-blue/[0.15]' : 'hover:bg-black/[0.02] dark:hover:bg-white/[0.02]'"
                @click="selectedDate = selectedDate === date ? null : date"
              >
                <td class="px-6 py-3 whitespace-nowrap font-mono font-semibold" :class="selectedDate === date ? 'text-ios-blue' : 'text-gray-800 dark:text-gray-200'">
                  {{ date }}
                  <span v-if="selectedDate === date" class="ml-2 text-[10px] bg-ios-blue text-white rounded px-1.5 py-0.5">筛选中</span>
                </td>
                <td class="px-6 py-3 whitespace-nowrap text-right font-mono" :class="selectedDate === date ? 'text-ios-blue' : 'text-gray-600 dark:text-gray-300'">
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

      <!-- 详细记录 -->
      <div class="rounded-[24px] border border-black/[0.04] bg-white/70 shadow-sm ios-glass dark:border-white/[0.04] dark:bg-[#1C1C1E]/70 overflow-hidden">
        <div class="px-6 py-5 border-b border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between">
          <h2 class="text-[16px] font-bold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <Clock class="w-4 h-4 text-gray-500" stroke-width="2.4" /> 
            {{ selectedDate ? `${selectedDate} 调用流水` : '近期调用流水' }}
          </h2>
          <div class="flex items-center gap-3">
            <button v-if="selectedDate" @click="selectedDate = null" class="text-[12px] font-bold text-ios-blue hover:underline">
              清除筛选
            </button>
            <span class="text-[12px] font-medium text-gray-500">
              {{ selectedDate ? `共 ${filteredRecords.length} 条记录` : '最高保留展示 5000 条数据' }}
            </span>
          </div>
        </div>
        
        <div v-if="filteredRecords.length === 0" class="p-12 text-center text-gray-500">
          暂无调用记录，开始对话后将在此实时展示。
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
                  <div class="flex items-center gap-1.5" :class="rec.status === 'ok' ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'">
                    <CheckCircle2 v-if="rec.status === 'ok'" class="w-4 h-4" stroke-width="2.5" />
                    <XCircle v-else class="w-4 h-4" stroke-width="2.5" />
                    <span class="font-bold text-[11px] uppercase">{{ rec.status }}</span>
                  </div>
                </td>
                <td class="px-6 py-3.5 whitespace-nowrap">
                  <span class="bg-black/5 dark:bg-white/10 px-2 py-0.5 rounded font-mono text-[11px] text-gray-700 dark:text-gray-300 font-semibold shadow-sm">
                    {{ rec.model || rec.request_model || 'unknown' }}
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
                  <span v-if="rec.api_key_short" class="font-mono text-[11px] text-gray-500 bg-gray-100 dark:bg-gray-800 px-1.5 py-0.5 rounded">
                    ...{{ rec.api_key_short }}
                  </span>
                  <span v-else class="text-gray-400">-</span>
                </td>
              </tr>
            </tbody>
          </table>
          
          <div v-if="totalPages > 1" class="px-6 py-4 border-t border-black/[0.04] dark:border-white/[0.04] flex items-center justify-between">
            <div class="text-[12px] text-gray-500">
              显示 {{ (currentPage - 1) * pageSize + 1 }} - {{ Math.min(currentPage * pageSize, filteredRecords.length) }} 条，共 {{ filteredRecords.length }} 条记录
            </div>
            <div class="flex items-center gap-1.5">
              <button
                :disabled="currentPage === 1"
                @click="currentPage--"
                class="px-2.5 py-1.5 rounded border border-black/[0.06] dark:border-white/[0.08] text-[12px] font-medium disabled:opacity-30 disabled:cursor-not-allowed hover:bg-black/[0.04] dark:hover:bg-white/[0.04] transition-colors"
              >
                上一页
              </button>
              <div class="px-3 text-[12px] font-bold text-gray-700 dark:text-gray-300">
                {{ currentPage }} / {{ totalPages }}
              </div>
              <button
                :disabled="currentPage === totalPages"
                @click="currentPage++"
                class="px-2.5 py-1.5 rounded border border-black/[0.06] dark:border-white/[0.08] text-[12px] font-medium disabled:opacity-30 disabled:cursor-not-allowed hover:bg-black/[0.04] dark:hover:bg-white/[0.04] transition-colors"
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
