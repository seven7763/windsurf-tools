<script setup lang="ts">
import { ref, computed } from "vue";
import {
  AlertTriangle,
  ArrowRightLeft,
  CheckCircle,
  KeyRound,
  Link2,
  Power,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Wrench,
  XCircle,
} from "lucide-vue-next";
import SessionPanel from "./SessionPanel.vue";
import IToggle from "./ios/IToggle.vue";
import SkeletonOverlay from "./common/SkeletonOverlay.vue";
import MitmPanelSkeleton from "./common/MitmPanelSkeleton.vue";
import { APIInfo } from "../api/wails";
import { confirmDialog, showToast } from "../utils/toast";
import { useSettingsStore } from "../stores/useSettingsStore";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { formatDateTimeAsiaShanghai } from "../utils/datetimeAsia";

const settingsStore = useSettingsStore();
const mitmStore = useMitmStatusStore();

const status = computed(() => mitmStore.status);
const loading = ref(false);
const error = ref("");

const poolCount = computed(() => status.value?.pool_status?.length ?? 0);
const totalReqs = computed(() => status.value?.total_requests ?? 0);
const healthyKeys = computed(
  () => status.value?.pool_status?.filter((item) => item.healthy).length ?? 0,
);
const runtimeExhaustedKeys = computed(
  () =>
    status.value?.pool_status?.filter((item) => item.runtime_exhausted)
      .length ?? 0,
);
const activeKey = computed(
  () => status.value?.pool_status?.find((item) => item.is_current) ?? null,
);
const panelBusy = computed(() => loading.value || mitmStore.switchLoading);

const mitmOnly = computed(() => settingsStore.settings?.mitm_only === true);
const mitmTunMode = computed(
  () => settingsStore.settings?.mitm_tun_mode === true,
);
type RecentMitmEvent = {
  at?: string;
  message?: string;
  tone?: string;
};
const recentEvents = computed<RecentMitmEvent[]>(() => {
  const raw = (
    status.value as unknown as { recent_events?: RecentMitmEvent[] } | null
  )?.recent_events;
  return Array.isArray(raw) ? raw.slice(0, 8) : [];
});
const lastProxyIssue = computed(() => {
  const rawKind = String(status.value?.last_error_kind || "").trim();
  const summary = String(status.value?.last_error_summary || "").trim();
  if (!rawKind && !summary) {
    return null;
  }
  const key = String(status.value?.last_error_key || "").trim();
  const at = String(status.value?.last_error_at || "").trim();
  const labelMap: Record<string, string> = {
    quota: "额度错误",
    internal: "上游内部错误",
    permission: "权限错误",
    grpc: "gRPC 错误",
  };
  return {
    kind: rawKind || "unknown",
    label: labelMap[rawKind] || "未知错误",
    summary: summary || "未提供更多细节",
    key,
    at,
  };
});

const lastProxyIssueTone = computed(() => {
  switch (lastProxyIssue.value?.kind) {
    case "quota":
      return "border-amber-500/20 bg-amber-500/[0.07] text-amber-700 dark:text-amber-300";
    case "permission":
      return "border-rose-500/20 bg-rose-500/[0.07] text-rose-700 dark:text-rose-300";
    case "internal":
      return "border-orange-500/20 bg-orange-500/[0.07] text-orange-700 dark:text-orange-300";
    default:
      return "border-slate-500/20 bg-slate-500/[0.07] text-slate-700 dark:text-slate-300";
  }
});

const recentEventToneClass = (tone?: string) => {
  switch (tone) {
    case "success":
      return "border-emerald-500/15 bg-emerald-500/[0.06] text-emerald-700 dark:text-emerald-300";
    case "warning":
      return "border-amber-500/15 bg-amber-500/[0.06] text-amber-700 dark:text-amber-300";
    case "danger":
      return "border-rose-500/15 bg-rose-500/[0.06] text-rose-700 dark:text-rose-300";
    default:
      return "border-black/[0.05] bg-black/[0.03] text-slate-700 dark:border-white/[0.06] dark:bg-white/[0.03] dark:text-slate-300";
  }
};

