<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  Copy,
  Globe,
  Play,
  Square,
  RefreshCw,
  Terminal,
  Zap,
  Shield,
  Info,
} from 'lucide-vue-next'
import IToggle from '../components/ios/IToggle.vue'
import { APIInfo } from '../api/wails'
import { showToast } from '../utils/toast'
import { useSettingsStore } from '../stores/useSettingsStore'
import { useMitmStatusStore } from '../stores/useMitmStatusStore'

const settingsStore = useSettingsStore()
const mitmStore = useMitmStatusStore()

type RelayStatusType = { running?: boolean; port?: number; url?: string }
const relayStatus = ref<RelayStatusType | null>(null)
const relayLoading = ref(false)
const testResult = ref('')
const testLoading = ref(false)

const relayPort = computed(() => settingsStore.settings?.openai_relay_port || 8787)
const relaySecret = computed(() => settingsStore.settings?.openai_relay_secret || '')
const relayURL = computed(() =>
  relayStatus.value?.running
    ? relayStatus.value.url || `http://127.0.0.1:${relayPort.value}`
    : `http://127.0.0.1:${relayPort.value}`,
)
const endpoint = computed(() => `${relayURL.value}/v1/chat/completions`)
const poolCount = computed(() => mitmStore.status?.pool_status?.length ?? 0)
const hasPool = computed(() => poolCount.value > 0)

const fetchRelayStatus = async () => {
  try {
    relayStatus.value = await APIInfo.getOpenAIRelayStatus()
  } catch (e) {
    console.error('getOpenAIRelayStatus error:', e)
  }
}

onMounted(() => {
  fetchRelayStatus()
  mitmStore.fetchStatus()
})

const handleToggle = async (on: boolean) => {
  relayLoading.value = true
  try {
    if (on) {
      await APIInfo.startOpenAIRelay(relayPort.value, relaySecret.value)
    } else {
      await APIInfo.stopOpenAIRelay()
    }
    await fetchRelayStatus()
    showToast(on ? 'Relay 已启动' : 'Relay 已停止', 'success')
  } catch (e: any) {
    showToast(`Relay ${on ? '启动' : '停止'}失败: ${String(e)}`, 'error')
  } finally {
    relayLoading.value = false
  }
}

const copyText = (text: string, label: string) => {
  navigator.clipboard.writeText(text).then(
    () => showToast(`已复制${label}`, 'success'),
    () => showToast(`复制${label}失败`, 'error'),
  )
}

const curlCmd = computed(() => {
  const secret = relaySecret.value
  const auth = secret ? ` -H "Authorization: Bearer ${secret}"` : ''
  return `curl "${endpoint.value}"${auth} -H "Content-Type: application/json" -d "{\\"model\\":\\"claude-3.5-sonnet\\",\\"stream\\":true,\\"messages\\":[{\\"role\\":\\"user\\",\\"content\\":\\"hello\\"}]}"`
})

const pythonExample = computed(() => {
  const secret = relaySecret.value
  const authLine = secret ? `    api_key="${secret}",` : `    api_key="no-key",`
  return `from openai import OpenAI

client = OpenAI(
    base_url="${relayURL.value}/v1",
${authLine}
)

stream = client.chat.completions.create(
    model="claude-3.5-sonnet",
    messages=[{"role": "user", "content": "hello"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")`
})

const handleTest = async () => {
  if (!relayStatus.value?.running) {
    showToast('请先启动 Relay', 'error')
    return
  }
  testLoading.value = true
  testResult.value = ''
  try {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    if (relaySecret.value) {
      headers['Authorization'] = `Bearer ${relaySecret.value}`
    }
    const resp = await fetch(endpoint.value, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        model: 'claude-3.5-sonnet',
        stream: false,
        messages: [{ role: 'user', content: 'Say "hello" in one word.' }],
      }),
    })
    const data = await resp.json()
    if (data.error) {
      testResult.value = `❌ 错误: ${data.error.message || JSON.stringify(data.error)}`
    } else if (data.choices?.[0]?.message?.content) {
      testResult.value = `✅ 成功: ${data.choices[0].message.content.slice(0, 200)}`
    } else {
      testResult.value = `⚠️ 未知响应: ${JSON.stringify(data).slice(0, 300)}`
    }
  } catch (e: any) {
    testResult.value = `❌ 请求失败: ${String(e)}`
  } finally {
    testLoading.value = false
  }
}
</script>

