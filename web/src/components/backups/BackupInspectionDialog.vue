<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import SavedMapViewport from '@/components/rooms/SavedMapViewport.vue';
import UiLoading from '@/components/ui/UiLoading.vue';
import { backupApi } from '@/lib/api.js';
import { decodeTerrain } from '@/lib/mapTerrain.js';
import { fmtSize, fmtTime } from '@/lib/format.js';
import { confirmAction } from '@/lib/confirm.js';
import { guard, toast } from '@/lib/toast.js';
import { useRoomsStore } from '@/stores/rooms.js';

const props = defineProps({ room: { type: Object, required: true }, name: { type: String, required: true } });
const emit = defineEmits(['close', 'restored']);
const rooms = useRoomsStore();
const job = ref(null), error = ref(''), starting = ref(false);
const map = ref(null), mapError = ref(''), mapBusy = ref(false), shard = ref(''), zoom = ref(1), selectedId = ref('');
const restoring = computed(() => job.value?.state === 'restoring');
const roomRunning = computed(() => rooms.list.find(room => room.id === props.room.id)?.running);
const worldShards = computed(() => (job.value?.report?.shards || []).filter(item => item.snapshots > 0));
let id = '', timer, disposed = false, mapRevision = 0;
const seasons = { autumn: '秋季', winter: '冬季', spring: '春季', summer: '夏季' };

onMounted(start);
onUnmounted(() => { disposed = true; clearTimeout(timer); mapRevision++; });
watch([() => job.value?.state, shard], ([state]) => { if (state === 'ready' && shard.value) loadMap(); });

async function start() {
  starting.value = true; error.value = ''; job.value = null; map.value = null; shard.value = '';
  try {
    const data = await backupApi.inspect(props.room.id, props.name);
    if (disposed) return;
    id = data.id;
    await poll();
  } catch (err) { if (!disposed) error.value = err.message; }
  finally { if (!disposed) starting.value = false; }
}
async function poll() {
  clearTimeout(timer);
  try {
    const data = await backupApi.check(id);
    if (disposed) return;
    job.value = data;
    if (data.state === 'ready' && !shard.value) shard.value = data.report.shards.find(item => item.snapshots > 0)?.name || '';
    if (data.state === 'restored') { emit('restored'); return; }
    if (data.state === 'checking' || data.state === 'restoring') timer = setTimeout(poll, 750);
  } catch (err) { if (!disposed) error.value = err.message; }
}
async function loadMap() {
  const revision = ++mapRevision;
  map.value = null; mapError.value = ''; mapBusy.value = true; zoom.value = 1; selectedId.value = '';
  try { const data = await backupApi.checkMap(id, shard.value); if (!disposed && revision === mapRevision) map.value = decodeTerrain(data); }
  catch (err) { if (!disposed && revision === mapRevision) mapError.value = err.message; }
  finally { if (!disposed && revision === mapRevision) mapBusy.value = false; }
}
async function restore() {
  if (!await confirmAction(`回滚到 ${props.name}？\n\n当前存档将被替换，恢复前版本会保留。房间设置、令牌和端口使用当前配置。`)) return;
  const result = await guard(() => backupApi.restoreChecked(props.room.id, id, job.value.report.sha256));
  if (result.ok) { job.value = { ...job.value, state: 'restoring' }; await poll(); }
}
async function close(value) {
  if (value || restoring.value || starting.value) return;
  clearTimeout(timer);
  if (id) backupApi.clearCheck(id).catch(err => toast(err.message, 'err'));
  emit('close');
}
</script>

