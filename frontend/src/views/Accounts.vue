<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useAccountStore } from "../stores/useAccountStore";
import { useMainViewStore } from "../stores/useMainViewStore";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import ImportModal from "../components/accounts/ImportModal.vue";
import AccountCardSkeleton from "../components/accounts/AccountCardSkeleton.vue";
import {
  ArrowRightLeft,
  Plus,
  Trash2,
  RefreshCcw,
  Users,
  ChevronRight,
  KeyRound,
  BarChart3,
  UserX,
  Search,
  X,
  CalendarDays,
  Clock,
  Download,
} from "lucide-vue-next";
import { APIInfo } from "../api/wails";
import {
  getPlanTone,
  isQuotaDepleted,
  isWeeklyQuotaBlocked,
} from "../utils/account";
import {
  SWITCH_PLAN_FILTER_TONES,
  type SwitchPlanTone,
} from "../utils/settingsModel";
import { models } from "../../wailsjs/go/models";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton.vue";
import { confirmDialog, showToast } from "../utils/toast";
import {
  formatDateTimeAsiaShanghai,
  formatResetCountdownZH,
  formatSyncTimeLine,
} from "../utils/datetimeAsia";
import { PRIMARY_POOL_LABEL } from "../utils/appMode";

const accountStore = useAccountStore();
const mainView = useMainViewStore();
const mitmStore = useMitmStatusStore();
const showImportModal = ref(false);
const quotaRefreshingIds = ref<Set<string>>(new Set());

// ═══ 单次遍历聚合：避免 tabsList / poolPlanCounts / freePlanAccountCount 各自遍历全量数组 ═══
const accountAgg = computed(() => {
  const counts: Partial<Record<SwitchPlanTone, number>> = {};
  for (const t of SWITCH_PLAN_FILTER_TONES) counts[t] = 0;
  let freeCount = 0;
  for (const a of accountStore.accounts) {
    const tone = getPlanTone(a.plan_name) as SwitchPlanTone;
    counts[tone] = (counts[tone] ?? 0) + 1;
    if (tone === "free") freeCount++;
  }
  return { counts, freeCount };
});

const poolPlanCounts = computed(() => accountAgg.value.counts);

const planSectionOrder = [
  "pro",
  "max",
  "team",
  "enterprise",
  "trial",
  "free",
  "unknown",
] as const;

const planSectionLabels: Record<string, string> = {
  pro: "Pro",
  max: "Max / Ultimate",
  team: "Teams",
  enterprise: "Enterprise",
  trial: "Trial",
  free: "Free",
  unknown: "未识别",
};

const searchQuery = ref("");
const activeTab = ref<string>("all");
type AccountQuickFilter =
  | "all"
  | "online"
  | "switchable"
  | "depleted"
  | "runtime_exhausted"
  | "low"
  | "pending"
  | "credential_gap";
const quickFilter = ref<AccountQuickFilter>("all");

const filteredAccounts = computed(() => {
  let list = accountStore.accounts;

  if (activeTab.value !== "all") {
    list = list.filter((a) => getPlanTone(a.plan_name) === activeTab.value);
  }
  if (quickFilter.value !== "all") {
    list = list.filter((a) => matchesQuickFilter(a, quickFilter.value));
  }

  const q = searchQuery.value.trim().toLowerCase();
  if (!q) {
    return list;
  }
  return list.filter(
    (a) =>
      (a.email?.toLowerCase().includes(q) ?? false) ||
      (a.nickname?.toLowerCase().includes(q) ?? false) ||
      (a.remark?.toLowerCase().includes(q) ?? false) ||
      (a.plan_name?.toLowerCase().includes(q) ?? false),
  );
});

const tabsList = computed(() => {
  const c = accountAgg.value.counts;
  const tabs = planSectionOrder
    .filter((k) => (c[k as SwitchPlanTone] ?? 0) > 0)
    .map((key) => ({
      key,
      label: planSectionLabels[key] || key,
      count: c[key as SwitchPlanTone] ?? 0,
    }));
  return [
    { key: "all", label: "全部", count: accountStore.accounts.length },
    ...tabs,
  ];
});

const freePlanAccountCount = computed(() => accountAgg.value.freeCount);

const accountSort = ref<"group" | "name" | "quota">("group");
const pageSize = ref(60);
const currentPage = ref(1);

const displayAccounts = computed(() => {
  const items = [...filteredAccounts.value];
  const mode = accountSort.value;

  if (mode === "group") {
    // 预计算 planTone → order index，避免 sort comparator 内重复调用 getPlanTone + indexOf
    const orderMap = new Map<string, number>();
    for (let i = 0; i < planSectionOrder.length; i++)
      orderMap.set(planSectionOrder[i], i);
    const fallback = planSectionOrder.length;
    const toneCache = new Map<string, number>();
    const getToneIdx = (plan: string) => {
      let idx = toneCache.get(plan);
      if (idx === undefined) {
        idx = orderMap.get(getPlanTone(plan)) ?? fallback;
        toneCache.set(plan, idx);
      }
      return idx;
    };
    items.sort((a, b) => {
      const d = getToneIdx(a.plan_name || "") - getToneIdx(b.plan_name || "");
      return d !== 0
        ? d
        : (a.email || "").localeCompare(b.email || "", "zh-CN");
    });
  } else if (mode === "name") {
    items.sort((a, b) => (a.email || "").localeCompare(b.email || "", "zh-CN"));
  } else if (mode === "quota") {
    items.sort((a, b) => {
      const pa =
        parseFloat(String(a.daily_remaining || "").replace("%", "")) || 0;
      const pb =
        parseFloat(String(b.daily_remaining || "").replace("%", "")) || 0;
      return pa - pb;
    });
  }
  return items;
});

