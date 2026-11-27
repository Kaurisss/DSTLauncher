<script setup>
import { computed, onUnmounted, ref, watch } from 'vue';
import { useTimeoutPoll } from '@vueuse/core';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import IconAction from '@/components/ui/IconAction.vue';
import { Skeleton } from '@/components/ui/skeleton';
import SavedMapViewport from '@/components/rooms/SavedMapViewport.vue';
import { decodeTerrain } from '@/lib/mapTerrain.js';
import {
  buildPairedLandmarkIconMap,
  formatWorldCoordinates,
  landmarkCoordinates,
  landmarkIconUrl,
  landmarkStyle,
  playerAvatarUrl,
  worldToMapPoint,
} from '@/lib/mapLandmarks.js';
import { mapApi, playerApi, procApi } from '@/lib/api.js';
import { CHARACTERS, fmtTime, shardLabel } from '@/lib/format.js';
import { useRoomsStore } from '@/stores/rooms.js';

defineProps({
  showLabel: { type: [Boolean, String], default: 'sm' },
});

const rooms = useRoomsStore();
const SKELETON_LANDMARKS = [
  { name: '64%', coord: '58px' },
  { name: '78%', coord: '64px' },
  { name: '52%', coord: '54px' },
  { name: '72%', coord: '62px' },
  { name: '60%', coord: '60px' },
  { name: '82%', coord: '56px' },
  { name: '56%', coord: '64px' },
  { name: '70%', coord: '58px' },
];
const SKELETON_LEGEND_WIDTHS = [
  '36px', '48px', '42px', '56px', '38px', '52px', '44px', '48px', '36px', '60px',
  '42px', '48px', '54px', '40px', '46px', '52px', '38px', '50px', '44px', '48px',
];
const open = ref(false);
const roomId = ref('');
const shard = ref('');
const data = ref(null);
const error = ref('');
const loading = ref(false);
const zoom = ref(1);
const showLandmarks = ref(true);
const showPlayers = ref(true);
const selectedId = ref('');
const selectedPlayerId = ref('');
const search = ref('');
const fetchedStatuses = ref([]);
const rawPlayers = ref([]);
const playersLoading = ref(false);
const playersError = ref('');

const room = computed(() => rooms.list.find(item => String(item.id) === roomId.value));
const activeStatuses = computed(() => (
  Number(roomId.value) === rooms.currentId && rooms.statusLoaded
    ? rooms.statuses
    : fetchedStatuses.value
));
const shardStatus = computed(() => activeStatuses.value.find(item => item.shard === shard.value) || null);
const shardRunning = computed(() => shardStatus.value?.state === 'running');
const canPollPlayers = computed(() => (
  open.value
  && showPlayers.value
  && !!roomId.value
  && !!shard.value
  && shardStatus.value?.state === 'running'
  && !!shardStatus.value?.playersReady
));
const landmarks = computed(() => (data.value?.landmarks || []).filter(point => `${point.name} ${point.kind}`.toLowerCase().includes(search.value.trim().toLowerCase())));
const iconKinds = computed(() => new Set(data.value?.icons || []));
const pairedIconMap = computed(() => buildPairedLandmarkIconMap(data.value?.landmarks || []));
const iconUrlOf = point => landmarkIconUrl(point, iconKinds.value, pairedIconMap.value);
const mapPlayers = computed(() => {
  if (!data.value) return [];
  const result = [];
  for (const player of rawPlayers.value) {
    if (player.shard !== shard.value || !player.inWorld) continue;
    const projected = worldToMapPoint(player.x, player.z, data.value);
    if (!projected) continue;
    result.push({
      ...player,
      worldX: player.x,
      worldZ: player.z,
      x: projected.x,
      y: projected.y,
      avatar: playerAvatarUrl(player.prefab),
      character: CHARACTERS[player.prefab] || player.prefab || '未知角色',
    });
  }
  return result;
});
const playerEmptyHint = computed(() => {
  if (!shardRunning.value) return '当前分片未运行';
  if (!shardStatus.value?.playersReady) return '等待玩家进入世界';
  if (playersError.value) return playersError.value;
  if (playersLoading.value) return '正在读取在线位置…';
  if (!mapPlayers.value.length) return '当前分片暂无在线玩家';
  return '';
});

let revision = 0;
let statusRevision = 0;
let playerRevision = 0;
let disposed = false;

async function syncRoomStatuses() {
  const id = Number(roomId.value);
  if (!open.value || !id || (id === rooms.currentId && rooms.statusLoaded)) {
    fetchedStatuses.value = [];
    return;
  }
  const current = ++statusRevision;
  try {
    const list = await procApi.status(id);
    if (!disposed && current === statusRevision && open.value && Number(roomId.value) === id) {
      fetchedStatuses.value = Array.isArray(list) ? list : [];
    }
  } catch {
    if (!disposed && current === statusRevision) fetchedStatuses.value = [];
  }
}

