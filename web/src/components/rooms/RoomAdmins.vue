<script setup>
// 房间管理员：维护 adminlist.txt（每行一个 Klei ID）。
//
// 名单在服务器启动时读取——运行中修改不拦着，但如实提示需重启生效。
// 操作对象由编辑页传入，不随顶部的当前房间选择变化。

import { computed, onMounted, ref, watch } from 'vue';

import Icon from '@/components/ui/Icon.vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { adminApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { lookupPlayer, playerOptionLabel, unseenPlayers } from '@/lib/format.js';
import { guard, toast } from '@/lib/toast.js';
import PlayerIdentity from '@/components/rooms/PlayerIdentity.vue';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';

const props = defineProps({ room: { type: Object, required: true } });

const admins = ref([]);
const seen = ref({});
const picked = ref('');
const file = ref('');
const needRestart = ref(false);
const loading = ref(false);
const loadError = ref('');
const input = ref('');
const busy = ref(false);
const candidates = computed(() => unseenPlayers(seen.value, admins.value));
function who(id) {
  const face = lookupPlayer(seen.value, id);
  return face?.name ? `${face.name}（${id}）` : id;
}
function onPick(id) {
  picked.value = id;
  input.value = id;
}
let reqId = 0;

async function load() {
  const req = ++reqId;
  const room = props.room;
  if (!room) {
    admins.value = [];
    seen.value = {};
    loadError.value = '';
    return;
  }
  loading.value = true;
  loadError.value = '';
  try {
    const data = await adminApi.list(room.id);
    if (req !== reqId || room.id !== props.room?.id) return;
    admins.value = data.admins || [];
    seen.value = data.seen || {};
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
watch(() => props.room?.id, () => {
  picked.value = '';
  load();
});

async function add() {
  const room = props.room;
  const id = input.value.trim();
  if (!room || !id || busy.value) return;
  if (!/^KU_[A-Za-z0-9_-]{1,32}$/i.test(id)) {
    toast('Klei ID 不合法：应为 KU_ 开头、后接字母、数字、下划线或短横线', 'err');
    return;
  }

  const targetRoomId = room.id;
  busy.value = true;
  try {
    const { ok, data } = await guard(() => adminApi.add(targetRoomId, id));
    if (!ok) return;
    if (props.room?.id === targetRoomId) {
      admins.value = data.admins;
      seen.value = data.seen || {};
      needRestart.value = !!data.needRestart;
      input.value = '';
      picked.value = '';
    }
    toast(`已添加管理员 ${who(id)}（重启后生效）`);
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
    if (!await confirmAction(`移除管理员 ${who(id)}？`)) return;

    const { ok, data } = await guard(() => adminApi.remove(targetRoomId, id));
    if (!ok) return;
    if (props.room?.id === targetRoomId) {
      admins.value = data.admins;
      seen.value = data.seen || {};
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
      <Select v-if="candidates.length" :model-value="picked" :disabled="busy" @update:model-value="onPick">
        <SelectTrigger class="mt-2 max-w-xl" aria-label="从进过房间的玩家选择">
          <SelectValue placeholder="从进过房间的玩家选择" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem v-for="player in candidates" :key="player.id" :value="player.id">{{ playerOptionLabel(player) }}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>
      <p class="field-tip" style="margin-top: 6px">
        游戏内按 ` 反引号打开控制台，输入 TheNet:GetUserID() 可查看自己的 Klei ID。
      </p>

      <div v-if="needRestart" class="notice" style="margin-top: 12px">
        <Icon name="info" size="16" />
        <span>房间正在运行：名单的增删将在<strong>下次重启房间</strong>后生效。</span>
      </div>

      <div v-if="admins.length" class="mt-3 grid gap-2">
        <div v-for="id in admins" :key="id" class="flex min-w-0 items-center gap-2 rounded-md border px-3 py-2">
          <PlayerIdentity class="min-w-0 flex-1" :id="id" :seen="seen" />
          <Button variant="destructive" size="sm" :aria-label="`移除管理员 ${who(id)}`" :disabled="busy" @click="remove(id)">
            移除
          </Button>
        </div>
      </div>
      <UiEmpty v-else icon="users" text="暂无管理员。房主（服务器持有者）默认就是管理员" />
    </template>
  </UiCard>
</template>
