<script setup>
// 玩家日志页：按分片、类型与关键词查看游戏内聊天、私聊、公告与玩家进出记录。

import { computed, onUnmounted, ref, watch } from 'vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiField from '@/components/ui/UiField.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useRoomsStore } from '@/stores/rooms.js';
import { logApi } from '@/lib/api.js';
import { roomShardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const shard = ref('');
const kind = ref('all');
const search = ref('');
const result = ref(null);
const error = ref('');
const loading = ref(false);
let generation = 0;
let timer;

const shards = computed(() => rooms.current?.shards || []);
const labels = {
  all: '全部类型',
  chat: '聊天',
  whisper: '私聊',
  join: '进入',
  leave: '离开',
  announcement: '公告',
};
const entries = computed(() => result.value?.entries || []);

watch(() => rooms.currentId, () => {
  const list = shards.value;
  shard.value = list.find(item => item.isMaster)?.name || list[0]?.name || '';
  search.value = '';
}, { immediate: true });

watch(() => shards.value.map(item => item.name).join(','), () => {
  if (!shards.value.some(item => item.name === shard.value)) {
    shard.value = shards.value.find(item => item.isMaster)?.name || shards.value[0]?.name || '';
  }
});

async function load(ticket) {
  if (ticket !== generation || !rooms.currentId || !shard.value) return;
  loading.value = true;
  try {
    const data = await logApi.chat(rooms.currentId, shard.value, kind.value, search.value.trim());
    if (ticket !== generation) return;
    result.value = data;
    error.value = '';
  } catch (err) {
    if (ticket !== generation) return;
    result.value = null;
    error.value = err.message;
  } finally {
    if (ticket === generation) {
      loading.value = false;
      timer = setTimeout(() => load(ticket), 5000);
    }
  }
}

function refresh() {
  clearTimeout(timer);
  load(++generation);
}

watch(() => [rooms.currentId, shard.value, kind.value, search.value], () => {
  clearTimeout(timer);
  const ticket = ++generation;
  result.value = null;
  error.value = '';
  loading.value = !!rooms.currentId && !!shard.value;
  timer = setTimeout(() => load(ticket), 250);
}, { immediate: true });

onUnmounted(() => {
  generation++;
  clearTimeout(timer);
});
</script>

<template>
  <UiCard>
    <template #title>玩家日志</template>
    <section aria-label="聊天与进出记录" class="min-w-0 space-y-4">
      <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-[1fr_1fr_2fr_auto] lg:items-end">
        <UiField label="记录分片" for-id="chat_shard" block class="min-w-0">
          <Select v-model="shard" :disabled="!shards.length">
            <SelectTrigger id="chat_shard" class="w-full min-w-0"><SelectValue placeholder="选择分片" /></SelectTrigger>
            <SelectContent position="popper" align="start">
              <SelectGroup>
                <SelectItem v-for="item in shards" :key="item.name" :value="item.name">
                  {{ roomShardLabel(rooms.current, item) }}{{ item.disabled ? '（停用）' : '' }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </UiField>
        <UiField label="记录类型" for-id="chat_kind" block class="min-w-0">
          <Select v-model="kind">
            <SelectTrigger id="chat_kind" class="w-full min-w-0"><SelectValue /></SelectTrigger>
            <SelectContent position="popper" align="start">
              <SelectGroup>
                <SelectItem v-for="(label, key) in labels" :key="key" :value="key">{{ label }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </UiField>
        <UiField label="搜索最近记录" for-id="chat_search" block class="min-w-0">
          <Input id="chat_search" v-model="search" placeholder="昵称、玩家 ID 或内容" maxlength="200" />
        </UiField>
        <Button variant="outline" :disabled="loading || !rooms.currentId || !shard" @click="refresh">刷新记录</Button>
      </div>

      <p v-if="result?.truncated" class="text-xs text-muted-foreground">已截取最近记录；搜索和筛选仅在最近日志范围内进行。</p>
      <p v-if="error" role="alert" class="break-words text-sm text-destructive">读取失败：{{ error }}</p>
      <p v-else-if="loading && !result" role="status" class="text-sm text-muted-foreground">读取记录中…</p>
      <UiEmpty v-else-if="!rooms.current" icon="users" text="请先选择房间" />
      <UiEmpty v-else-if="!result?.exists" icon="inbox" text="该分片还没有聊天日志，开服并产生聊天或进出记录后显示" />
      <UiEmpty v-else-if="!entries.length" icon="inbox" text="最近日志中没有匹配的记录" />
      <ol v-else class="grid max-h-[calc(100svh-19rem)] min-h-72 content-start gap-2 overflow-y-auto pr-1" aria-label="游戏聊天记录">
        <li v-for="entry in entries" :key="entry.offset" class="grid min-w-0 gap-1 rounded-lg border border-border/80 bg-muted/20 px-3.5 py-2.5 text-sm">
          <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span class="font-mono">{{ entry.time }}</span>
            <span class="rounded bg-background px-1.5 py-0.5 font-medium text-foreground shadow-2xs">{{ labels[entry.kind] || entry.kind }}</span>
            <strong class="break-all font-medium text-foreground">{{ entry.name }}</strong>
            <span v-if="entry.userid" class="break-all font-mono">{{ entry.userid }}</span>
          </div>
          <p class="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{{ entry.message }}</p>
        </li>
      </ol>
    </section>
  </UiCard>
</template>
