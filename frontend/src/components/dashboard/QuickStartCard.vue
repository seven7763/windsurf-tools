<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { BookOpen, ChevronDown, ChevronUp } from 'lucide-vue-next'

const STORAGE_KEY = 'windsurf-tools.quickstart.collapsed'
const collapsed = ref(true)

onMounted(() => {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === '0') {
      collapsed.value = false
    }
  } catch {
    /* ignore */
  }
})

const toggle = () => {
  collapsed.value = !collapsed.value
  try {
    localStorage.setItem(STORAGE_KEY, collapsed.value ? '1' : '0')
  } catch {
    /* ignore */
  }
}

const steps = [
  { title: '导入账号', desc: '在「账号池」批量导入 JWT / Refresh / API Key / 邮箱密码，本机加密存储。' },
  { title: '填写 Windsurf 路径', desc: '「设置」中配置安装目录；若主要用「文件切号」而非 MITM，建议开启「切号后自动重启 Windsurf」以加载新登录态。' },
  {
    title: '无感换号（推荐 MITM）',
    desc: '在控制台启用 MITM（CA + Hosts + 代理），号池密钥由代理侧轮换，不必反复写 windsurf_auth；一般无需重启 Windsurf IDE。',
  },
  {
    title: '额度与补充切号',
    desc: '总览/账号池可同步额度。若关闭「仅 MITM」并开启用尽自动切号，会写 windsurf_auth 下一席；界面常需重载窗口才与文件一致。',
  },
]
</script>

<template>
  <div
    class="ios-glass rounded-[24px] border border-black/[0.06] dark:border-white/[0.08] overflow-hidden transition-shadow hover:shadow-lg hover:shadow-black/5 dark:hover:shadow-black/20"
  >
    <button
      type="button"
      class="no-drag-region w-full flex items-center justify-between gap-4 px-6 py-4 text-left hover:bg-black/[0.02] dark:hover:bg-white/[0.04] transition-colors"
      @click="toggle"
    >
      <div class="flex items-center gap-3 min-w-0">
        <div class="w-10 h-10 rounded-2xl bg-sky-500/15 flex items-center justify-center text-sky-600 dark:text-sky-400 shrink-0">
          <BookOpen class="w-5 h-5" stroke-width="2.5" />
        </div>
        <div class="min-w-0">
          <h2 class="text-[17px] font-semibold tracking-tight">快速上手</h2>
          <p class="text-[12px] text-ios-textSecondary dark:text-ios-textSecondaryDark truncate">
            四步：账号池 → 路径 → MITM 无感（推荐）→ 额度与补充切号
          </p>
        </div>
      </div>
      <component :is="collapsed ? ChevronDown : ChevronUp" class="w-5 h-5 shrink-0 text-ios-textSecondary" />
    </button>

    <div v-show="!collapsed" class="px-6 pb-6 pt-0 space-y-4 border-t border-black/[0.05] dark:border-white/[0.06]">
      <ol class="space-y-3">
        <li
          v-for="(s, i) in steps"
          :key="s.title"
          class="flex gap-3 text-[13px] leading-relaxed"
        >
          <span
            class="shrink-0 w-7 h-7 rounded-full bg-ios-blue/10 text-ios-blue font-bold text-[12px] flex items-center justify-center"
          >
            {{ i + 1 }}
          </span>
          <div>
            <p class="font-semibold text-ios-text dark:text-ios-textDark">{{ s.title }}</p>
            <p class="text-ios-textSecondary dark:text-ios-textSecondaryDark mt-0.5">{{ s.desc }}</p>
          </div>
        </li>
      </ol>
    </div>
  </div>
</template>
