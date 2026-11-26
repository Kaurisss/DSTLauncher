<script setup>
// 控制台页：满屏实时日志流、分片轻控、关键词与告警筛选、快捷指令与命令历史。

import { computed, ref, watch } from 'vue';
import { useClipboard } from '@vueuse/core';
import { TabsList, TabsRoot, TabsTrigger } from 'reka-ui';
import { PhCheck, PhCopy, PhMagnifyingGlass, PhX } from '@phosphor-icons/vue';

import UiCard from '@/components/ui/UiCard.vue';
import { Empty, EmptyContent, EmptyDescription } from '@/components/ui/empty';
import LogPane from '@/components/overview/LogPane.vue';
import IconAction from '@/components/ui/IconAction.vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { useLogs } from '@/composables/useLogs.js';
import { useProcessControl } from '@/composables/useProcessControl.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useUiStore } from '@/stores/ui.js';
import { STATE_LABEL, shardLabel } from '@/lib/format.js';
import { guard, toast } from '@/lib/toast.js';

const rooms = useRoomsStore();
const system = useSystemStore();
const ui = useUiStore();
const proc = useProcessControl();
const { channels, clear } = useLogs();

const active = ref('');
const filterText = ref('');
const level = ref('all');
const { copy, copied, isSupported: clipboardSupported } = useClipboard({ copiedDuring: 1600, legacy: true });
const paneRef = ref(null);

const label = shard => shardLabel(shard, rooms.current?.locations?.[shard]);

watch([() => rooms.currentId, () => rooms.shards.join(',')], ([roomId], [previousRoomId]) => {
  if (roomId !== previousRoomId || !rooms.shards.includes(active.value)) {
    active.value = rooms.shards[0] || '';
  }
}, { immediate: true });

const activeStatus = computed(() => (
  rooms.statuses.find(s => s.shard === active.value) || { shard: active.value, state: 'stopped', placeholder: true }
));
const activeLive = computed(() => activeStatus.value.state === 'running' || activeStatus.value.state === 'starting');
const activeStateText = computed(() => (
  activeStatus.value.placeholder ? '未启动' : STATE_LABEL[activeStatus.value.state] || activeStatus.value.state
));
const activeBadgeClass = computed(() => {
  const st = activeStatus.value.state;
  if (st === 'running') return 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400';
  if (st === 'starting' || st === 'stopping') return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400';
  if (st === 'crashed' || st === 'failed') return 'border-destructive/30 bg-destructive/10 text-destructive';
  return '';
});

const lineCounts = computed(() => {
  const total = channels[active.value]?.lines?.length || 0;
  const c = paneRef.value?.counts;
  return {
    visible: c ? c.visible : total,
    total: c ? c.total : total,
  };
});

async function copyLogs() {
  const text = paneRef.value?.getVisibleText?.() || '';
  if (!text) {
    toast('当前没有可复制的日志', 'warn');
    return;
  }
  if (!clipboardSupported.value) {
    toast('当前连接不支持剪贴板访问，请在日志区手动选中复制', 'warn');
    return;
  }
  await guard(() => copy(text), {
    success: `已复制 ${lineCounts.value.visible} 行日志`,
  });
}
</script>

