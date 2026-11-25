<script setup>
import { ref, watch } from 'vue';
import { TabsList, TabsRoot, TabsTrigger } from 'reka-ui';

import UiEmpty from '@/components/ui/UiEmpty.vue';
import LogPane from '@/components/overview/LogPane.vue';
import { CodeBlock, CodeBlockActions, CodeBlockHeader } from '@/components/ui/code-block';
import IconAction from '@/components/ui/IconAction.vue';
import { useLogs } from '@/composables/useLogs.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const { channels, clear } = useLogs();
const active = ref('');
const label = shard => shardLabel(shard, rooms.current?.locations?.[shard]);
watch([() => rooms.currentId, () => rooms.shards.join(',')], ([roomId], [previousRoomId]) => {
  if (roomId !== previousRoomId || !rooms.shards.includes(active.value)) active.value = rooms.shards[0] || '';
}, { immediate: true });
</script>

<template>
  <section class="min-w-0" aria-label="实时日志">
    <h2 class="sr-only">实时日志</h2>
    <UiEmpty v-if="!rooms.shards.length" text="还没有可用的房间。先在「房间」页新建一个，再回来看日志。" />
    <TabsRoot v-else v-model="active" class="min-w-0">
      <CodeBlock size="sm">
        <CodeBlockHeader class="flex-wrap border-b border-border/75 bg-muted/30 px-3 py-2">
          <TabsList aria-label="日志分片" class="log-tabs">
            <TabsTrigger v-for="shard in rooms.shards" :key="shard" :value="shard" class="log-tab">
              <span class="log-tab-dot" :class="channels[shard]?.connected ? 'is-connected' : ''" aria-hidden="true" />
              <span>{{ label(shard) }}</span>
              <span class="log-pane-sub">{{ shard }}</span>
              <span class="sr-only">{{ channels[shard]?.connected ? '已连接' : '重连中' }}</span>
            </TabsTrigger>
          </TabsList>
          <CodeBlockActions class="log-pane-tools">
            <IconAction variant="ghost" show-label="sm" icon="clean" label="清屏" @click="clear(active)" />
          </CodeBlockActions>
        </CodeBlockHeader>
        <KeepAlive :key="rooms.currentId">
          <LogPane :key="active" :shard="active" />
        </KeepAlive>
      </CodeBlock>
    </TabsRoot>
  </section>
</template>
