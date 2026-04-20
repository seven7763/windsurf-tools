<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import {
  Activity,
  ArrowRight,
  Globe,
  KeyRound,
  Link2,
  RefreshCcw,
  ShieldCheck,
  TriangleAlert,
  Users,
} from "lucide-vue-next";
import PageLoadingSkeleton from "../components/common/PageLoadingSkeleton.vue";
import SkeletonOverlay from "../components/common/SkeletonOverlay.vue";
import MitmPanel from "../components/MitmPanel.vue";
import { useAccountStore } from "../stores/useAccountStore";
import { useMainViewStore } from "../stores/useMainViewStore";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { useRelayStatusStore } from "../stores/useRelayStatusStore";
import {
  getAccountHealth,
  isWeeklyQuotaBlocked,
  truncateMiddle,
} from "../utils/account";

const accountStore = useAccountStore();
const mainView = useMainViewStore();
const mitmStore = useMitmStatusStore();
const relayStore = useRelayStatusStore();
const refreshing = ref(false);

const fetchRelayStatus = async () => {
  await relayStore.fetchStatus(true);
};

const refreshOverview = async () => {
  refreshing.value = true;
  try {
    await Promise.all([
      accountStore.fetchAccounts(true),
      mitmStore.fetchStatus(),
      fetchRelayStatus(),
    ]);
  } finally {
    refreshing.value = false;
  }
};

onMounted(() => {
  void Promise.all([
    accountStore.ensureAccountsLoaded(),
    mitmStore.ensureStatusLoaded(),
    relayStore.ensureStatusLoaded(),
  ]);
});

const booting = computed(
  () =>
    !accountStore.hasLoadedOnce ||
    !mitmStore.hasLoadedOnce ||
    !relayStore.hasLoadedOnce,
);

const totalAccounts = computed(() => accountStore.accounts.length);
const healthyAccounts = computed(
  () =>
    accountStore.accounts.filter(
      (account) => getAccountHealth(account) === "healthy",
    ).length,
);
const criticalAccounts = computed(
  () =>
    accountStore.accounts.filter(
      (account) => getAccountHealth(account) === "critical",
    ).length,
);
const expiredAccounts = computed(
  () =>
    accountStore.accounts.filter(
      (account) => getAccountHealth(account) === "expired",
    ).length,
);
const blockedAccounts = computed(
  () =>
    accountStore.accounts.filter((account) => isWeeklyQuotaBlocked(account))
      .length,
);
const activeKey = computed(
  () => mitmStore.status?.pool_status?.find((item) => item.is_current) ?? null,
);
const relayRunning = computed(() => relayStore.status?.running === true);

const topSummaryCards = computed(() => [
  {
    key: "pool",
    label: "号池总数",
    value: String(totalAccounts.value),
    detail:
      healthyAccounts.value > 0
        ? `健康 ${healthyAccounts.value} 个`
        : "等待可用账号",
    tone: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
    icon: Users,
  },
  {
    key: "mitm",
    label: "MITM 状态",
    value: mitmStore.status?.running ? "运行中" : "未启动",
    detail: mitmStore.status?.running
      ? activeKey.value?.key_short
        ? `当前 ${truncateMiddle(activeKey.value.key_short, 10, 5)}`
        : "等待活跃 Key"
      : "先完成证书、Hosts 与启用",
    tone: mitmStore.status?.running
      ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
      : "bg-amber-500/10 text-amber-700 dark:text-amber-300",
    icon: ShieldCheck,
  },
  {
    key: "relay",
    label: "Relay",
    value: relayRunning.value ? "已启动" : "未启动",
    detail: relayRunning.value
      ? `127.0.0.1:${relayStore.status?.port || 8787}`
      : "需要时可单独启动",
    tone: relayRunning.value
      ? "bg-violet-500/10 text-violet-700 dark:text-violet-300"
      : "bg-slate-500/10 text-slate-700 dark:text-slate-300",
    icon: Globe,
  },
  {
    key: "sessions",
    label: "活跃会话",
    value: String(mitmStore.status?.session_count ?? 0),
    detail:
      (mitmStore.status?.session_count ?? 0) > 0
        ? `${mitmStore.status?.session_count} 个对话绑定中`
        : "暂无活跃会话绑定",
    tone: "bg-violet-500/10 text-violet-700 dark:text-violet-300",
    icon: Link2,
  },
  {
    key: "requests",
    label: "代理请求",
    value: String(mitmStore.status?.total_requests ?? 0),
    detail:
      blockedAccounts.value > 0
        ? `周额度阻断 ${blockedAccounts.value}`
        : "暂未发现周额度阻断",
    tone: "bg-fuchsia-500/10 text-fuchsia-700 dark:text-fuchsia-300",
    icon: Activity,
  },
]);