const emptyPoolHint = computed(() =>
  mitmOnly.value
    ? "号池为空 — 请在侧栏「号池 (MITM)」导入带 sk-ws- API Key 的账号，纯 MITM 轮换完全依赖这里的号池"
    : "号池为空 — 请先在侧栏「号池 (MITM)」导入带 API Key 的账号",
);

const statusTone = computed(() =>
  status.value?.running
    ? {
        chip: "bg-emerald-500/12 text-emerald-700 dark:text-emerald-300",
        panel: "border-emerald-500/15 bg-emerald-500/[0.06]",
        dot: "bg-emerald-400",
        label: "代理运行中",
        detail: activeKey.value?.key_short
          ? `当前活跃 ${activeKey.value.key_short}`
          : "流量已接入本机 MITM",
      }
    : {
        chip: "bg-slate-500/12 text-slate-700 dark:text-slate-300",
        panel:
          "border-black/[0.06] bg-black/[0.03] dark:border-white/[0.08] dark:bg-white/[0.04]",
        dot: "bg-slate-400 dark:bg-slate-500",
        label: "代理未启动",
        detail: "启动后会按号池顺序轮换 JWT / API Key，请先确认 CA 与 Hosts。",
      },
);

const setupCards = computed(() => [
  {
    key: "ca",
    title: "CA 证书",
    subtitle: status.value?.ca_installed
      ? "系统已信任"
      : "点击安装到系统信任库",
    ready: status.value?.ca_installed === true,
    onClick: handleSetupCA,
  },
  {
    key: "hosts",
    title: "Hosts 劫持",
    subtitle: status.value?.hosts_mapped
      ? "域名已指向本机 MITM"
      : "点击写入 hosts 映射",
    ready: status.value?.hosts_mapped === true,
    onClick: handleSetupHosts,
  },
]);

const fetchStatus = (force = false) => mitmStore.fetchStatus(force);

const handleToggle = async (on: boolean) => {
  loading.value = true;
  error.value = "";
  try {
    if (on) {
      await APIInfo.startMitmProxy();
    } else {
      await APIInfo.stopMitmProxy();
    }
    await fetchStatus(true);
  } catch (e: any) {
    error.value = String(e);
  } finally {
    loading.value = false;
  }
};

const handleSwitchToNext = async () => {
  error.value = "";
  try {
    const target = await mitmStore.switchToNext();
    showToast(`MITM 已手动切到下一席位：${target || "已切换"}`, "success");
  } catch (e: any) {
    error.value = `手动切换失败: ${String(e)}`;
  }
};

const handleSetupCA = async () => {
  loading.value = true;
  error.value = "";
  try {
    await APIInfo.setupMitmCA();
    await fetchStatus(true);
    showToast("CA 证书已生成并安装到系统信任库", "success");
  } catch (e: any) {
    error.value = `CA 安装失败: ${String(e)}`;
  } finally {
    loading.value = false;
  }
};

const handleSetupHosts = async () => {
  loading.value = true;
  error.value = "";
  try {
    await APIInfo.setupMitmHosts();
    await fetchStatus(true);
    showToast("Hosts 已配置", "success");
  } catch (e: any) {
    error.value = `Hosts 配置失败（Linux 会尝试 pkexec/sudo 提权）: ${String(e)}`;
  } finally {
    loading.value = false;
  }
};

const handleTeardown = async () => {
  const ok = await confirmDialog(
    "确认卸载？将停止代理、移除 hosts 和 CA 证书",
    {
      confirmText: "卸载",
      cancelText: "取消",
      destructive: true,
    },
  );
  if (!ok) return;
  loading.value = true;
  error.value = "";
  try {
    await APIInfo.teardownMitm();
    await fetchStatus(true);
    showToast("已卸载完成", "success");
  } catch (e: any) {
    error.value = String(e);
  } finally {
    loading.value = false;
  }
};
</script>

