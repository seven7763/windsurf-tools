<script setup lang="ts">
import { AlertTriangle, CheckCircle2, AlertCircle, Info } from 'lucide-vue-next'
import { toastQueue } from '../../utils/toast'
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed bottom-6 left-1/2 z-[150] flex -translate-x-1/2 flex-col gap-2 pointer-events-none w-[min(440px,calc(100vw-2rem))]"
      aria-live="polite"
    >
      <TransitionGroup
        enter-active-class="transition duration-200 ease-out"
        enter-from-class="opacity-0 translate-y-2"
        enter-to-class="opacity-100 translate-y-0"
        leave-active-class="transition duration-150 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
        move-class="transition duration-200"
      >
        <div
          v-for="t in toastQueue"
          :key="t.id"
          class="pointer-events-auto flex gap-3 rounded-[18px] border px-4 py-3 shadow-lg backdrop-blur-xl text-[14px] leading-snug"
          :class="[
            t.kind === 'success'
              ? 'bg-ios-green/12 border-ios-green/25 text-ios-greenDark dark:text-emerald-300'
              : t.kind === 'error'
                ? 'bg-ios-red/12 border-ios-red/25 text-ios-redDark dark:text-red-300'
                : t.kind === 'warning'
                  ? 'bg-amber-500/12 border-amber-500/25 text-amber-800 dark:text-amber-300'
                  : 'bg-white/92 dark:bg-[#1c1c1e]/92 border-black/8 dark:border-white/10 text-ios-text dark:text-ios-textDark',
          ]"
        >
          <div class="shrink-0 pt-0.5">
            <CheckCircle2 v-if="t.kind === 'success'" class="w-5 h-5" stroke-width="2.2" />
            <AlertCircle v-else-if="t.kind === 'error'" class="w-5 h-5" stroke-width="2.2" />
            <AlertTriangle v-else-if="t.kind === 'warning'" class="w-5 h-5" stroke-width="2.2" />
            <Info v-else class="w-5 h-5 text-ios-blue" stroke-width="2.2" />
          </div>
          <p class="flex-1 whitespace-pre-line break-words">{{ t.message }}</p>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>
