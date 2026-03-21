<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  AlertTriangle,
  CheckCircle,
  KeyRound,
  Power,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Wrench,
  XCircle,
} from 'lucide-vue-next'
import IToggle from './ios/IToggle.vue'
import { APIInfo } from '../api/wails'
import { confirmDialog, showToast } from '../utils/toast'
import { useSettingsStore } from '../stores/useSettingsStore'
import { useMitmStatusStore } from '../stores/useMitmStatusStore'

const settingsStore = useSettingsStore()
const mitmStore = useMitmStatusStore()

const status = computed(() => mitmStore.status)
const loading = ref(false)
const error = ref('')

const poolCount = computed(() => status.value?.pool_status?.length ?? 0)
const totalReqs = computed(() => status.value?.total_requests ?? 0)
const healthyKeys = computed(() => status.value?.pool_status?.filter((item) => item.healthy).length ?? 0)
const activeKey = computed(() => status.value?.pool_status?.find((item) => item.is_current) ?? null)

const mitmOnly = computed(() => settingsStore.settings?.mitm_only === true)
const mitmTunMode = computed(() => settingsStore.settings?.mitm_tun_mode === true)

const emptyPoolHint = computed(() =>
  mitmOnly.value
    ? '号池为空 — 请在侧栏「账号池」导入带 sk-ws- API Key（仅 MITM 依赖号池轮换）'
    : '号池为空 — 请先在侧栏「账号池」导入带 API Key 的账号',
)

const statusTone = computed(() =>
  status.value?.running
    ? {
        chip: 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300',
        panel: 'border-emerald-500/15 bg-emerald-500/[0.06]',
        dot: 'bg-emerald-400',
        label: '代理运行中',
        detail: activeKey.value?.key_short ? `当前活跃 ${activeKey.value.key_short}` : '流量已接入本机 MITM',
      }
    : {
        chip: 'bg-slate-500/12 text-slate-700 dark:text-slate-300',
        panel: 'border-black/[0.06] bg-black/[0.03] dark:border-white/[0.08] dark:bg-white/[0.04]',
        dot: 'bg-slate-400 dark:bg-slate-500',
        label: '代理未启动',
        detail: '启动后会按号池顺序轮换 JWT / API Key，请先确认 CA 与 Hosts。',
      },
)

const setupCards = computed(() => [
  {
    key: 'ca',
    title: 'CA 证书',
    subtitle: status.value?.ca_installed ? '系统已信任' : '点击安装到系统信任库',
    ready: status.value?.ca_installed === true,
    onClick: handleSetupCA,
  },
  {
    key: 'hosts',
    title: 'Hosts 劫持',
    subtitle: status.value?.hosts_mapped ? '域名已指向本机 MITM' : '点击写入 hosts 映射',
    ready: status.value?.hosts_mapped === true,
    onClick: handleSetupHosts,
  },
])

const fetchStatus = () => mitmStore.fetchStatus()