const totalPages = computed(() =>
  Math.max(1, Math.ceil(displayAccounts.value.length / pageSize.value)),
);

// 筛选/搜索变化时重置到第一页
watch([activeTab, quickFilter, searchQuery, accountSort], () => {
  currentPage.value = 1;
});

const pagedAccounts = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value;
  return displayAccounts.value.slice(start, start + pageSize.value);
});

const paginationRange = computed(() => {
  const total = totalPages.value;
  const cur = currentPage.value;
  const maxButtons = 7;
  if (total <= maxButtons) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }
  let start = Math.max(1, cur - Math.floor(maxButtons / 2));
  let end = start + maxButtons - 1;
  if (end > total) {
    end = total;
    start = end - maxButtons + 1;
  }
  return Array.from({ length: end - start + 1 }, (_, i) => start + i);
});

const emptyStateTitle = computed(() => {
  return "先导入 MITM 号池";
});

const emptyStateBody = computed(() => {
  return "只要把带 API Key 的账号导入号池，MITM 代理和 OpenAI Relay 就能直接接管轮换。";
});

const goRelay = () => {
  mainView.activeTab = "Relay";
};

onMounted(() => {
  void accountStore.ensureAccountsLoaded();
  void mitmStore.ensureStatusLoaded();
});

const handleDelete = async (id: string) => {
  const ok = await confirmDialog("是否确认移除该账号？", {
    confirmText: "移除",
    cancelText: "取消",
    destructive: true,
  });
  if (ok) {
    await accountStore.deleteAccount(id);
  }
};

const handleCleanExpired = async () => {
  try {
    const n = await accountStore.cleanExpiredAccounts();
    showToast(`已清理 ${n} 个过期账号`, "success");
  } catch (e: unknown) {
    showToast(`清理失败: ${String(e)}`, "error");
  }
};

const handleDeleteFreePlans = async () => {
  const n = freePlanAccountCount.value;
  if (n === 0) {
    showToast("当前没有 Free 计划的账号", "info");
    return;
  }
  const ok = await confirmDialog(`将永久删除 ${n} 个免费计划账号，不可恢复。`, {
    confirmText: "删除",
    cancelText: "取消",
    destructive: true,
  });
  if (!ok) return;

  try {
    const deleted = await accountStore.deleteFreePlanAccounts();
    showToast(`已删除 ${deleted} 个免费账号`, "success");
  } catch (e: unknown) {
    showToast(`删除失败: ${String(e)}`, "error");
  }
};

const handleRefreshTokens = async () => {
  try {
    const map = await accountStore.refreshAllTokens();
    const entries = Object.entries(map || {});
    const ok = entries.filter(([, v]) => String(v).includes("成功")).length;
    showToast(`刷新完成：${ok} / ${entries.length}`, "success");
  } catch (e: unknown) {
    showToast(`刷新失败: ${String(e)}`, "error");
  }
};

const handleRefreshAllQuotas = async () => {
  try {
    const map = await accountStore.refreshAllQuotas();
    const entries = Object.entries(map || {});
    const synced = entries.filter(([, v]) =>
      String(v).includes("已同步"),
    ).length;
    showToast(
      `额度同步完成：已更新 ${synced} 共 ${entries.length} 条`,
      "success",
    );
  } catch (e: unknown) {
    showToast(`同步额度失败: ${String(e)}`, "error");
  }
};

const handleSwitchNextSeat = async () => {
  try {
    const target = await mitmStore.switchToNext();
    showToast(`MITM 已切到下一席位：${target || "已切换"}`, "success");
  } catch (e: unknown) {
    showToast(`手动切换失败: ${String(e)}`, "error");
  }
};

const handleRefreshOneQuota = async (id: string, email: string) => {
  if (quotaRefreshingIds.value.has(id)) {
    return;
  }
  const next = new Set(quotaRefreshingIds.value);
  next.add(id);
  quotaRefreshingIds.value = next;
  try {
    await accountStore.refreshAccountQuota(id);
    showToast(`${email} 额度已更新`, "success");
  } catch (e: unknown) {
    showToast(`刷新额度失败: ${String(e)}`, "error");
  } finally {
    const done = new Set(quotaRefreshingIds.value);
    done.delete(id);
    quotaRefreshingIds.value = done;
  }
};

const isAccountCardRefreshing = (id: string) =>
  quotaRefreshingIds.value.has(id);

const handleSwitchMitmToAccount = async (acc: models.Account) => {
  try {
    const target = await mitmStore.switchToAccount(acc.id);
    showToast(`MITM 已切到：${target || acc.email || "目标账号"}`, "success");
  } catch (e: unknown) {
    showToast(`切换到该账号失败: ${String(e)}`, "error");
  }
};

const isAccountSwitching = (id: string) =>
  mitmStore.switchLoading && mitmStore.switchTargetAccountId === id;

// ═══ 按套餐分组操作 ═══
const planGroupFilter = ref("");

const PLAN_TONE_LABELS: Record<string, string> = {
  pro: "Pro",
  trial: "Trial",
  free: "Free",
  team: "Teams",
  enterprise: "Enterprise",
  max: "Max",
  unknown: "未知",
};