<template>
  <UiCard flush fill role="region" aria-label="实时控制台" class="min-w-0">
    <div v-if="!rooms.loaded && !rooms.current" role="status" aria-label="正在加载控制台" class="flex min-h-0 flex-1 flex-col">
      <div class="flex items-center justify-between gap-2 border-b border-border/75 bg-muted/30 px-3 py-2">
        <div class="flex items-center gap-2">
          <Skeleton class="h-7 w-24 rounded-md" />
          <Skeleton class="h-7 w-24 rounded-md" />
          <Skeleton class="h-5 w-12 rounded-full" />
        </div>
        <div class="flex items-center gap-1.5">
          <Skeleton class="h-7 w-16 rounded-md" />
          <Skeleton class="h-7 w-20 rounded-md" />
        </div>
      </div>
      <div class="flex items-center justify-between gap-2 border-b border-border/60 bg-card px-3 py-2">
        <Skeleton class="h-7.5 w-64 rounded-md" />
        <Skeleton class="h-7 w-36 rounded-md" />
      </div>
      <div class="flex-1 p-4">
        <div class="grid gap-2">
          <Skeleton v-for="w in ['w-3/4', 'w-2/3', 'w-4/5', 'w-1/2', 'w-3/5']" :key="w" :class="['h-3.5 rounded', w]" />
        </div>
      </div>
      <div class="flex items-center gap-2 border-t border-border/75 px-3 py-2.5">
        <Skeleton class="h-8 w-20 rounded-md" />
        <Skeleton class="h-8 flex-1 rounded-md" />
        <Skeleton class="h-8 w-16 rounded-md" />
      </div>
    </div>

    <Empty v-else-if="!rooms.current || !rooms.shards.length" class="flex-1 justify-center">
      <EmptyDescription>还没有可用的房间或启用分片，请先在「房间管理」中配置。</EmptyDescription>
      <EmptyContent>
        <Button size="sm" variant="outline" @click="ui.openRoomEditor()">前往房间管理</Button>
      </EmptyContent>
    </Empty>

    <TabsRoot v-else v-model="active" class="flex min-h-0 min-w-0 flex-1 flex-col">
      <!-- 顶部主控制栏：分片标签 + 轻控启停 + 日志操作 -->
      <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border/75 bg-muted/30 px-3 py-2">
        <div class="flex min-w-0 flex-wrap items-center gap-2">
          <TabsList aria-label="日志分片" class="log-tabs">
            <TabsTrigger v-for="shard in rooms.shards" :key="shard" :value="shard" class="log-tab">
              <span class="log-tab-dot" :class="channels[shard]?.connected ? 'is-connected' : ''" aria-hidden="true" />
              <span>{{ label(shard) }}</span>
              <span class="log-pane-sub">{{ shard }}</span>
              <span class="sr-only">{{ channels[shard]?.connected ? '已连接' : '重连中' }}</span>
            </TabsTrigger>
          </TabsList>
          <Badge variant="secondary" :class="['h-5 px-2 text-[11px] font-medium', activeBadgeClass]">
            {{ activeStateText }}
          </Badge>
        </div>

        <div class="ml-auto flex flex-wrap items-center gap-1.5">
          <template v-if="activeLive">
            <IconAction variant="outline" show-label="md" icon="restart" label="重启此分片" text="重启" :disabled="rooms.starting || system.installing" @click="proc.restart(active)" />
            <IconAction variant="destructive" show-label="md" icon="stop" label="停止此分片" text="停止" :disabled="rooms.starting" @click="proc.stop(active)" />
          </template>
          <IconAction
            v-else
            variant="default"
            show-label="md"
            :icon="system.loaded && !system.installed ? 'download' : 'play'"
            :label="rooms.starting ? '请稍候…' : (system.loaded && !system.installed ? '安装并启动' : '启动此分片')"
            :text="rooms.starting ? '请稍候…' : (system.loaded && !system.installed ? '安装并启动' : '启动')"
            :disabled="rooms.starting"
            @click="proc.start(active)"
          />

          <Separator orientation="vertical" class="mx-0.5 hidden h-4 sm:block" />

          <Button
            type="button"
            variant="outline"
            size="sm"
            class="h-7 gap-1.5 ps-2 pe-2.5 text-xs"
            :disabled="!lineCounts.visible"
            @click="copyLogs"
          >
            <component :is="copied ? PhCheck : PhCopy" :class="['size-3.5', copied && 'text-emerald-600 dark:text-emerald-400']" aria-hidden="true" />
            <span class="hidden sm:inline">{{ copied ? '已复制' : '复制日志' }}</span>
          </Button>
          <IconAction variant="ghost" show-label="sm" icon="clean" label="清屏" @click="clear(active)" />
        </div>
      </div>

      <!-- 次级筛选栏：关键词过滤 + 告警/错误聚焦 -->
      <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border/60 bg-card px-3 py-2">
        <div class="relative min-w-0 flex-1 basis-56 max-w-md">
          <PhMagnifyingGlass class="pointer-events-none absolute start-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            v-model="filterText"
            class="h-7.5 bg-background ps-8 pe-8 text-xs"
            aria-label="筛选实时日志"
            placeholder="筛选关键词（如 Error、workshop、玩家名）"
          />
          <button
            v-if="filterText"
            type="button"
            aria-label="清除日志筛选"
            class="absolute end-1.5 top-1/2 grid size-5 -translate-y-1/2 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
            @click="filterText = ''"
          >
            <PhX class="size-3" aria-hidden="true" />
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2.5">
          <div class="seg" role="group" aria-label="日志级别筛选">
            <button
              type="button"
              class="seg-btn"
              :class="{ 'is-active': level === 'all' }"
              :aria-pressed="level === 'all'"
              @click="level = 'all'"
            >
              全部
            </button>
            <button
              type="button"
              class="seg-btn"
              :class="{ 'is-active': level === 'issues' }"
              :aria-pressed="level === 'issues'"
              @click="level = 'issues'"
            >
              仅告警与错误
            </button>
          </div>
          <span class="text-xs text-muted-foreground tabular-nums">
            <template v-if="filterText.trim() || level === 'issues'">
              匹配 {{ lineCounts.visible }} / {{ lineCounts.total }} 行
            </template>
            <template v-else>
              共 {{ lineCounts.total }} 行
            </template>
          </span>
        </div>
      </div>

      <!-- 满屏实时日志区 + 底部命令输入栏 -->
      <KeepAlive :key="rooms.currentId">
        <LogPane
          :key="active"
          ref="paneRef"
          :shard="active"
          :filter="filterText"
          :level="level"
          full-height
        />
      </KeepAlive>
    </TabsRoot>
  </UiCard>
</template>
