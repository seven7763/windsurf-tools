<script setup lang="ts">
import { onMounted, ref, computed } from "vue";
import {
  CheckCircle2,
  HardDrive,
  Loader2,
  RotateCcw,
  Shield,
  Sparkles,
  Trash2,
  Zap,
} from "lucide-vue-next";
import { APIInfo } from "../api/wails";
import { showToast, confirmDialog } from "../utils/toast";

// ── 磁盘分析 ──
interface CleanupCategory {
  id: string;
  name: string;
  description: string;
  size_bytes: number;
  size_human: string;
  file_count: number;
  safe: boolean;
}
interface DiskUsage {
  categories: CleanupCategory[];
  total_bytes: number;
  total_human: string;
}
interface CleanupResult {
  category: string;
  success: boolean;
  freed_bytes: number;
  freed_human: string;
  deleted_dirs: number;
  error?: string;
}

// ── 性能优化 ──
interface PerformanceTip {
  id: string;
  title: string;
  description: string;
  impact: string;
  auto_fix: boolean;
}

const diskUsage = ref<DiskUsage | null>(null);
const tips = ref<PerformanceTip[]>([]);
const loadingDisk = ref(false);
const loadingTips = ref(false);
const cleaning = ref(false);
const applyingTips = ref(false);
const selectedCategories = ref<Set<string>>(new Set());
const lastCleanResults = ref<CleanupResult[]>([]);

const safeTotalHuman = computed(() => {
  if (!diskUsage.value) return "0 B";
  const safe = diskUsage.value.categories.filter((c) => c.safe);
  const bytes = safe.reduce((sum, c) => sum + c.size_bytes, 0);
  return humanSize(bytes);
});

function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  return `${(mb / 1024).toFixed(2)} GB`;
}

function toggleCategory(id: string) {
  if (selectedCategories.value.has(id)) {
    selectedCategories.value.delete(id);
  } else {
    selectedCategories.value.add(id);
  }
  // Force reactivity
  selectedCategories.value = new Set(selectedCategories.value);
}

function selectAllSafe() {
  if (!diskUsage.value) return;
  diskUsage.value.categories
    .filter((c) => c.safe && c.size_bytes > 0)
    .forEach((c) => selectedCategories.value.add(c.id));
  selectedCategories.value = new Set(selectedCategories.value);
}

async function fetchDiskUsage() {
  loadingDisk.value = true;
  try {
    diskUsage.value = await (APIInfo as any).getWindsurfDiskUsage();
  } catch (e: any) {
    showToast("获取磁盘占用失败: " + e.message, "error");
  } finally {
    loadingDisk.value = false;
  }
}

async function fetchTips() {
  loadingTips.value = true;
  try {
    tips.value = await (APIInfo as any).getPerformanceTips();
  } catch (e: any) {
    console.error("GetPerformanceTips error:", e);
  } finally {
    loadingTips.value = false;
  }
}

async function cleanSelected() {
  const ids = [...selectedCategories.value];
  if (ids.length === 0) {
    showToast("请先选择要清理的类别", "warning");
    return;
  }
  const hasCascade = ids.includes("cascade");
  if (hasCascade) {
    const ok = await confirmDialog(
      "Cascade 对话缓存包含 AI 对话历史，清理后无法恢复。确定继续？",
    );
    if (!ok) return;
  }
  cleaning.value = true;
  try {
    const results: CleanupResult[] = await (APIInfo as any).cleanupWindsurf(
      ids,
    );
    lastCleanResults.value = results;
    const totalFreed = results.reduce((s, r) => s + r.freed_bytes, 0);
    showToast(`清理完成，释放 ${humanSize(totalFreed)}`, "success");
    selectedCategories.value = new Set();
    await fetchDiskUsage();
  } catch (e: any) {
    showToast("清理失败: " + e.message, "error");
  } finally {
    cleaning.value = false;
  }
}

async function quickCleanStartup() {
  cleaning.value = true;
  try {
    const results: CleanupResult[] = await (
      APIInfo as any
    ).cleanupStartupCache();
    lastCleanResults.value = results;
    const totalFreed = results.reduce((s, r) => s + r.freed_bytes, 0);
    showToast(`启动缓存已清理，释放 ${humanSize(totalFreed)}`, "success");
    await fetchDiskUsage();
  } catch (e: any) {
    showToast("清理失败: " + e.message, "error");
  } finally {
    cleaning.value = false;
  }
}

