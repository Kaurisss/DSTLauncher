<script setup>
// 概览右上大卡：宿主机内存、存档盘空间、容器内存配额与当前房间进程资源监控。

import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useTimeoutPoll } from '@vueuse/core';

import UiCard from '@/components/ui/UiCard.vue';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { hostApi } from '@/lib/api.js';
import { fmtSize } from '@/lib/format.js';

defineProps({
  processValue: { type: String, default: '—' },
  processSub: { type: String, default: '未运行' },
  processCpu: { type: Number, default: 0 },
  processRunning: { type: Boolean, default: false },
  processLoading: { type: Boolean, default: false },
});

const snapshot = ref(null);
const error = ref('');
let disposed = false;

const memory = computed(() => snapshot.value?.memory);
const disk = computed(() => snapshot.value?.disk);
const container = computed(() => snapshot.value?.containerMemory);
const containerLow = computed(() => !!container.value && container.value.availableBytes / container.value.totalBytes <= 0.1);
const memoryUsed = computed(() => (memory.value ? memory.value.totalBytes - memory.value.availableBytes : 0));
const memoryPct = computed(() => (memory.value ? (memoryUsed.value / memory.value.totalBytes) * 100 : 0));
const memoryLow = computed(() => !!memory.value && memoryUsed.value / memory.value.totalBytes >= 0.9);
const diskUsedPct = computed(() => (disk.value ? ((disk.value.totalBytes - disk.value.availableBytes) / disk.value.totalBytes) * 100 : 0));
const diskLow = computed(() => !!disk.value
  && (disk.value.availableBytes / disk.value.totalBytes < 0.1
    || disk.value.availableBytes < 5 * 1024 ** 3));
const errors = computed(() => [
  error.value,
  snapshot.value?.memoryError ? '内存读取失败：' + snapshot.value.memoryError : '',
  snapshot.value?.diskError ? '存档盘读取失败：' + snapshot.value.diskError : '',
  snapshot.value?.containerMemoryError ? '容器内存读取失败：' + snapshot.value.containerMemoryError : '',
].filter(Boolean));
const loading = computed(() => !snapshot.value && !errors.value.length);

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
  }
}

const { resume } = useTimeoutPoll(refresh, 5000, { immediate: false });

onMounted(async () => {
  await refresh();
  if (!disposed) resume();
});
onUnmounted(() => {
  disposed = true;
});
</script>

