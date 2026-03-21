<script setup lang="ts">
defineProps<{
  variant: 'accounts' | 'settings'
}>()
</script>

<template>
  <!-- 账号池：卡片网格骨架 -->
  <div
    v-if="variant === 'accounts'"
    class="flex-1 min-h-[360px] md:min-h-[420px] flex flex-col"
    aria-busy="true"
    aria-label="加载中"
  >
    <div class="flex items-center gap-2 mb-6 px-0.5">
      <div class="ios-skeleton h-4 w-28 rounded-lg" />
      <div class="ios-skeleton h-4 w-16 rounded-full ml-auto" />
    </div>
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5 flex-1 content-start">
      <div
        v-for="i in 6"
        :key="i"
        class="ios-glass rounded-[22px] p-5 border border-black/[0.04] dark:border-white/[0.06] ios-page-enter flex flex-col gap-4 min-h-[140px]"
        :style="{ animationDelay: `${(i - 1) * 55}ms` }"
      >
        <div class="flex justify-between gap-3">
          <div class="ios-skeleton h-5 w-[55%] rounded-lg" />
          <div class="ios-skeleton h-6 w-14 rounded-full shrink-0" />
        </div>
        <div class="ios-skeleton h-3 w-4/5 rounded-md" />
        <div class="ios-skeleton h-3 w-1/2 rounded-md" />
        <div class="flex gap-2 mt-auto pt-1">
          <div class="ios-skeleton h-8 flex-1 rounded-[12px]" />
          <div class="ios-skeleton h-8 w-20 rounded-[12px]" />
        </div>
      </div>
    </div>
  </div>

  <!-- 设置：分组块骨架 -->
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
        <div class="ios-skeleton h-9 w-9 rounded-xl shrink-0" />
        <div class="ios-skeleton h-4 w-36 rounded-lg" />
      </div>
      <div class="p-6 space-y-4">
        <div class="ios-skeleton h-4 w-full rounded-lg" />
        <div class="ios-skeleton h-4 w-[92%] rounded-lg" />
        <div class="ios-skeleton h-4 w-[70%] rounded-lg" />
        <div class="flex flex-wrap gap-3 pt-2">
          <div class="ios-skeleton h-10 w-full max-w-[200px] rounded-full" />
          <div class="ios-skeleton h-10 w-24 rounded-full" />
        </div>
      </div>
    </div>
  </div>
</template>
