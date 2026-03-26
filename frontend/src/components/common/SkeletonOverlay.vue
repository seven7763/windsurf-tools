<script setup lang="ts">
withDefaults(
  defineProps<{
    active: boolean;
    label?: string;
    overlayClass?: string;
    contentClass?: string;
  }>(),
  {
    label: "加载中",
    overlayClass: "",
    contentClass: "opacity-0 pointer-events-none select-none",
  },
);
</script>

<template>
  <div class="relative">
    <div :class="active ? contentClass : ''">
      <slot />
    </div>

    <Transition name="skeleton-overlay">
      <div
        v-if="active"
        class="absolute inset-0 z-10"
        :class="overlayClass"
        aria-busy="true"
        :aria-label="label"
      >
        <slot name="skeleton" />
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.skeleton-overlay-enter-active,
.skeleton-overlay-leave-active {
  transition: opacity 0.22s cubic-bezier(0.2, 0.8, 0.2, 1);
}

.skeleton-overlay-enter-from,
.skeleton-overlay-leave-to {
  opacity: 0;
}
</style>