<template>
  <Dialog :open="true" @update:open="close">
    <DialogContent class="flex max-h-[92dvh] flex-col overflow-y-auto sm:max-w-4xl" :close-disabled="restoring || starting" @escape-key-down="(restoring || starting) && $event.preventDefault()" @interact-outside="(restoring || starting) && $event.preventDefault()">
      <DialogHeader>
        <DialogTitle>备份恢复检查</DialogTitle>
        <DialogDescription>{{ room.name }}，{{ name }}</DialogDescription>
      </DialogHeader>
      <p v-if="error || job?.error" role="alert" class="break-words text-sm text-destructive">{{ error || job.error }}</p>
      <div v-if="starting || job?.state === 'checking' || restoring" role="status" class="grid gap-2">
        <UiLoading />
        <p class="text-sm">{{ restoring ? '正在复核备份并恢复存档…' : '正在校验 ZIP 和各分片存档…' }}<span v-if="job?.total && !restoring" class="ml-2 tabular-nums">{{ job.done }} / {{ job.total }}</span></p>
      </div>
      <template v-if="job?.report">
        <div class="flex flex-wrap items-center gap-3 text-sm text-muted-foreground"><Badge :variant="job.report.canRestore ? 'secondary' : 'destructive'">{{ job.report.canRestore ? 'ZIP 完整性检查通过' : '备份内容不完整' }}</Badge><time>{{ fmtTime(job.report.createdAt) }}</time><span>{{ job.report.files }} 个文件</span><span>{{ fmtSize(job.report.bytes) }}</span></div>
        <ul class="grid gap-3 sm:grid-cols-2" aria-label="备份分片检查">
          <li v-for="item in job.report.shards" :key="item.name" class="grid gap-1 rounded-md border p-3 text-sm">
            <span class="font-medium">{{ item.name }}</span>
            <span class="text-muted-foreground">{{ item.snapshots }} 个世界保存点，{{ item.playerDirectories }} 个玩家保存目录</span>
            <span v-if="item.state">{{ item.state.day ? `第 ${item.state.day} 天` : '天数未知' }}，{{ seasons[item.state.season] || item.state.season || '季节未知' }}</span>
            <time v-if="item.state?.savedAt && new Date(item.state.savedAt).getFullYear() > 2000" class="text-xs text-muted-foreground">保存于 {{ fmtTime(item.state.savedAt) }}</time>
            <span class="text-xs text-muted-foreground">世界配置：{{ item.worldConfig ? '有' : '无' }}，模组配置：{{ item.modsConfig ? '有' : '无' }}</span>
            <span v-if="item.stateError" class="text-xs text-muted-foreground">天数/季节读取失败：{{ item.stateError }}</span>
          </li>
        </ul>
        <ul v-if="job.report.warnings.length" class="grid gap-1 text-sm text-muted-foreground" aria-label="备份检查提示"><li v-for="warning in job.report.warnings" :key="warning">{{ warning }}</li></ul>
        <template v-if="worldShards.length && job.state === 'ready'">
          <div class="flex flex-wrap items-center gap-2">
            <Select v-model="shard"><SelectTrigger aria-label="备份地图分片" class="min-w-32"><SelectValue /></SelectTrigger><SelectContent position="popper" align="start"><SelectGroup><SelectItem v-for="item in worldShards" :key="item.name" :value="item.name">{{ item.name }}</SelectItem></SelectGroup></SelectContent></Select>
            <Button size="sm" variant="outline" :disabled="!map || zoom <= 1" aria-label="缩小备份地图" @click="zoom = Math.max(1, zoom / 1.5)">−</Button>
            <Button size="sm" variant="outline" :disabled="!map" @click="zoom = 1">{{ Math.round(zoom * 100) }}%</Button>
            <Button size="sm" variant="outline" :disabled="!map || zoom >= 8" aria-label="放大备份地图" @click="zoom = Math.min(8, zoom * 1.5)">+</Button>
            <Button v-if="mapError" size="sm" variant="outline" :disabled="mapBusy" @click="loadMap">重试地图</Button>
          </div>
          <p v-if="mapError" class="text-sm text-muted-foreground">地图预览不可用：{{ mapError }}</p>
          <UiLoading v-else-if="mapBusy" />
          <SavedMapViewport v-else-if="map" :key="shard" v-model:zoom="zoom" :map="map" :selected-id="selectedId" class="h-72 shrink-0 rounded-md" @select="selectedId = $event" />
        </template>
      </template>
      <p v-if="job?.state === 'restored'" role="status" class="break-words text-sm">恢复完成。恢复前的存档保留于 {{ job.previousDirectory || '原存档备份目录' }}。</p>
      <p v-if="roomRunning && job?.state === 'ready'" role="status" class="text-sm text-muted-foreground">房间正在运行，请先停服再恢复。</p>
      <div class="flex flex-wrap gap-2">
        <Button v-if="job?.state === 'ready'" :disabled="!job.report.canRestore || roomRunning || starting" @click="restore">确认恢复此备份</Button>
        <Button v-if="job?.state === 'failed' || error" variant="outline" :disabled="starting || restoring" @click="start">重新检查</Button>
        <Button v-if="job?.state === 'restored'" variant="outline" @click="close(false)">完成</Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