async function loadPlayers() {
  const req = ++playerRevision;
  const targetRoom = Number(roomId.value);
  const targetShard = shard.value;
  if (!canPollPlayers.value) {
    playersLoading.value = false;
    pausePlayers();
    return;
  }
  playersLoading.value = true;
  playersError.value = '';
  try {
    const res = await playerApi.list(targetRoom, targetShard);
    if (disposed || req !== playerRevision || !open.value || Number(roomId.value) !== targetRoom || shard.value !== targetShard) return;
    rawPlayers.value = (res.players || []).filter(item => item.shard === targetShard);
    playersError.value = res.errors?.[targetShard] || '';
  } catch (err) {
    if (disposed || req !== playerRevision || !open.value || Number(roomId.value) !== targetRoom || shard.value !== targetShard) return;
    rawPlayers.value = [];
    playersError.value = err.message;
  } finally {
    if (!disposed && req === playerRevision) playersLoading.value = false;
  }
}

const { pause: pausePlayers, resume: resumePlayers } = useTimeoutPoll(loadPlayers, 5000, { immediate: false });

async function reloadPlayers() {
  pausePlayers();
  await loadPlayers();
  if (!disposed && canPollPlayers.value) resumePlayers();
}

watch(open, value => {
  if (value) {
    roomId.value = String(rooms.currentId || rooms.list[0]?.id || '');
    shard.value = room.value?.shards[0]?.name || '';
  } else {
    revision++;
    statusRevision++;
    playerRevision++;
    loading.value = false;
    playersLoading.value = false;
    playersError.value = '';
    rawPlayers.value = [];
    selectedPlayerId.value = '';
    pausePlayers();
  }
});
watch(roomId, () => {
  shard.value = room.value?.shards[0]?.name || '';
  syncRoomStatuses();
});
watch([open, () => rooms.currentId, () => rooms.statusLoaded], () => {
  if (open.value) syncRoomStatuses();
});
watch([open, roomId, shard], () => { if (open.value) refresh(); });
watch([open, roomId, shard, showPlayers, canPollPlayers], () => {
  pausePlayers();
  playerRevision++;
  playersError.value = '';
  if (!canPollPlayers.value) {
    rawPlayers.value = [];
    playersLoading.value = false;
    selectedPlayerId.value = '';
    return;
  }
  reloadPlayers();
});
watch([roomId, shard], () => {
  rawPlayers.value = [];
  selectedPlayerId.value = '';
});
watch(mapPlayers, list => {
  if (selectedPlayerId.value && !list.some(item => item.userid === selectedPlayerId.value)) {
    selectedPlayerId.value = '';
  }
});
watch(zoom, value => {
  if (value === 1) {
    selectedId.value = '';
    selectedPlayerId.value = '';
  }
});
onUnmounted(() => {
  disposed = true;
  revision++;
  statusRevision++;
  playerRevision++;
  pausePlayers();
});

async function refresh() {
  const current = ++revision;
  data.value = null;
  error.value = '';
  zoom.value = 1;
  selectedId.value = '';
  selectedPlayerId.value = '';
  search.value = '';
  if (!roomId.value || !shard.value) return;
  loading.value = true;
  try {
    const result = await mapApi.get(Number(roomId.value), shard.value);
    const decoded = await decodeTerrain(result);
    if (current === revision) data.value = decoded;
  } catch (err) { if (current === revision) error.value = err.message; }
  finally { if (current === revision) loading.value = false; }
}

function refreshAll() {
  refresh();
  if (canPollPlayers.value) reloadPlayers();
}

function selectLandmark(id) {
  selectedPlayerId.value = '';
  selectedId.value = id;
  showLandmarks.value = true;
}

