<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useAccountStore } from "../stores/useAccountStore";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { APIInfo } from "../api/wails";
import { WindowShow } from "../../wailsjs/runtime/runtime";
import { Loader2, Maximize2, RefreshCcw } from "lucide-vue-next";
import {
  formatResetCountdownZH,
  formatSyncTimeLine,
} from "../utils/datetimeAsia";
import {
  getPlanLabel,
  isQuotaDepleted,
  isWeeklyQuotaBlocked,
  parsePercent,
} from "../utils/account";
import SkeletonBlock from "./common/SkeletonBlock.vue";
import SkeletonOverlay from "./common/SkeletonOverlay.vue";

const accountStore = useAccountStore();
const mitmStore = useMitmStatusStore();

let pollTimer: ReturnType<typeof setInterval> | null = null;
const refreshBusy = ref(false);

const activePoolKey = computed(
  () => mitmStore.status?.pool_status?.find((item) => item.is_current) ?? null,
);

const currentAccount = computed(() => {
  const short = String(activePoolKey.value?.key_short || "")
    .trim()
    .replace(/\.\.\.$/, "");
  if (!short) {
    return null;
  }
  return (
    accountStore.accounts.find((a) =>
      String(a.windsurf_api_key || "")
        .trim()
        .startsWith(short),
    ) ?? null
  );
});

const getEmailUsername = (email?: string) => {
  const clean = String(email || "").trim();
  if (!clean) {
    return "";
  }
  const [local] = clean.split("@");
  return local || clean;
};

const currentTitle = computed(() => {
  const acc = currentAccount.value;
  if (acc) {
    const nickname = String(acc.nickname || "").trim();
    return nickname || getEmailUsername(acc.email) || "当前账号";
  }
  if (activePoolKey.value?.key_short) {
    return activePoolKey.value.key_short;
  }
  return mitmStore.status?.running ? "等待活跃 Key" : "MITM 未启动";
});

const currentEmail = computed(() => {
  const raw =
    currentAccount.value?.email ||
    String(activePoolKey.value?.key_short || "").trim() ||
    "回到主窗口导入 API Key 或启动 MITM";
  return raw.length > 34 ? `${raw.slice(0, 32)}…` : raw;
});

const planLabel = computed(() =>
  currentAccount.value ? getPlanLabel(currentAccount.value.plan_name) : "MITM",
);
const dailyPctText = computed(
  () => currentAccount.value?.daily_remaining || "待同步",
);
const weeklyPctText = computed(() => {
  const acc = currentAccount.value;
  if (!acc) {
    return "待同步";
  }
  if (acc.weekly_remaining) {
    return acc.weekly_remaining;
  }
  if (isWeeklyQuotaBlocked(acc)) {
    return "官方缺失";
  }
  return "待同步";
});

const normalizePercent = (value?: string) => {
  const n = parsePercent(value);
  if (n === null) {
    return 0;
  }
  return Math.max(0, Math.min(100, n));
};

const dailyPercent = computed(() =>
  normalizePercent(currentAccount.value?.daily_remaining),
);
const weeklyPercent = computed(() =>
  normalizePercent(currentAccount.value?.weekly_remaining),
);

const quotaTone = computed<"good" | "warn" | "danger" | "muted">(() => {
  if (activePoolKey.value?.runtime_exhausted) {
    return "danger";
  }
  if (!currentAccount.value) {
    return activePoolKey.value ? "good" : "muted";
  }
  if (isQuotaDepleted(currentAccount.value)) {
    return "danger";
  }
  const candidates = [dailyPercent.value, weeklyPercent.value].filter(
    (v) => Number.isFinite(v) && v > 0,
  );
  const fallback = [dailyPercent.value, weeklyPercent.value].filter((v) =>
    Number.isFinite(v),
  );
  const values = candidates.length > 0 ? candidates : fallback;
  if (!values.length) {
    return "muted";
  }
  const lowest = Math.min(...values);
  if (lowest <= 0) {
    return "danger";
  }
  if (lowest < 20) {
    return "warn";
  }
  return "good";
});

