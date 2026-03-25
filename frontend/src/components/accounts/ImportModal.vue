<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import ISegmented from '../ios/ISegmented.vue'
import { APIInfo } from '../../api/wails'
import { useAccountStore } from '../../stores/useAccountStore'
import {
  AlertCircle,
  CheckCircle2,
  KeyRound,
  Loader2,
  Mail,
  RefreshCcw,
  Shield,
  Sparkles,
  X,
} from 'lucide-vue-next'
import { toAPIKeyItems, toEmailPasswordItems, toJWTItems, toTokenItems } from '../../utils/importParse'
import { importBatched } from '../../utils/importBatch'
import { showToast } from '../../utils/toast'
import { main } from '../../../wailsjs/go/models'

const props = defineProps<{ isOpen: boolean }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const accountStore = useAccountStore()

const modes = [
  { label: '邮箱/密码', value: 'password' },
  { label: 'Refresh Token', value: 'refresh_token' },
  { label: 'API Key', value: 'api_key' },
  { label: 'JWT', value: 'jwt' },
]

const currentMode = ref('password')
const inputText = ref('')
const isLoading = ref(false)
const results = ref<main.ImportResult[]>([])

watch(() => props.isOpen, (open: boolean) => {
  if (!open) {
    results.value = []
  }
})

const modeMetaMap = {
  password: {
    title: '邮箱 / 密码',
    subtitle: '适合批量登录拉取 Refresh Token 与 API Key，导入最完整。',
    icon: Mail,
    tint: 'text-ios-blue',
    chip: 'bg-ios-blue/10 text-ios-blue',
  },
  refresh_token: {
    title: 'Refresh Token',
    subtitle: '优先用于已有 Firebase 会话的批量恢复，成功率更稳。',
    icon: RefreshCcw,
    tint: 'text-emerald-600 dark:text-emerald-400',
    chip: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
  },
  api_key: {
    title: 'API Key',
    subtitle: '适合 MITM 号池接入，会自动换取 JWT 并写入账号信息。',
    icon: KeyRound,
    tint: 'text-violet-600 dark:text-violet-300',
    chip: 'bg-violet-500/10 text-violet-700 dark:text-violet-300',
  },
  jwt: {
    title: 'JWT',
    subtitle: '适合先批量拿票据再导入，速度快，对大批量更友好。',
    icon: Shield,
    tint: 'text-amber-600 dark:text-amber-300',
    chip: 'bg-amber-500/10 text-amber-700 dark:text-amber-300',
  },
} as const

const currentModeMeta = computed(() => modeMetaMap[currentMode.value as keyof typeof modeMetaMap] ?? modeMetaMap.password)

const lineCount = computed(() => inputText.value.split('\n').map((l) => l.trim()).filter(Boolean).length)
const successCount = computed(() => results.value.filter((r) => r.success).length)
const failureCount = computed(() => results.value.filter((r) => !r.success).length)

const handleImport = async () => {
  const lines = inputText.value.split('\n').map((l) => l.trim()).filter(Boolean)
  if (!lines.length) {
    return
  }
  isLoading.value = true
  results.value = []
  try {
    let batch: main.ImportResult[] = []
    switch (currentMode.value) {
      case 'api_key':
        batch = await importBatched(
          toAPIKeyItems(lines),
          (slice) => APIInfo.importByAPIKey(slice),
          (acc) => {
            results.value = acc
          },
        )
        break
      case 'jwt':
        batch = await importBatched(
          toJWTItems(lines),
          (slice) => APIInfo.importByJWT(slice),
          (acc) => {
            results.value = acc
          },
        )
        break
      case 'refresh_token':
        batch = await importBatched(
          toTokenItems(lines),
          (slice) => APIInfo.importByRefreshToken(slice),
          (acc) => {
            results.value = acc
          },
        )
        break
      case 'password': {
        const items = toEmailPasswordItems(lines)
        if (!items.length) {
          showToast(
            '未解析到有效行。支持：JSON；账号:/邮箱:/卡号: + 密码:；---- 分隔；tab/逗号分隔；「邮箱 密码」；含「密码不对就这个」会尝试第二个密码；续行「密码:」可自动合并。',
            'info',
            8000,
          )
          isLoading.value = false
          return
        }
        batch = await importBatched(
          items,
          (slice) => APIInfo.importByEmailPassword(slice),
          (acc) => {
            results.value = acc
          },
        )
        break
      }
      default:
        break
    }
    results.value = batch || []
    await accountStore.fetchAccounts()
    const ok = (batch || []).filter((r) => r.success).length
    const total = (batch || []).length
    if (total > 0) {
      showToast(`导入结束：成功 ${ok} / ${total} 条`, ok === total ? 'success' : 'info', 5000)
    }
    inputText.value = ''
  } catch (e) {
    console.error(e)
    showToast(`导入失败: ${String(e)}`, 'error')
  } finally {
    isLoading.value = false
  }
}
</script>

