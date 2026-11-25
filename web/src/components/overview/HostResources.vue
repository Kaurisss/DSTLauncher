<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue';

import UiStat from '@/components/ui/UiStat.vue';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { hostApi } from '@/lib/api.js';
import { fmtSize } from '@/lib/format.js';

const snapshot = ref(null);
const error = ref('');
let disposed = false;
let timer;

const memory = computed(() => snapshot.value?.memory);
const disk = computed(() => snapshot.value?.disk);
const container = computed(() => snapshot.value?.containerMemory);
const containerLow = computed(() => !!container.value && container.value.availableBytes / container.value.totalBytes <= 0.1);
const memoryUsed = computed(() => memory.value
  ? memory.value.totalBytes - memory.value.availableBytes : 0);
const memoryLow = computed(() => !!memory.value
  && memoryUsed.value / memory.value.totalBytes >= 0.9);
const diskLow = computed(() => !!disk.value
  && (disk.value.availableBytes / disk.value.totalBytes < 0.1
    || disk.value.availableBytes < 5 * 1024 ** 3));
const errors = computed(() => [
  error.value,
  snapshot.value?.memoryError ? '内存读取失败：' + snapshot.value.memoryError : '',
  snapshot.value?.diskError ? '存档盘读取失败：' + snapshot.value.diskError : '',
  snapshot.value?.containerMemoryError ? '容器内存读取失败：' + snapshot.value.containerMemoryError : '',
].filter(Boolean));

async function refresh() {
  try {
    const data = await hostApi.stats();
    if (disposed) return;
    snapshot.value = data;
    error.value = '';
  } catch (err) {
    if (disposed) return;
    snapshot.value = null;
    error.value = '宿主机资源读取失败：' + err.message;
  } finally {
    // Schedule after completion: slow requests must not overlap.
    if (!disposed) timer = setTimeout(refresh, 5000);
  }
}

onMounted(refresh);
onUnmounted(() => {
  disposed = true;
  clearTimeout(timer);
});
</script>

<template>
  <section aria-label="宿主机资源" class="grid min-w-0 content-start gap-3">
    <div class="stat-row">
      <slot />
      <UiStat
        label="宿主机内存"
        :loading="!snapshot && !errors.length"
        :value="memory ? ((memoryUsed / memory.totalBytes) * 100).toFixed(1) + '%' : '—'"
        :sub="memory ? '已用 ' + fmtSize(memoryUsed) + ' / ' + fmtSize(memory.totalBytes) : '读取失败'"
        :tone="memoryLow ? 'warn' : ''"
      >
        <template #loading>
          <Skeleton class="mt-2 h-1 w-full rounded-full" />
        </template>
        <Progress
          v-if="memory"
          :class="['mt-2 h-1', memoryLow && '[&>[data-slot=progress-indicator]]:bg-amber-500']"
          :model-value="memoryUsed / memory.totalBytes * 100"
          aria-label="宿主机内存使用率"
        />
      </UiStat>
      <Tooltip :disabled="!snapshot?.diskPath">
        <TooltipTrigger as-child>
          <UiStat
            label="存档盘"
            :loading="!snapshot && !errors.length"
            :value="disk ? (((disk.totalBytes - disk.availableBytes) / disk.totalBytes) * 100).toFixed(1) + '%' : '—'"
            :sub="disk ? '可用 ' + fmtSize(disk.availableBytes) + ' / 总容量 ' + fmtSize(disk.totalBytes) : '读取失败'"
            :tabindex="snapshot?.diskPath ? 0 : undefined"
            :tone="diskLow ? 'warn' : ''"
          >
            <template #loading>
              <Skeleton class="mt-2 h-1 w-full rounded-full" />
            </template>
            <Progress
              v-if="disk"
              :class="['mt-2 h-1', diskLow && '[&>[data-slot=progress-indicator]]:bg-amber-500']"
              :model-value="(disk.totalBytes - disk.availableBytes) / disk.totalBytes * 100"
              aria-label="存档盘已用空间"
            />
          </UiStat>
        </TooltipTrigger>
        <TooltipContent class="break-all">{{ snapshot?.diskPath }}</TooltipContent>
      </Tooltip>
    </div>
    <div v-if="container" class="host-container flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground" :class="{ 'is-warn': containerLow }">
      <span>容器内存配额</span>
      <strong class="font-medium text-foreground tabular-nums">{{ fmtSize(container.availableBytes) }}</strong>
      <span class="tabular-nums">剩余可用 / 限额 {{ fmtSize(container.totalBytes) }}</span>
      <Progress
        :class="['h-1 w-24', containerLow && '[&>[data-slot=progress-indicator]]:bg-amber-500']"
        :model-value="(container.totalBytes - container.availableBytes) / container.totalBytes * 100"
        aria-label="容器内存使用率"
      />
    </div>
    <div v-if="errors.length" role="status" class="grid gap-1 break-words rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-700 dark:text-amber-400">
      <p v-for="message in errors" :key="message">{{ message }}</p>
    </div>
  </section>
</template>
