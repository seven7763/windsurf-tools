<script setup lang="ts">
import SkeletonBlock from "../common/SkeletonBlock.vue";
</script>

<template>
  <div
    class="relative overflow-hidden rounded-[22px] border border-black/[0.05] bg-white shadow-sm dark:border-white/[0.08] dark:bg-[#1C1C1E]"
    aria-busy="true"
    aria-label="账号卡片加载中"
  >
    <div
      class="absolute inset-x-0 top-0 h-1.5 bg-gradient-to-r from-sky-400 via-cyan-300 to-emerald-300 opacity-90"
    />
    <div class="flex h-full flex-col p-5">
      <div class="flex items-start justify-between gap-3">
        <div class="flex min-w-0 flex-wrap items-center gap-2">
          <SkeletonBlock class="h-6 w-16 rounded-full" />
          <SkeletonBlock class="h-6 w-20 rounded-full" />
        </div>
        <div
          class="flex gap-1 rounded-full border border-black/5 bg-gray-50/95 p-1 dark:border-white/5 dark:bg-black/20"
        >
          <SkeletonBlock class="h-[30px] w-[30px] rounded-full" />
          <SkeletonBlock class="h-[30px] w-[30px] rounded-full" />
          <SkeletonBlock class="h-[30px] w-[30px] rounded-full" />
        </div>
      </div>

      <div class="mt-4 space-y-3">
        <SkeletonBlock class="h-8 w-[68%] rounded-xl" />
        <SkeletonBlock class="h-4 w-[82%] rounded-lg" />
        <div class="flex gap-2">
          <SkeletonBlock class="h-6 w-24 rounded-full" />
          <SkeletonBlock class="h-6 w-20 rounded-full" />
        </div>
      </div>

      <div class="mt-4 grid grid-cols-2 gap-3">
        <div
          class="rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
        >
          <SkeletonBlock class="h-3 w-16 rounded-md" />
          <SkeletonBlock class="mt-3 h-5 w-full rounded-lg" />
          <SkeletonBlock class="mt-2 h-3 w-[72%] rounded-md" />
        </div>
        <div
          class="rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-3 dark:border-white/[0.06] dark:bg-white/[0.04]"
        >
          <SkeletonBlock class="h-3 w-16 rounded-md" />
          <SkeletonBlock class="mt-3 h-5 w-full rounded-lg" />
          <SkeletonBlock class="mt-2 h-3 w-[56%] rounded-md" />
        </div>
      </div>

      <div
        class="mt-4 rounded-[18px] border border-black/[0.05] bg-black/[0.025] p-4 dark:border-white/[0.06] dark:bg-white/[0.04]"
      >
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <SkeletonBlock class="h-3 w-12 rounded-md" />
            <SkeletonBlock class="h-3 w-14 rounded-md" />
          </div>
          <SkeletonBlock class="h-2 w-full rounded-full" />
          <SkeletonBlock class="h-3 w-[42%] rounded-md" />
        </div>

        <div class="mt-5 space-y-3">
          <div class="flex items-center justify-between">
            <SkeletonBlock class="h-3 w-12 rounded-md" />
            <SkeletonBlock class="h-3 w-14 rounded-md" />
          </div>
          <SkeletonBlock class="h-2 w-full rounded-full" />
          <SkeletonBlock class="h-3 w-[38%] rounded-md" />
        </div>
      </div>
    </div>
  </div>
</template>