function selectPlayer(userid) {
  selectedId.value = '';
  selectedPlayerId.value = userid;
  showPlayers.value = true;
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child><IconAction icon="map" variant="outline" :show-label="showLabel" label="地图预览" :disabled="!rooms.list.length" /></DialogTrigger>
    <DialogContent class="flex max-h-[94dvh] flex-col overflow-y-auto sm:max-w-6xl">
      <DialogHeader class="shrink-0">
        <DialogTitle>存档地图预览</DialogTitle>
        <DialogDescription>最近保存点的地形、地标与运行中分片的在线玩家位置。</DialogDescription>
      </DialogHeader>
      <FieldGroup class="grid shrink-0 grid-cols-2 gap-3">
        <Field>
          <FieldLabel for="map-room">房间</FieldLabel>
          <Select v-model="roomId"><SelectTrigger id="map-room" class="w-full min-w-0" aria-label="地图房间"><SelectValue /></SelectTrigger><SelectContent position="popper" align="start"><SelectGroup><SelectItem v-for="item in rooms.list" :key="item.id" :value="String(item.id)">{{ item.name }}</SelectItem></SelectGroup></SelectContent></Select>
        </Field>
        <Field>
          <FieldLabel for="map-shard">分片</FieldLabel>
          <Select v-model="shard"><SelectTrigger id="map-shard" class="w-full min-w-0" aria-label="地图分片"><SelectValue /></SelectTrigger><SelectContent position="popper" align="start"><SelectGroup><SelectItem v-for="item in room?.shards || []" :key="item.name" :value="item.name">{{ shardLabel(item.name, room?.locations?.[item.name]) }}{{ item.disabled ? '（停用）' : '' }}</SelectItem></SelectGroup></SelectContent></Select>
        </Field>
      </FieldGroup>
      <div class="flex shrink-0 flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" :disabled="!data || zoom <= 1" aria-label="缩小地图" @click="zoom = Math.max(1, zoom / 1.5)">−</Button>
        <Button size="sm" variant="outline" :disabled="!data" aria-label="重置地图缩放" @click="zoom = 1">{{ Math.round(zoom * 100) }}%</Button>
        <Button size="sm" variant="outline" :disabled="!data || zoom >= 8" aria-label="放大地图" @click="zoom = Math.min(8, zoom * 1.5)">+</Button>
        <label class="flex items-center gap-2 text-sm"><Checkbox v-model="showLandmarks" />地标</label>
        <label class="flex items-center gap-2 text-sm">
          <Checkbox v-model="showPlayers" />
          <span>在线玩家</span>
          <span v-if="canPollPlayers" class="text-xs text-muted-foreground tabular-nums">({{ mapPlayers.length }})</span>
        </label>
        <IconAction icon="refresh" variant="ghost" label="刷新地图" :disabled="loading" @click="refreshAll" />
        <time v-if="data" class="ml-auto text-xs text-muted-foreground">保存于 {{ fmtTime(data.savedAt) }}</time>
        <Skeleton v-else-if="loading" class="ml-auto h-4 w-[148px] rounded" aria-hidden="true" />
      </div>
      <p v-if="error" role="alert" class="break-words text-sm text-destructive">{{ error }}</p>
      <div v-else-if="loading" role="status" aria-label="正在读取世界保存点" class="grid h-[min(58dvh,36rem)] min-h-[27rem] shrink-0 grid-rows-[minmax(15rem,1fr)_12rem] overflow-hidden rounded-md border md:min-h-[24rem] md:grid-cols-[minmax(0,1fr)_15rem] md:grid-rows-1">
        <Skeleton class="size-full rounded-none" />
        <div class="flex min-h-0 flex-col gap-2 border-t p-3 md:border-t-0 md:border-l" aria-hidden="true">
          <Skeleton class="h-8 w-full shrink-0 rounded-lg" />
          <div class="grid min-h-0 flex-1 content-start gap-1 overflow-hidden">
            <div v-for="(row, i) in SKELETON_LANDMARKS" :key="i" class="flex items-center gap-3 rounded-lg border border-transparent px-2 py-2">
              <Skeleton class="size-6 shrink-0 rounded-md" />
              <div class="grid min-w-0 flex-1 gap-0.5">
                <div class="flex h-5 items-center"><Skeleton class="h-3.5 rounded" :style="{ width: row.name }" /></div>
                <div class="flex h-4 items-center"><Skeleton class="h-3 rounded" :style="{ width: row.coord }" /></div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div v-else-if="data" class="grid h-[min(58dvh,36rem)] min-h-[27rem] shrink-0 grid-rows-[minmax(15rem,1fr)_12rem] overflow-hidden rounded-md border md:min-h-[24rem] md:grid-cols-[minmax(0,1fr)_15rem] md:grid-rows-1">
        <SavedMapViewport
          v-model:zoom="zoom"
          :map="data"
          :selected-id="selectedId"
          :show-landmarks="showLandmarks"
          :players="mapPlayers"
          :show-players="showPlayers"
          :selected-player-id="selectedPlayerId"
          @select="selectLandmark"
          @select-player="selectPlayer"
        />
        <aside class="flex min-h-0 flex-col gap-2 border-t p-3 md:border-t-0 md:border-l" aria-label="玩家与地标定位">
          <section v-if="showPlayers" class="flex shrink-0 flex-col gap-1.5 border-b pb-2.5" aria-label="地图在线玩家">
            <div class="flex items-center justify-between text-xs font-medium text-muted-foreground">
              <span class="inline-flex items-center gap-1.5">
                <span class="size-2 rounded-full" :class="canPollPlayers ? 'bg-emerald-500' : 'bg-muted-foreground/40'" aria-hidden="true" />
                <span>在线玩家</span>
              </span>
              <span v-if="canPollPlayers" class="tabular-nums">{{ mapPlayers.length }} 人</span>
            </div>
            <ul v-if="mapPlayers.length" class="grid max-h-44 content-start gap-1 overflow-y-auto" aria-label="在线玩家位置列表">
              <li v-for="player in mapPlayers" :key="player.userid">
                <Button
                  :variant="selectedPlayerId === player.userid ? 'secondary' : 'ghost'"
                  class="h-auto w-full justify-start gap-2.5 px-2 py-1.5"
                  :aria-label="`定位玩家${player.name || player.userid}（${formatWorldCoordinates(player.worldX, player.worldZ)}）`"
                  :aria-pressed="selectedPlayerId === player.userid"
                  @click="selectPlayer(player.userid)"
                >
                  <img
                    :src="player.avatar"
                    alt=""
                    aria-hidden="true"
                    width="24"
                    height="24"
                    class="size-6 shrink-0 rounded-full object-cover ring-2"
                    :class="player.ghost ? 'opacity-70 ring-destructive/70' : 'ring-emerald-500/80'"
                  />
                  <span class="grid min-w-0 gap-0.5 text-left">
                    <span class="flex items-center gap-1.5">
                      <span class="truncate text-sm">{{ player.name || player.userid }}</span>
                      <span v-if="player.ghost" class="shrink-0 text-[11px] font-medium text-destructive">幽灵</span>
                    </span>
                    <span class="truncate text-xs font-normal text-muted-foreground tabular-nums">{{ player.character }} · {{ formatWorldCoordinates(player.worldX, player.worldZ) }}</span>
                  </span>
                </Button>
              </li>
            </ul>
            <p v-else class="px-1 text-xs" :class="playersError ? 'text-amber-700 dark:text-amber-400' : 'text-muted-foreground'">{{ playerEmptyHint }}</p>
          </section>
          <Input v-model="search" placeholder="搜索地标" aria-label="搜索地标" class="shrink-0" />
          <ul class="grid min-h-0 flex-1 content-start gap-1 overflow-y-auto" aria-label="地图地标清单">
            <li v-for="point in landmarks" :key="point.id">
              <Button :variant="selectedId === point.id ? 'secondary' : 'ghost'" class="h-auto w-full justify-start gap-3 px-2 py-2" :aria-label="`定位${point.name}（${landmarkCoordinates(point, data)}）`" :aria-pressed="selectedId === point.id" @click="selectLandmark(point.id)">
                <img v-if="iconUrlOf(point)" :src="iconUrlOf(point)" alt="" aria-hidden="true" width="24" height="24" class="size-6 shrink-0 object-contain" />
                <component :is="landmarkStyle(point.kind).icon" v-else data-icon="inline-start" :style="{ color: landmarkStyle(point.kind).color }" weight="bold" />
                <span class="grid min-w-0 gap-0.5 text-left"><span class="truncate">{{ point.name }}</span><span class="text-xs font-normal text-muted-foreground tabular-nums">{{ landmarkCoordinates(point, data) }}</span></span>
              </Button>
            </li>
            <li v-if="!landmarks.length" class="p-2 text-sm text-muted-foreground">{{ search ? '未找到地标' : '此保存点没有已识别地标' }}</li>
          </ul>
        </aside>
      </div>
      <div v-if="loading" class="flex max-h-16 shrink-0 flex-wrap gap-x-4 gap-y-2 overflow-hidden" aria-hidden="true">
        <div v-for="(w, i) in SKELETON_LEGEND_WIDTHS" :key="i" class="flex h-4 items-center gap-1.5">
          <Skeleton class="size-2.5 shrink-0 rounded-sm" />
          <Skeleton class="h-3 rounded" :style="{ width: w }" />
        </div>
      </div>
      <ul v-else-if="data" class="flex max-h-16 shrink-0 flex-wrap gap-x-4 gap-y-2 overflow-y-auto text-xs text-muted-foreground" aria-label="地形图例">
        <li v-for="terrain in data.terrainLegend" :key="terrain.id" class="flex items-center gap-1.5"><span class="size-2.5 shrink-0 rounded-sm" :style="{ background: terrain.color }" aria-hidden="true" />{{ terrain.label }}</li>
      </ul>
    </DialogContent>
  </Dialog>
</template>
