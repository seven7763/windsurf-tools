<script setup lang="ts">
import { computed } from 'vue'
import { Check, Layers } from 'lucide-vue-next'
import {
  formatSwitchPlanFilterSummary,
  normalizeSwitchPlanFilter,
  switchPlanFilterToneOptions,
  type SwitchPlanTone,
} from '../../utils/settingsModel'
import PlanDistributionDonut from './PlanDistributionDonut.vue'
import { PLAN_TONE_CHART_COLORS, countAccountsInFilter, sumPlanToneCounts } from '../../utils/planToneChart'

const props = withDefaults(
  defineProps<{
    modelValue: string
    /** compact：账号池顶栏卡片 */
    variant?: 'default' | 'compact'
    /** 号池内各计划账号数；有则显示环形图与覆盖率 */
    poolCounts?: Partial<Record<SwitchPlanTone, number>>
  }>(),
  { variant: 'default', poolCounts: undefined },
)

const emit = defineEmits<{
  'update:modelValue': [string]
}>()

const normalized = computed(() => normalizeSwitchPlanFilter(props.modelValue))

const isUnrestricted = computed(() => normalized.value === 'all')

const selectedTones = computed(() => {
  if (normalized.value === 'all') {
    return new Set<string>()
  }
  return new Set(normalized.value.split(',').filter(Boolean))
})

const summary = computed(() => formatSwitchPlanFilterSummary(props.modelValue))

const poolTotal = computed(() => sumPlanToneCounts(props.poolCounts ?? {}))

const coveredInFilter = computed(() => {
  if (!props.poolCounts || poolTotal.value <= 0) return 0
  if (isUnrestricted.value) return poolTotal.value
  return countAccountsInFilter(props.poolCounts, selectedTones.value)
})

const coveragePct = computed(() => {
  if (poolTotal.value <= 0) return 0
  return Math.round((coveredInFilter.value / poolTotal.value) * 100)
})

function emitValue(v: string) {
  emit('update:modelValue', normalizeSwitchPlanFilter(v))
}

function onAllChange(checked: boolean) {
  if (checked) {
    emitValue('all')
    return
  }
  emitValue('pro,trial')
}

function toggleTone(tone: SwitchPlanTone) {
  if (isUnrestricted.value) {
    emitValue(tone)
    return
  }
  const next = new Set(selectedTones.value)
  if (next.has(tone)) {
    next.delete(tone)
    if (next.size === 0) {
      emitValue('all')
      return
    }
  } else {
    next.add(tone)
  }
  emitValue([...next].join(','))
}

function chipActive(tone: SwitchPlanTone): boolean {
  if (isUnrestricted.value) return true
  return selectedTones.value.has(tone)
}
</script>

