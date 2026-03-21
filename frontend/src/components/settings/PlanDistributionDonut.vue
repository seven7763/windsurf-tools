<script setup lang="ts">
import { computed } from 'vue'
import { SWITCH_PLAN_FILTER_TONES, switchPlanFilterToneOptions, type SwitchPlanTone } from '../../utils/settingsModel'
import {
  PLAN_TONE_CHART_COLORS,
  buildPlanPoolConicGradient,
  sumPlanToneCounts,
  type PlanToneCounts,
} from '../../utils/planToneChart'

const props = defineProps<{
  counts: PlanToneCounts
  /** compact：更小环与字号 */
  compact?: boolean
}>()

const total = computed(() => sumPlanToneCounts(props.counts))

const gradient = computed(() => buildPlanPoolConicGradient(props.counts))

const legendItems = computed(() => {
  const out: Array<{ tone: SwitchPlanTone; label: string; count: number; color: string }> = []
  const labelMap = new Map(switchPlanFilterToneOptions.map((o) => [o.value, o.label]))
  for (const t of SWITCH_PLAN_FILTER_TONES) {
    const c = props.counts[t] ?? 0
    if (c <= 0) continue
    out.push({
      tone: t,
      label: labelMap.get(t) ?? t,
      count: c,
      color: PLAN_TONE_CHART_COLORS[t],
    })
  }
  return out
})

const sizeClass = computed(() => (props.compact ? 'w-[100px] h-[100px]' : 'w-[120px] h-[120px]'))
const holeClass = computed(() => (props.compact ? 'inset-[20%]' : 'inset-[18%]'))
const totalTextClass = computed(() => (props.compact ? 'text-[20px]' : 'text-[24px]'))
</script>

<template>
  <div class="flex flex-col items-center gap-2 shrink-0">
    <div class="relative" :class="sizeClass">
      <!-- 底环：无数据 -->
      <div
        v-if="!gradient"
        class="absolute inset-0 rounded-full bg-black/[0.06] dark:bg-white/[0.08] ring-1 ring-inset ring-black/[0.06] dark:ring-white/[0.08]"
      />
      <!-- 彩色环 -->
      <div
        v-else
        class="absolute inset-0 rounded-full ring-1 ring-inset ring-black/[0.06] dark:ring-white/[0.1] shadow-inner"
        :style="{ background: gradient }"
      />
      <div
        class="absolute flex flex-col items-center justify-center rounded-full bg-white dark:bg-[#1C1C1E] shadow-[inset_0_0_0_1px_rgba(0,0,0,0.06)] dark:shadow-[inset_0_0_0_1px_rgba(255,255,255,0.08)]"
        :class="holeClass"
      >
        <span
          class="font-bold tabular-nums leading-none text-ios-text dark:text-ios-textDark"
          :class="totalTextClass"
          >{{ total }}</span
        >
        <span class="text-[9px] font-semibold text-ios-textSecondary dark:text-ios-textSecondaryDark mt-0.5"
          >账号</span
        >
      </div>
    </div>
    <!-- 微型图例 -->
    <ul
      v-if="legendItems.length"
      class="w-full max-w-[140px] space-y-1 text-[10px] leading-tight"
    >
      <li
        v-for="row in legendItems"
        :key="row.tone"
        class="flex items-center justify-between gap-2 text-ios-textSecondary dark:text-ios-textSecondaryDark"
      >
        <span class="flex items-center gap-1.5 min-w-0">
          <span
            class="w-1.5 h-1.5 rounded-full shrink-0 ring-1 ring-black/10 dark:ring-white/20"
            :style="{ backgroundColor: row.color }"
          />
          <span class="truncate">{{ row.label }}</span>
        </span>
        <span class="font-semibold tabular-nums text-ios-text/80 dark:text-ios-textDark/80">{{ row.count }}</span>
      </li>
    </ul>
  </div>
</template>