const planGroupCount = computed(() => {
  if (!planGroupFilter.value) return 0;
  return poolPlanCounts.value[planGroupFilter.value as SwitchPlanTone] ?? 0;
});

const handleDeleteByPlanGroup = async () => {
  const tone = planGroupFilter.value;
  if (!tone) {
    showToast("请先选择套餐类型", "warning");
    return;
  }
  const cnt = planGroupCount.value;
  if (cnt === 0) {
    showToast(`没有 ${PLAN_TONE_LABELS[tone] ?? tone} 类型的账号`, "info");
    return;
  }
  const label = PLAN_TONE_LABELS[tone] ?? tone;
  const ok = await confirmDialog(
    `将永久删除所有「${label}」套餐的 ${cnt} 个账号，不可恢复。`,
    {
      confirmText: "删除",
      cancelText: "取消",
      destructive: true,
    },
  );
  if (!ok) return;
  try {
    const n = await APIInfo.deleteAccountsByGroup(tone);
    await accountStore.fetchAccounts(true);
    planGroupFilter.value = "";
    showToast(`已删除 ${n} 个「${label}」账号`, "success");
  } catch (e: unknown) {
    showToast(`删除失败: ${String(e)}`, "error");
  }
};

const handleExportByPlanGroup = async () => {
  const tone = planGroupFilter.value;
  if (!tone) {
    showToast("请先选择套餐类型", "warning");
    return;
  }
  const label = PLAN_TONE_LABELS[tone] ?? tone;
  try {
    const filePath = await APIInfo.exportAccountsByGroup(tone);
    showToast(
      `「${label}」的 ${planGroupCount.value} 个账号已导出到：\n${filePath}`,
      "success",
      6000,
    );
  } catch (e: unknown) {
    const msg = String(e);
    if (msg.includes("已取消")) return;
    showToast(`导出失败: ${msg}`, "error");
  }
};

const parseQuotaWidth = (str: string) => {
  if (!str) return "0%";
  const n = parseFloat(String(str).replace("%", "").trim());
  if (!Number.isFinite(n)) return "0%";
  return `${Math.max(0, Math.min(100, n))}%`;
};

const looksLikeDateish = (value: string) =>
  /^\d{4}[./-]\d{1,2}[./-]\d{1,2}(?:\s+\d{1,2}:\d{2})?$/.test(value.trim());

const getEmailUsername = (email?: string) => {
  const clean = String(email || "").trim();
  if (!clean) {
    return "";
  }
  const [local] = clean.split("@");
  return local || clean;
};

const getAccountUsername = (acc: models.Account) => {
  const emailUsername = getEmailUsername(acc.email);
  if (emailUsername) {
    return emailUsername;
  }
  const nickname = String(acc.nickname || "").trim();
  return nickname || "未命名账号";
};

const getAccountNickname = (acc: models.Account) => {
  const nickname = String(acc.nickname || "").trim();
  if (!nickname || looksLikeDateish(nickname)) {
    return "";
  }
  if (nickname.toLowerCase() === getAccountUsername(acc).toLowerCase()) {
    return "";
  }
  return nickname;
};

const getAccountDisplayName = (acc: models.Account) =>
  getAccountNickname(acc) || getAccountUsername(acc);

const shouldShowUsernameMeta = (acc: models.Account) => {
  const nickname = getAccountNickname(acc);
  const username = getAccountUsername(acc);
  if (!nickname || !username) {
    return false;
  }
  return nickname.toLowerCase() !== username.toLowerCase();
};

const getAccountRemark = (acc: models.Account) =>
  String(acc.remark || "").trim();

const getQuotaColor = (str: string) => {
  const n = parseFloat(String(str).replace("%", "").trim());
  if (!Number.isFinite(n)) return "bg-gray-400";
  if (n > 50) return "bg-ios-green";
  if (n > 20) return "bg-yellow-500";
  return "bg-ios-red";
};

const isExpiredAccount = (acc: models.Account) => {
  const status = String(acc.status || "").toLowerCase();
  if (status === "disabled" || status === "expired") {
    return true;
  }
  if (!acc.subscription_expires_at) {
    return false;
  }
  const ts = Date.parse(acc.subscription_expires_at);
  return Number.isFinite(ts) && ts < Date.now();
};

function findMitmPoolRuntime(acc: models.Account) {
  const key = String(acc.windsurf_api_key || "").trim();
  if (!key) {
    return null;
  }
  return (
    mitmStore.status?.pool_status?.find((item) => {
      const short = String(item.key_short || "")
        .trim()
        .replace(/\.\.\.$/, "");
      return short && key.startsWith(short);
    }) ?? null
  );
}

const isCurrentOnline = (acc: models.Account) => {
  return Boolean(findMitmPoolRuntime(acc)?.is_current);
};

const getBoundSessionCount = (acc: models.Account) => {
  const runtime = findMitmPoolRuntime(acc);
  return runtime?.bound_session_count ?? 0;
};

type CardStateTone = "online" | "ready" | "warning" | "danger" | "pending";