const toneClasses = computed(() => {
  switch (quotaTone.value) {
    case "danger":
      return {
        shell:
          "border-rose-500/25 bg-[linear-gradient(135deg,rgba(255,255,255,0.72),rgba(255,236,240,0.48))] dark:bg-[linear-gradient(135deg,rgba(38,12,18,0.78),rgba(18,10,14,0.62))]",
        orb: "bg-rose-400/45",
        value: "text-rose-700 dark:text-rose-200",
        badge: "bg-rose-500/12 text-rose-700 dark:text-rose-200",
        bar: "from-rose-500 via-orange-400 to-amber-300",
      };
    case "warn":
      return {
        shell:
          "border-amber-500/25 bg-[linear-gradient(135deg,rgba(255,255,255,0.74),rgba(255,244,220,0.55))] dark:bg-[linear-gradient(135deg,rgba(52,32,8,0.78),rgba(24,18,10,0.62))]",
        orb: "bg-amber-400/45",
        value: "text-amber-700 dark:text-amber-200",
        badge: "bg-amber-500/12 text-amber-700 dark:text-amber-200",
        bar: "from-amber-500 via-yellow-400 to-lime-300",
      };
    case "good":
      return {
        shell:
          "border-cyan-400/20 bg-[linear-gradient(135deg,rgba(255,255,255,0.78),rgba(222,247,255,0.55))] dark:bg-[linear-gradient(135deg,rgba(8,28,40,0.82),rgba(9,16,24,0.64))]",
        orb: "bg-cyan-400/40",
        value: "text-cyan-700 dark:text-cyan-200",
        badge: "bg-emerald-500/12 text-emerald-700 dark:text-emerald-200",
        bar: "from-cyan-500 via-sky-400 to-emerald-300",
      };
    default:
      return {
        shell:
          "border-white/20 bg-[linear-gradient(135deg,rgba(255,255,255,0.74),rgba(240,242,248,0.48))] dark:bg-[linear-gradient(135deg,rgba(28,28,30,0.82),rgba(18,18,20,0.64))]",
        orb: "bg-slate-400/35",
        value: "text-slate-700 dark:text-slate-200",
        badge: "bg-slate-500/12 text-slate-700 dark:text-slate-200",
        bar: "from-slate-500 via-slate-400 to-slate-300",
      };
  }
});

const quotaHeadline = computed(() => {
  if (!currentAccount.value) {
    return activePoolKey.value ? "就绪" : "未启动";
  }
  return dailyPctText.value;
});

const quotaCaption = computed(() => {
  if (!currentAccount.value) {
    return activePoolKey.value
      ? "MITM 已接管，等待号池额度同步"
      : "先导入账号并启动 MITM";
  }
  return "当前活跃账号日剩余额度";
});

const nextResetHint = computed(() => {
  if (currentAccount.value?.daily_reset_at) {
    return formatResetCountdownZH(currentAccount.value.daily_reset_at);
  }
  if (currentAccount.value?.weekly_reset_at) {
    return formatResetCountdownZH(currentAccount.value.weekly_reset_at);
  }
  if (
    activePoolKey.value?.runtime_exhausted &&
    activePoolKey.value.cooldown_until
  ) {
    return formatResetCountdownZH(activePoolKey.value.cooldown_until);
  }
  return "等待下一次额度同步";
});

const syncHint = computed(() => {
  if (currentAccount.value?.last_quota_update) {
    return `同步 ${formatSyncTimeLine(currentAccount.value.last_quota_update)}`;
  }
  if (activePoolKey.value?.key_short) {
    return `活跃 ${activePoolKey.value.key_short}`;
  }
  return "尚未同步";
});
const toolbarBooting = computed(
  () => !mitmStore.hasLoadedOnce || !accountStore.hasLoadedOnce,
);

async function refreshSnapshot(force = false) {
  if (refreshBusy.value) {
    return;
  }
  refreshBusy.value = true;
  try {
    await Promise.all([
      accountStore.fetchAccounts(force),
      mitmStore.fetchStatus(force),
    ]);
    const acc = currentAccount.value;
    if (force && acc?.id) {
      await accountStore.refreshAccountQuota(acc.id);
    }
  } catch {
    // 工具栏保持安静，避免把小窗变成错误弹窗中心
  } finally {
    refreshBusy.value = false;
  }
}

async function openMain() {
  await APIInfo.restoreMainWindowLayout();
  WindowShow();
}

const pollTick = () => {
  if (
    typeof document !== "undefined" &&
    document.visibilityState !== "visible"
  ) {
    return;
  }
  void refreshSnapshot();
};

const onVisibilityChange = () => {
  if (
    typeof document !== "undefined" &&
    document.visibilityState === "visible"
  ) {
    void refreshSnapshot();
  }
};

onMounted(() => {
  void refreshSnapshot(true);
  pollTimer = setInterval(pollTick, 60_000);
  document.addEventListener("visibilitychange", onVisibilityChange);
});

onUnmounted(() => {
  document.removeEventListener("visibilitychange", onVisibilityChange);
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
});
</script>