<template>
  <section aria-label="宿主机资源" class="flex h-full min-w-0 flex-col">
    <UiCard class="h-full" fill>
      <template #title>
        <span class="inline-flex items-center gap-2">
          <span>资源监控</span>
          <Badge
            v-if="memoryLow || diskLow || containerLow"
            variant="secondary"
            class="h-5 border-amber-500/30 bg-amber-500/10 px-1.5 text-[11px] font-medium text-amber-700 dark:text-amber-400"
          >
            资源偏高
          </Badge>
        </span>
      </template>

      <div class="flex min-h-0 flex-1 flex-col gap-2.5">
        <!-- 宿主机内存 + 存档盘两列监控 -->
        <div class="grid flex-1 gap-2.5 sm:grid-cols-2">
          <div class="flex min-w-0 flex-col justify-between rounded-lg border border-border/80 bg-muted/25 px-3 py-2.5">
            <div class="flex items-center justify-between gap-2">
              <span class="text-[11.5px] font-medium text-muted-foreground">宿主机内存</span>
              <Skeleton v-if="loading" class="h-3 w-24 rounded" />
              <span v-else-if="memory" class="truncate text-[11px] text-muted-foreground tabular-nums">
                {{ fmtSize(memoryUsed) }} / {{ fmtSize(memory.totalBytes) }}
              </span>
            </div>
            <div v-if="loading" role="status" aria-label="读取宿主机内存" class="mt-1 grid gap-2">
              <div class="flex h-[22.5px] items-center"><Skeleton class="h-4.5 w-14 rounded" /></div>
              <Skeleton class="h-1 w-full rounded-full" />
            </div>
            <template v-else>
              <div class="mt-1 text-lg font-semibold leading-tight tracking-tight tabular-nums">
                {{ memory ? memoryPct.toFixed(1) + '%' : '—' }}
              </div>
              <Progress
                v-if="memory"
                :class="['mt-2 h-1', memoryLow && '[&>[data-slot=progress-indicator]]:bg-amber-500']"
                :model-value="memoryPct"
                aria-label="宿主机内存使用率"
              />
              <span v-else class="mt-1 text-[11px] text-muted-foreground">读取失败</span>
            </template>
          </div>

          <Tooltip :disabled="!snapshot?.diskPath">
            <TooltipTrigger as-child>
              <div
                class="flex min-w-0 flex-col justify-between rounded-lg border border-border/80 bg-muted/25 px-3 py-2.5"
                :tabindex="snapshot?.diskPath ? 0 : undefined"
              >
                <div class="flex items-center justify-between gap-2">
                  <span class="text-[11.5px] font-medium text-muted-foreground">存档盘</span>
                  <Skeleton v-if="loading" class="h-3 w-28 rounded" />
                  <span v-else-if="disk" class="truncate text-[11px] text-muted-foreground tabular-nums">
                    余 {{ fmtSize(disk.availableBytes) }} / {{ fmtSize(disk.totalBytes) }}
                  </span>
                </div>
                <div v-if="loading" role="status" aria-label="读取存档盘空间" class="mt-1 grid gap-2">
                  <div class="flex h-[22.5px] items-center"><Skeleton class="h-4.5 w-14 rounded" /></div>
                  <Skeleton class="h-1 w-full rounded-full" />
                </div>
                <template v-else>
                  <div class="mt-1 text-lg font-semibold leading-tight tracking-tight tabular-nums">
                    {{ disk ? diskUsedPct.toFixed(1) + '%' : '—' }}
                  </div>
                  <Progress
                    v-if="disk"
                    :class="['mt-2 h-1', diskLow && '[&>[data-slot=progress-indicator]]:bg-amber-500']"
                    :model-value="diskUsedPct"
                    aria-label="存档盘已用空间"
                  />
                  <span v-else class="mt-1 text-[11px] text-muted-foreground">读取失败</span>
                </template>
              </div>
            </TooltipTrigger>
            <TooltipContent class="break-all">{{ snapshot?.diskPath }}</TooltipContent>
          </Tooltip>
        </div>

        <!-- 当前房间进程资源占用 -->
        <div class="flex min-w-0 flex-col justify-between rounded-lg border border-border/80 bg-muted/25 px-3 py-2.5">
          <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
            <span class="text-[11.5px] font-medium text-muted-foreground">当前房间进程占用</span>
            <Skeleton v-if="processLoading" class="h-4 w-20 rounded" />
            <div v-else class="flex items-baseline gap-2.5 tabular-nums">
              <span class="text-sm font-semibold text-foreground">{{ processValue }}</span>
              <span class="text-xs text-muted-foreground">{{ processSub }}</span>
            </div>
          </div>
          <Skeleton v-if="processLoading" class="mt-2 h-1 w-full rounded-full" />
          <Progress
            v-else
            :class="['mt-2 h-1', processCpu > 50 && '[&>[data-slot=progress-indicator]]:bg-amber-500']"
            :model-value="processRunning ? Math.min(100, processCpu) : 0"
            aria-label="进程 CPU 占比"
          />
        </div>

        <!-- 可选：容器内存配额 -->
        <div
          v-if="container"
          class="host-container flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-border/60 bg-muted/15 px-3 py-2 text-xs text-muted-foreground"
          :class="{ 'is-warn': containerLow }"
        >
          <span>容器内存配额</span>
          <strong class="font-medium text-foreground tabular-nums">{{ fmtSize(container.availableBytes) }}</strong>
          <span class="tabular-nums">剩余可用 / 限额 {{ fmtSize(container.totalBytes) }}</span>
          <Progress
            :class="['ml-auto h-1 w-24', containerLow && '[&>[data-slot=progress-indicator]]:bg-amber-500']"
            :model-value="(container.totalBytes - container.availableBytes) / container.totalBytes * 100"
            aria-label="容器内存使用率"
          />
        </div>

        <div
          v-if="errors.length"
          role="status"
          class="grid gap-1 break-words rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-400"
        >
          <p v-for="message in errors" :key="message">{{ message }}</p>
        </div>
      </div>
    </UiCard>
  </section>
</template>