const getCardStateMeta = (
  acc: models.Account,
): { tone: CardStateTone; label: string } => {
  // 1. 运行时见底（MITM 代理标记的实时额度耗尽）优先
  const mitmRuntime = findMitmPoolRuntime(acc);
  if (mitmRuntime?.runtime_exhausted) {
    return {
      tone: "danger",
      label: isCurrentOnline(acc) ? "当前活跃 · 运行时见底" : "运行时见底",
    };
  }

  // 2. 当前活跃席位
  if (isCurrentOnline(acc)) {
    return {
      tone: "online",
      label: "当前活跃",
    };
  }

  // 3. 已过期
  if (isExpiredAccount(acc)) {
    return {
      tone: "danger",
      label: "已过期",
    };
  }

  // 4. 额度检查（需要有 API Key 且有同步过额度数据才有意义）
  const daily = parseFloat(
    String(acc.daily_remaining || "")
      .replace("%", "")
      .trim(),
  );
  const weekly = parseFloat(
    String(acc.weekly_remaining || "")
      .replace("%", "")
      .trim(),
  );
  const dailyKnown = Number.isFinite(daily);
  const weeklyKnown = Number.isFinite(weekly);
  const weeklyBlocked = isWeeklyQuotaBlocked(acc);
  const exhausted = isQuotaDepleted(acc);
  const lowQuota =
    (dailyKnown && daily > 0 && daily < 20) ||
    (weeklyKnown && weekly > 0 && weekly < 20);

  if (weeklyBlocked) {
    return {
      tone: "danger",
      label: "周限不可用",
    };
  }

  if (exhausted) {
    return {
      tone: "danger",
      label: "额度见底",
    };
  }

  if (lowQuota) {
    return {
      tone: "warning",
      label: "额度偏低",
    };
  }

  // 5. 从未同步过额度且没有 API Key → 待补
  if (!acc.windsurf_api_key) {
    return {
      tone: "pending",
      label: "待补 API Key",
    };
  }

  // 6. 从未同步过任何额度数据 → 待同步
  if (
    !acc.subscription_expires_at &&
    !dailyKnown &&
    !weeklyKnown &&
    !acc.last_quota_update
  ) {
    return {
      tone: "pending",
      label: "待同步",
    };
  }

  // 7. 一切正常
  return {
    tone: "ready",
    label: "可参与轮换",
  };
};

const getCardStatePanelClass = (tone: CardStateTone) => {
  switch (tone) {
    case "online":
      return "border-emerald-500/15 bg-emerald-500/[0.07]";
    case "ready":
      return "border-ios-blue/15 bg-ios-blue/[0.06]";
    case "warning":
      return "border-amber-500/15 bg-amber-500/[0.07]";
    case "danger":
      return "border-rose-500/15 bg-rose-500/[0.07]";
    case "pending":
      return "border-black/[0.08] bg-black/[0.03] dark:border-white/[0.08] dark:bg-white/[0.04]";
    default:
      return "border-slate-500/12 bg-slate-500/[0.06]";
  }
};

const hasApiKey = (acc: models.Account) =>
  Boolean(String(acc.windsurf_api_key || "").trim());

const matchesQuickFilter = (
  acc: models.Account,
  filter: AccountQuickFilter,
) => {
  const meta = getCardStateMeta(acc);
  switch (filter) {
    case "online":
      return meta.tone === "online";
    case "switchable":
      return meta.tone === "online" || meta.tone === "ready";
    case "depleted":
      return meta.tone === "danger";
    case "runtime_exhausted":
      return Boolean(findMitmPoolRuntime(acc)?.runtime_exhausted);
    case "low":
      return meta.tone === "warning";
    case "pending":
      return meta.tone === "pending";
    case "credential_gap":
      return !hasApiKey(acc);
    case "all":
    default:
      return true;
  }
};

const quickFilterOptions = computed<
  Array<{ key: AccountQuickFilter; label: string; count: number }>
>(() => {
  const options: Array<{ key: AccountQuickFilter; label: string }> = [
    { key: "all", label: "全部" },
    { key: "online", label: "当前活跃" },
    { key: "switchable", label: "可切换" },
    { key: "depleted", label: "额度见底" },
    { key: "runtime_exhausted", label: "运行时见底" },
    { key: "low", label: "额度偏低" },
    { key: "pending", label: "待同步" },
    { key: "credential_gap", label: "待补 API Key" },
  ];
  return options.map((option) => ({
    ...option,
    count:
      option.key === "all"
        ? accountStore.accounts.length
        : accountStore.accounts.filter((acc) =>
            matchesQuickFilter(acc, option.key),
          ).length,
  }));
});

const hasListFilters = computed(
  () =>
    activeTab.value !== "all" ||
    quickFilter.value !== "all" ||
    Boolean(searchQuery.value.trim()),
);

const clearListFilters = () => {
  activeTab.value = "all";
  quickFilter.value = "all";
  searchQuery.value = "";
};

const getPlanAccentClass = (acc: models.Account) => {
  switch (getPlanTone(acc.plan_name)) {
    case "pro":
      return "from-ios-blue via-sky-400 to-cyan-300";
    case "max":
      return "from-violet-500 via-fuchsia-400 to-rose-300";
    case "team":
      return "from-indigo-500 via-blue-400 to-cyan-300";
    case "enterprise":
      return "from-slate-600 via-slate-500 to-slate-300";
    case "trial":
      return "from-amber-500 via-orange-400 to-yellow-300";
    case "free":
      return "from-slate-400 via-slate-300 to-slate-200";
    default:
      return "from-gray-400 via-gray-300 to-gray-200";
  }
};
</script>

