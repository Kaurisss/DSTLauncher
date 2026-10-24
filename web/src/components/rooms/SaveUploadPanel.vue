<script setup>
import { computed, ref, watch } from 'vue';

import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Field, FieldGroup, FieldLabel, FieldTitle } from '@/components/ui/field';
import UiPill from '@/components/ui/UiPill.vue';
import UiProgress from '@/components/ui/UiProgress.vue';
import Icon from '@/components/ui/Icon.vue';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { saveApi } from '@/lib/api.js';
import { toast } from '@/lib/toast.js';
import { fmtSize } from '@/lib/format.js';

const props = defineProps({
  rooms: { type: Array, default: () => [] },
  presetRoomId: { type: Number, default: null },
  currentRoomId: { type: Number, default: null },
});
const emit = defineEmits(['close', 'done']);
const opener = typeof document === 'undefined' ? null : document.activeElement;
const MODE_OVERWRITE = 'overwrite';
const MODE_NEW = 'new';
const mode = ref(props.presetRoomId ? MODE_OVERWRITE : MODE_NEW);
const roomId = ref(props.presetRoomId || props.currentRoomId || props.rooms[0]?.id || null);
const file = ref(null);
const fileInput = ref(null);
const dragging = ref(false);
const busy = ref(false);
const percent = ref(0);
const error = ref('');
const result = ref(null);
const selectedRoom = computed(() => props.rooms.find(room => room.id === roomId.value) || null);
const targetRoomLabel = computed(() => selectedRoom.value
  ? `${selectedRoom.value.name}（${selectedRoom.value.key}）` : '选择房间');
const running = computed(() => (selectedRoom.value?.statuses || []).some(status =>
  ['running', 'starting', 'stopping'].includes(status.state)));
const busyText = computed(() => percent.value >= 100
  ? '上传完成，正在解压并导入…' : `正在上传 ${percent.value}%`);
const submitLabel = computed(() => mode.value === MODE_NEW ? '上传并新建房间'
  : selectedRoom.value ? `覆盖「${selectedRoom.value.name}」` : '上传');
const canSubmit = computed(() => !busy.value && !!file.value
  && (mode.value === MODE_NEW || (!!selectedRoom.value && !running.value)));

watch(() => props.rooms.length, count => {
  if (!count && !busy.value) {
    mode.value = MODE_NEW;
    roomId.value = null;
  }
}, { immediate: true });

function openFilePicker() {
  if (!busy.value) fileInput.value?.click();
}

function chooseFile(picked) {
  if (busy.value || !picked) return;
  error.value = '';
  file.value = null;
  result.value = null;
  if (!/\.zip$/i.test(picked.name)) {
    error.value = '请选择 .zip 压缩包（Klei 下载的整包，或面板导出的备份）';
    return;
  }
  file.value = picked;
}

function onPick(event) {
  chooseFile(event.target.files?.[0]);
  event.target.value = '';
}

function onDrop(event) {
  dragging.value = false;
  chooseFile(event.dataTransfer?.files?.[0]);
}

function onOpenChange(open) {
  if (!open && !busy.value) emit('close');
}

function preventWhileBusy(event) {
  if (busy.value) event.preventDefault();
}

function restoreFocus(event) {
  if (opener?.isConnected) {
    event.preventDefault();
    opener.focus();
  }
}

async function submit() {
  if (!canSubmit.value) return;
  const uploadMode = mode.value;
  const targetId = roomId.value;
  error.value = '';
  result.value = null;
  busy.value = true;
  percent.value = 0;
  try {
    const data = await saveApi.upload(file.value,
      { mode: uploadMode, roomId: uploadMode === MODE_OVERWRITE ? targetId : undefined },
      progress => { percent.value = progress; });
    result.value = data?.summary || null;
    file.value = null;
    toast(uploadMode === MODE_NEW ? '存档已导入并新建房间' : '存档已导入，房间配置已覆盖');
    emit('done', data);
  } catch (err) {
    error.value = err.message;
  } finally {
    busy.value = false;
    percent.value = 0;
  }
}
</script>