<template>
  <div
    v-if="isOpen"
    class="fixed inset-0 z-[100] flex animate-in fade-in duration-300 items-end sm:items-center justify-center bg-black/40 dark:bg-black/60 backdrop-blur-md"
  >
    <div
      class="bg-ios-bg dark:bg-ios-bgDark w-full sm:w-[540px] h-[90vh] sm:h-auto sm:max-h-[85vh] rounded-t-3xl sm:rounded-[28px] shadow-[0_20px_60px_-10px_rgba(0,0,0,0.3)] dark:shadow-[0_20px_60px_-10px_rgba(0,0,0,0.8)] ring-1 ring-white/50 dark:ring-white/10 flex flex-col transform transition-transform animate-in slide-in-from-bottom-12 duration-[400ms] ease-[cubic-bezier(0.16,1,0.3,1)] overflow-hidden"
    >
      <div
        class="px-5 py-4 border-b border-black/[0.06] dark:border-white/[0.06] bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.14),transparent_38%),linear-gradient(180deg,rgba(255,255,255,0.84),rgba(255,255,255,0.72))] dark:bg-[radial-gradient(circle_at_top_left,rgba(96,165,250,0.16),transparent_38%),linear-gradient(180deg,rgba(28,28,30,0.92),rgba(28,28,30,0.82))] backdrop-blur-xl flex justify-between items-start shrink-0"
      >
        <div class="flex min-w-0 items-start gap-3">
          <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-white/90 text-ios-blue shadow-[0_10px_24px_rgba(37,99,235,0.16)] dark:bg-white/10">
            <Sparkles class="h-5 w-5" stroke-width="2.4" />
          </div>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="font-bold text-[17px] tracking-tight text-ios-text dark:text-ios-textDark">批量导入</h3>
              <span class="rounded-full bg-black/[0.05] px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-ios-textSecondary dark:bg-white/[0.08] dark:text-ios-textSecondaryDark">
                {{ currentModeMeta.title }}
              </span>
            </div>
            <p class="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
              {{ currentModeMeta.subtitle }}
            </p>
          </div>
        </div>
        <button
          type="button"
          class="no-drag-region p-1.5 rounded-full bg-black/5 dark:bg-white/10 hover:bg-black/10 dark:hover:bg-white/20 transition-all ios-btn"
          @click="emit('close')"
        >
          <X class="w-5 h-5 text-ios-textSecondary dark:text-ios-textSecondaryDark" stroke-width="2.5" />
        </button>
      </div>

      <div class="p-5 flex-1 overflow-y-auto">
        <ISegmented v-model="currentMode" :options="modes" class="mb-5 h-8 flex-shrink-0" />

        <div class="mb-5 grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div class="rounded-[18px] border border-black/[0.05] bg-white/80 px-4 py-3 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
            <div class="text-[11px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark">待导入</div>
            <div class="mt-2 flex items-end gap-2">
              <span class="text-[24px] font-extrabold leading-none text-ios-text dark:text-ios-textDark">{{ lineCount }}</span>
              <span class="pb-0.5 text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark">条</span>
            </div>
          </div>
          <div class="rounded-[18px] border border-emerald-500/15 bg-emerald-500/[0.06] px-4 py-3 shadow-sm">
            <div class="text-[11px] font-bold uppercase tracking-[0.18em] text-emerald-700/75 dark:text-emerald-300/80">成功</div>
            <div class="mt-2 flex items-end gap-2">
              <span class="text-[24px] font-extrabold leading-none text-emerald-700 dark:text-emerald-300">{{ successCount }}</span>
              <span class="pb-0.5 text-[11px] font-medium text-emerald-700/70 dark:text-emerald-300/80">条</span>
            </div>
          </div>
          <div class="rounded-[18px] border border-rose-500/15 bg-rose-500/[0.05] px-4 py-3 shadow-sm">
            <div class="text-[11px] font-bold uppercase tracking-[0.18em] text-rose-700/75 dark:text-rose-300/80">失败</div>
            <div class="mt-2 flex items-end gap-2">
              <span class="text-[24px] font-extrabold leading-none text-rose-700 dark:text-rose-300">{{ failureCount }}</span>
              <span class="pb-0.5 text-[11px] font-medium text-rose-700/70 dark:text-rose-300/80">条</span>
            </div>
          </div>
        </div>

        <div class="mb-4 rounded-[20px] border border-black/[0.05] bg-white/75 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
          <div class="mb-2 flex items-center gap-2">
            <div class="flex h-8 w-8 items-center justify-center rounded-xl bg-black/[0.04] dark:bg-white/[0.06]">
              <component :is="currentModeMeta.icon" class="h-4 w-4" :class="currentModeMeta.tint" stroke-width="2.4" />
            </div>
            <div>
              <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">{{ currentModeMeta.title }}</div>
              <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">每行一条；凭证与备注用空格分隔（首列为凭证）</div>
            </div>
          </div>
          <div class="text-xs text-ios-textSecondary dark:text-ios-textSecondaryDark leading-relaxed space-y-1">
          <p v-if="currentMode === 'password'">
            支持多种粘贴格式（账号:/邮箱:、----、tab、引号逗号等）；同一邮箱多行只保留最后一条；主密码失败会自动试
            <code class="px-1 rounded bg-black/5 dark:bg-white/10">alt_password</code>
            或行内「密码不对就这个」后的第二个密码。大批量易卡顿可先改用下方 JWT 模式。
          </p>
          <p v-if="currentMode === 'jwt'">
            推荐与
            <code class="px-1 rounded bg-black/5 dark:bg-white/10">_quick_key.py</code>
            同链路：
            <code class="px-1 rounded bg-black/5 dark:bg-white/10">python tools/batch_quick_jwt.py</code>
            批量得到 Windsurf
            <code class="px-1 rounded bg-black/5 dark:bg-white/10">GetUserJwt</code>
            票据。若仅需 Firebase
            <code class="px-1 rounded bg-black/5 dark:bg-white/10">idToken</code>
            ，可用
            <code class="px-1 rounded bg-black/5 dark:bg-white/10">tools/email-password-to-firebase-jwt.mjs</code>
            。
          </p>
          <p v-if="currentMode === 'api_key' || currentMode === 'refresh_token'">
            大批量导入时会自动分批处理，过程中结果区会实时刷新。
          </p>
          </div>
        </div>

        <div class="rounded-[22px] border border-black/[0.06] bg-white/75 p-4 shadow-[0_14px_32px_rgba(15,23,42,0.06)] dark:border-white/[0.06] dark:bg-black/20">
          <div class="mb-3 flex items-center justify-between gap-3">
            <div>
              <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">批量输入</div>
              <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">支持粘贴多行，导入过程会逐批回填结果。</div>
            </div>
            <span class="rounded-full px-2.5 py-1 text-[10px] font-bold tracking-wide" :class="currentModeMeta.chip">
              {{ currentModeMeta.title }}
            </span>
          </div>

          <textarea
            v-model="inputText"
            class="no-drag-region w-full h-[190px] bg-[linear-gradient(180deg,rgba(255,255,255,0.95),rgba(246,249,252,0.9))] dark:bg-[linear-gradient(180deg,rgba(10,10,12,0.75),rgba(18,18,20,0.88))] border border-black/10 dark:border-white/10 p-4 rounded-[18px] focus:outline-none focus:ring-2 focus:ring-ios-blue/50 dark:focus:ring-ios-blue/30 resize-none font-mono text-[13px] shadow-inner transition-all"
            placeholder="粘贴多行内容..."
          />
        </div>

        <div v-if="results.length" class="mt-5 space-y-3 max-h-48 overflow-y-auto pr-1">
          <div class="flex items-center justify-between">
            <h4 class="text-xs font-semibold uppercase tracking-wider text-ios-textSecondary dark:text-ios-textSecondaryDark">
              导入结果
            </h4>
            <span class="text-[11px] font-medium text-ios-textSecondary dark:text-ios-textSecondaryDark">
              已处理 {{ results.length }} 条
            </span>
          </div>
          <div
            v-for="(r, i) in results"
            :key="i"
            class="text-xs p-3 rounded-[18px] flex items-center justify-between shadow-sm border backdrop-blur-sm"
            :class="
              r.success
                ? 'bg-emerald-500/[0.08] border-emerald-500/15 text-emerald-700 dark:text-emerald-300'
                : 'bg-rose-500/[0.07] border-rose-500/15 text-rose-700 dark:text-rose-300'
            "
          >
            <span class="font-semibold truncate max-w-[260px] mr-2" :title="r.email">{{ r.email }}</span>
            <div class="flex items-center shrink-0 font-medium">
              <CheckCircle2 v-if="r.success" class="w-4 h-4 mr-1" />
              <AlertCircle v-else class="w-4 h-4 mr-1" />
              {{ r.success ? '成功' : r.error || '失败' }}
            </div>
          </div>
        </div>
      </div>

      <div
        class="p-5 border-t border-black/[0.06] dark:border-white/[0.06] bg-white/70 dark:bg-[#1C1C1E]/70 backdrop-blur-xl shrink-0"
      >
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0">
            <div class="text-[12px] font-semibold text-ios-text dark:text-ios-textDark">
              {{ lineCount > 0 ? `准备导入 ${lineCount} 条` : '等待粘贴内容' }}
            </div>
            <div class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark">
              {{ isLoading ? '正在分批提交并同步账号池…' : '导入完成后会自动刷新账号池列表' }}
            </div>
          </div>
          <button
            type="button"
            class="no-drag-region h-[48px] min-w-[144px] px-5 bg-gradient-to-b from-[#3b82f6] to-ios-blue text-white rounded-[16px] font-semibold text-[16px] ios-btn flex items-center justify-center disabled:opacity-50 shadow-md shadow-ios-blue/20 ring-1 ring-black/5 ring-inset active:ring-black/10"
            :disabled="isLoading || !inputText.trim()"
            @click="handleImport"
          >
            <Loader2 v-if="isLoading" class="w-5 h-5 ios-spinner mr-2" />
            {{ isLoading ? '导入中…' : '开始导入' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