<template>
  <!-- 账号池：仪表盘风格 -->
  <div v-if="variant === 'compact'" class="w-full">
    <div
      class="ios-glass rounded-[22px] border border-black/[0.06] dark:border-white/[0.08] overflow-hidden shadow-[0_8px_30px_-12px_rgba(0,0,0,0.12)] dark:shadow-[0_8px_30px_-12px_rgba(0,0,0,0.35)]"
    >
      <div
        class="px-4 py-3 flex items-center gap-2 border-b border-black/[0.05] dark:border-white/[0.06] bg-gradient-to-r from-violet-500/[0.07] to-transparent dark:from-violet-500/[0.12]"
      >
        <div
          class="w-8 h-8 rounded-xl bg-white/80 dark:bg-white/10 flex items-center justify-center shadow-sm ring-1 ring-black/5 dark:ring-white/10"
        >
          <Layers class="w-4 h-4 text-violet-600 dark:text-violet-300" stroke-width="2.25" />
        </div>
        <div class="min-w-0 flex-1">
          <h3 class="text-[15px] font-bold tracking-tight text-ios-text dark:text-ios-textDark">下一席位范围</h3>
          <p class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark mt-0.5">
            无感切换「下一席」时只在勾选的计划池内轮换，与号池实际分布对照如下。
          </p>
        </div>
      </div>

      <div class="p-4 flex flex-col sm:flex-row gap-5 sm:gap-6">
        <PlanDistributionDonut :counts="poolCounts ?? {}" compact />

        <div class="flex-1 min-w-0 space-y-3">
          <div
            v-if="poolTotal > 0"
            class="rounded-[14px] bg-black/[0.03] dark:bg-white/[0.04] px-3 py-2.5 space-y-1.5"
          >
            <div class="flex items-center justify-between text-[11px]">
              <span class="text-ios-textSecondary dark:text-ios-textSecondaryDark">轮换覆盖号池</span>
              <span class="font-bold tabular-nums text-ios-text dark:text-ios-textDark">
                {{ coveredInFilter }} / {{ poolTotal }}
                <span class="text-ios-textSecondary font-semibold ml-1">{{ coveragePct }}%</span>
              </span>
            </div>
            <div class="h-2 rounded-full bg-black/[0.06] dark:bg-white/[0.1] overflow-hidden">
              <div
                class="h-full rounded-full bg-gradient-to-r from-ios-blue to-violet-500 transition-[width] duration-500 ease-out"
                :style="{ width: `${coveragePct}%` }"
              />
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="no-drag-region inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-[12px] font-semibold transition-all ios-btn ring-1"
              :class="
                isUnrestricted
                  ? 'bg-emerald-500/15 text-emerald-800 dark:text-emerald-300 ring-emerald-500/30'
                  : 'bg-black/[0.05] dark:bg-white/[0.08] text-ios-textSecondary ring-black/10 dark:ring-white/10'
              "
              @click="onAllChange(!isUnrestricted)"
            >
              <Check v-if="isUnrestricted" class="w-3.5 h-3.5" stroke-width="2.5" />
              全部计划
            </button>
          </div>

          <div v-if="!isUnrestricted" class="flex flex-wrap gap-2">
            <button
              v-for="opt in switchPlanFilterToneOptions"
              :key="opt.value"
              type="button"
              class="no-drag-region inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[11px] font-semibold transition-all ios-btn ring-1 border-0"
              :class="
                chipActive(opt.value)
                  ? 'text-ios-text dark:text-ios-textDark ring-ios-blue/40 bg-ios-blue/10 dark:bg-ios-blue/20'
                  : 'text-ios-textSecondary opacity-55 ring-black/10 dark:ring-white/10 bg-black/[0.03] dark:bg-white/[0.04]'
              "
              :style="
                chipActive(opt.value)
                  ? { boxShadow: `inset 0 0 0 1px ${PLAN_TONE_CHART_COLORS[opt.value]}33` }
                  : {}
              "
              @click="toggleTone(opt.value)"
            >
              <span
                class="w-1.5 h-1.5 rounded-full shrink-0"
                :style="{ backgroundColor: PLAN_TONE_CHART_COLORS[opt.value] }"
              />
              {{ opt.label }}
            </button>
          </div>

          <p
            v-if="isUnrestricted"
            class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark leading-relaxed"
          >
            当前：<span class="font-semibold text-ios-text dark:text-ios-textDark">{{ summary }}</span>
          </p>
        </div>
      </div>
    </div>
  </div>

  <!-- 设置页等：经典纵向 -->
  <div v-else class="p-6 space-y-6">
    <div v-if="poolCounts" class="flex flex-col sm:flex-row gap-4 items-start">
      <PlanDistributionDonut :counts="poolCounts" />
      <div
        v-if="poolTotal > 0"
        class="flex-1 min-w-0 rounded-[14px] bg-black/[0.03] dark:bg-white/[0.04] px-3 py-2 space-y-1.5 w-full sm:max-w-xs"
      >
        <div class="flex items-center justify-between text-[11px]">
          <span class="text-ios-textSecondary dark:text-ios-textSecondaryDark">轮换覆盖号池</span>
          <span class="font-bold tabular-nums text-ios-text dark:text-ios-textDark">
            {{ coveredInFilter }} / {{ poolTotal }}
          </span>
        </div>
        <div class="h-2 rounded-full bg-black/[0.06] dark:bg-white/[0.1] overflow-hidden">
          <div
            class="h-full rounded-full bg-gradient-to-r from-ios-blue to-violet-500 transition-[width] duration-500 ease-out"
            :style="{ width: `${coveragePct}%` }"
          />
        </div>
      </div>
    </div>

    <div class="space-y-3">
      <label class="flex items-start gap-2 cursor-pointer select-none text-[14px]">
        <input
          type="checkbox"
          class="no-drag-region mt-0.5 rounded border-black/20 dark:border-white/30"
          :checked="isUnrestricted"
          @change="onAllChange(($event.target as HTMLInputElement).checked)"
        />
        <span class="font-semibold">全部计划（不限制）</span>
      </label>

      <div v-if="!isUnrestricted" class="flex flex-wrap gap-2 pl-0.5">
        <button
          v-for="opt in switchPlanFilterToneOptions"
          :key="opt.value"
          type="button"
          class="no-drag-region inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-[12px] font-semibold transition-all ios-btn ring-1"
          :class="
            chipActive(opt.value)
              ? 'text-ios-text dark:text-ios-textDark ring-ios-blue/35 bg-ios-blue/10'
              : 'text-ios-textSecondary opacity-60 ring-black/10 dark:ring-white/10 bg-black/[0.03] dark:bg-white/[0.04]'
          "
          @click="toggleTone(opt.value)"
        >
          <span
            class="w-1.5 h-1.5 rounded-full"
            :style="{ backgroundColor: PLAN_TONE_CHART_COLORS[opt.value] }"
          />
          {{ opt.label }}
        </button>
      </div>

      <p class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark leading-relaxed">
        当前范围：<span class="font-medium text-ios-text dark:text-ios-textDark">{{ summary }}</span>
      </p>
    </div>
  </div>
</template>
