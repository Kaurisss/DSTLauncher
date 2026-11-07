<script setup>
// 备份页：整房间备份、回滚、删除。

import { computed, onMounted, ref, watch } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiLoading from '@/components/ui/UiLoading.vue';
import IconAction from '@/components/ui/IconAction.vue';
import BackupTable from '@/components/backups/BackupTable.vue';
import RollbackDialog from '@/components/overview/RollbackDialog.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { backupApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

const rooms = useRoomsStore();

const items = ref([]);
const loading = ref(false);
const loadError = ref('');
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
// 用 current 而不是 currentId 做触发（原因见 ModsView）：currentId 刷新后
// 值不变 watch 不到，current 要等房间列表加载完才从 null 变成房间对象
watch(() => rooms.current, () => { items.value = []; load(); });

async function create() {
  const room = rooms.current;
  if (!room) return;
  toast('正在备份…');
  // 备份前后端会先让运行中的分片落盘，可能等好几秒
  const { ok } = await guard(() => backupApi.create(room.id));
  if (ok) {
    toast('备份完成');
    load();
  }
}

async function restore(name) {
  const room = rooms.current;
  if (!room) return;
  if (!await confirmAction(`回滚到 ${name}？\n\n当前存档将被覆盖，需先停止服务器。`)) return;

  const { ok } = await guard(
    () => backupApi.restore(room.id, name),
    { success: '回滚完成，请启动服务器' },
  );
  if (ok) load();
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
        <RollbackDialog />
        <IconAction icon="archive" label="立即备份" :disabled="!rooms.current" @click="create" />
      </template>
    </UiToolbar>

    <div data-slot="page-scroll" class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-[var(--panel-padding)]" tabindex="0" role="region" aria-label="备份列表">
      <UiLoading v-if="!rooms.loaded" />
      <UiEmpty v-else-if="!rooms.current" icon="archive" text="请先在「房间」页新建一个房间" />
      <UiLoading v-else-if="loading && !items.length" />
      <UiEmpty v-else-if="loadError" icon="alert" :text="'读取备份失败：' + loadError" />
      <UiEmpty v-else-if="!items.length" icon="archive" text="暂无备份" />
      <BackupTable v-else :items="items" @restore="restore" @remove="remove" @export="exportSave" />
    </div>
  </UiCard>
</template>
