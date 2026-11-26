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
import RoomProfilesDialog from '@/components/rooms/RoomProfilesDialog.vue';
import MigrationDialog from '@/components/backups/MigrationDialog.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

const rooms = useRoomsStore();
const ui = useUiStore();
const editor = ref({ open: false, roomId: null, section: 'general', key: 0 });
const preparing = ref(false);
const uploader = ref({ open: false, roomId: null, key: 0 });
const deleting = ref(false);
const editingRoom = computed(() => rooms.list.find(room => room.id === editor.value.roomId) || null);
let pendingFocusId = null;

onMounted(async () => {
  await guard(() => rooms.load());
  if (!rooms.meta) await guard(() => rooms.loadMeta());
  const pending = ui.consumePendingEditRoom();
  if (pending !== null) await openEditor(pending.roomId || null, pending.section || 'general');
});

function reload() {
  guard(() => rooms.load());
}

async function onViewEnter() {
  await nextTick();
  document.querySelector('#page-content [data-slot="page-scroll"]')?.scrollTo({ top: 0 });
  if (pendingFocusId) {
    const focusId = pendingFocusId;
    pendingFocusId = null;
    document.getElementById(focusId)?.focus({ preventScroll: true });
  }
}

async function openEditor(roomId = null, section = 'general') {
  if (preparing.value) return;
  preparing.value = true;
  try {
    if (!rooms.meta) {
      const { ok } = await guard(() => rooms.loadMeta());
      if (!ok) return;
    }
    pendingFocusId = section === 'network' ? 'f_token' : 'f_name';
    editor.value = { open: true, roomId: roomId ? Number(roomId) : null, section, key: editor.value.key + 1 };
  } finally {
    preparing.value = false;
  }
}

function closeEditor() {
  pendingFocusId = 'new-room-button';
  editor.value.open = false;
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

async function onProfileCreated(room) {
  const { ok } = await guard(() => rooms.load());
  if (!ok) return;
  rooms.select(room.id);
  await openEditor(room.id);
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
    <Transition name="view-switch" mode="out-in" @enter="onViewEnter">
      <div v-if="editor.open" :key="`editor-${editor.key}`" class="flex min-h-0 flex-1 flex-col">
        <RoomEditor
          v-if="!editor.roomId || editingRoom"
          :key="editor.key"
          :room="editingRoom"
          :meta="rooms.meta"
          :initial-section="editor.section"
          @close="closeEditor"
          @saved="closeEditor"
        >
          <template v-if="editingRoom" #access>
            <RoomAdmins :key="editingRoom.id" :room="editingRoom" />
            <RoomPlayerLists :key="`lists-${editingRoom.id}`" :room="editingRoom" />
          </template>
        </RoomEditor>
        <UiCard v-else>
          <template #title>房间已不存在</template>
          <template #actions><IconAction variant="outline" show-label icon="chevron" icon-class="rotate-180" label="返回房间列表" @click="closeEditor" /></template>
        </UiCard>
      </div>

      <UiCard v-else key="list" flush fill>
        <UiToolbar aria-label="房间工具栏" leading-class="flex-none">
          <Badge variant="secondary" class="whitespace-nowrap tabular-nums">共 {{ rooms.list.length }} 个房间</Badge>
          <template #actions>
            <RoomProfilesDialog @created="onProfileCreated" />
            <MigrationDialog />
            <IconAction id="new-room-button" show-label="sm" icon="plus" label="新建房间" :disabled="preparing" @click="openEditor()" />
            <IconAction variant="outline" show-label="sm" icon="upload" label="上传存档" @click="openUpload()" />
            <IconAction variant="outline" icon="refresh" label="刷新" @click="reload" />
          </template>
        </UiToolbar>
        <div data-slot="page-scroll" class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-[var(--panel-padding)]" tabindex="0" role="region" aria-label="房间列表">
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
    </Transition>

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
