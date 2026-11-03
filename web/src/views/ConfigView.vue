<script>
/**
 * 上次的预览结果，按房间缓存。
 * App.vue 切页会重建本组件，不缓存的话每次切回来都会先清空再填内容，
 * 肉眼就是一闪。必须是模块级（普通 script 块），写在 setup 里每次实例都是新的。
 */
const cache = new Map();
</script>

<script setup>
// 配置预览页。
//
// 这里刻意只读：房间表单是唯一事实来源，同时允许手改 ini
// 会让两边互相覆盖，用户无法判断哪份才是最新的。

import { onMounted, ref, watch } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiLoading from '@/components/ui/UiLoading.vue';
import IconAction from '@/components/ui/IconAction.vue';
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
  <UiCard flush fill>
    <UiToolbar aria-label="配置预览工具栏">
      <Tooltip v-if="clusterDir">
        <TooltipTrigger as-child>
          <p class="flex min-w-0 items-center gap-1 text-xs text-muted-foreground" tabindex="0"><span class="shrink-0">存档目录：</span><code class="truncate bg-transparent p-0">{{ clusterDir }}</code></p>
        </TooltipTrigger>
        <TooltipContent class="max-w-sm break-all">{{ clusterDir }}</TooltipContent>
      </Tooltip>
      <span v-else class="text-sm text-muted-foreground">存档目录</span>
      <template #actions>
        <IconAction variant="outline" icon="refresh" label="刷新" :disabled="loading || !rooms.current" @click="load" />
      </template>
    </UiToolbar>

    <div data-slot="page-scroll" class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-3" tabindex="0" role="region" aria-label="配置文件列表">
      <UiLoading v-if="!rooms.loaded" />
      <UiEmpty v-else-if="!rooms.current" icon="fileCode" text="请先在「房间」页新建一个房间" />
      <UiLoading v-else-if="loading && !files.length" />
      <UiEmpty v-else-if="loadError" icon="alert" :text="'读取失败：' + loadError" />
      <UiEmpty v-else-if="!files.length" icon="fileCode" text="尚未生成配置文件" />
      <ConfigFiles v-else :files="files" />
    </div>
  </UiCard>
</template>
