<script setup>
// 备份页：整房间备份、回滚、删除。

import { computed, onMounted, onUnmounted, ref, watch } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import { Skeleton } from '@/components/ui/skeleton';
import IconAction from '@/components/ui/IconAction.vue';
import BackupTable from '@/components/backups/BackupTable.vue';
import BackupInspectionDialog from '@/components/backups/BackupInspectionDialog.vue';
import RollbackDialog from '@/components/overview/RollbackDialog.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { backupApi, backupRemoteApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

const rooms = useRoomsStore();
const ui = useUiStore();

const items = ref([]);
const loading = ref(false);
const loadError = ref('');
const inspection = ref(null);
const remote = ref(null);
const remoteError = ref('');
let remoteTimer, disposed = false;
const backupDir = computed(() => (items.value[0]?.path || '').replace(/[\\/][^\\/]+$/, ''));

async function load() {
  const room = rooms.current;
  if (!room) {
    items.value = [];
    loadError.value = '';
    return;
  }
  loading.value = true;
  loadError.value = '';
  try {
    items.value = (await backupApi.list(room.id)) || [];
  } catch (err) {
    loadError.value = err.message;
  } finally {
    loading.value = false;
  }
}

onMounted(load);
async function refreshRemote() {
  clearTimeout(remoteTimer);
  try { const data = await backupRemoteApi.get(); if (!disposed) { remote.value = data; remoteError.value = ''; } }
  catch (err) { if (!disposed && !err.unauthorized) remoteError.value = '读取异地备份状态失败：' + err.message; }
  finally { if (!disposed) remoteTimer = setTimeout(refreshRemote, 3000); }
}
onMounted(refreshRemote);
onUnmounted(() => { disposed = true; clearTimeout(remoteTimer); });
// 用 current 而不是 currentId 做触发（原因见 ModsView）：currentId 刷新后
// 值不变 watch 不到，current 要等房间列表加载完才从 null 变成房间对象
watch(() => rooms.current, () => { items.value = []; load(); });

async function create() {
  const room = rooms.current;
  if (!room) return;
  toast('正在备份…');
  // 备份前后端会先让运行中的分片落盘，可能等好几秒
  const { ok, data } = await guard(() => backupApi.create(room.id));
  if (ok) {
    toast(data.syncError ? '本地备份已完成，异地同步未排队：' + data.syncError : data.syncQueued ? '备份完成，已加入异地同步队列' : '备份完成', data.syncError ? 'err' : undefined);
    load();
  }
}

function restore(name) {
  const room = rooms.current;
  if (!room) return;
  inspection.value = { room, name };
}
async function sync(name) {
  const room = rooms.current;
  if (!room) return;
  await guard(() => backupRemoteApi.sync(room.id, name), { success: '已加入异地同步队列' });
  await refreshRemote();
}

async function remove(name) {
  const room = rooms.current;
  if (!room) return;
  if (!await confirmAction(`删除备份 ${name}？`)) return;

  const { ok } = await guard(() => backupApi.remove(room.id, name), { success: '已删除' });
  if (ok) load();
}

function exportSave(name) {
  const room = rooms.current;
  if (!room) return;
  // 下载失败时浏览器自己会弹错，这里不拦
  backupApi.download(room.id, name);
}
</script>

<template>
  <UiCard flush fill>
    <UiToolbar aria-label="备份工具栏">
      <Badge variant="secondary" class="shrink-0 whitespace-nowrap tabular-nums">共 {{ items.length }} 个备份</Badge>
      <Tooltip v-if="backupDir">
        <TooltipTrigger as-child><code class="min-w-0 truncate bg-transparent p-0 text-xs text-muted-foreground" tabindex="0" aria-label="备份目录">{{ backupDir }}</code></TooltipTrigger>
        <TooltipContent class="max-w-sm break-all">{{ backupDir }}</TooltipContent>
      </Tooltip>
      <template #actions>
        <IconAction variant="outline" show-label="lg" icon="plug" label="异地备份设置" text="异地备份" @click="ui.openSettings('maintenance')" />
        <RollbackDialog show-label="sm" />
        <IconAction show-label="sm" icon="archive" label="立即备份" :disabled="!rooms.current" @click="create" />
      </template>
    </UiToolbar>

    <div data-slot="page-scroll" class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-[var(--panel-padding)]" tabindex="0" role="region" aria-label="备份列表">
      <p v-if="remoteError" role="alert" class="mb-3 text-sm text-destructive">{{ remoteError }}</p>
      <div v-if="!rooms.loaded || (loading && !items.length)" role="status" aria-label="加载中" class="min-w-0 overflow-hidden rounded-md border">
        <div class="flex items-center justify-between gap-4 border-b bg-muted/40 px-4 py-3">
          <Skeleton class="h-3.5 w-12" />
          <Skeleton class="h-3.5 w-12" />
          <Skeleton class="h-3.5 w-12" />
        </div>
        <div v-for="i in 4" :key="i" class="flex items-center justify-between gap-4 border-b px-4 py-3 last:border-b-0">
          <Skeleton class="h-4 w-40" />
          <Skeleton class="h-4 w-16" />
          <div class="flex gap-2">
            <Skeleton v-for="j in 3" :key="j" class="size-7 rounded-md" />
          </div>
        </div>
      </div>
      <UiEmpty v-else-if="!rooms.current">
        <span>请先在「房间管理」页新建一个房间</span>
        <Button size="sm" variant="outline" class="mt-2" @click="ui.openRoomEditor()">前往新建房间</Button>
      </UiEmpty>
      <UiEmpty v-else-if="loadError" :text="'读取备份失败：' + loadError" />
      <UiEmpty v-else-if="!items.length" text="暂无备份" />
      <BackupTable v-else :items="items" :sync-enabled="!!remote?.enabled" :sync-results="(remote?.results || []).filter(item => item.room === rooms.currentId)" @restore="restore" @remove="remove" @export="exportSave" @sync="sync" />
    </div>
    <BackupInspectionDialog v-if="inspection" :room="inspection.room" :name="inspection.name" @close="inspection = null" @restored="load" />
  </UiCard>
</template>
