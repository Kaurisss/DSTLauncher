<script setup>
// 实时日志容器：为每个启用中的分片渲染一个独立面板。
//
// 分片数量由房间决定（未开启洞穴时只有 Master），因此这里是动态列表，
// 而不是写死两个框。

import { computed } from 'vue';

import UiEmpty from '@/components/ui/UiEmpty.vue';
import LogPane from '@/components/overview/LogPane.vue';
import ChatLogPanel from '@/components/overview/ChatLogPanel.vue';
import { Button } from '@/components/ui/button';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const ui = useUiStore();
const mode = computed({ get: () => ui.logView, set: value => { ui.logView = value; } });
const shardNames = computed(() => rooms.shards.map(name =>
  shardLabel(name, rooms.current?.locations?.[name])).join('、'));
</script>

<template>
  <section class="log-section" aria-label="实时日志">
    <div class="log-section-head">
      <h2>实时日志</h2>
      <p>{{ shardNames }}是独立进程，日志分屏查看</p>
    </div>

    <div role="group" aria-label="日志视图" class="mb-3 flex flex-wrap gap-2">
      <Button size="sm" :variant="mode === 'runtime' ? 'default' : 'outline'" :aria-pressed="mode === 'runtime'" @click="mode = 'runtime'">运行日志</Button>
      <Button size="sm" :variant="mode === 'chat' ? 'default' : 'outline'" :aria-pressed="mode === 'chat'" @click="mode = 'chat'">聊天与进出记录</Button>
    </div>

    <ChatLogPanel v-if="mode === 'chat'" />
    <div v-else-if="!rooms.shards.length" class="log-empty">
      <UiEmpty icon="inbox" text="还没有可用的房间。先在「房间」页新建一个，再回来看日志。" />
    </div>

    <div v-else class="log-grid">
      <LogPane v-for="s in rooms.shards" :key="s" :shard="s" />
    </div>
  </section>
</template>
