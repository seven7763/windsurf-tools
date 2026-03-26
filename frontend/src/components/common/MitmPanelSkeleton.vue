<script setup lang="ts">
import SkeletonBlock from "./SkeletonBlock.vue";
</script>

<template>
  <div class="space-y-5 p-6">
    <div
      class="rounded-[22px] border border-black/[0.06] bg-black/[0.03] px-4 py-4 shadow-sm dark:border-white/[0.08] dark:bg-white/[0.04]"
    >
      <div class="flex items-center justify-between gap-4">
        <div class="min-w-0 flex-1 space-y-3">
          <SkeletonBlock class="h-4 w-28 rounded-lg" />
          <SkeletonBlock class="h-4 w-[70%] rounded-lg" />
        </div>
        <SkeletonBlock class="h-9 w-20 rounded-full" />
      </div>
    </div>

    <div
      class="rounded-[18px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
    >
      <div class="mb-3 flex items-center justify-between gap-3">
        <SkeletonBlock class="h-4 w-28 rounded-lg" />
        <SkeletonBlock class="h-5 w-16 rounded-full" />
      </div>
      <div class="space-y-2">
        <SkeletonBlock class="h-12 w-full rounded-[16px]" />
        <SkeletonBlock class="h-12 w-full rounded-[16px]" />
        <SkeletonBlock class="h-12 w-full rounded-[16px]" />
      </div>
    </div>

    <div class="space-y-3">
      <SkeletonBlock class="h-4 w-24 rounded-lg" />
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <SkeletonBlock class="h-[84px] rounded-[18px]" />
        <SkeletonBlock class="h-[84px] rounded-[18px]" />
      </div>
    </div>

    <div
      class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
    >
      <div class="mb-3 flex items-center justify-between gap-3">
        <SkeletonBlock class="h-4 w-32 rounded-lg" />
        <SkeletonBlock class="h-5 w-20 rounded-full" />
      </div>
      <div class="space-y-2">
        <SkeletonBlock class="h-11 w-full rounded-[16px]" />
        <SkeletonBlock class="h-11 w-full rounded-[16px]" />
        <SkeletonBlock class="h-11 w-full rounded-[16px]" />
        <SkeletonBlock class="h-11 w-full rounded-[16px]" />
      </div>
    </div>

    <SkeletonBlock class="h-11 w-full rounded-[16px]" />
  </div>
</template>