<template>
  <Dialog :open="true" @update:open="onOpenChange">
    <DialogContent
      class="flex flex-col overflow-hidden sm:max-w-lg"
      :close-disabled="busy"
      @escape-key-down="preventWhileBusy"
      @interact-outside="preventWhileBusy"
      @close-auto-focus="restoreFocus"
    >
      <DialogHeader class="shrink-0">
        <DialogTitle>上传存档</DialogTitle>
        <DialogDescription class="sr-only">导入 ZIP 存档，新建房间或覆盖已停止的房间。</DialogDescription>
      </DialogHeader>

      <div class="-mx-4 min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-1">
        <FieldGroup v-if="!result" class="gap-4">
          <Field>
            <FieldTitle>存档压缩包</FieldTitle>
            <div
              role="button"
              :tabindex="busy ? -1 : 0"
              :aria-disabled="busy || undefined"
              class="dropzone flex-wrap focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              :class="{ 'is-over': dragging && !busy, 'is-filled': !!file }"
              @click="openFilePicker"
              @keydown.enter.prevent="openFilePicker"
              @keydown.space.prevent="openFilePicker"
              @dragover.prevent="dragging = true"
              @dragleave.prevent="dragging = false"
              @drop.prevent="onDrop"
            >
              <Icon :name="file ? 'archive' : 'upload'" size="22" />
              <div class="dropzone-text">
                <b>{{ file?.name || '点击选择或将 .zip 存档拖到这里' }}</b>
                <span v-if="file">{{ fmtSize(file.size) }}</span>
              </div>
              <Button v-if="file" type="button" variant="outline" size="sm" :disabled="busy" @click.stop="openFilePicker">
                重新选择
              </Button>
            </div>
            <input ref="fileInput" class="dropzone-input" type="file" accept=".zip,application/zip" :disabled="busy" aria-label="存档压缩包" @change="onPick">
          </Field>

          <Field>
            <FieldTitle>导入到</FieldTitle>
            <RadioGroup v-model="mode" class="flex flex-wrap items-center gap-5">
              <label class="flex cursor-pointer items-center gap-2 text-sm">
                <RadioGroupItem :value="MODE_NEW" :disabled="busy" /><span>新建房间</span>
              </label>
              <label class="flex cursor-pointer items-center gap-2 text-sm">
                <RadioGroupItem :value="MODE_OVERWRITE" :disabled="!rooms.length || busy" /><span>覆盖已有房间</span>
              </label>
            </RadioGroup>
          </Field>

          <Field v-if="mode === MODE_OVERWRITE">
            <FieldLabel for="s_target">目标房间</FieldLabel>
            <Select :model-value="roomId ? String(roomId) : ''" :disabled="busy || !rooms.length" @update:model-value="value => roomId = Number(value)">
              <SelectTrigger id="s_target" class="w-full min-w-0"><SelectValue :placeholder="targetRoomLabel" /></SelectTrigger>
              <SelectContent position="popper">
                <SelectGroup>
                  <SelectItem v-for="room in rooms" :key="room.id" :value="String(room.id)">{{ room.name }}（{{ room.key }}）</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>

          <Alert v-if="mode === MODE_OVERWRITE && running">
            <AlertTitle>房间正在运行</AlertTitle>
            <AlertDescription>请先停止该房间的全部分片，再覆盖存档。</AlertDescription>
          </Alert>
          <Alert v-else-if="mode === MODE_OVERWRITE && selectedRoom" variant="destructive">
            <AlertTitle class="break-all">将替换「{{ selectedRoom.name }}」的存档</AlertTitle>
            <AlertDescription>世界数据、模组和玩家名单按压缩包导入；端口、集群密钥与主分片 IP 保留。</AlertDescription>
          </Alert>

          <div v-if="busy" role="status" class="grid gap-2">
            <UiProgress :value="percent" />
            <p class="text-sm text-muted-foreground">{{ busyText }}</p>
          </div>
          <Alert v-if="error" variant="destructive">
            <AlertTitle>导入失败</AlertTitle><AlertDescription>{{ error }}</AlertDescription>
          </Alert>
        </FieldGroup>

        <section v-else aria-label="导入结果" class="grid gap-3">
          <h3 class="text-sm font-medium">导入结果</h3>
          <div class="import-summary">
            <div class="import-row"><span>房间</span><b>{{ result.roomName }}</b><UiPill :tone="result.newRoom ? 'ok' : 'info'">{{ result.newRoom ? '新建' : '覆盖' }}</UiPill></div>
            <div class="import-row"><span>分片</span><b>{{ (result.shards || []).join(' + ') || '—' }}</b></div>
            <div class="import-row"><span>世界数据</span><b>{{ result.hasSave ? '已导入' : '存档包里没有世界数据' }}</b></div>
            <div class="import-row">
              <span>模组</span>
              <b v-if="result.modsTotal">已导入 {{ result.modsTotal }} 个<template v-if="(result.modsMissing || []).length">，其中 {{ result.modsMissing.length }} 个未下载，需到「模组」页点「更新模组」</template></b>
              <b v-else>存档里没有模组配置，房间原有模组保持不变</b>
            </div>
            <div class="import-row"><span>令牌</span><b>{{ result.tokenSource }}</b></div>
            <div v-if="(result.worldFiles || []).length" class="import-row"><span>世界设置</span><div class="import-list"><b v-for="(item, index) in result.worldFiles" :key="index">{{ item }}</b></div></div>
            <div v-if="(result.listFiles || []).length" class="import-row"><span>玩家名单</span><b>{{ result.listFiles.join('、') }}</b></div>
            <div v-if="(result.notes || []).length" class="import-row"><span>说明</span><div class="import-list"><b v-for="(item, index) in result.notes" :key="index">{{ item }}</b></div></div>
            <div v-if="(result.warnings || []).length" class="import-row"><span>警告</span><div class="import-list import-list-warn"><b v-for="(item, index) in result.warnings" :key="index">{{ item }}</b></div></div>
          </div>
        </section>
      </div>

      <DialogFooter class="shrink-0">
        <DialogClose v-if="result" as-child><Button>完成</Button></DialogClose>
        <template v-else>
          <DialogClose as-child><Button variant="outline" :disabled="busy">取消</Button></DialogClose>
          <Button class="min-w-0 max-w-full" :disabled="!canSubmit" @click="submit">
            <Icon name="upload" size="14" /><span class="truncate">{{ busy ? '处理中…' : submitLabel }}</span>
          </Button>
        </template>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
