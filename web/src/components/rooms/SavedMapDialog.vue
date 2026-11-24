<script setup>
import { computed, ref, watch } from 'vue';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import IconAction from '@/components/ui/IconAction.vue';
import UiLoading from '@/components/ui/UiLoading.vue';
import SavedMapViewport from '@/components/rooms/SavedMapViewport.vue';
import { decodeTerrain } from '@/lib/mapTerrain.js';
import { landmarkStyle, landmarkCoordinates } from '@/lib/mapLandmarks.js';
import { mapApi } from '@/lib/api.js';
import { fmtTime, shardLabel } from '@/lib/format.js';
import { useRoomsStore } from '@/stores/rooms.js';

const rooms = useRoomsStore();
const open = ref(false);
const roomId = ref('');
const shard = ref('');
const data = ref(null);
const error = ref('');
const loading = ref(false);
const zoom = ref(1);
const showLandmarks = ref(true);
const selectedId = ref('');
const search = ref('');
const room = computed(() => rooms.list.find(item => String(item.id) === roomId.value));
const landmarks = computed(() => (data.value?.landmarks || []).filter(point => `${point.name} ${point.kind}`.toLowerCase().includes(search.value.trim().toLowerCase())));
let revision = 0;

watch(open, value => {
  if (value) {
    roomId.value = String(rooms.currentId || rooms.list[0]?.id || '');
    shard.value = room.value?.shards[0]?.name || '';
  } else { revision++; loading.value = false; }
});
watch(roomId, () => { shard.value = room.value?.shards[0]?.name || ''; });
watch([open, roomId, shard], () => { if (open.value) refresh(); });
watch(zoom, value => { if (value === 1) selectedId.value = ''; });

async function refresh() {
  const current = ++revision;
  data.value = null;
  error.value = '';
  zoom.value = 1;
  selectedId.value = '';
  search.value = '';
  if (!roomId.value || !shard.value) return;
  loading.value = true;
  try {
    const result = await mapApi.get(Number(roomId.value), shard.value);
    if (current === revision) data.value = decodeTerrain(result);
  } catch (err) { if (current === revision) error.value = err.message; }
  finally { if (current === revision) loading.value = false; }
}

function selectLandmark(id) { selectedId.value = id; showLandmarks.value = true; }
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child><IconAction icon="pin" variant="outline" label="地图预览" :disabled="!rooms.list.length" /></DialogTrigger>
    <DialogContent class="flex max-h-[94dvh] flex-col overflow-y-auto sm:max-w-6xl">
      <DialogHeader class="shrink-0">
        <DialogTitle>存档地图预览</DialogTitle>
        <DialogDescription>最近保存点的地形与地标。</DialogDescription>
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
        <IconAction icon="refresh" variant="ghost" label="刷新地图" :disabled="loading" @click="refresh" />
        <time v-if="data" class="ml-auto text-xs text-muted-foreground">保存于 {{ fmtTime(data.savedAt) }}</time>
      </div>
      <p v-if="error" role="alert" class="break-words text-sm text-destructive">{{ error }}</p>
      <UiLoading v-else-if="loading" aria-label="正在读取世界保存点" />
      <div v-else-if="data" class="grid h-[min(58dvh,36rem)] min-h-[27rem] shrink-0 grid-rows-[minmax(15rem,1fr)_12rem] overflow-hidden rounded-md border md:min-h-[24rem] md:grid-cols-[minmax(0,1fr)_15rem] md:grid-rows-1">
        <SavedMapViewport v-model:zoom="zoom" :map="data" :selected-id="selectedId" :show-landmarks="showLandmarks" @select="selectLandmark" />
        <aside class="flex min-h-0 flex-col gap-2 border-t p-3 md:border-t-0 md:border-l" aria-label="地标定位">
          <Input v-model="search" placeholder="搜索地标" aria-label="搜索地标" class="shrink-0" />
          <ul class="grid min-h-0 content-start gap-1 overflow-y-auto" aria-label="地图地标清单">
            <li v-for="point in landmarks" :key="point.id">
              <Button :variant="selectedId === point.id ? 'secondary' : 'ghost'" class="h-auto w-full justify-start gap-3 px-2 py-2" :aria-label="`定位${point.name}（${landmarkCoordinates(point, data)}）`" :aria-pressed="selectedId === point.id" @click="selectLandmark(point.id)">
                <component :is="landmarkStyle(point.kind).icon" data-icon="inline-start" :style="{ color: landmarkStyle(point.kind).color }" weight="bold" />
                <span class="grid min-w-0 gap-0.5 text-left"><span class="truncate">{{ point.name }}</span><span class="text-xs font-normal text-muted-foreground tabular-nums">{{ landmarkCoordinates(point, data) }}</span></span>
              </Button>
            </li>
            <li v-if="!landmarks.length" class="p-2 text-sm text-muted-foreground">{{ search ? '未找到地标' : '此保存点没有已识别地标' }}</li>
          </ul>
        </aside>
      </div>
      <ul v-if="data" class="flex max-h-16 shrink-0 flex-wrap gap-x-4 gap-y-2 overflow-y-auto text-xs text-muted-foreground" aria-label="地形图例">
        <li v-for="terrain in data.terrainLegend" :key="terrain.id" class="flex items-center gap-1.5"><span class="size-2.5 shrink-0 rounded-sm" :style="{ background: terrain.color }" aria-hidden="true" />{{ terrain.label }}</li>
      </ul>
    </DialogContent>
  </Dialog>
</template>
