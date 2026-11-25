<script setup>
// 房间管理员：维护 adminlist.txt（每行一个 Klei ID）。
//
// 名单在服务器启动时读取——运行中修改不拦着，但如实提示需重启生效。
// 操作对象由编辑页传入，不随顶部的当前房间选择变化。

import { computed, onMounted, ref, watch } from 'vue';
import { PhInfo } from '@phosphor-icons/vue';

import UiCard from '@/components/ui/UiCard.vue';
import { Empty, EmptyDescription } from '@/components/ui/empty';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
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
    <template #hint>写入 adminlist.txt，修改后需重启房间生效</template>
    <template #actions>
      <IconAction variant="outline" icon="refresh" label="刷新管理员" :disabled="loading || busy" @click="load" />
    </template>

    <div class="min-w-0 space-y-3">
      <p v-if="file" class="truncate rounded-md bg-muted/45 px-2.5 py-1.5 font-mono text-[11px] text-muted-foreground" :title="file">
        {{ file }}
      </p>

      <Empty v-if="!room"><EmptyDescription>请先在「房间」页新建一个房间</EmptyDescription></Empty>
      <p v-else-if="loadError" role="alert" class="text-sm text-destructive">读取失败：{{ loadError }}</p>
      <div v-else-if="loading" role="status" aria-label="读取管理员中" class="grid gap-2">
        <div v-for="i in 2" :key="i" class="flex items-center gap-2 rounded-lg border border-border/80 px-3 py-2">
          <Skeleton class="size-8 shrink-0 rounded-md" />
          <div class="grid flex-1 gap-1.5">
            <Skeleton class="h-4 w-32" />
            <Skeleton class="h-3 w-40" />
          </div>
          <Skeleton class="size-7 shrink-0 rounded-md" />
        </div>
      </div>

      <template v-else>
        <form class="flex flex-wrap gap-2" @submit.prevent="add">
          <Input
            v-model="input"
            aria-label="管理员 Klei ID"
            class="min-w-0 flex-1 basis-48 font-mono"
            placeholder="输入 Klei ID（KU_ 开头）"
            autocomplete="off"
            spellcheck="false"
            :disabled="busy"
          />
          <IconAction type="submit" show-label icon="plus" label="添加" :disabled="busy || !input.trim()" />
          <Select v-if="candidates.length" :model-value="picked" :disabled="busy" @update:model-value="onPick">
            <SelectTrigger class="w-full min-w-0 basis-full" aria-label="从进过房间的玩家选择">
              <SelectValue placeholder="从进过房间的玩家选择" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem v-for="player in candidates" :key="player.id" :value="player.id">{{ playerOptionLabel(player) }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </form>

        <p class="text-xs text-muted-foreground">
          游戏内按 <code>`</code> 打开控制台，输入 <code>TheNet:GetUserID()</code> 可查看自己的 Klei ID。
        </p>

        <div v-if="needRestart" role="status" class="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/8 px-3 py-2 text-xs text-amber-800 dark:text-amber-300">
          <PhInfo class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <span>房间正在运行：名单增删将在下次重启房间后生效。</span>
        </div>

        <ul v-if="admins.length" class="grid gap-2" aria-label="管理员列表">
          <li v-for="id in admins" :key="id" class="flex min-w-0 items-center gap-2 rounded-lg border border-border/80 px-3 py-2">
            <PlayerIdentity class="min-w-0 flex-1" :id="id" :seen="seen" />
            <IconAction
              variant="ghost"
              class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              icon="trash"
              :label="`移除管理员 ${who(id)}`"
              :disabled="busy"
              @click="remove(id)"
            />
          </li>
        </ul>
        <Empty v-else><EmptyDescription>暂无管理员。房主（服务器持有者）默认就是管理员</EmptyDescription></Empty>
      </template>
    </div>
  </UiCard>
</template>
