<script setup lang="ts">
import { AlertTriangle } from 'lucide-vue-next'
import { confirmState, resolveConfirm } from '../../utils/toast'
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="confirmState.visible"
        class="fixed inset-0 z-[200] flex items-center justify-center p-4 bg-black/30 backdrop-blur-sm"
        @click.self="resolveConfirm(false)"
      >
        <Transition
          enter-active-class="transition duration-200 ease-out"
          enter-from-class="opacity-0 scale-95"
          enter-to-class="opacity-100 scale-100"
          leave-active-class="transition duration-150 ease-in"
          leave-from-class="opacity-100 scale-100"
          leave-to-class="opacity-0 scale-95"
        >
          <div
            v-if="confirmState.visible"
            class="w-[min(100%,320px)] rounded-[20px] bg-white/95 dark:bg-[#1c1c1e]/95 backdrop-blur-xl shadow-2xl overflow-hidden border border-black/5 dark:border-white/10"
            role="dialog"
            aria-modal="true"
          >
            <div class="px-5 pt-5 pb-4">
              <div class="flex gap-3">
                <div
                  v-if="confirmState.destructive"
                  class="shrink-0 w-10 h-10 rounded-full bg-ios-red/15 flex items-center justify-center text-ios-red"
                >
                  <AlertTriangle class="w-5 h-5" stroke-width="2.5" />
                </div>
                <p class="text-[15px] leading-snug text-ios-text dark:text-ios-textDark font-medium">
                  {{ confirmState.message }}
                </p>
              </div>
            </div>
            <div class="flex border-t border-black/10 dark:border-white/10">
              <button
                type="button"
                class="no-drag-region flex-1 py-3.5 text-[16px] font-semibold text-ios-blue active:bg-black/5 dark:active:bg-white/5"
                @click="resolveConfirm(false)"
              >
                {{ confirmState.cancelText }}
              </button>
              <div class="w-px bg-black/10 dark:bg-white/10" aria-hidden="true" />
              <button
                type="button"
                class="no-drag-region flex-1 py-3.5 text-[16px] font-semibold"
                :class="
                  confirmState.destructive
                    ? 'text-ios-red active:bg-ios-red/10'
                    : 'text-ios-blue active:bg-black/5 dark:active:bg-white/5'
                "
                @click="resolveConfirm(true)"
              >
                {{ confirmState.confirmText }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