<template>
  <div class="p-6 space-y-6 max-w-4xl mx-auto">
    <!-- 头部 -->
    <div class="rounded-[28px] border border-black/[0.05] dark:border-white/[0.06] overflow-hidden shadow-[0_20px_48px_-20px_rgba(15,23,42,0.18)] ios-glass">
      <div class="border-b border-black/[0.05] dark:border-white/[0.06] bg-[radial-gradient(circle_at_top_left,rgba(139,92,246,0.14),transparent_35%),linear-gradient(180deg,rgba(255,255,255,0.82),rgba(255,255,255,0.68))] px-6 py-5 dark:bg-[radial-gradient(circle_at_top_left,rgba(167,139,250,0.18),transparent_35%),linear-gradient(180deg,rgba(28,28,30,0.94),rgba(28,28,30,0.84))]">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="flex min-w-0 items-start gap-3">
            <div
              class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl shadow-inner"
              :class="relayStatus?.running ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300' : 'bg-violet-500/10 text-violet-600 dark:text-violet-300'"
            >
              <Globe class="h-5 w-5" stroke-width="2.4" />
            </div>
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="text-[17px] font-bold text-ios-text dark:text-ios-textDark">OpenAI 兼容中转</h2>
                <span
                  class="rounded-full px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide"
                  :class="relayStatus?.running
                    ? 'bg-emerald-500/12 text-emerald-700 dark:text-emerald-300'
                    : 'bg-slate-500/12 text-slate-700 dark:text-slate-300'"
                >
                  {{ relayStatus?.running ? '运行中' : '未启动' }}
                </span>
              </div>
              <p class="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
                本地 OpenAI 兼容 API 端点，复用 MITM 号池 JWT 自动轮换。支持流式 SSE，可对接 ChatGPT-Next-Web、LobeChat、OpenAI SDK 等任意客户端。
              </p>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-2 text-right">
            <div class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]">
              <div class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark">号池</div>
              <div class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark">{{ poolCount }}</div>
            </div>
            <div class="rounded-[16px] bg-white/80 px-3 py-2 shadow-sm ring-1 ring-black/[0.04] dark:bg-white/[0.05] dark:ring-white/[0.06]">
              <div class="text-[10px] font-bold uppercase tracking-[0.18em] text-ios-textSecondary dark:text-ios-textSecondaryDark">端口</div>
              <div class="mt-1 text-[18px] font-extrabold text-ios-text dark:text-ios-textDark">{{ relayPort }}</div>
            </div>
          </div>
        </div>
      </div>

      <div class="space-y-5 p-6">
        <!-- 开关 -->
        <div
          class="flex flex-col gap-4 rounded-[22px] border px-4 py-4 shadow-sm sm:flex-row sm:items-center sm:justify-between"
          :class="relayStatus?.running
            ? 'border-emerald-500/15 bg-emerald-500/[0.06]'
            : 'border-black/[0.06] bg-black/[0.03] dark:border-white/[0.08] dark:bg-white/[0.04]'"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-2 text-[13px] font-bold text-ios-text dark:text-ios-textDark">
              <span
                class="h-2.5 w-2.5 rounded-full"
                :class="relayStatus?.running ? 'bg-emerald-400 shadow-[0_0_10px_rgba(52,211,153,0.45)]' : 'bg-slate-400 dark:bg-slate-500'"
              />
              {{ relayStatus?.running ? 'Relay 运行中' : 'Relay 未启动' }}
            </div>
            <p class="mt-1 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
              {{ relayStatus?.running ? `监听 127.0.0.1:${relayStatus.port || relayPort}` : '启动后号池中的 API Key 将自动用于对话请求' }}
            </p>
          </div>
          <IToggle
            :modelValue="!!relayStatus?.running"
            @update:modelValue="handleToggle"
            :disabled="relayLoading || !hasPool"
          />
        </div>

        <!-- 号池为空提示 -->
        <div
          v-if="!hasPool"
          class="rounded-[18px] border border-amber-500/15 bg-amber-500/[0.06] px-4 py-3"
        >
          <div class="flex items-start gap-3">
            <Info class="mt-0.5 h-4 w-4 shrink-0 text-amber-600 dark:text-amber-300" stroke-width="2.4" />
            <div class="text-[12px] leading-relaxed text-amber-700 dark:text-amber-300">
              号池为空，请先在「号池」页面通过 <strong>API Key 导入</strong> 添加 <code class="rounded bg-black/5 px-1 dark:bg-white/10">sk-ws-01-...</code> 格式的 Key。Relay 复用 MITM 号池。
            </div>
          </div>
        </div>

        <!-- Endpoint 信息 -->
        <div v-if="relayStatus?.running" class="space-y-3">
          <div class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
            <div class="flex items-center gap-2 mb-3">
              <Zap class="h-4 w-4 text-violet-500" stroke-width="2.4" />
              <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">接入信息</div>
            </div>

            <div class="space-y-2.5">
              <!-- Base URL -->
              <div class="flex items-center gap-2">
                <div class="flex-1 min-w-0 rounded-[14px] border border-black/[0.05] bg-black/[0.02] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.03]">
                  <div class="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">Base URL</div>
                  <div class="mt-0.5 truncate font-mono text-[12px] font-semibold text-ios-text dark:text-ios-textDark select-all">
                    {{ relayURL }}/v1
                  </div>
                </div>
                <button
                  type="button"
                  class="no-drag-region flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-black/[0.06] bg-white/80 text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05]"
                  title="复制 Base URL"
                  @click="copyText(`${relayURL}/v1`, 'Base URL')"
                >
                  <Copy class="h-3.5 w-3.5" stroke-width="2.4" />
                </button>
              </div>

              <!-- Endpoint -->
              <div class="flex items-center gap-2">
                <div class="flex-1 min-w-0 rounded-[14px] border border-emerald-500/15 bg-emerald-500/[0.04] px-3 py-2.5">
                  <div class="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">Chat Endpoint</div>
                  <div class="mt-0.5 truncate font-mono text-[12px] font-semibold text-ios-text dark:text-ios-textDark select-all">
                    {{ endpoint }}
                  </div>
                </div>
                <button
                  type="button"
                  class="no-drag-region flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-black/[0.06] bg-white/80 text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05]"
                  title="复制 Endpoint"
                  @click="copyText(endpoint, 'Endpoint')"
                >
                  <Copy class="h-3.5 w-3.5" stroke-width="2.4" />
                </button>
              </div>

              <!-- API Key -->
              <div v-if="relaySecret" class="flex items-center gap-2">
                <div class="flex-1 min-w-0 rounded-[14px] border border-black/[0.05] bg-black/[0.02] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.03]">
                  <div class="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">API Key (Bearer)</div>
                  <div class="mt-0.5 font-mono text-[12px] text-ios-text dark:text-ios-textDark select-all">
                    {{ relaySecret }}
                  </div>
                </div>
                <button
                  type="button"
                  class="no-drag-region flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-black/[0.06] bg-white/80 text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05]"
                  title="复制 API Key"
                  @click="copyText(relaySecret, 'API Key')"
                >
                  <Copy class="h-3.5 w-3.5" stroke-width="2.4" />
                </button>
              </div>
              <div v-else class="rounded-[14px] border border-black/[0.05] bg-black/[0.02] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.03]">
                <div class="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">API Key</div>
                <div class="mt-0.5 text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark">未设置（无需鉴权）· 可在「设置」中配置</div>
              </div>

              <!-- 模型列表 -->
              <div class="rounded-[14px] border border-black/[0.05] bg-black/[0.02] px-3 py-2.5 dark:border-white/[0.06] dark:bg-white/[0.03]">
                <div class="text-[10px] font-bold uppercase tracking-[0.15em] text-ios-textSecondary dark:text-ios-textSecondaryDark">可用模型</div>
                <div class="mt-1 flex flex-wrap gap-1.5">
                  <span v-for="m in ['gpt-4', 'gpt-4o', 'claude-3.5-sonnet', 'cascade']" :key="m"
                    class="rounded-full bg-black/[0.04] px-2 py-0.5 text-[10px] font-bold tracking-wide text-ios-textSecondary dark:bg-white/[0.06] dark:text-ios-textSecondaryDark"
                  >{{ m }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- 快速测试 -->
          <div class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
            <div class="flex items-center justify-between gap-3 mb-3">
              <div class="flex items-center gap-2">
                <Terminal class="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark" stroke-width="2.4" />
                <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">快速测试</div>
              </div>
              <button
                type="button"
                class="no-drag-region flex items-center gap-1.5 rounded-[12px] px-3 py-1.5 text-[11px] font-semibold transition-all ios-btn"
                :class="testLoading
                  ? 'bg-slate-500/10 text-slate-500'
                  : 'bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/15 dark:text-emerald-300'"
                :disabled="testLoading"
                @click="handleTest"
              >
                <RefreshCw v-if="testLoading" class="h-3 w-3 animate-spin" stroke-width="2.4" />
                <Play v-else class="h-3 w-3" stroke-width="2.4" />
                {{ testLoading ? '测试中...' : '发送测试请求' }}
              </button>
            </div>

            <div
              v-if="testResult"
              class="rounded-[14px] border px-3 py-2.5 text-[12px] font-medium leading-relaxed break-words"
              :class="testResult.startsWith('✅')
                ? 'border-emerald-500/15 bg-emerald-500/[0.05] text-emerald-700 dark:text-emerald-300'
                : 'border-rose-500/15 bg-rose-500/[0.05] text-rose-700 dark:text-rose-300'"
            >
              {{ testResult }}
            </div>

            <div class="mt-3 space-y-2">
              <button
                type="button"
                class="no-drag-region flex w-full items-center justify-center gap-2 rounded-[14px] border border-black/[0.06] bg-white/80 px-3 py-2.5 text-[11px] font-semibold text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark"
                @click="copyText(curlCmd, 'curl 命令')"
              >
                <Copy class="h-3 w-3" stroke-width="2.4" />
                复制 curl 命令（Windows CMD）
              </button>
              <button
                type="button"
                class="no-drag-region flex w-full items-center justify-center gap-2 rounded-[14px] border border-black/[0.06] bg-white/80 px-3 py-2.5 text-[11px] font-semibold text-ios-textSecondary shadow-sm transition-all ios-btn hover:bg-black/[0.04] dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-ios-textSecondaryDark"
                @click="copyText(pythonExample, 'Python 示例')"
              >
                <Copy class="h-3 w-3" stroke-width="2.4" />
                复制 Python OpenAI SDK 示例
              </button>
            </div>
          </div>
        </div>

        <!-- 未运行说明 -->
        <div v-if="!relayStatus?.running" class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]">
          <div class="flex items-center gap-2 mb-3">
            <Shield class="h-4 w-4 text-ios-textSecondary dark:text-ios-textSecondaryDark" stroke-width="2.4" />
            <div class="text-[13px] font-bold text-ios-text dark:text-ios-textDark">使用说明</div>
          </div>
          <div class="space-y-2.5 text-[12px] leading-relaxed text-ios-textSecondary dark:text-ios-textSecondaryDark">
            <div class="flex gap-2.5">
              <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-violet-500/10 text-[10px] font-bold text-violet-600 dark:text-violet-300">1</span>
              <span>在「号池」页面通过 <strong class="text-ios-text dark:text-ios-textDark">API Key 导入</strong> 添加 <code class="rounded bg-black/5 px-1 dark:bg-white/10">sk-ws-01-...</code> 账号</span>
            </div>
            <div class="flex gap-2.5">
              <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-violet-500/10 text-[10px] font-bold text-violet-600 dark:text-violet-300">2</span>
              <span>打开上方开关启动 Relay（端口和密钥可在「设置」中修改）</span>
            </div>
            <div class="flex gap-2.5">
              <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-violet-500/10 text-[10px] font-bold text-violet-600 dark:text-violet-300">3</span>
              <span>将 Base URL <code class="rounded bg-black/5 px-1 dark:bg-white/10">http://127.0.0.1:{{ relayPort }}/v1</code> 填入你的 OpenAI 客户端</span>
            </div>
            <div class="flex gap-2.5">
              <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-violet-500/10 text-[10px] font-bold text-violet-600 dark:text-violet-300">4</span>
              <span>额度耗尽时自动轮转到下一个号池 Key，无需手动切换</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
