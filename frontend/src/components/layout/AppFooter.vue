<script setup lang="ts">
import { ExternalLink } from 'lucide-vue-next'
import { APP_VERSION, REPO_URL } from '../../utils/appMeta'
</script>

<template>
  <footer
    class="shrink-0 px-5 py-2.5 border-t border-ios-divider dark:border-ios-dividerDark bg-white/50 dark:bg-[#1C1C1E]/60 backdrop-blur-md flex flex-wrap items-center justify-center gap-x-4 gap-y-1 text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
  >
    <span class="font-mono tabular-nums">v{{ APP_VERSION }}</span>
    <span class="opacity-40" aria-hidden="true">·</span>
    <a
      :href="REPO_URL"
      target="_blank"
      rel="noopener noreferrer"
      class="no-drag-region inline-flex items-center gap-1 hover:text-ios-blue transition-colors"
    >
      本仓库
      <ExternalLink class="w-3 h-3" />
    </a>
  </footer>
</template>
