<script setup lang="ts">
import { computed } from "vue";
import { Link2, Unlink, Clock, Hash } from "lucide-vue-next";
import { useMitmStatusStore } from "../stores/useMitmStatusStore";
import { showToast } from "../utils/toast";
import { formatDateTimeAsiaShanghai } from "../utils/datetimeAsia";

const mitmStore = useMitmStatusStore();

const sessions = computed(() => mitmStore.activeSessions());
const sessionCount = computed(() => mitmStore.sessionCount());

const handleUnbind = async (convIDShort: string) => {
  const prefix = convIDShort.replace(/\.{3}$/, "");
  const ok = await mitmStore.unbindSession(prefix);
  if (ok) {
    showToast(`已解绑会话 ${convIDShort}`, "success");
  } else {
    showToast(`解绑失败: ${convIDShort}`, "error");
  }
};
</script>

<template>
  <div
    v-if="sessionCount > 0"
    class="rounded-[22px] border border-black/[0.05] bg-white/70 p-4 shadow-sm dark:border-white/[0.06] dark:bg-white/[0.04]"
  >
    <div class="mb-3 flex items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <div
          class="flex h-8 w-8 items-center justify-center rounded-xl bg-violet-500/10 text-violet-600 dark:text-violet-300"
        >
          <Link2 class="h-4 w-4" stroke-width="2.4" />
        </div>
        <div>
          <div
            class="text-[13px] font-bold text-ios-text dark:text-ios-textDark"
          >
            会话绑定
          </div>
          <div
            class="text-[11px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
          >
            每个对话固定路由到同一个号池 Key，避免上下文错乱。
          </div>
        </div>
      </div>
      <span
        class="rounded-full bg-violet-500/10 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-violet-700 dark:text-violet-300"
      >
        {{ sessionCount }} 活跃
      </span>
    </div>

    <div class="space-y-2 max-h-48 overflow-y-auto pr-1">
      <div
        v-for="(s, index) in sessions"
        :key="`${s.conv_id || s.conv_id_short}-${index}`"
        class="flex items-center justify-between gap-3 rounded-[16px] border border-black/[0.05] bg-black/[0.03] px-3 py-2.5 text-[12px] dark:border-white/[0.06] dark:bg-white/[0.03]"
      >
        <div class="flex min-w-0 items-center gap-2.5">
          <Hash
            class="h-3.5 w-3.5 shrink-0 text-ios-textSecondary dark:text-ios-textSecondaryDark"
            stroke-width="2.4"
          />
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span
                v-if="s.title"
                class="truncate text-ios-text dark:text-ios-textDark font-semibold"
                :title="s.title"
                >{{ s.title }}</span
              >
              <span
                v-else
                class="truncate font-mono text-ios-text dark:text-ios-textDark"
                :title="s.conv_id"
                >{{ s.conv_id || s.conv_id_short }}</span
              >
              <span
                class="shrink-0 rounded-full bg-ios-blue/10 px-2 py-0.5 text-[10px] font-bold text-ios-blue"
              >
                {{ s.pool_key_short }}
              </span>
            </div>
            <div
              class="mt-0.5 flex items-center gap-2 text-[10px] text-ios-textSecondary dark:text-ios-textSecondaryDark"
            >
              <span v-if="s.title" class="font-mono opacity-60">{{ s.conv_id || s.conv_id_short }}</span>
              <span class="flex items-center gap-1">
                <Clock class="h-3 w-3" stroke-width="2" />
                {{ formatDateTimeAsiaShanghai(s.last_seen_at) }}
              </span>
              <span>{{ s.request_count }} 次请求</span>
            </div>
          </div>
        </div>
        <button
          type="button"
          class="no-drag-region shrink-0 rounded-full border border-rose-500/15 bg-rose-500/[0.06] p-1.5 text-rose-600 transition-colors hover:bg-rose-500/[0.12] dark:text-rose-400"
          title="解除绑定"
          @click="handleUnbind(s.conv_id || s.conv_id_short)"
        >
          <Unlink class="h-3 w-3" stroke-width="2.4" />
        </button>
      </div>
    </div>
  </div>
</template>