<template>
  <div
    class="ios-glass rounded-[28px] border border-black/[0.05] dark:border-white/[0.06] overflow-hidden shadow-[0_20px_48px_-20px_rgba(15,23,42,0.28)]"
  >
    <div
      class="border-b border-black/[0.05] dark:border-white/[0.06] bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.14),transparent_35%),linear-gradient(180deg,rgba(255,255,255,0.82),rgba(255,255,255,0.68))] px-6 py-5 dark:bg-[radial-gradient(circle_at_top_left,rgba(96,165,250,0.18),transparent_35%),linear-gradient(180deg,rgba(28,28,30,0.94),rgba(28,28,30,0.84))]"
    >
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div class="flex min-w-0 items-start gap-3">
          <div
            class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl shadow-inner"
            :class="
              status?.running
                ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300'
                : 'bg-ios-blue/10 text-ios-blue'
            "
          >
            <component
              :is="status?.running ? ShieldCheck : Shield"
              class="h-5 w-5"
              stroke-width="2.4"
            />
          </div>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2
                class="text-[17px] font-bold text-ios-text dark:text-ios-textDark"
              >
                MITM 无感换号代理
              </h2>
              <span
                class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                :class="statusTone.chip"
              >
                {{ statusTone.label }}
              </span>
            </div>
            <p
              class="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              当前产品默认走纯 MITM：流量经本机代理轮换 JWT / API Key，号池与
              Relay 会直接复用这套轮换状态。
            </p>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-2 text-right sm:grid-cols-5">
          <div
            class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]"
          >
            <div
              class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              号池
            </div>
            <div
              class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark"
            >
              {{ poolCount }}
            </div>
          </div>
          <div
            class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]"
          >
            <div
              class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              健康
            </div>
            <div
              class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark"
            >
              {{ healthyKeys }}
            </div>
          </div>
          <div
            class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]"
          >
            <div
              class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              运行时见底
            </div>
            <div
              class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark"
            >
              {{ runtimeExhaustedKeys }}
            </div>
          </div>
          <div
            class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]"
          >
            <div
              class="text-[10px] font-bold uppercase tracking-[0.18em] text-violet-600 dark:text-violet-300"
            >
              会话
            </div>
            <div
              class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark"
            >
              {{ status?.session_count ?? 0 }}
            </div>
          </div>
          <div
            class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]"
          >
            <div
              class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              请求
            </div>
            <div
              class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark"
            >
              {{ totalReqs }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <SkeletonOverlay
      :active="panelBusy"
      label="MITM 面板处理中"
      overlayClass="rounded-b-[28px] bg-white/40 p-0 backdrop-blur-[2px] dark:bg-[#1C1C1E]/40"
    >
      <div class="space-y-5 p-6">
        <div
          class="flex flex-col gap-4 rounded-[22px] border px-4 py-4 shadow-sm sm:flex-row sm:items-center sm:justify-between"
          :class="statusTone.panel"
        >
          <div class="min-w-0">
            <div
              class="flex items-center gap-2 text-[13px] font-bold text-ios-text dark:text-ios-textDark"
            >
              <span
                class="h-2.5 w-2.5 rounded-full"
                :class="[
                  statusTone.dot,
                  status?.running
                    ? 'shadow-[0_0_10px_rgba(52,211,153,0.45)]'
                    : '',
                ]"
              />
              {{ statusTone.label }}
            </div>
            <p
              class="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              {{ statusTone.detail }}
            </p>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-2">
            <button
              type="button"
              class="no-drag-region inline-flex items-center gap-2 rounded-full border border-ios-blue/15 bg-ios-blue/[0.08] px-3.5 py-2 text-[12px] font-semibold text-ios-blue transition-colors ios-btn hover:bg-ios-blue/[0.12] disabled:opacity-50 dark:text-blue-300"
              :disabled="loading || mitmStore.switchLoading || poolCount === 0"
              @click="handleSwitchToNext"
            >
              <ArrowRightLeft
                class="h-3.5 w-3.5"
                :class="mitmStore.switchLoading ? 'animate-pulse' : ''"
                stroke-width="2.4"
              />
              下一席位
            </button>
            <IToggle
              :modelValue="!!status?.running"
              @update:modelValue="handleToggle"
              :disabled="
                loading ||
                mitmStore.switchLoading ||
                (!status?.running &&
                  (!status?.ca_installed || !status?.hosts_mapped))
              "
            />
          </div>
        </div>
        <div
          v-if="lastProxyIssue"
          class="rounded-[18px] border px-4 py-3 shadow-sm"
          :class="lastProxyIssueTone"
        >
          <div class="flex items-start gap-3">
            <div
              class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-black/[0.05] dark:bg-white/[0.08]"
            >
              <AlertTriangle class="h-4 w-4" stroke-width="2.4" />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <div class="text-[13px] font-bold">最近一次上游错误</div>
                <span
                  class="rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide dark:bg-white/[0.08]"
                >
                  {{ lastProxyIssue.label }}
                </span>
                <span
                  v-if="lastProxyIssue.key"
                  class="rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-mono dark:bg-white/[0.08]"
                >
                  {{ lastProxyIssue.key }}
                </span>
              </div>
              <div class="mt-1 text-[12px] leading-relaxed break-words">
                {{ lastProxyIssue.summary }}
              </div>
              <div v-if="lastProxyIssue.at" class="mt-2 text-[11px] opacity-80">
                {{ formatDateTimeAsiaShanghai(lastProxyIssue.at) }}
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="recentEvents.length"
          class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
        >
          <div class="mb-3 flex items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-xl bg-black/[0.04] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark"
              >
                <Sparkles class="h-4 w-4" stroke-width="2.4" />
              </div>
              <div>
                <div
                  class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
                >
                  最近代理事件
                </div>
                <div
                  class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  便于确认 JWT 预热、轮转和上游错误是否真的发生。
                </div>
              </div>
            </div>
            <span
              class="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark"
            >
              {{ recentEvents.length }} 条
            </span>
          </div>

          <div class="space-y-2 max-h-56 overflow-y-auto pr-1">
            <div
              v-for="(event, index) in recentEvents"
              :key="`${event.at || 'mitm'}-${index}`"
              class="rounded-[16px] border px-3 py-2.5"
              :class="recentEventToneClass(event.tone)"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0 flex-1">
                  <div
                    class="break-words text-[12px] font-medium leading-relaxed"
                  >
                    {{ event.message || "未提供事件详情" }}
                  </div>
                  <div
                    v-if="event.at"
                    class="mt-1 text-[10px] opacity-80"
                    :title="formatDateTimeAsiaShanghai(event.at)"
                  >
                    {{ formatDateTimeAsiaShanghai(event.at) }}
                  </div>
                </div>
                <span
                  class="shrink-0 rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide bg-black/[0.05] dark:bg-white/[0.08]"
                >
                  {{ event.tone || "info" }}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="mitmTunMode"
          class="rounded-[18px] border border-ios-blue/20 bg-ios-blue/[0.06] dark:bg-ios-blue/[0.12] px-4 py-3 text-[13px] text-ios-text dark:text-ios-textDark leading-relaxed space-y-1.5"
        >
          <p class="font-semibold">与 TUN / 全局代理并存</p>
          <p class="text-ios-textSecondary dark:text-ios-textSecondaryDark">
            若系统已开 Clash / sing-box 等 TUN，请保证
            <code
              class="font-mono text-[12px] px-1 rounded bg-black/5 dark:bg-white/10"
              >server.self-serve.windsurf.com</code
            >
            等域名与下方 Hosts 一致（指向本机
            MITM），或在代理规则中对该域名走直连/本机，避免流量绕过本应用 MITM。
          </p>
        </div>

        <div
          class="rounded-[18px] border border-ios-blue/20 bg-ios-blue/[0.06] px-4 py-3 text-[12px] leading-relaxed text-ios-text dark:text-ios-textDark"
        >
          Windows 桌面包现在默认请求管理员权限启动，便于直接管理 Hosts、安装 CA
          证书和控制后台服务，避免运行中再因为权限不足中断流程。
        </div>

        <div class="space-y-3">
          <div class="flex items-center gap-2">
            <div
              class="flex h-8 w-8 items-center justify-center rounded-xl bg-black/[0.04] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark"
            >
              <Wrench class="h-4 w-4" stroke-width="2.4" />
            </div>
            <div>
              <div
                class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
              >
                前置条件
              </div>
              <div
                class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
              >
                证书与 hosts 这两步完成后，MITM 路径才会真正接管流量。
              </div>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <button
              v-for="item in setupCards"
              :key="item.key"
              type="button"
              class="no-drag-region flex items-center justify-between rounded-[18px] border px-4 py-3 text-left shadow-sm transition-all ios-btn hover:-translate-y-0.5"
              :class="
                item.ready
                  ? 'border-emerald-500/15 bg-emerald-500/[0.06]'
                  : 'border-amber-500/15 bg-amber-500/[0.06]'
              "
              :disabled="loading"
              @click="item.onClick"
            >
              <div class="flex min-w-0 items-center gap-3">
                <div
                  class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl"
                  :class="
                    item.ready
                      ? 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-300'
                      : 'bg-amber-500/12 text-amber-600 dark:text-amber-300'
                  "
                >
                  <component
                    :is="item.ready ? CheckCircle : AlertTriangle"
                    class="h-4.5 w-4.5"
                    stroke-width="2.4"
                  />
                </div>
                <div class="min-w-0">
                  <div
                    class="truncate text-[13px] font-bold text-ios-text dark:text-ios-textDark"
                  >
                    {{ item.title }}
                  </div>
                  <div
                    class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                  >
                    {{ item.subtitle }}
                  </div>
                </div>
              </div>
              <span
                class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                :class="
                  item.ready
                    ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                    : 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
                "
              >
                {{ item.ready ? "READY" : "SETUP" }}
              </span>
            </button>
          </div>
        </div>

        <div
          v-if="status?.pool_status?.length"
          class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
        >
          <div class="mb-3 flex items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-xl bg-ios-blue/10 text-ios-blue"
              >
                <KeyRound class="h-4 w-4" stroke-width="2.4" />
              </div>
              <div>
                <div
                  class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
                >
                  号池活跃状态
                </div>
                <div
                  class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
                >
                  当前活跃 key 会优先标记，便于确认轮换是否生效。
                </div>
              </div>
            </div>
            <span
              class="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark"
            >
              {{ poolCount }} keys
            </span>
          </div>

          <div class="space-y-2 max-h-56 overflow-y-auto pr-1">
            <div
              v-for="k in status!.pool_status"
              :key="k.key_short"
              class="flex items-center justify-between gap-3 rounded-[16px] border px-3 py-2.5 text-[12px] font-mono transition-all"
              :class="{
                'border-emerald-500/15 bg-emerald-500/[0.07]':
                  k.is_current && k.healthy && !k.runtime_exhausted,
                'border-amber-500/15 bg-amber-500/[0.08]': k.runtime_exhausted,
                'border-rose-500/15 bg-rose-500/[0.06]':
                  !k.runtime_exhausted && !k.healthy,
                'border-black/[0.05] bg-black/[0.03] dark:border-white/[0.06] dark:bg-white/[0.03]':
                  !k.is_current && k.healthy,
              }"
            >
              <div class="flex min-w-0 items-center gap-2.5">
                <span
                  class="h-2 w-2 rounded-full shrink-0"
                  :class="{
                    'bg-amber-500': k.runtime_exhausted,
                    'bg-emerald-500':
                      !k.runtime_exhausted && k.healthy && k.has_jwt,
                    'bg-sky-500':
                      !k.runtime_exhausted && k.healthy && !k.has_jwt,
                    'bg-rose-500': !k.runtime_exhausted && !k.healthy,
                  }"
                />
                <span class="truncate text-ios-text dark:text-ios-textDark">{{
                  k.key_short
                }}</span>
                <span
                  v-if="k.is_current"
                  class="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-emerald-700 dark:text-emerald-300"
                  >ACTIVE</span
                >
                <span
                  v-if="k.runtime_exhausted"
                  class="rounded-full bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-amber-700 dark:text-amber-300"
                  >RUNTIME EXHAUSTED</span
                >
              </div>
              <div
                class="flex items-center gap-3 shrink-0 text-ios-textSecondary dark:text-ios-textSecondaryDark"
              >
                <span>{{ k.success_count }}/{{ k.request_count }}</span>
                <span v-if="k.total_exhausted > 0" class="text-rose-500"
                  >⟲{{ k.total_exhausted }}</span
                >
                <span
                  v-if="k.runtime_exhausted && k.cooldown_until"
                  class="rounded-full bg-black/[0.05] px-2 py-0.5 text-[10px] font-semibold dark:bg-white/[0.08]"
                  :title="formatDateTimeAsiaShanghai(k.cooldown_until)"
                >
                  冷却至 {{ formatDateTimeAsiaShanghai(k.cooldown_until) }}
                </span>
                <span
                  v-if="k.bound_session_count > 0"
                  class="flex items-center gap-1 rounded-full bg-violet-500/10 px-2 py-0.5 text-[10px] font-bold text-violet-700 dark:text-violet-300"
                  :title="`${k.bound_session_count} 个会话绑定到此 Key`"
                >
                  <Link2 class="h-2.5 w-2.5" stroke-width="2.4" />
                  {{ k.bound_session_count }}
                </span>
                <component
                  :is="k.has_jwt ? CheckCircle : XCircle"
                  class="h-3.5 w-3.5"
                  :class="k.has_jwt ? 'text-emerald-500' : 'text-gray-400'"
                  stroke-width="2.4"
                />
              </div>
            </div>
          </div>
        </div>

        <div
          v-else-if="status"
          class="rounded-[20px] border border-dashed border-black/[0.08] bg-black/[0.02] px-4 py-5 text-[13px] text-ios-textSecondary dark:border-white/[0.08] dark:bg-white/[0.03] dark:text-ios-textSecondaryDark"
        >
          <div class="flex items-start gap-3">
            <div
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-black/[0.04] dark:bg-white/[0.06]"
            >
              <Sparkles
                class="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark"
                stroke-width="2.4"
              />
            </div>
            <div>
              <div
                class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
              >
                号池待补全
              </div>
              <div class="mt-1 leading-relaxed">{{ emptyPoolHint }}</div>
            </div>
          </div>
        </div>

        <SessionPanel />

        <div
          v-if="error"
          class="rounded-[18px] border border-rose-500/15 bg-rose-500/[0.06] p-3 text-[12px] text-rose-700 dark:text-rose-300"
        >
          <div class="flex items-start gap-2">
            <ShieldAlert class="mt-0.5 h-4 w-4 shrink-0" stroke-width="2.4" />
            <span>{{ error }}</span>
          </div>
        </div>

        <button
          type="button"
          class="no-drag-region flex w-full items-center justify-center gap-2 rounded-[16px] border border-rose-500/12 bg-rose-500/[0.06] px-4 py-3 text-[12px] font-semibold text-rose-700 transition-colors ios-btn hover:bg-rose-500/[0.11] disabled:opacity-50 dark:text-rose-300"
          :disabled="loading"
          @click="handleTeardown"
        >
          <Power class="h-3.5 w-3.5" stroke-width="2.4" />
          卸载 MITM（停止代理 + 移除 Hosts / CA）
        </button>
      </div>
      <template #skeleton>
        <MitmPanelSkeleton />
      </template>
    </SkeletonOverlay>
  </div>
</template>