<template>
  <div class="p-6 md:p-8 flex flex-col max-w-6xl mx-auto w-full min-h-0">
    <div
      class="flex flex-wrap items-center justify-between gap-4 mb-4 shrink-0"
    >
      <div class="flex items-center gap-4">
        <div>
          <h1 class="text-[32px] font-bold tracking-tight">
            {{ PRIMARY_POOL_LABEL }}
          </h1>
          <p
            class="text-[13px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-1"
          >
            只维护 API Key / JWT / 官方额度快照。当前活跃席位和运行时见底都以
            MITM 代理现场状态为准。
          </p>
        </div>
        <button
          type="button"
          class="no-drag-region flex items-center px-5 py-2.5 bg-gradient-to-b from-[#3b82f6] to-ios-blue text-white rounded-full font-semibold text-[14px] ios-btn shadow-md ring-1 ring-black/5 whitespace-nowrap shrink-0"
          @click="showImportModal = true"
        >
          <Plus class="w-[18px] h-[18px] mr-1" stroke-width="2.5" />
          批量导入
        </button>
      </div>
      <div class="flex flex-wrap gap-2 justify-end items-center">
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-ios-blue/10 text-ios-blue dark:text-blue-300 rounded-full font-semibold text-[14px] ios-btn hover:bg-ios-blue/15 transition-colors disabled:opacity-50"
          :disabled="
            mitmStore.switchLoading || !mitmStore.status?.pool_status?.length
          "
          @click="handleSwitchNextSeat"
        >
          <ArrowRightLeft
            class="w-[18px] h-[18px] mr-1.5"
            :class="
              mitmStore.switchLoading && !mitmStore.switchTargetAccountId
                ? 'animate-pulse'
                : ''
            "
            stroke-width="2.5"
          />
          下一席位
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-black/5 dark:bg-white/10 text-ios-text dark:text-ios-textDark rounded-full font-semibold text-[14px] ios-btn hover:bg-black/10 dark:hover:bg-white/15 transition-colors disabled:opacity-50"
          :disabled="accountStore.actionLoading"
          @click="handleRefreshTokens"
        >
          <KeyRound class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          刷新凭证
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 rounded-full font-semibold text-[14px] ios-btn hover:bg-emerald-500/15 transition-colors disabled:opacity-50"
          :disabled="accountStore.actionLoading"
          @click="handleRefreshAllQuotas"
        >
          <BarChart3 class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          同步额度
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-ios-red/10 text-ios-red dark:text-ios-redDark rounded-full font-semibold text-[14px] ios-btn hover:bg-ios-red/20 transition-colors"
          @click="handleCleanExpired"
        >
          <Trash2 class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          清理过期
        </button>
        <button
          type="button"
          class="no-drag-region flex items-center px-4 py-2 bg-amber-500/12 text-amber-900 dark:text-amber-300 rounded-full font-semibold text-[14px] ios-btn hover:bg-amber-500/18 transition-colors disabled:opacity-50"
          @click="handleDeleteFreePlans"
        >
          <UserX class="w-[18px] h-[18px] mr-1.5" stroke-width="2.5" />
          删除免费
        </button>

        <!-- 按套餐分组操作 -->
        <div
          class="flex items-center gap-1.5 ml-1 pl-2 border-l border-black/10 dark:border-white/10"
        >
          <select
            v-model="planGroupFilter"
            class="no-drag-region h-[36px] rounded-full bg-black/5 dark:bg-white/10 px-3 pr-7 text-[13px] font-semibold text-ios-text dark:text-ios-textDark outline-none cursor-pointer appearance-none"
          >
            <option value="">按套餐操作…</option>
            <option
              v-for="tone in SWITCH_PLAN_FILTER_TONES"
              :key="tone"
              :value="tone"
            >
              {{ PLAN_TONE_LABELS[tone] ?? tone }} ({{
                poolPlanCounts[tone] ?? 0
              }})
            </option>
          </select>
          <template v-if="planGroupFilter">
            <button
              type="button"
              class="no-drag-region flex items-center px-3 py-2 bg-ios-red/10 text-ios-red rounded-full font-semibold text-[12px] ios-btn hover:bg-ios-red/20 transition-colors"
              :title="`删除所有「${PLAN_TONE_LABELS[planGroupFilter] ?? planGroupFilter}」账号`"
              @click="handleDeleteByPlanGroup"
            >
              <Trash2 class="w-[14px] h-[14px] mr-1" stroke-width="2.5" />
              删除该组
            </button>
            <button
              type="button"
              class="no-drag-region flex items-center px-3 py-2 bg-violet-500/10 text-violet-700 dark:text-violet-300 rounded-full font-semibold text-[12px] ios-btn hover:bg-violet-500/20 transition-colors"
              :title="`导出「${PLAN_TONE_LABELS[planGroupFilter] ?? planGroupFilter}」账号到剪贴板`"
              @click="handleExportByPlanGroup"
            >
              <Download class="w-[14px] h-[14px] mr-1" stroke-width="2.5" />
              导出该组
            </button>
          </template>
        </div>

      </div>
    </div>

    <!-- 顶部计划类型导航条 -->
    <div
      class="flex items-center gap-2 mb-6 overflow-x-auto no-scrollbar shrink-0 pb-1"
    >
      <button
        v-for="tab in tabsList"
        :key="tab.key"
        type="button"
        class="no-drag-region flex items-center gap-2 px-4 py-2 rounded-full font-bold text-[14px] transition-all whitespace-nowrap"
        :class="
          activeTab === tab.key
            ? 'bg-ios-text text-white dark:bg-white dark:text-black shadow-md'
            : 'bg-black/5 dark:bg-white/5 text-ios-textSecondary hover:bg-black/10 dark:hover:bg-white/10'
        "
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
        <span
          class="px-2 py-0.5 rounded-full text-[11px] font-bold"
          :class="
            activeTab === tab.key
              ? 'bg-white/20 dark:bg-black/10'
              : 'bg-black/5 dark:bg-white/10'
          "
        >
          {{ tab.count }}
        </span>
      </button>
    </div>

    <div
      v-if="accountStore.accounts.length > 0"
      class="mb-5 flex flex-wrap items-center gap-2 shrink-0"
    >
      <button
        v-for="item in quickFilterOptions"
        :key="item.key"
        type="button"
        class="no-drag-region inline-flex items-center gap-2 rounded-full px-3.5 py-2 text-[12px] font-bold transition-colors"
        :class="
          quickFilter === item.key
            ? 'bg-ios-blue text-white shadow-sm'
            : 'bg-black/[0.04] text-ios-textSecondary hover:bg-black/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark dark:hover:bg-white/[0.1]'
        "
        @click="quickFilter = item.key"
      >
        <span>{{ item.label }}</span>
        <span
          class="rounded-full px-2 py-0.5 text-[10px] font-black"
          :class="
            quickFilter === item.key
              ? 'bg-white/20 text-white'
              : 'bg-black/[0.05] text-ios-textSecondary dark:bg-white/[0.08] dark:text-ios-textSecondaryDark'
          "
        >
          {{ item.count }}
        </span>
      </button>
    </div>

    <div
      v-if="accountStore.accounts.length > 0"
      class="flex flex-col sm:flex-row gap-3 mb-6 shrink-0 max-w-6xl"
    >
      <div class="relative flex-1 min-w-0">
        <Search
          class="absolute left-3.5 top-1/2 -translate-y-1/2 w-[18px] h-[18px] text-ios-textSecondary opacity-60 pointer-events-none"
        />
        <input
          v-model="searchQuery"
          type="search"
          placeholder="搜索邮箱、昵称、备注、计划…"
          class="no-drag-region w-full pl-11 pr-10 py-2.5 rounded-[14px] bg-black/[0.04] border border-black/[0.06] text-[14px] outline-none focus:ring-2 focus:ring-ios-blue/25"
        />
        <button
          v-if="searchQuery"
          type="button"
          class="no-drag-region absolute right-2 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full hover:bg-black/10 text-ios-textSecondary"
          @click="searchQuery = ''"
        >
          <X class="w-4 h-4 mx-auto" stroke-width="2.5" />
        </button>
      </div>
      <select
        v-model="accountSort"
        class="no-drag-region shrink-0 px-4 py-2.5 rounded-[14px] bg-black/[0.04] border border-black/[0.06] text-[13px] font-medium outline-none focus:ring-2 focus:ring-ios-blue/25"
      >
        <option value="group">按分组（默认）</option>
        <option value="name">按邮箱 A→Z</option>
        <option value="quota">按日剩余额度 ↑</option>
      </select>
    </div>

    <PageLoadingSkeleton
      v-if="accountStore.isLoading"
      variant="accounts"
      class="flex-1"
    />

    <div
      v-else-if="accountStore.accounts.length === 0"
      class="flex flex-col items-center justify-center flex-1 text-ios-textSecondary"
    >
      <div
        class="w-24 h-24 mb-6 rounded-3xl bg-black/5 flex items-center justify-center"
      >
        <Users class="w-12 h-12 opacity-50" />
      </div>
      <p class="text-[18px] font-bold text-ios-text dark:text-ios-textDark">
        {{ emptyStateTitle }}
      </p>
      <p
        class="mt-3 max-w-[560px] text-center text-[13px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark"
      >
        {{ emptyStateBody }}
      </p>
      <div class="mt-5 flex flex-wrap items-center justify-center gap-2">
        <button
          type="button"
          class="no-drag-region inline-flex items-center gap-2 rounded-full bg-ios-blue px-4 py-2.5 text-[13px] font-bold text-white transition-colors hover:bg-blue-500 ios-btn"
          @click="showImportModal = true"
        >
          <Plus class="h-4 w-4" stroke-width="2.4" />
          导入账号
        </button>
        <button
          type="button"
          class="no-drag-region inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2.5 text-[13px] font-bold text-gray-700 transition-colors hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-gray-200 dark:hover:bg-white/[0.08] ios-btn"
          @click="goRelay"
        >
          <ChevronRight class="h-4 w-4" stroke-width="2.4" />
          打开 Relay
        </button>
      </div>
    </div>

    <div
      v-else-if="
        accountStore.accounts.length > 0 && displayAccounts.length === 0
      "
      class="flex flex-col items-center justify-center flex-1 py-16 text-ios-textSecondary"
    >
      <Search class="w-12 h-12 opacity-50 mb-4" />
      <p class="text-[17px] font-medium">
        {{ searchQuery.trim() ? "未找到匹配的账号" : "当前筛选下没有账号" }}
      </p>
      <button
        v-if="hasListFilters"
        class="mt-3 text-[14px] font-semibold text-ios-blue ios-btn"
        @click="clearListFilters"
      >
        清除筛选
      </button>
    </div>

    <div v-else class="pb-10 min-h-0">
      <div
        class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5 auto-rows-max"
      >
        <template v-for="acc in pagedAccounts" :key="acc.id">
          <AccountCardSkeleton v-if="isAccountCardRefreshing(acc.id)" />
          <div
            v-else
            :class="[
              'bg-white dark:bg-[#1C1C1E] rounded-[22px] flex flex-col relative overflow-hidden transition-all duration-300 ease-out hover:shadow-lg hover:-translate-y-0.5',
              isCurrentOnline(acc)
                ? 'border-2 border-ios-green/40 dark:border-ios-greenDark/40 shadow-[0_0_0_1px_rgba(52,199,89,0.12)]'
                : 'border border-black/[0.05] dark:border-white/[0.08] shadow-sm',
            ]"
          >
            <div
              class="absolute inset-x-0 top-0 h-1.5 bg-gradient-to-r opacity-95"
              :class="getPlanAccentClass(acc)"
            />
            <div class="relative z-10 flex h-full flex-col p-5">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0 flex flex-wrap items-center gap-2">
                  <span
                    class="rounded-full bg-[#F0F5FF] px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.18em] text-ios-blue dark:bg-ios-blue/20"
                  >
                    {{ acc.plan_name || "unknown" }}
                  </span>
                  <span
                    class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full border px-2.5 py-1 text-[10px] font-bold tracking-[0.14em] text-gray-800 dark:text-gray-100"
                    :class="getCardStatePanelClass(getCardStateMeta(acc).tone)"
                  >
                    {{ getCardStateMeta(acc).label }}
                  </span>
                  <span
                    v-if="getBoundSessionCount(acc) > 0"
                    class="inline-flex shrink-0 items-center whitespace-nowrap rounded-full bg-violet-500/10 border border-violet-500/15 px-2 py-1 text-[10px] font-bold text-violet-700 dark:text-violet-300"
                    :title="`${getBoundSessionCount(acc)} 个会话绑定到此账号`"
                  >
                    {{ getBoundSessionCount(acc) }} 会话
                  </span>
                </div>

                <div
                  class="flex shrink-0 gap-1 rounded-full border border-black/5 bg-gray-50/95 p-1 shadow-sm dark:border-white/5 dark:bg-black/20"
                >
                  <button
                    type="button"
                    class="flex h-[30px] w-[30px] min-w-[30px] items-center justify-center rounded-full bg-white text-ios-blue shadow-sm transition hover:scale-105 dark:bg-black/40 ios-btn disabled:opacity-45"
                    :disabled="
                      !hasApiKey(acc) ||
                      isCurrentOnline(acc) ||
                      mitmStore.switchLoading
                    "
                    title="手动切到这个 MITM 账号"
                    @click="handleSwitchMitmToAccount(acc)"
                  >
                    <ArrowRightLeft
                      class="h-[15px] w-[15px]"
                      :class="isAccountSwitching(acc.id) ? 'animate-pulse' : ''"
                      stroke-width="2.5"
                    />
                  </button>
                  <button
                    type="button"
                    class="flex h-[30px] w-[30px] min-w-[30px] items-center justify-center rounded-full bg-white text-emerald-600 shadow-sm transition hover:scale-105 dark:bg-black/40 ios-btn"
                    :disabled="
                      isAccountCardRefreshing(acc.id) ||
                      isAccountSwitching(acc.id)
                    "
                    title="只刷新这张卡的额度与订阅信息"
                    @click="handleRefreshOneQuota(acc.id, acc.email)"
                  >
                    <RefreshCcw class="h-[15px] w-[15px]" stroke-width="2.5" />
                  </button>
                  <button
                    type="button"
                    class="flex h-[30px] w-[30px] min-w-[30px] items-center justify-center rounded-full bg-white text-ios-red shadow-sm transition hover:scale-105 dark:bg-black/40 ios-btn"
                    :disabled="isAccountSwitching(acc.id)"
                    title="移除账号"
                    @click="handleDelete(acc.id)"
                  >
                    <Trash2 class="h-4 w-4" />
                  </button>
                </div>
              </div>

              <div class="mt-4 min-w-0">
                <div
                  class="truncate text-[24px] font-bold tracking-tight text-ios-text dark:text-ios-textDark"
                  :title="getAccountDisplayName(acc)"
                >
                  {{ getAccountDisplayName(acc) }}
                </div>
                <div
                  class="mt-2 truncate text-[13px] font-medium text-gray-600 dark:text-gray-300"
                  :title="acc.email || '未填写邮箱'"
                >
                  {{ acc.email || "未填写邮箱" }}
                </div>
                <div
                  v-if="shouldShowUsernameMeta(acc) || getAccountRemark(acc)"
                  class="mt-3 flex flex-wrap gap-2"
                >
                  <span
                    v-if="shouldShowUsernameMeta(acc)"
                    class="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-semibold text-ios-textSecondary dark:bg-white/[0.08] dark:text-ios-textSecondaryDark"
                    :title="`@${getAccountUsername(acc)}`"
                  >
                    @{{ getAccountUsername(acc) }}
                  </span>
                  <span
                    v-if="getAccountRemark(acc)"
                    class="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-semibold text-ios-textSecondary dark:bg-white/[0.08] dark:text-ios-textSecondaryDark"
                    :title="getAccountRemark(acc)"
                  >
                    {{ getAccountRemark(acc) }}
                  </span>
                </div>
              </div>

              <div class="mt-4 grid grid-cols-2 gap-3">
                <div
                  class="rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
                >
                  <div
                    class="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                  >
                    <CalendarDays class="h-3.5 w-3.5 opacity-70" />
                    到期时间
                  </div>
                  <div
                    class="mt-2 text-[13px] font-semibold leading-snug text-ios-text dark:text-ios-textDark"
                  >
                    {{
                      acc.subscription_expires_at
                        ? formatDateTimeAsiaShanghai(
                            acc.subscription_expires_at,
                          )
                        : "待同步"
                    }}
                  </div>
                </div>

                <div
                  class="rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
                >
                  <div
                    class="flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                  >
                    <Clock class="h-3.5 w-3.5 opacity-70" />
                    额度同步
                  </div>
                  <div
                    class="mt-2 text-[13px] font-semibold leading-snug text-ios-text dark:text-ios-textDark"
                  >
                    {{
                      acc.last_quota_update
                        ? formatSyncTimeLine(acc.last_quota_update)
                        : "未同步"
                    }}
                  </div>
                  <div
                    v-if="acc.last_quota_update"
                    class="mt-1 truncate text-[10px] text-gray-500 dark:text-gray-400"
                    :title="formatDateTimeAsiaShanghai(acc.last_quota_update)"
                  >
                    {{ formatDateTimeAsiaShanghai(acc.last_quota_update) }}
                  </div>
                </div>
              </div>

              <div
                class="mt-4 rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-4 dark:border-white/[0.06] dark:bg-white/[0.04]"
              >
                <div class="space-y-1.5">
                  <div
                    class="flex items-center justify-between text-[11px] font-bold text-gray-800 dark:text-gray-200"
                  >
                    <span>日额度</span>
                    <span>{{ acc.daily_remaining || "—" }}</span>
                  </div>
                  <div
                    class="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-white/10"
                  >
                    <div
                      class="h-full rounded-full transition-all duration-500 ease-out"
                      :class="getQuotaColor(acc.daily_remaining || '')"
                      :style="{
                        width: parseQuotaWidth(acc.daily_remaining || ''),
                      }"
                    />
                  </div>
                  <div
                    v-if="acc.daily_reset_at"
                    class="truncate pt-1 text-[10px] font-medium text-gray-500 dark:text-gray-400"
                    :title="formatDateTimeAsiaShanghai(acc.daily_reset_at)"
                  >
                    {{ formatResetCountdownZH(acc.daily_reset_at) }}
                  </div>
                </div>

                <div class="mt-4 space-y-1.5">
                  <div
                    class="flex items-center justify-between text-[11px] font-bold text-gray-800 dark:text-gray-200"
                  >
                    <span>周额度</span>
                    <span>{{
                      acc.weekly_remaining ||
                      (isWeeklyQuotaBlocked(acc) ? "官方缺失" : "—")
                    }}</span>
                  </div>
                  <div
                    class="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-white/10"
                  >
                    <div
                      class="h-full rounded-full transition-all duration-500 ease-out"
                      :class="getQuotaColor(acc.weekly_remaining || '')"
                      :style="{
                        width: parseQuotaWidth(acc.weekly_remaining || ''),
                      }"
                    />
                  </div>
                  <div
                    v-if="acc.weekly_reset_at"
                    class="truncate pt-1 text-[10px] font-medium text-gray-500 dark:text-gray-400"
                    :title="formatDateTimeAsiaShanghai(acc.weekly_reset_at)"
                  >
                    {{ formatResetCountdownZH(acc.weekly_reset_at) }}
                  </div>
                  <div
                    v-if="isWeeklyQuotaBlocked(acc)"
                    class="pt-1 text-[10px] font-semibold text-rose-600 dark:text-rose-300"
                  >
                    官方未返回周额度，按不可用处理
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <!-- 分页控件 -->
      <div
        v-if="totalPages > 1 || displayAccounts.length > 30"
        class="mt-6 flex flex-wrap items-center justify-between gap-3"
      >
        <div
          class="text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark font-medium"
        >
          共 {{ displayAccounts.length }} 条，第 {{ currentPage }}/{{
            totalPages
          }}
          页
        </div>
        <div class="flex items-center gap-2">
          <select
            v-model.number="pageSize"
            class="no-drag-region rounded-lg border border-black/[0.06] bg-black/[0.03] px-2.5 py-1.5 text-[12px] font-medium outline-none dark:border-white/[0.08] dark:bg-white/[0.04]"
          >
            <option :value="30">30 / 页</option>
            <option :value="60">60 / 页</option>
            <option :value="120">120 / 页</option>
            <option :value="300">300 / 页</option>
          </select>
          <button
            type="button"
            class="no-drag-region rounded-lg border border-black/[0.06] bg-white px-3 py-1.5 text-[12px] font-bold transition hover:bg-black/[0.04] disabled:opacity-40 dark:border-white/[0.08] dark:bg-white/[0.06]"
            :disabled="currentPage <= 1"
            @click="currentPage = Math.max(1, currentPage - 1)"
          >
            上一页
          </button>
          <button
            v-for="p in paginationRange"
            :key="p"
            type="button"
            class="no-drag-region h-8 min-w-[32px] rounded-lg text-[12px] font-bold transition"
            :class="
              p === currentPage
                ? 'bg-ios-blue text-white shadow-sm'
                : 'border border-black/[0.06] bg-white hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.06]'
            "
            @click="currentPage = p"
          >
            {{ p }}
          </button>
          <button
            type="button"
            class="no-drag-region rounded-lg border border-black/[0.06] bg-white px-3 py-1.5 text-[12px] font-bold transition hover:bg-black/[0.04] disabled:opacity-40 dark:border-white/[0.08] dark:bg-white/[0.06]"
            :disabled="currentPage >= totalPages"
            @click="currentPage = Math.min(totalPages, currentPage + 1)"
          >
            下一页
          </button>
        </div>
      </div>
    </div>

    <ImportModal :isOpen="showImportModal" @close="showImportModal = false" />
  </div>
</template>