const actionCards = computed(() => [
  {
    key: "accounts",
    title: "管理号池",
    body: "导入 API Key、刷新额度、检查健康状态与到期账号。",
    tab: "Accounts" as const,
  },
  {
    key: "relay",
    title: "配置 Relay",
    body: "查看 8787 端口、复制接入地址，验证 OpenAI 兼容调用。",
    tab: "Relay" as const,
  },
  {
    key: "settings",
    title: "调整 MITM 设置",
    body: "确认后台服务、自动刷新、自动切换与启动参数。",
    tab: "Settings" as const,
  },
]);

const nextSteps = computed(() => {
  if (totalAccounts.value === 0) {
    return [
      "先去号池导入带 sk-ws- 前缀的 API Key。",
      "导入后回到这里确认健康账号数量与当前活跃 Key。",
      "需要 OpenAI 兼容接口时，再去 Relay 页面启动本地中转。",
    ];
  }
  if (!mitmStore.status?.ca_installed || !mitmStore.status?.hosts_mapped) {
    return [
      "先在下方 MITM 面板完成 CA 证书与 Hosts 配置。",
      "配置完成后打开 MITM 开关，让流量真正接管到本机代理。",
      "如果系统里还有 Clash 或 sing-box，注意为目标域名保留本机接管路径。",
    ];
  }
  if (!mitmStore.status?.running) {
    return [
      "前置条件已满足，直接在下方 MITM 面板里打开代理。",
      "启动后观察“当前活跃 Key”是否出现，并确认最近代理事件开始刷新。",
      "若还要给其它客户端复用，再启动 Relay。",
    ];
  }
  return [
    "MITM 已经接管流量，可以在号池页重点关注低额度和过期账号。",
    "如果需要外部客户端对接，前往 Relay 页面复制 Base URL 与 Endpoint。",
    "遇到异常时，先看最近代理事件和最近一次上游错误，再决定是否切去设置页调整策略。",
  ];
});
</script>