async function applyAllFixes() {
  applyingTips.value = true;
  try {
    const results: Record<string, string> = await (
      APIInfo as any
    ).applyAllPerformanceFixes();
    const applied = Object.values(results).filter(
      (v) => v === "已应用",
    ).length;
    const skipped = Object.values(results).filter(
      (v) => v === "已存在，跳过",
    ).length;
    showToast(
      `性能优化: ${applied} 项已应用, ${skipped} 项已存在`,
      "success",
    );
  } catch (e: any) {
    showToast("应用失败: " + e.message, "error");
  } finally {
    applyingTips.value = false;
  }
}

function impactColor(impact: string) {
  switch (impact) {
    case "high":
      return "text-red-500 bg-red-500/10";
    case "medium":
      return "text-amber-500 bg-amber-500/10";
    default:
      return "text-emerald-500 bg-emerald-500/10";
  }
}
function impactLabel(impact: string) {
  switch (impact) {
    case "high":
      return "高";
    case "medium":
      return "中";
    default:
      return "低";
  }
}

onMounted(() => {
  void fetchDiskUsage();
  void fetchTips();
});
</script>

<template>
  <div class="p-6 space-y-6 max-w-4xl mx-auto">
    <!-- 磁盘占用分析 -->
    <section>
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2">
          <HardDrive class="w-5 h-5 text-ios-blue" stroke-width="2.2" />
          <h2
            class="text-lg font-bold text-ios-text dark:text-ios-textDark"
          >
            Windsurf 磁盘占用
          </h2>
        </div>
        <div class="flex items-center gap-2">
          <button
            @click="fetchDiskUsage"
            :disabled="loadingDisk"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-semibold text-ios-textSecondary hover:bg-black/5 dark:hover:bg-white/10 transition-colors ios-btn"
          >
            <RotateCcw
              class="w-3.5 h-3.5"
              :class="{ 'animate-spin': loadingDisk }"
              stroke-width="2.2"
            />
            刷新
          </button>
        </div>
      </div>

      <div v-if="loadingDisk && !diskUsage" class="text-center py-8">
        <Loader2 class="w-6 h-6 animate-spin mx-auto text-ios-blue" />
        <div
          class="mt-2 text-sm text-ios-textSecondary dark:text-ios-textSecondaryDark"
        >
          分析中...
        </div>
      </div>

      <template v-else-if="diskUsage">
        <!-- 汇总卡片 -->
        <div
          class="grid grid-cols-3 gap-3 mb-4"
        >
          <div
            class="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
          >
            <div
              class="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              总占用
            </div>
            <div
              class="mt-1 text-xl font-extrabold text-ios-text dark:text-ios-textDark"
            >
              {{ diskUsage.total_human }}
            </div>
          </div>
          <div
            class="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
          >
            <div
              class="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              可安全清理
            </div>
            <div
              class="mt-1 text-xl font-extrabold text-emerald-600 dark:text-emerald-400"
            >
              {{ safeTotalHuman }}
            </div>
          </div>
          <div
            class="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
          >
            <div
              class="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              类别数
            </div>
            <div
              class="mt-1 text-xl font-extrabold text-ios-text dark:text-ios-textDark"
            >
              {{ diskUsage.categories.length }}
            </div>
          </div>
        </div>

        <!-- 类别列表 -->
        <div class="space-y-2">
          <div
            v-for="cat in diskUsage.categories"
            :key="cat.id"
            @click="cat.size_bytes > 0 ? toggleCategory(cat.id) : undefined"
            class="flex items-center gap-3 rounded-2xl border px-4 py-3 transition-all duration-200"
            :class="[
              selectedCategories.has(cat.id)
                ? 'border-ios-blue/30 bg-ios-blue/[0.04] ring-1 ring-ios-blue/20'
                : 'border-black/[0.05] bg-white/60 dark:border-white/[0.06] dark:bg-white/[0.04]',
              cat.size_bytes > 0
                ? 'cursor-pointer hover:border-ios-blue/20'
                : 'opacity-50 cursor-default',
            ]"
          >
            <div
              class="w-5 h-5 rounded-md border-2 flex items-center justify-center shrink-0 transition-colors"
              :class="
                selectedCategories.has(cat.id)
                  ? 'bg-ios-blue border-ios-blue'
                  : 'border-gray-300 dark:border-gray-600'
              "
            >
              <CheckCircle2
                v-if="selectedCategories.has(cat.id)"
                class="w-3.5 h-3.5 text-white"
                stroke-width="3"
              />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span
                  class="text-sm font-semibold text-ios-text dark:text-ios-textDark"
                  >{{ cat.name }}</span
                >
                <span
                  v-if="cat.safe"
                  class="rounded-full bg-emerald-500/10 px-1.5 py-0.5 text-[9px] font-bold text-emerald-600 dark:text-emerald-400"
                  >安全</span
                >
                <span
                  v-else
                  class="rounded-full bg-red-500/10 px-1.5 py-0.5 text-[9px] font-bold text-red-500"
                  >谨慎</span
                >
              </div>
              <div
                class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-0.5"
              >
                {{ cat.description }}
              </div>
            </div>
            <div class="text-right shrink-0">
              <div
                class="text-sm font-bold"
                :class="
                  cat.size_bytes > 100 * 1024 * 1024
                    ? 'text-red-500'
                    : cat.size_bytes > 10 * 1024 * 1024
                      ? 'text-amber-500'
                      : 'text-ios-text dark:text-ios-textDark'
                "
              >
                {{ cat.size_human }}
              </div>
              <div
                class="text-[10px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
              >
                {{ cat.file_count }} 文件
              </div>
            </div>
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="mt-4 flex items-center gap-3">
          <button
            @click="selectAllSafe"
            class="flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-ios-blue bg-ios-blue/10 hover:bg-ios-blue/20 transition-colors ios-btn"
          >
            <Shield class="w-3.5 h-3.5" stroke-width="2.2" />
            全选安全项
          </button>
          <button
            @click="cleanSelected"
            :disabled="cleaning || selectedCategories.size === 0"
            class="flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-gradient-to-b from-red-500 to-red-600 shadow-sm hover:shadow-md transition-all ios-btn disabled:opacity-40"
          >
            <Loader2
              v-if="cleaning"
              class="w-3.5 h-3.5 animate-spin"
              stroke-width="2.2"
            />
            <Trash2 v-else class="w-3.5 h-3.5" stroke-width="2.2" />
            清理选中 ({{ selectedCategories.size }})
          </button>
          <button
            @click="quickCleanStartup"
            :disabled="cleaning"
            class="flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-gradient-to-b from-amber-500 to-amber-600 shadow-sm hover:shadow-md transition-all ios-btn disabled:opacity-40"
          >
            <Zap class="w-3.5 h-3.5" stroke-width="2.2" />
            一键清理启动缓存
          </button>
        </div>

        <!-- 清理结果 -->
        <div
          v-if="lastCleanResults.length > 0"
          class="mt-4 rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
        >
          <div
            class="text-[11px] font-bold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark mb-2"
          >
            清理结果
          </div>
          <div class="space-y-1">
            <div
              v-for="r in lastCleanResults"
              :key="r.category"
              class="flex items-center justify-between text-xs"
            >
              <span class="text-ios-text dark:text-ios-textDark font-medium">{{
                r.category
              }}</span>
              <span
                :class="
                  r.success
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : 'text-red-500'
                "
              >
                {{ r.success ? `释放 ${r.freed_human}` : r.error }}
              </span>
            </div>
          </div>
        </div>
      </template>
    </section>

    <!-- 性能优化建议 -->
    <section>
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2">
          <Sparkles class="w-5 h-5 text-amber-500" stroke-width="2.2" />
          <h2
            class="text-lg font-bold text-ios-text dark:text-ios-textDark"
          >
            性能优化建议
          </h2>
        </div>
        <button
          @click="applyAllFixes"
          :disabled="applyingTips"
          class="flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold text-white bg-gradient-to-b from-ios-blue to-blue-600 shadow-sm hover:shadow-md transition-all ios-btn disabled:opacity-40"
        >
          <Loader2
            v-if="applyingTips"
            class="w-3.5 h-3.5 animate-spin"
            stroke-width="2.2"
          />
          <Zap v-else class="w-3.5 h-3.5" stroke-width="2.2" />
          一键优化
        </button>
      </div>

      <div v-if="loadingTips" class="text-center py-4">
        <Loader2 class="w-5 h-5 animate-spin mx-auto text-ios-blue" />
      </div>
      <div v-else class="space-y-2">
        <div
          v-for="tip in tips"
          :key="tip.id"
          class="rounded-2xl border border-black/[0.05] bg-white/60 px-4 py-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span
                  class="text-sm font-semibold text-ios-text dark:text-ios-textDark"
                  >{{ tip.title }}</span
                >
                <span
                  class="rounded-full px-1.5 py-0.5 text-[9px] font-bold"
                  :class="impactColor(tip.impact)"
                >
                  影响: {{ impactLabel(tip.impact) }}
                </span>
                <span
                  v-if="tip.auto_fix"
                  class="rounded-full bg-ios-blue/10 px-1.5 py-0.5 text-[9px] font-bold text-ios-blue"
                  >可自动</span
                >
                <span
                  v-else
                  class="rounded-full bg-gray-500/10 px-1.5 py-0.5 text-[9px] font-bold text-gray-500"
                  >手动</span
                >
              </div>
              <div
                class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-1 leading-relaxed"
              >
                {{ tip.description }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
