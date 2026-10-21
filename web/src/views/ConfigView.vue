<script>
/**
 * 上次的预览结果，按房间缓存。
 * App.vue 切页会重建本组件，不缓存的话每次切回来都先铺一次骨架再填内容，
 * 肉眼就是一闪。必须是模块级（普通 script 块），写在 setup 里每次实例都是新的。
 */
const cache = new Map();
</script>

<script setup>
// 配置预览页。
//
// 这里刻意只读：房间表单是唯一事实来源，同时允许手改 ini
// 会让两边互相覆盖，用户无法判断哪份才是最新的。

import { computed, onMounted, ref, watch } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiSkeleton from '@/components/ui/UiSkeleton.vue';
import ConfigFiles from '@/components/config/ConfigFiles.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { configApi } from '@/lib/api.js';

const rooms = useRoomsStore();

const cached = cache.get(rooms.currentId);
const files = ref(cached ? cached.files : []);
const clusterDir = ref(cached ? cached.clusterDir : '');
/** 当前 files 属于哪个房间；换房间时要先切到新房间的缓存 */
let shownRoomId = cached ? rooms.currentId : null;
const loading = ref(false);
const loadError = ref('');

/**
 * 预览产出的文件数：每个启用分片一份 server.ini，再加 cluster.ini
 * 与 cluster_token.txt（后端不分模式都会写出这份）。
 * 骨架按这个数量铺卡，否则加载完页面从 1 张卡涨到 6 张，整页往下蹦一截。
 */
const cardCount = computed(() => {
  const room = rooms.current;
  if (!room) return 2;
  return (room.shards || []).filter(s => !s.disabled).length + 2;
});

async function load() {
  const room = rooms.current;
  if (!room) {
    files.value = [];
    clusterDir.value = '';
    shownRoomId = null;
    return;
  }
  // 换房间：先切到该房间的缓存（没有就清空），不能继续显示上一个房间的配置
  if (shownRoomId !== room.id) {
    const c = cache.get(room.id);
    files.value = c ? c.files : [];
    clusterDir.value = c ? c.clusterDir : '';
    shownRoomId = room.id;
  }
  loading.value = true;
  loadError.value = '';
  try {
    const data = await configApi.preview(room.id);
    files.value = data.files || [];
    clusterDir.value = data.clusterDir || '';
    cache.set(room.id, { files: files.value, clusterDir: clusterDir.value });
  } catch (err) {
    loadError.value = err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(load);
// 用 current 而不是 currentId 做触发（原因见 ModsView）：currentId 刷新后
// 值不变 watch 不到，current 要等房间列表加载完才从 null 变成房间对象
watch(() => rooms.current, load);
</script>

<template>
  <UiCard>
    <template #title>配置预览</template>
    <template #hint>cluster.ini / server.ini 由「房间」表单生成，这里只读</template>
    <template #actions>
      <button class="btn btn-sm" @click="load">刷新</button>
    </template>

    <p v-if="clusterDir" class="field-tip" style="margin-bottom: 14px">
      存档目录：{{ clusterDir }}
    </p>

    <UiSkeleton v-if="!rooms.loaded" variant="code" :rows="cardCount" />
    <UiEmpty v-else-if="!rooms.current" icon="fileCode" text="请先在「房间」页新建一个房间" />
    <!-- 有缓存就不铺骨架：切走再切回时内容直接在线，只做静默刷新 -->
    <UiSkeleton v-else-if="loading && !files.length" variant="code" :rows="cardCount" />
    <UiEmpty v-else-if="loadError" icon="alert" :text="'读取失败：' + loadError" />
    <UiEmpty v-else-if="!files.length" icon="fileCode" text="尚未生成配置文件" />
    <ConfigFiles v-else :files="files" />
  </UiCard>
</template>