<template>
  <PageLoadingSkeleton v-if="booting" variant="dashboard" class="w-full" />

  <SkeletonOverlay v-else :active="refreshing" label="总览刷新中">
    <div class="space-y-6 p-6">
      <section
        class="ios-glass overflow-hidden rounded-[28px] border border-black/[0.05] shadow-[0_20px_48px_-20px_rgba(15,23,42,0.18)] dark:border-white/[0.06]"
      >
        <div
          class="border-b border-black/[0.05] bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.16),transparent_35%),linear-gradient(180deg,rgba(255,255,255,0.82),rgba(255,255,255,0.68))] px-6 py-5 dark:border-white/[0.06] dark:bg-[radial-gradient(circle_at_top_left,rgba(96,165,250,0.18),transparent_35%),linear-gradient(180deg,rgba(28,28,30,0.94),rgba(28,28,30,0.84))]"
        >
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div class="flex min-w-0 items-start gap-3">
              <div
                class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-ios-blue/10 text-ios-blue shadow-inner"
              >
                <ShieldCheck class="h-5 w-5" stroke-width="2.4" />
              </div>
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h1
                    class="text-[17px] font-bold text-ios-text dark:text-ios-textDark"
                  >
                    MITM 总览
                  </h1>
                  <span
                    class="rounded-full bg-ios-blue/10 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-ios-blue"
                  >
                    Pure MITM
                  </span>
                </div>
                <p
                  class="mt-1 max-w-3xl text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  这里保留纯 MITM 模式最关键的启用链路：看号池健康、完成 CA 与
                  Hosts、打开代理、确认当前活跃 Key，并快速跳去 Relay 与设置页。
                </p>
              </div>
            </div>

            <button
              type="button"
              class="no-drag-region inline-flex items-center gap-2 rounded-full border border-black/[0.06] bg-white/80 px-4 py-2 text-[12px] font-semibold text-ios-text shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textDark"
              :disabled="refreshing"
              @click="refreshOverview"
            >
              <RefreshCcw
                class="h-3.5 w-3.5"
                :class="refreshing ? 'animate-spin' : ''"
                stroke-width="2.4"
              />
              {{ refreshing ? "刷新中..." : "刷新总览" }}
            </button>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 p-6 md:grid-cols-2 xl:grid-cols-4">
          <article
            v-for="card in topSummaryCards"
            :key="card.key"
            class="rounded-[22px] border border-black/[0.05] bg-white/75 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <div
                  class="text-[11px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  {{ card.label }}
                </div>
                <div
                  class="mt-2 text-[24px] font-extrabold text-ios-text dark:text-ios-textDark"
                >
                  {{ card.value }}
                </div>
              </div>
              <div
                class="flex h-10 w-10 items-center justify-center rounded-2xl"
                :class="card.tone"
              >
                <component
                  :is="card.icon"
                  class="h-4.5 w-4.5"
                  stroke-width="2.4"
                />
              </div>
            </div>
            <div
              class="mt-3 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              {{ card.detail }}
            </div>
          </article>
        </div>
      </section>

      <section
        class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.25fr)_360px]"
      >
        <MitmPanel />

        <div class="space-y-6">
          <div
            class="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]"
          >
            <div class="flex items-center gap-2">
              <div
                class="flex h-9 w-9 items-center justify-center rounded-2xl bg-violet-500/10 text-violet-600 dark:text-violet-300"
              >
                <KeyRound class="h-4 w-4" stroke-width="2.4" />
              </div>
              <div>
                <div
                  class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
                >
                  额度风险概览
                </div>
                <div
                  class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  先看哪些号已经危险，再决定是否批量清理或刷新。
                </div>
              </div>
            </div>

            <div class="mt-4 grid grid-cols-2 gap-3">
              <div
                class="rounded-[18px] bg-black/[0.03] px-4 py-3 dark:bg-white/[0.04]"
              >
                <div
                  class="text-[10px] font-bold uppercase tracking-[0.14em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  健康
                </div>
                <div
                  class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark"
                >
                  {{ healthyAccounts }}
                </div>
              </div>
              <div class="rounded-[18px] bg-amber-500/[0.06] px-4 py-3">
                <div
                  class="text-[10px] font-bold uppercase tracking-[0.14em] text-amber-700 dark:text-amber-300"
                >
                  低额度
                </div>
                <div
                  class="mt-1 text-[18px] font-extrabold text-amber-700 dark:text-amber-300"
                >
                  {{ criticalAccounts }}
                </div>
              </div>
              <div class="rounded-[18px] bg-rose-500/[0.06] px-4 py-3">
                <div
                  class="text-[10px] font-bold uppercase tracking-[0.14em] text-rose-700 dark:text-rose-300"
                >
                  已过期
                </div>
                <div
                  class="mt-1 text-[18px] font-extrabold text-rose-700 dark:text-rose-300"
                >
                  {{ expiredAccounts }}
                </div>
              </div>
              <div class="rounded-[18px] bg-orange-500/[0.06] px-4 py-3">
                <div
                  class="text-[10px] font-bold uppercase tracking-[0.14em] text-orange-700 dark:text-orange-300"
                >
                  周阻断
                </div>
                <div
                  class="mt-1 text-[18px] font-extrabold text-orange-700 dark:text-orange-300"
                >
                  {{ blockedAccounts }}
                </div>
              </div>
            </div>
          </div>

          <div
            class="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]"
          >
            <div class="flex items-center gap-2">
              <div
                class="flex h-9 w-9 items-center justify-center rounded-2xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-300"
              >
                <Activity class="h-4 w-4" stroke-width="2.4" />
              </div>
              <div>
                <div
                  class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
                >
                  下一步
                </div>
                <div
                  class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  纯 MITM 模式下，最短路径就是这三步。
                </div>
              </div>
            </div>

            <div class="mt-4 space-y-2">
              <div
                v-for="(step, index) in nextSteps"
                :key="`${index}-${step}`"
                class="flex items-start gap-3 rounded-[16px] border border-black/[0.05] bg-black/[0.02] px-3 py-3 text-[12px] leading-relaxed text-ios-text dark:border-white/[0.06] dark:bg-white/[0.03] dark:text-ios-textDark"
              >
                <span
                  class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-ios-blue/10 text-[10px] font-bold text-ios-blue"
                >
                  {{ index + 1 }}
                </span>
                <span>{{ step }}</span>
              </div>
            </div>
          </div>

          <div
            class="ios-glass rounded-[24px] border border-black/[0.05] p-5 shadow-[0_16px_36px_-22px_rgba(15,23,42,0.18)] dark:border-white/[0.06]"
          >
            <div class="mb-3 flex items-center gap-2">
              <div
                class="flex h-9 w-9 items-center justify-center rounded-2xl bg-sky-500/10 text-sky-600 dark:text-sky-300"
              >
                <ArrowRight class="h-4 w-4" stroke-width="2.4" />
              </div>
              <div>
                <div
                  class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
                >
                  快速跳转
                </div>
                <div
                  class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  保留总览，但把重操作仍放回各自页面。
                </div>
              </div>
            </div>

            <div class="space-y-2.5">
              <button
                v-for="item in actionCards"
                :key="item.key"
                type="button"
                class="no-drag-region flex w-full items-start justify-between gap-3 rounded-[18px] border border-black/[0.05] bg-white/70 px-4 py-3 text-left shadow-sm transition-all ios-btn hover:-translate-y-0.5 dark:border-white/[0.06] dark:bg-white/[0.04]"
                @click="mainView.activeTab = item.tab"
              >
                <div>
                  <div
                    class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
                  >
                    {{ item.title }}
                  </div>
                  <div
                    class="mt-1 text-[11px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark"
                  >
                    {{ item.body }}
                  </div>
                </div>
                <ArrowRight
                  class="mt-0.5 h-4 w-4 shrink-0 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                  stroke-width="2.4"
                />
              </button>
            </div>
          </div>

          <div
            v-if="blockedAccounts > 0"
            class="rounded-[20px] border border-amber-500/18 bg-amber-500/[0.07] px-4 py-3 text-[12px] leading-relaxed text-amber-800 dark:text-amber-300"
          >
            <div class="flex items-start gap-3">
              <TriangleAlert
                class="mt-0.5 h-4 w-4 shrink-0"
                stroke-width="2.4"
              />
              <div>
                当前检测到
                {{ blockedAccounts }}
                个账号处于“周额度阻断”状态。即使日额度看起来还有值，这类账号也不应再参与可用候选。
              </div>
            </div>
          </div>
        </div>
      </section>
    </div>
    <template #skeleton>
      <PageLoadingSkeleton variant="dashboard" class="w-full" />
    </template>
  </SkeletonOverlay>
</template>