const handleToggle = async (on: boolean) => {
  loading.value = true
  error.value = ''
  try {
    if (on) {
      await APIInfo.startMitmProxy()
    } else {
      await APIInfo.stopMitmProxy()
    }
    await fetchStatus()
  } catch (e: any) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

const handleSetupCA = async () => {
  loading.value = true
  error.value = ''
  try {
    await APIInfo.setupMitmCA()
    await fetchStatus()
    showToast('CA 证书已生成并安装到系统信任库', 'success')
  } catch (e: any) {
    error.value = `CA 安装失败: ${String(e)}`
  } finally {
    loading.value = false
  }
}

const handleSetupHosts = async () => {
  loading.value = true
  error.value = ''
  try {
    await APIInfo.setupMitmHosts()
    await fetchStatus()
    showToast('Hosts 已配置', 'success')
  } catch (e: any) {
    error.value = `Hosts 配置失败(需要管理员权限): ${String(e)}`
  } finally {
    loading.value = false
  }
}

const handleTeardown = async () => {
  const ok = await confirmDialog('确认卸载？将停止代理、移除 hosts 和 CA 证书', {
    confirmText: '卸载',
    cancelText: '取消',
    destructive: true,
  })
  if (!ok) return
  loading.value = true
  error.value = ''
  try {
    await APIInfo.teardownMitm()
    await fetchStatus()
    showToast('已卸载完成', 'success')
  } catch (e: any) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="ios-glass rounded-[28px] border border-black/[0.05] dark:border-white/[0.06] overflow-hidden shadow-[0_20px_48px_-20px_rgba(15,23,42,0.28)]">
    <div class="border-b border-black/[0.05] dark:border-white/[0.06] bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.14),transparent_35%),linear-gradient(180deg,rgba(255,255,255,0.82),rgba(255,255,255,0.68))] px-6 py-5 dark:bg-[radial-gradient(circle_at_top_left,rgba(96,165,250,0.18),transparent_35%),linear-gradient(180deg,rgba(28,28,30,0.94),rgba(28,28,30,0.84))]">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div class="flex min-w-0 items-start gap-3">
          <div
            class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl shadow-inner"
            :class="status?.running ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300' : 'bg-ios-blue/10 text-ios-blue'"
          >
            <component :is="status?.running ? ShieldCheck : Shield" class="h-5 w-5" stroke-width="2.4" />
          </div>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-[17px] font-bold text-ios-text dark:text-ios-textDark">MITM 无感换号代理</h2>
              <span class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide" :class="statusTone.chip">
                {{ statusTone.label }}
              </span>
            </div>
            <p class="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
              推荐主路径：流量经本机 MITM 轮换 JWT，无需改 <code class="rounded bg-black/5 px-1 dark:bg-white/10">windsurf_auth</code>，通常也无需重启 IDE。
            </p>
          </div>
        </div>

        <div class="grid grid-cols-3 gap-2 text-right">
          <div class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]">
            <div class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark">号池</div>
            <div class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark">{{ poolCount }}</div>
          </div>
          <div class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]">
            <div class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark">健康</div>
            <div class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark">{{ healthyKeys }}</div>
          </div>
          <div class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]">
            <div class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark">请求</div>
            <div class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark">{{ totalReqs }}</div>
          </div>
        </div>
      </div>
    </div>

    <div class="space-y-5 p-6">
      <div
        class="flex flex-col gap-4 rounded-[22px] border px-4 py-4 shadow-sm sm:flex-row sm:items-center sm:justify-between"
        :class="statusTone.panel"
      >
        <div class="min-w-0">
          <div class="flex items-center gap-2 text-[13px] font-bold text-ios-text dark:text-ios-textDark">
            <span class="h-2.5 w-2.5 rounded-full shadow-[0_0_10px_rgba(52,211,153,0.45)]" :class="statusTone.dot" />
            {{ statusTone.label }}
          </div>
          <p class="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
            {{ statusTone.detail }}
          </p>
        </div>
        <IToggle
          :modelValue="!!status?.running"
          @update:modelValue="handleToggle"
          :disabled="loading"
        />
      </div>

      <div
        v-if="mitmTunMode"
        class="rounded-[18px] border border-ios-blue/20 bg-ios-blue/[0.06] dark:bg-ios-blue/[0.12] px-4 py-3 text-[13px] text-ios-text dark:text-ios-textDark leading-relaxed space-y-1.5"
      >
        <p class="font-semibold">与 TUN / 全局代理并存</p>
        <p class="text-ios-textSecondary dark:text-ios-textSecondaryDark">
          若系统已开 Clash / sing-box 等 TUN，请保证
          <code class="font-mono text-[12px] px-1 rounded bg-black/5 dark:bg-white/10">server.self-serve.windsurf.com</code>
          等域名与下方 Hosts 一致（指向本机 MITM），或在代理规则中对该域名走直连/本机，避免流量绕过本应用 MITM。
        </p>
      </div>

      <div class="space-y-3">
        <div class="flex items-center gap-2">
          <div class="flex h-8 w-8 items-center justify-center rounded-xl bg-black/[0.04] text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark">
            <Wrench class="h-4 w-4" stroke-width="2.4" />
          </div>
          <div>
            <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">前置条件</div>
            <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">证书与 hosts 这两步完成后，MITM 路径才会真正接管流量。</div>
          </div>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <button
            v-for="item in setupCards"
            :key="item.key"
            type="button"
            class="no-drag-region flex items-center justify-between rounded-[18px] border px-4 py-3 text-left shadow-sm transition-all ios-btn hover:-translate-y-0.5"
            :class="item.ready ? 'border-emerald-500/15 bg-emerald-500/[0.06]' : 'border-amber-500/15 bg-amber-500/[0.06]'"
            :disabled="loading"
            @click="item.onClick"
          >
            <div class="flex min-w-0 items-center gap-3">
              <div
                class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl"
                :class="item.ready ? 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-300' : 'bg-amber-500/12 text-amber-600 dark:text-amber-300'"
              >
                <component :is="item.ready ? CheckCircle : AlertTriangle" class="h-4.5 w-4.5" stroke-width="2.4" />
              </div>
              <div class="min-w-0">
                <div class="truncate text-[13px] font-bold text-ios-text dark:text-ios-textDark">{{ item.title }}</div>
                <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">{{ item.subtitle }}</div>
              </div>
            </div>
            <span class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide" :class="item.ready ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'bg-amber-500/10 text-amber-700 dark:text-amber-300'">
              {{ item.ready ? 'READY' : 'SETUP' }}
            </span>
          </button>
        </div>
      </div>

      <div v-if="status?.pool_status?.length" class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <div class="flex h-8 w-8 items-center justify-center rounded-xl bg-ios-blue/10 text-ios-blue">
              <KeyRound class="h-4 w-4" stroke-width="2.4" />
            </div>
            <div>
              <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">号池活跃状态</div>
              <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">当前活跃 key 会优先标记，便于确认轮换是否生效。</div>
            </div>
          </div>
          <span class="rounded-full bg-black/[0.04] px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark">
            {{ poolCount }} keys
          </span>
        </div>

        <div class="space-y-2 max-h-56 overflow-y-auto pr-1">
          <div
            v-for="k in status!.pool_status"
            :key="k.key_short"
            class="flex items-center justify-between gap-3 rounded-[16px] border px-3 py-2.5 text-[12px] font-mono transition-all"
            :class="{
              'border-emerald-500/15 bg-emerald-500/[0.07]': k.is_current && k.healthy,
              'border-rose-500/15 bg-rose-500/[0.06]': !k.healthy,
              'border-black/[0.05] bg-black/[0.03] dark:border-white/[0.06] dark:bg-white/[0.03]': !k.is_current && k.healthy,
            }"
          >
            <div class="flex min-w-0 items-center gap-2.5">
              <span
                class="h-2 w-2 rounded-full shrink-0"
                :class="{
                  'bg-emerald-500': k.healthy && k.has_jwt,
                  'bg-amber-500': k.healthy && !k.has_jwt,
                  'bg-rose-500': !k.healthy,
                }"
              />
              <span class="truncate text-ios-text dark:text-ios-textDark">{{ k.key_short }}</span>
              <span v-if="k.is_current" class="rounded-full bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-emerald-700 dark:text-emerald-300">ACTIVE</span>
            </div>
            <div class="flex items-center gap-3 shrink-0 text-ios-textSecondary dark:text-ios-textSecondaryDark">
              <span>{{ k.success_count }}/{{ k.request_count }}</span>
              <span v-if="k.total_exhausted > 0" class="text-rose-500">⟲{{ k.total_exhausted }}</span>
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
          <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-black/[0.04] dark:bg-white/[0.06]">
            <Sparkles class="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark" stroke-width="2.4" />
          </div>
          <div>
            <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">号池待补全</div>
            <div class="mt-1 leading-relaxed">{{ emptyPoolHint }}</div>
          </div>
        </div>
      </div>

      <div v-if="error" class="rounded-[18px] border border-rose-500/15 bg-rose-500/[0.06] p-3 text-[12px] text-rose-700 dark:text-rose-300">
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
  </div>
</template>
