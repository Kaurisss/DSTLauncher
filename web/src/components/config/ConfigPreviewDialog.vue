<script>
/** 按房间缓存最近一次配置预览，重复打开弹窗时无需闪烁等待。 */
const cache = new Map();
</script>

<script setup>
import { computed, ref, watch } from 'vue';
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger,
} from '@/components/ui/dialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Skeleton } from '@/components/ui/skeleton';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import IconAction from '@/components/ui/IconAction.vue';
import ConfigFiles from '@/components/config/ConfigFiles.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { configApi } from '@/lib/api.js';

const props = defineProps({
  /** 显式指定要预览的房间；省略时跟随当前选中房间。 */
  room: { type: Object, default: null },
  label: { type: String, default: '配置预览' },
  showLabel: { type: [Boolean, String], default: true },
  variant: { type: String, default: 'outline' },
});

const rooms = useRoomsStore();
const targetRoom = computed(() => props.room || rooms.current);

const open = ref(false);
const files = ref([]);
const clusterDir = ref('');
const loading = ref(false);
const loadError = ref('');
let reqId = 0;

function syncCache(roomId) {
  const cached = roomId ? cache.get(roomId) : null;
  files.value = cached ? cached.files : [];
  clusterDir.value = cached ? cached.clusterDir : '';
}

async function load() {
  const room = targetRoom.value;
  if (!room) {
    files.value = [];
    clusterDir.value = '';
    loadError.value = '';
    return;
  }
  const ticket = ++reqId;
  loading.value = true;
  loadError.value = '';
  try {
    const data = await configApi.preview(room.id);
    if (ticket !== reqId || targetRoom.value?.id !== room.id) return;
    files.value = data.files || [];
    clusterDir.value = data.clusterDir || '';
    cache.set(room.id, { files: files.value, clusterDir: clusterDir.value });
  } catch (err) {
    if (ticket === reqId) loadError.value = err.message;
  } finally {
    if (ticket === reqId) loading.value = false;
  }
}

watch(() => targetRoom.value?.id, id => {
  reqId++;
  open.value = false;
  loadError.value = '';
  syncCache(id);
});

watch(open, value => {
  if (!value) return;
  syncCache(targetRoom.value?.id);
  load();
});
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <IconAction icon="fileCode" :variant="variant" :show-label="showLabel" :label="label" :disabled="!targetRoom" />
    </DialogTrigger>
    <DialogContent class="flex max-h-[85dvh] flex-col overflow-hidden sm:max-w-3xl">
      <DialogHeader class="shrink-0">
        <DialogTitle>配置预览{{ targetRoom?.name ? `：${targetRoom.name}` : '' }}</DialogTitle>
        <DialogDescription>只读预览由面板生成的配置文件，修改配置请在房间编辑中保存。</DialogDescription>
      </DialogHeader>

      <div class="flex shrink-0 items-center justify-between gap-3 border-b pb-3">
        <Tooltip v-if="clusterDir">
          <TooltipTrigger as-child>
            <p class="flex min-w-0 items-center gap-1 text-xs text-muted-foreground" tabindex="0">
              <span class="shrink-0">存档目录：</span>
              <code class="truncate bg-transparent p-0">{{ clusterDir }}</code>
            </p>
          </TooltipTrigger>
          <TooltipContent class="max-w-sm break-all">{{ clusterDir }}</TooltipContent>
        </Tooltip>
        <span v-else class="text-xs text-muted-foreground">存档目录</span>
        <IconAction variant="outline" icon="refresh" label="刷新" :disabled="loading || !targetRoom" @click="load" />
      </div>

      <div data-slot="page-scroll" class="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1" tabindex="0" role="region" aria-label="配置文件列表">
        <div v-if="loading && !files.length" role="status" aria-label="加载中" class="grid min-w-0 gap-4">
          <div v-for="i in 2" :key="i" class="overflow-hidden rounded-xl border">
            <div class="flex items-center justify-between gap-2 border-b bg-muted/40 px-4 py-2.5">
              <div class="flex items-center gap-2">
                <Skeleton class="h-4 w-44" />
                <Skeleton class="h-5 w-14 rounded-full" />
              </div>
              <Skeleton class="size-7 rounded-md" />
            </div>
            <div class="grid gap-2 p-4">
              <Skeleton class="h-3.5 w-3/4" />
              <Skeleton class="h-3.5 w-1/2" />
              <Skeleton class="h-3.5 w-5/6" />
              <Skeleton class="h-3.5 w-2/3" />
            </div>
          </div>
        </div>
        <UiEmpty v-else-if="!targetRoom" text="请先在「房间管理」页新建一个房间" />
        <UiEmpty v-else-if="loadError" :text="'读取失败：' + loadError" />
        <UiEmpty v-else-if="!files.length" text="尚未生成配置文件" />
        <ConfigFiles v-else :files="files" />
      </div>
    </DialogContent>
  </Dialog>
</template>
