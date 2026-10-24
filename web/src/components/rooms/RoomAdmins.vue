<script setup>
// 房间管理员：维护 adminlist.txt（每行一个 Klei ID）。
//
// 名单在服务器启动时读取——运行中修改不拦着，但如实提示需重启生效。
// 操作对象由编辑页传入，不随顶部的当前房间选择变化。

import { onMounted, ref, watch } from 'vue';

import Icon from '@/components/ui/Icon.vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { adminApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

const props = defineProps({ room: { type: Object, required: true } });

const admins = ref([]);
const file = ref('');
const needRestart = ref(false);
const loading = ref(false);
const loadError = ref('');
const input = ref('');
const busy = ref(false);
let reqId = 0;

async function load() {
  const req = ++reqId;
  const room = props.room;
  if (!room) {
    admins.value = [];
    loadError.value = '';
    return;
  }
  loading.value = true;
  loadError.value = '';
  try {
    const data = await adminApi.list(room.id);
    if (req !== reqId || room.id !== props.room?.id) return;
    admins.value = data.admins || [];
    file.value = data.file || '';
    needRestart.value = !!data.needRestart;
  } catch (err) {
    if (req !== reqId || room.id !== props.room?.id) return;
    loadError.value = err.message;
  } finally {
    if (req === reqId) loading.value = false;
  }
}

onMounted(load);
watch(() => props.room?.id, load);

async function add() {
  const room = props.room;
  const id = input.value.trim();
  if (!room || !id || busy.value) return;
  if (!/^KU_[A-Za-z0-9]{1,32}$/i.test(id)) {
    toast('Klei ID 不合法：应为 KU_ 开头、后接字母数字', 'err');
    return;
  }

  const targetRoomId = room.id;
  busy.value = true;
  try {
    const { ok, data } = await guard(() => adminApi.add(targetRoomId, id));
    if (!ok) return;
    if (props.room?.id === targetRoomId) {
      admins.value = data.admins;
      needRestart.value = !!data.needRestart;
      input.value = '';
    }
    toast(`已添加管理员 ${id.toUpperCase().slice(0, 8)}…（重启后生效）`);
  } finally {
    busy.value = false;
  }
}

async function remove(id) {
  const room = props.room;
  if (!room || busy.value) return;
  const targetRoomId = room.id;
  busy.value = true;
  try {
    if (!await confirmAction(`移除管理员 ${id}？`)) return;

    const { ok, data } = await guard(() => adminApi.remove(targetRoomId, id));
    if (!ok) return;
    if (props.room?.id === targetRoomId) {
      admins.value = data.admins;
      needRestart.value = !!data.needRestart;
    }
    toast('已移除（重启后生效）');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <UiCard>
    <template #title>房间管理员</template>
    <template #hint>写入 adminlist.txt；名单在服务器启动时读取，修改后需重启房间生效</template>

    <p v-if="file" class="path-line" style="margin-bottom: 12px">{{ file }}</p>

    <UiEmpty v-if="!room" icon="users" text="请先在「房间」页新建一个房间" />
    <p v-else-if="loadError" class="field-tip">读取失败：{{ loadError }}</p>

    <template v-else>
      <div class="inline-row" style="max-width: 560px">
        <Input
          v-model="input"
          class="mono"
          placeholder="输入 Klei ID（KU_ 开头）"
          autocomplete="off"
          spellcheck="false"
          @keydown.enter="add"
        />
        <Button :disabled="busy || !input.trim()" @click="add">
          <Icon name="plus" size="14" />添加
        </Button>
      </div>
      <p class="field-tip" style="margin-top: 6px">
        游戏内按 ` 反引号打开控制台，输入 TheNet:GetUserID() 可查看自己的 Klei ID。
      </p>

      <div v-if="needRestart" class="notice" style="margin-top: 12px">
        <Icon name="info" size="16" />
        <span>房间正在运行：名单的增删将在<strong>下次重启房间</strong>后生效。</span>
      </div>

      <div v-if="admins.length" class="admin-list">
        <div v-for="id in admins" :key="id" class="admin-row">
          <Icon class="admin-row-icon" name="users" size="14" />
          <span class="mono">{{ id }}</span>
          <Button variant="destructive" size="sm" class="h-6 rounded-full px-2 text-xs" :disabled="busy" @click="remove(id)">
            移除
          </Button>
        </div>
      </div>
      <UiEmpty v-else icon="users" text="暂无管理员。房主（服务器持有者）默认就是管理员" />
    </template>
  </UiCard>
</template>
