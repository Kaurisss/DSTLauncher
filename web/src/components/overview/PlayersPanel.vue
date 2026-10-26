<script setup>
import { computed, onUnmounted, ref, watch } from 'vue';
import {
  DialogRoot, DialogPortal, DialogOverlay, DialogContent,
  DialogTitle, DialogDescription, DialogClose,
} from 'reka-ui';

import UiCard from '@/components/ui/UiCard.vue';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { adminApi, playerApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { CHARACTERS, shardLabel } from '@/lib/format.js';
import { toast } from '@/lib/toast.js';
import { useRoomsStore } from '@/stores/rooms.js';

const rooms = useRoomsStore();
const queryableShards = computed(() => rooms.statusLoaded ? rooms.statuses
  .filter(status => status.state === 'running' && status.playersReady && rooms.shards.includes(status.shard))
  .map(status => status.shard).sort().join(',') : '');
const canQuery = computed(() => !!rooms.current && !!queryableShards.value);
const players = ref([]);
const errors = ref({});
const loading = ref(false);
const loadError = ref('');
const actionError = ref('');
const busy = ref(false);
const teleport = ref(null);
const targetId = ref('');
let requestId = 0;
let disposed = false;
let timer;

const labelOf = name => shardLabel(name, rooms.current?.locations?.[name]);

const LABELS = { kick: '踢出', ban: '封禁', revive: '复活', teleport: '传送', admin: '设为管理员' };
const targets = computed(() => teleport.value ? players.value.filter(player =>
  player.shard === teleport.value.shard && player.userid !== teleport.value.userid && player.inWorld) : []);
const validTarget = computed(() => targets.value.some(player => player.userid === targetId.value));
const teleportOpen = computed({
  get: () => !!teleport.value,
  set: value => { if (!value && !busy.value) teleport.value = null; },
});

function canTeleport(player) {
  return player.inWorld && players.value.some(target =>
    target.shard === player.shard && target.userid !== player.userid && target.inWorld);
}

async function load() {
  clearTimeout(timer);
  const req = ++requestId;
  const roomId = rooms.currentId;
  if (!canQuery.value) {
    loading.value = false;
    return;
  }
  loading.value = true;
  loadError.value = '';
  try {
    const data = await playerApi.list(roomId);
    if (disposed || req !== requestId || roomId !== rooms.currentId) return;
    players.value = data.players || [];
    errors.value = data.errors || {};
  } catch (err) {
    if (disposed || req !== requestId || roomId !== rooms.currentId) return;
    players.value = [];
    errors.value = {};
    loadError.value = err.message;
  } finally {
    if (!disposed && req === requestId) {
      loading.value = false;
      timer = setTimeout(load, 15000);
    }
  }
}

watch([() => rooms.current?.id, queryableShards], () => {
  clearTimeout(timer);
  players.value = [];
  errors.value = {};
  loadError.value = '';
  actionError.value = '';
  teleport.value = null;
  targetId.value = '';
  load();
}, { immediate: true });
onUnmounted(() => {
  disposed = true;
  requestId++;
  clearTimeout(timer);
});

async function act(player, action, target = '') {
  if (busy.value || !rooms.current) return;
  const roomId = rooms.currentId;
  const roomName = rooms.current.name;
  const name = player.name || player.userid;
  const label = LABELS[action];
  busy.value = true;
  actionError.value = '';
  try {
    if (action !== 'teleport') {
      const description = action === 'ban' ? '封禁后该玩家将无法重新加入此房间。'
        : action === 'kick' ? '玩家会断开连接，之后仍可重新加入。'
          : action === 'admin' ? '写入房间管理员名单，下次重启房间后生效。'
            : '将当前分片中的幽灵玩家复活。';
      if (!await confirmAction({
        title: `${label}「${name}」？`,
        description: `${player.userid} · 房间「${roomName}」 · ${labelOf(player.shard)}。${description}`,
        destructive: action === 'kick' || action === 'ban',
      })) return;
    }
    // A confirmation opened for an old room must never submit after switching rooms.
    if (disposed || roomId !== rooms.currentId) return;
    if (action === 'admin') {
      await adminApi.add(roomId, player.userid);
    } else {
      await playerApi.act(roomId, { shard: player.shard, userid: player.userid, action, ...(target ? { target } : {}) });
    }
    if (disposed || roomId !== rooms.currentId) return;
    teleport.value = null;
    toast(action === 'admin' ? '管理员已保存，下次重启房间后生效' : `${label}指令已执行`);
    await load();
  } catch (err) {
    if (!disposed && roomId === rooms.currentId) actionError.value = err.message;
  } finally {
    busy.value = false;
  }
}

function openTeleport(player) {
  teleport.value = player;
  targetId.value = '';
}

function submitTeleport() {
  if (teleport.value && validTarget.value) act(teleport.value, 'teleport', targetId.value);
}

function preventWhileBusy(event) {
  if (busy.value) event.preventDefault();
}
</script>

<template>
  <section aria-label="在线玩家" class="min-w-0">
    <UiCard>
      <template #title>在线玩家</template>
      <template #actions>
        <Button variant="outline" size="sm" :disabled="!canQuery || loading || busy" @click="load">
          {{ loading ? '查询中…' : '刷新玩家' }}
        </Button>
      </template>

      <p v-if="!rooms.current" class="text-sm text-muted-foreground">请先选择房间</p>
      <p v-else-if="!rooms.statusLoaded" class="text-sm text-muted-foreground">等待分片状态</p>
      <p v-else-if="!canQuery && rooms.statuses.some(status => status.state === 'running')" class="text-sm text-muted-foreground">等待玩家进入世界</p>
      <p v-else-if="!canQuery" class="text-sm text-muted-foreground">{{ rooms.running ? '等待分片启动' : '房间未运行' }}</p>
      <p v-else-if="loading && !players.length" role="status" class="text-sm text-muted-foreground">正在查询玩家…</p>
      <p v-else-if="loadError" role="alert" class="break-words text-sm text-destructive">{{ loadError }}</p>
      <p v-else-if="!players.length && !Object.keys(errors).length" class="text-sm text-muted-foreground">
        暂无在线玩家
      </p>
      <div v-if="Object.keys(errors).length" role="status" class="mb-3 grid gap-1 break-words text-sm text-amber-700 dark:text-amber-400">
        <p v-for="(message, shard) in errors" :key="shard">{{ labelOf(shard) }}：{{ message }}</p>
      </div>
      <p v-if="actionError && !teleportOpen" role="alert" class="mb-3 break-words text-sm text-destructive">{{ actionError }}</p>

      <div v-if="players.length" class="max-h-80 divide-y divide-border overflow-y-auto">
        <article
          v-for="player in players" :key="`${player.shard}:${player.userid}`"
          :aria-label="`${player.name || player.userid} ${player.userid} ${labelOf(player.shard)}`"
          class="flex min-w-0 flex-wrap items-center justify-between gap-3 py-3 first:pt-0 last:pb-0"
        >
          <div class="min-w-0 flex-1 basis-48">
            <div class="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm">
              <strong class="break-all font-medium">{{ player.name || player.userid }}</strong>
              <span class="break-all font-mono text-xs text-muted-foreground">{{ player.userid }}</span>
              <span v-if="player.admin" class="rounded bg-primary/10 px-1.5 py-0.5 text-xs font-medium text-primary">管理员</span>
              <span v-else-if="player.adminSaved" class="text-xs text-muted-foreground">管理员待重启生效</span>
            </div>
            <p class="mt-0.5 break-words text-xs text-muted-foreground">
              {{ labelOf(player.shard) }} · {{ CHARACTERS[player.prefab] || player.prefab || '选角中' }}
              · {{ player.days == null ? '天数未知' : player.days + ' 天' }}{{ player.ghost ? ' · 幽灵' : '' }}
            </p>
          </div>
          <div class="flex max-w-full flex-wrap gap-1.5">
            <Button variant="outline" size="sm" :disabled="busy" @click="act(player, 'kick')">踢出</Button>
            <Button variant="destructive" size="sm" :disabled="busy" @click="act(player, 'ban')">封禁</Button>
            <Button variant="outline" size="sm" :disabled="busy || !player.inWorld || !player.ghost" @click="act(player, 'revive')">复活</Button>
            <Button variant="outline" size="sm" :disabled="busy || !canTeleport(player)" @click="openTeleport(player)">传送</Button>
            <Button variant="outline" size="sm" :disabled="busy || player.admin || player.adminSaved || !/^KU_[A-Za-z0-9_-]{1,32}$/.test(player.userid)" @click="act(player, 'admin')">设为管理员</Button>
          </div>
        </article>
      </div>
    </UiCard>

    <DialogRoot v-model:open="teleportOpen">
      <DialogPortal>
        <DialogOverlay class="fixed inset-0 z-50 bg-black/20" />
        <DialogContent
          class="fixed left-1/2 top-1/2 z-50 grid w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl border bg-popover p-6 text-popover-foreground shadow-lg"
          @escape-key-down="preventWhileBusy" @interact-outside="preventWhileBusy"
        >
          <DialogTitle class="break-all text-lg font-semibold">传送「{{ teleport?.name || teleport?.userid }}」</DialogTitle>
          <DialogDescription class="text-sm text-muted-foreground">将玩家移动到所选同分片玩家的位置。</DialogDescription>
          <form class="grid gap-4" @submit.prevent="submitTeleport">
            <Select v-model="targetId" :disabled="busy">
              <SelectTrigger aria-label="传送目标" class="w-full"><SelectValue placeholder="选择目标玩家" /></SelectTrigger>
              <SelectContent>
                <SelectItem v-for="target in targets" :key="target.userid" :value="target.userid">{{ target.name || target.userid }} · {{ target.userid }}</SelectItem>
              </SelectContent>
            </Select>
            <p v-if="actionError" role="alert" class="break-words text-sm text-destructive">{{ actionError }}</p>
            <div class="flex justify-end gap-2">
              <DialogClose as-child><Button type="button" variant="outline" :disabled="busy">取消</Button></DialogClose>
              <Button type="submit" :disabled="busy || !validTarget">{{ busy ? '执行中…' : '确认传送' }}</Button>
            </div>
          </form>
        </DialogContent>
      </DialogPortal>
    </DialogRoot>
  </section>
</template>
