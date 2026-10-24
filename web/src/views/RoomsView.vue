<script setup>
import { computed, nextTick, onMounted, ref } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import Icon from '@/components/ui/Icon.vue';
import { Button } from '@/components/ui/button';
import RoomGrid from '@/components/rooms/RoomGrid.vue';
import RoomEditor from '@/components/rooms/RoomEditor.vue';
import RoomAdmins from '@/components/rooms/RoomAdmins.vue';
import SaveUploadPanel from '@/components/rooms/SaveUploadPanel.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

const rooms = useRoomsStore();
const editor = ref({ open: false, roomId: null, key: 0 });
const editorRef = ref(null);
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
    document.getElementById('page-content')?.scrollTo({ top: 0 });
    document.getElementById('f_name')?.focus({ preventScroll: true });
  } finally {
    preparing.value = false;
  }
}

async function closeEditor() {
  editor.value.open = false;
  await nextTick();
  document.getElementById('page-content')?.scrollTo({ top: 0 });
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
  <div class="stack">
    <template v-if="editor.open">
      <nav aria-label="房间导航" class="flex min-w-0 items-center gap-2 text-sm">
        <Button variant="ghost" size="sm" :disabled="editorRef?.saving" @click="closeEditor">
          <Icon name="chevron" class="rotate-180" size="16" />返回房间列表
        </Button>
        <span aria-hidden="true" class="text-muted-foreground">/</span>
        <span aria-current="page">{{ editor.roomId ? '编辑房间' : '新建房间' }}</span>
      </nav>
      <RoomEditor
        v-if="!editor.roomId || editingRoom"
        ref="editorRef"
        :key="editor.key"
        :room="editingRoom"
        :meta="rooms.meta"
        @close="closeEditor"
        @saved="closeEditor"
      />
      <UiCard v-else>
        <template #title>房间已不存在</template>
      </UiCard>
      <RoomAdmins v-if="editingRoom" :key="editingRoom.id" :room="editingRoom" />
    </template>

    <UiCard v-else>
      <template #title>房间列表</template>
      <template #actions>
        <Button id="new-room-button" size="sm" :disabled="preparing" @click="openEditor()">
          <Icon name="plus" size="14" />新建房间
        </Button>
        <Button variant="outline" size="sm" @click="openUpload()">
          <Icon name="upload" size="14" />上传存档
        </Button>
        <Button variant="outline" size="sm" @click="reload">
          <Icon name="refresh" size="14" />刷新
        </Button>
      </template>
      <RoomGrid
        :rooms="rooms.list"
        :current-id="rooms.currentId"
        @select="onSelect"
        @edit="openEditor"
        @remove="onRemove"
        @upload="openUpload"
      />
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
