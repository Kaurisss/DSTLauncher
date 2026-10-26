<script setup>
import { ref, watch } from 'vue';
import { TabsList, TabsRoot, TabsTrigger } from 'reka-ui';

import UiEmpty from '@/components/ui/UiEmpty.vue';
import LogPane from '@/components/overview/LogPane.vue';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useLogs } from '@/composables/useLogs.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const { channels, clear } = useLogs();
const active = ref('');
const pane = ref(null);
const label = shard => shardLabel(shard, rooms.current?.locations?.[shard]);
watch([() => rooms.currentId, () => rooms.shards.join(',')], ([roomId], [previousRoomId]) => {
  if (roomId !== previousRoomId || !rooms.shards.includes(active.value)) active.value = rooms.shards[0] || '';
}, { immediate: true });
</script>

<template>
  <section class="min-w-0" aria-label="实时日志">
    <h2 class="sr-only">实时日志</h2>
    <UiEmpty v-if="!rooms.shards.length" icon="inbox" text="还没有可用的房间。先在「房间」页新建一个，再回来看日志。" />
    <TabsRoot v-else v-model="active" class="log-pane">
      <header class="log-pane-head">
        <TabsList aria-label="日志分片" class="log-tabs">
          <TabsTrigger v-for="shard in rooms.shards" :key="shard" :value="shard" class="log-tab">
            <span class="log-tab-dot" :class="channels[shard]?.connected ? 'is-connected' : ''" aria-hidden="true" />
            {{ label(shard) }}
            <span class="log-pane-sub">{{ shard }}</span>
            <span class="sr-only">{{ channels[shard]?.connected ? '已连接' : '重连中' }}</span>
          </TabsTrigger>
        </TabsList>
        <div class="log-pane-tools">
          <label class="log-autoscroll">
            <Tooltip>
              <TooltipTrigger as-child>
                <Switch :model-value="pane?.autoscroll ?? true" :aria-label="`${label(active)} 自动滚动`" @update:model-value="value => pane.autoscroll = value" />
              </TooltipTrigger>
              <TooltipContent>关闭后可以安静地往上翻历史，滚回底部会自动重新打开</TooltipContent>
            </Tooltip>
            <span>自动滚动</span>
          </label>
          <Button variant="ghost" size="sm" @click="clear(active)">清屏</Button>
        </div>
      </header>
      <KeepAlive :key="rooms.currentId">
        <LogPane :key="active" ref="pane" :shard="active" />
      </KeepAlive>
    </TabsRoot>
  </section>
</template>
