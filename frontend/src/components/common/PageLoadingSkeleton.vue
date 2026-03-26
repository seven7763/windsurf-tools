<script setup lang="ts">
import SkeletonBlock from "./SkeletonBlock.vue";

defineProps<{
  variant: "dashboard" | "accounts" | "relay" | "settings";
}>();
</script>

<template>
  <div
    v-if="variant === 'dashboard'"
    class="space-y-6 w-full p-6"
    aria-busy="true"
    aria-label="加载中"
  >
    <div
      class="ios-glass rounded-[28px] border border-black/[0.05] overflow-hidden dark:border-white/[0.08]"
    >
      <div class="border-b border-black/[0.05] p-6 dark:border-white/[0.08]">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="flex min-w-0 items-start gap-3">
            <SkeletonBlock class="h-11 w-11 rounded-2xl shrink-0" />
            <div class="min-w-0 space-y-3">
              <SkeletonBlock class="h-5 w-28 rounded-lg" />
              <SkeletonBlock class="h-4 w-80 rounded-lg" />
            </div>
          </div>
          <SkeletonBlock class="h-10 w-28 rounded-full" />
        </div>
      </div>
      <div class="grid grid-cols-1 gap-3 p-6 md:grid-cols-2 xl:grid-cols-4">
        <div
          v-for="i in 4"
          :key="`dashboard-kpi-${i}`"
          class="ios-glass rounded-[22px] border border-black/[0.05] p-5 dark:border-white/[0.08]"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="space-y-3">
              <SkeletonBlock class="h-3.5 w-20 rounded-lg" />
              <SkeletonBlock class="h-6 w-24 rounded-lg" />
            </div>
            <SkeletonBlock class="h-10 w-10 rounded-2xl shrink-0" />
          </div>
          <SkeletonBlock class="mt-4 h-4 w-full rounded-lg" />
        </div>
      </div>
    </div>

    <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1.25fr)_360px]">
      <div class="space-y-4">
        <div
          v-for="i in 3"
          :key="`dashboard-panel-${i}`"
          class="ios-glass rounded-[24px] border border-black/[0.05] p-5 dark:border-white/[0.08]"
        >
          <SkeletonBlock class="h-5 w-32 rounded-lg" />
          <div class="mt-4 space-y-3">
            <SkeletonBlock class="h-16 w-full rounded-[18px]" />
            <SkeletonBlock class="h-16 w-full rounded-[18px]" />
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div
          v-for="i in 3"
          :key="`dashboard-side-${i}`"
          class="ios-glass rounded-[24px] border border-black/[0.05] p-5 dark:border-white/[0.08]"
        >
          <SkeletonBlock class="h-4 w-24 rounded-lg" />
          <div class="mt-4 space-y-3">
            <SkeletonBlock class="h-12 w-full rounded-[16px]" />
            <SkeletonBlock class="h-12 w-full rounded-[16px]" />
            <SkeletonBlock class="h-12 w-full rounded-[16px]" />
          </div>
        </div>
      </div>
    </div>
  </div>

  <div
    v-else-if="variant === 'accounts'"
    class="flex-1 min-h-[360px] md:min-h-[420px] flex flex-col"
    aria-busy="true"
    aria-label="加载中"
  >
    <div class="mb-6 flex items-center gap-2 px-0.5">
      <SkeletonBlock class="h-4 w-28 rounded-lg" />
      <SkeletonBlock class="ml-auto h-4 w-16 rounded-full" />
    </div>
    <div
      class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5 flex-1 content-start"
    >
      <div
        v-for="i in 6"
        :key="i"
        class="ios-glass rounded-[22px] p-5 border border-black/[0.04] dark:border-white/[0.06] ios-page-enter flex flex-col gap-4 min-h-[140px]"
        :style="{ animationDelay: `${(i - 1) * 55}ms` }"
      >
        <div class="flex justify-between gap-3">
          <SkeletonBlock class="h-5 w-[55%] rounded-lg" />
          <SkeletonBlock class="h-6 w-14 rounded-full shrink-0" />
        </div>
        <SkeletonBlock class="h-3 w-4/5 rounded-md" />
        <SkeletonBlock class="h-3 w-1/2 rounded-md" />
        <div class="mt-auto flex gap-2 pt-1">
          <SkeletonBlock class="h-8 flex-1 rounded-[12px]" />
          <SkeletonBlock class="h-8 w-20 rounded-[12px]" />
        </div>
      </div>
    </div>
  </div>

  <div
    v-else-if="variant === 'relay'"
    class="space-y-6 w-full max-w-4xl mx-auto p-6"
    aria-busy="true"
    aria-label="加载中"
  >
    <div
      class="ios-glass rounded-[28px] border border-black/[0.05] overflow-hidden dark:border-white/[0.08]"
    >
      <div class="border-b border-black/[0.05] p-6 dark:border-white/[0.08]">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="flex min-w-0 items-start gap-3">
            <SkeletonBlock class="h-11 w-11 rounded-2xl shrink-0" />
            <div class="min-w-0 space-y-3">
              <SkeletonBlock class="h-5 w-40 rounded-lg" />
              <SkeletonBlock class="h-4 w-72 rounded-lg" />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <SkeletonBlock class="h-16 w-24 rounded-[16px]" />
            <SkeletonBlock class="h-16 w-24 rounded-[16px]" />
          </div>
        </div>
      </div>
      <div class="space-y-5 p-6">
        <SkeletonBlock class="h-20 rounded-[22px]" />
        <SkeletonBlock class="h-24 rounded-[18px]" />
        <div
          v-for="i in 3"
          :key="`relay-block-${i}`"
          class="ios-glass rounded-[22px] border border-black/[0.05] p-5 dark:border-white/[0.08]"
        >
          <SkeletonBlock class="h-4 w-28 rounded-lg" />
          <div class="mt-4 space-y-3">
            <SkeletonBlock class="h-11 w-full rounded-[14px]" />
            <SkeletonBlock class="h-11 w-full rounded-[14px]" />
          </div>
        </div>
      </div>
    </div>
  </div>

  <div
    v-else
    class="space-y-7 w-full pb-4"
    aria-busy="true"
    aria-label="加载中"
  >
    <div
      v-for="block in 3"
      :key="block"
      class="ios-glass rounded-[24px] overflow-hidden border border-black/[0.05] dark:border-white/[0.08] ios-page-enter"
      :style="{ animationDelay: `${(block - 1) * 80}ms` }"
    >
      <div
        class="px-6 py-4 flex items-center gap-3 border-b border-black/[0.05] dark:border-white/[0.06] bg-black/[0.02] dark:bg-white/[0.03]"
      >
        <SkeletonBlock class="h-9 w-9 rounded-xl shrink-0" />
        <SkeletonBlock class="h-4 w-36 rounded-lg" />
      </div>
      <div class="p-6 space-y-4">
        <SkeletonBlock class="h-4 w-full rounded-lg" />
        <SkeletonBlock class="h-4 w-[92%] rounded-lg" />
        <SkeletonBlock class="h-4 w-[70%] rounded-lg" />
        <div class="flex flex-wrap gap-3 pt-2">
          <SkeletonBlock class="h-10 w-full max-w-[200px] rounded-full" />
          <SkeletonBlock class="h-10 w-24 rounded-full" />
        </div>
      </div>
    </div>
  </div>
</template>