<template>
  <div class="h-full w-full px-3 py-2.5">
    <div
      class="group relative flex h-full items-stretch overflow-hidden rounded-[26px] border shadow-[0_14px_40px_rgba(15,23,42,0.22)] backdrop-blur-2xl select-none"
      :class="toneClasses.shell"
    >
      <div
        class="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(255,255,255,0.55),transparent_52%)] opacity-80"
      />
      <div
        class="pointer-events-none absolute inset-y-0 right-[-18px] w-[120px] rounded-full blur-3xl"
        :class="toneClasses.orb"
      />
      <div
        class="pointer-events-none absolute inset-x-6 bottom-0 h-px bg-gradient-to-r from-transparent via-white/50 to-transparent"
      />

      <div
        v-if="toolbarBooting"
        class="relative flex w-full items-center gap-4 px-4 py-3"
        aria-busy="true"
        aria-label="工具栏加载中"
      >
        <div class="min-w-0 flex-[1.1] space-y-2.5">
          <div class="flex items-center gap-2">
            <SkeletonBlock class="h-5 w-16 rounded-full" />
            <SkeletonBlock class="h-5 w-14 rounded-full" />
          </div>
          <SkeletonBlock class="h-5 w-[58%] rounded-lg" />
          <SkeletonBlock class="h-3 w-[72%] rounded-md" />
          <SkeletonBlock class="h-6 w-24 rounded-full" />
        </div>

        <div class="min-w-0 flex-[0.92] space-y-3">
          <div class="flex items-center justify-between gap-3">
            <SkeletonBlock class="h-7 w-16 rounded-xl" />
            <SkeletonBlock class="h-3 w-20 rounded-md" />
          </div>
          <SkeletonBlock class="h-2.5 w-full rounded-full" />
          <SkeletonBlock class="h-3 w-[64%] rounded-md" />
        </div>

        <div class="flex min-w-[132px] flex-col gap-2">
          <SkeletonBlock class="h-[56px] w-full rounded-[18px]" />
          <SkeletonBlock class="h-[56px] w-full rounded-[18px]" />
        </div>

        <div
          class="flex min-w-[108px] flex-col items-end justify-between self-stretch"
        >
          <div class="flex items-center gap-2">
            <SkeletonBlock class="h-10 w-10 rounded-2xl" />
            <SkeletonBlock class="h-10 w-10 rounded-2xl" />
          </div>
          <SkeletonBlock class="h-[60px] w-full rounded-[18px]" />
        </div>
      </div>

      <SkeletonOverlay v-else :active="refreshBusy" label="工具栏刷新中">
        <div class="relative flex w-full items-center gap-4 px-4 py-3">
          <div class="min-w-0 flex-[1.1]">
            <div class="flex items-center gap-2">
              <span
                class="rounded-full bg-black/5 px-2.5 py-1 text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:bg-white/10 dark:text-ios-textSecondaryDark"
              >
                活跃席位
              </span>
              <span
                class="rounded-full px-2.5 py-1 text-[10px] font-bold tracking-[0.16em]"
                :class="toneClasses.badge"
              >
                {{ planLabel }}
              </span>
            </div>
            <div
              class="mt-2.5 truncate text-[16px] font-black tracking-[0.01em] text-ios-text dark:text-ios-textDark"
            >
              {{ currentTitle }}
            </div>
            <div
              class="mt-1 truncate text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark"
              :title="currentAccount?.email || activePoolKey?.key_short || ''"
            >
              {{ currentEmail }}
            </div>
            <div
              class="mt-2 inline-flex items-center gap-1.5 rounded-full bg-black/5 px-2.5 py-1 text-[10px] font-semibold text-ios-textSecondary dark:bg-white/10 dark:text-ios-textSecondaryDark"
            >
              <span
                class="inline-block h-1.5 w-1.5 rounded-full bg-emerald-400"
              />
              {{ syncHint }}
            </div>
          </div>

          <div class="min-w-0 flex-[0.92]">
            <div
              class="text-[11px] font-semibold uppercase tracking-[0.26em] text-ios-textSecondary/80 dark:text-ios-textSecondaryDark/80"
            >
              剩余额度
            </div>
            <div class="mt-1 flex items-end gap-2">
              <div
                class="truncate text-[34px] font-black leading-none tracking-[-0.05em]"
                :class="toneClasses.value"
              >
                {{ quotaHeadline }}
              </div>
            </div>
            <div
              class="mt-1.5 text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              {{ quotaCaption }}
            </div>
            <div
              class="mt-3 h-2.5 overflow-hidden rounded-full bg-black/[0.08] dark:bg-white/[0.08]"
            >
              <div
                class="h-full rounded-full bg-gradient-to-r transition-all duration-500 ease-out"
                :class="toneClasses.bar"
                :style="{ width: `${dailyPercent}%` }"
              />
            </div>
          </div>

          <div class="flex min-w-[132px] flex-col gap-2">
            <div
              class="rounded-[18px] border border-white/20 bg-white/35 px-3 py-2 backdrop-blur-xl dark:border-white/10 dark:bg-black/20"
            >
              <div
                class="flex items-center justify-between text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
              >
                <span>日</span>
                <span>{{ dailyPctText }}</span>
              </div>
              <div
                class="mt-2 h-1.5 overflow-hidden rounded-full bg-black/[0.08] dark:bg-white/[0.08]"
              >
                <div
                  class="h-full rounded-full bg-gradient-to-r transition-all duration-500 ease-out"
                  :class="toneClasses.bar"
                  :style="{ width: `${dailyPercent}%` }"
                />
              </div>
            </div>

            <div
              class="rounded-[18px] border border-white/20 bg-white/35 px-3 py-2 backdrop-blur-xl dark:border-white/10 dark:bg-black/20"
            >
              <div
                class="flex items-center justify-between text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
              >
                <span>周</span>
                <span>{{ weeklyPctText }}</span>
              </div>
              <div
                class="mt-2 h-1.5 overflow-hidden rounded-full bg-black/[0.08] dark:bg-white/[0.08]"
              >
                <div
                  class="h-full rounded-full bg-gradient-to-r transition-all duration-500 ease-out"
                  :class="toneClasses.bar"
                  :style="{ width: `${weeklyPercent}%` }"
                />
              </div>
            </div>
          </div>

          <div
            class="flex min-w-[108px] flex-col items-end justify-between self-stretch"
          >
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="no-drag-region inline-flex h-10 w-10 items-center justify-center rounded-2xl border border-white/25 bg-white/45 text-ios-text shadow-sm backdrop-blur-xl transition hover:bg-white/70 dark:border-white/10 dark:bg-black/25 dark:text-ios-textDark dark:hover:bg-black/35"
                title="刷新当前账号额度"
                @click="refreshSnapshot(true)"
              >
                <Loader2
                  v-if="refreshBusy"
                  class="h-4 w-4 ios-spinner"
                  stroke-width="2.2"
                />
                <RefreshCcw v-else class="h-4 w-4" stroke-width="2.2" />
              </button>
              <button
                type="button"
                class="no-drag-region inline-flex h-10 w-10 items-center justify-center rounded-2xl border border-white/25 bg-[#0F172A]/90 text-white shadow-sm backdrop-blur-xl transition hover:bg-[#111c2f] dark:border-white/10"
                title="打开主窗口"
                @click="openMain"
              >
                <Maximize2 class="h-4 w-4" stroke-width="2.2" />
              </button>
            </div>

            <div
              class="w-full rounded-[18px] border border-white/20 bg-white/35 px-3 py-2 text-right backdrop-blur-xl dark:border-white/10 dark:bg-black/20"
            >
              <div
                class="text-[10px] font-bold uppercase tracking-[0.16em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
              >
                下次刷新
              </div>
              <div
                class="mt-1 text-[11px] font-semibold leading-snug text-ios-text dark:text-ios-textDark"
              >
                {{ nextResetHint }}
              </div>
            </div>
          </div>
        </div>
        <template #skeleton>
          <div
            class="relative flex w-full items-center gap-4 px-4 py-3"
            aria-busy="true"
            aria-label="工具栏刷新中"
          >
            <div class="min-w-0 flex-[1.1] space-y-2.5">
              <div class="flex items-center gap-2">
                <SkeletonBlock class="h-5 w-16 rounded-full" />
                <SkeletonBlock class="h-5 w-14 rounded-full" />
              </div>
              <SkeletonBlock class="h-5 w-[58%] rounded-lg" />
              <SkeletonBlock class="h-3 w-[72%] rounded-md" />
              <SkeletonBlock class="h-6 w-24 rounded-full" />
            </div>

            <div class="min-w-0 flex-[0.92] space-y-3">
              <div class="flex items-center justify-between gap-3">
                <SkeletonBlock class="h-7 w-16 rounded-xl" />
                <SkeletonBlock class="h-3 w-20 rounded-md" />
              </div>
              <SkeletonBlock class="h-2.5 w-full rounded-full" />
              <SkeletonBlock class="h-3 w-[64%] rounded-md" />
            </div>

            <div class="flex min-w-[132px] flex-col gap-2">
              <SkeletonBlock class="h-[56px] w-full rounded-[18px]" />
              <SkeletonBlock class="h-[56px] w-full rounded-[18px]" />
            </div>

            <div
              class="flex min-w-[108px] flex-col items-end justify-between self-stretch"
            >
              <div class="flex items-center gap-2">
                <SkeletonBlock class="h-10 w-10 rounded-2xl" />
                <SkeletonBlock class="h-10 w-10 rounded-2xl" />
              </div>
              <SkeletonBlock class="h-[60px] w-full rounded-[18px]" />
            </div>
          </div>
        </template>
      </SkeletonOverlay>
    </div>
  </div>
</template>
