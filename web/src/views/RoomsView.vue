<script setup>
import { computed, nextTick, onMounted, ref } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Badge } from '@/components/ui/badge';
import IconAction from '@/components/ui/IconAction.vue';
import RoomGrid from '@/components/rooms/RoomGrid.vue';
import RoomEditor from '@/components/rooms/RoomEditor.vue';
import RoomAdmins from '@/components/rooms/RoomAdmins.vue';
import RoomPlayerLists from '@/components/rooms/RoomPlayerLists.vue';
import SaveUploadPanel from '@/components/rooms/SaveUploadPanel.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

const rooms = useRoomsStore();
const editor = ref({ open: false, roomId: null, key: 0 });
const preparing = ref(false);
const uploader = ref({ open: false, roomId: null, key: 0 });
const deleting = ref(false);
const editingRoom = computed(() => rooms.list.find(room => room.id === editor.value.roomId) || null);

onMounted(async () => {
  await guard(() => rooms.load());
  if (!rooms.meta) await guard(() => rooms.loadMeta());
});

function reload() {
  guard(() => rooms.load());
}

async function openEditor(roomId = null) {
  if (preparing.value) return;
  preparing.value = true;
  try {
    if (!rooms.meta) {
      const { ok } = await guard(() => rooms.loadMeta());
      if (!ok) return;
    }
    editor.value = { open: true, roomId: roomId ? Number(roomId) : null, key: editor.value.key + 1 };
    await nextTick();
    document.querySelector('#page-content [data-slot="page-scroll"]')?.scrollTo({ top: 0 });
    document.getElementById('f_name')?.focus({ preventScroll: true });
  } finally {
    preparing.value = false;
  }
}

async function closeEditor() {
  editor.value.open = false;
  await nextTick();
  document.querySelector('#page-content [data-slot="page-scroll"]')?.scrollTo({ top: 0 });
  document.getElementById('new-room-button')?.focus({ preventScroll: true });
}

function onSelect(id) {
  rooms.select(id);
}

function openUpload(roomId = null) {
  uploader.value = { open: true, roomId: roomId ? Number(roomId) : null, key: uploader.value.key + 1 };
}

function closeUpload() {
  uploader.value.open = false;
}

async function onUploaded(data) {
  const { ok } = await guard(() => rooms.load());
  if (ok && data?.room?.id) rooms.select(data.room.id);
}

async function onRemove(id) {
  if (deleting.value) return;
  const room = rooms.list.find(item => item.id === Number(id));
  const name = room ? room.name : '房间';
  deleting.value = true;
  try {
    if (!await confirmAction(`删除「${name}」？\n\n只会从面板移除，磁盘上的存档目录会保留。`)) return;
    const { ok, data } = await guard(() => rooms.remove(id));
    if (ok) toast(data?.message || '房间已删除');
  } finally {
    deleting.value = false;
  }
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <template v-if="editor.open">
      <RoomEditor
        v-if="!editor.roomId || editingRoom"
        :key="editor.key"
        :room="editingRoom"
        :meta="rooms.meta"
        @close="closeEditor"
        @saved="closeEditor"
      >
        <RoomAdmins v-if="editingRoom" :key="editingRoom.id" :room="editingRoom" />
        <RoomPlayerLists v-if="editingRoom" :key="`lists-${editingRoom.id}`" :room="editingRoom" />
      </RoomEditor>
      <UiCard v-else>
        <template #title>房间已不存在</template>
        <template #actions><IconAction variant="outline" icon="chevron" icon-class="rotate-180" label="返回房间列表" @click="closeEditor" /></template>
      </UiCard>
    </template>

    <UiCard v-else flush fill>
      <UiToolbar aria-label="房间工具栏" leading-class="flex-none">
        <Badge variant="secondary" class="whitespace-nowrap tabular-nums">共 {{ rooms.list.length }} 个房间</Badge>
        <template #actions>
          <IconAction id="new-room-button" icon="plus" label="新建房间" :disabled="preparing" @click="openEditor()" />
          <IconAction variant="outline" icon="upload" label="上传存档" @click="openUpload()" />
          <IconAction variant="outline" icon="refresh" label="刷新" @click="reload" />
        </template>
      </UiToolbar>
      <div data-slot="page-scroll" class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-3" tabindex="0" role="region" aria-label="房间列表">
        <RoomGrid
          :rooms="rooms.list"
          :current-id="rooms.currentId"
          @select="onSelect"
          @edit="openEditor"
          @remove="onRemove"
          @upload="openUpload"
        />
      </div>
    </UiCard>

    <SaveUploadPanel
      v-if="uploader.open"
      :key="uploader.key"
      :rooms="rooms.list"
      :preset-room-id="uploader.roomId"
      :current-room-id="rooms.currentId"
      @close="closeUpload"
      @done="onUploaded"
    />
  </div>
</template>
