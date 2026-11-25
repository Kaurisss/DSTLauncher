<script setup>
// 分片状态列表与启停按钮。

import { computed, onMounted, onUnmounted, ref, watch } from 'vue';

import UiEmpty from '@/components/ui/UiEmpty.vue';
import WorldState from '@/components/overview/WorldState.vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { worldStateApi } from '@/lib/api.js';
import IconAction from '@/components/ui/IconAction.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useUiStore } from '@/stores/ui.js';
import { useProcessControl } from '@/composables/useProcessControl.js';
import { STATE_LABEL, fmtSize, shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const system = useSystemStore();
const ui = useUiStore();
const proc = useProcessControl();
const snapshots = ref([]);
const snapshotError = ref('');
const loadingSnapshots = ref(false);
let timer;
let generation = 0;

onMounted(() => {
  watch(() => rooms.current?.id, id => {
    generation++;
    clearTimeout(timer);
    snapshots.value = [];
    snapshotError.value = '';
    loadingSnapshots.value = !!id;
    if (id) refreshSnapshots(id, generation);
  }, { immediate: true });
});
onUnmounted(() => { generation++; clearTimeout(timer); });

// 后端只返回「有进程记录」的分片（已启动过、被停止过都算），
// 从未启动过的分片没有记录。
//
// 所以不能「有记录就整份用记录」：只启动一个分片时，
// 其它分片的行会跟着消失，用户看不到它们的启动按钮。
// 按房间启用中的分片列表对齐，缺的分片补成 stopped 占位行。
const rows = computed(() => {
  const byShard = new Map(rooms.statuses.map(s => [s.shard, s]));
  const byWorld = new Map(snapshots.value.map(item => [item.shard, item]));
  return rooms.shards.map(name => ({
    ...(byShard.get(name) || { shard: name, state: 'stopped', placeholder: true }),
    snapshot: byWorld.get(name),
  }));
});

const isLive = s => s.state === 'running' || s.state === 'starting';

function label(s) {
  return shardLabel(s.shard, rooms.current?.locations?.[s.shard]);
}

function dotClass(s) {
  if (s.state === 'running') return 'bg-emerald-500';
  if (s.state === 'starting' || s.state === 'stopping') return 'bg-amber-500';
  if (s.state === 'crashed' || s.state === 'failed') return 'bg-destructive';
  return 'bg-muted-foreground/35';
}

function badgeClass(s) {
  if (s.state === 'running') {
    return 'border-emerald-500/25 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400';
  }
  if (s.state === 'starting' || s.state === 'stopping') {
    return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400';
  }
  if (s.state === 'crashed' || s.state === 'failed') {
    return 'border-destructive/30 bg-destructive/10 text-destructive';
  }
  return '';
}

function meta(s) {
  return [
    s.uptime,
    s.pid ? 'PID ' + s.pid : '',
    s.port ? '端口 ' + s.port : '',
    s.restarts ? '重启 ' + s.restarts + ' 次' : '',
  ].filter(Boolean).join(' ');
}

// 首次采样没有基准算不出占用率，后端返回 0；此时显示「—」比「0.0%」诚实
function cpuText(s) {
  if (!s.cpuPct) return '—';
  return s.cpuPct < 0.05 ? '< 0.1%' : s.cpuPct.toFixed(1) + '%';
}

function memText(s) {
  return s.memBytes ? fmtSize(s.memBytes) : '—';
}

/**
 * 该分片启动时的游戏版本与当前安装版本不一致。
 *
 * 游戏更新后，已经在跑的进程加载的还是旧版本，必须重启才会生效——
 * 否则会出现"更新了却报 mod 不兼容""新功能没有"这类困惑。
 * 判据是分片自己记录的启动版本，所以重启后提示会自动消失。
 */
function staleVersion(s) {
  return !!s.version && !!system.version && s.version !== system.version;
}

function staleTip(s) {
  return `该分片启动于版本 ${s.version}，当前安装版本是 ${system.version}。重启后才会使用新版本。`;
}

async function refreshSnapshots(id, requestGeneration) {
  try {
    const data = await worldStateApi.get(id);
    if (requestGeneration !== generation) return;
    snapshots.value = data || [];
    snapshotError.value = '';
  } catch (err) {
    if (requestGeneration !== generation) return;
    snapshots.value = [];
    snapshotError.value = err.message;
  } finally {
    if (requestGeneration === generation) {
      loadingSnapshots.value = false;
      timer = setTimeout(() => refreshSnapshots(id, requestGeneration), 15000);
    }
  }
}
</script>

<template>
  <UiEmpty v-if="!rooms.current">
    <span>请先在「房间管理」页新建一个房间</span>
    <Button size="sm" variant="outline" class="mt-2" @click="ui.openRoomEditor()">前往新建房间</Button>
  </UiEmpty>

  <section
    v-else
    aria-label="分片状态"
    :class="['shard-grid grid gap-2.5', rows.length > 1 ? 'sm:grid-cols-2' : 'grid-cols-1']"
  >
    <div v-for="s in rows" :key="s.shard" class="shard-row" :aria-label="label(s)">
      <div class="shard-row-head">
        <div class="shard-main">
          <span aria-hidden="true" :class="['size-2 shrink-0 rounded-full', dotClass(s)]" />
          <span class="shard-name truncate">{{ label(s) }}</span>
          <Tooltip :disabled="!meta(s) && !isLive(s)">
            <TooltipTrigger as-child>
              <Badge
                variant="secondary"
                :class="['h-5 shrink-0 px-1.5 text-[11px] font-medium', badgeClass(s)]"
                :tabindex="meta(s) || isLive(s) ? 0 : undefined"
              >
                {{ s.placeholder ? '未启动' : STATE_LABEL[s.state] || s.state }}
              </Badge>
            </TooltipTrigger>
            <TooltipContent>
              <p v-if="meta(s)">{{ meta(s) }}</p>
              <p v-if="isLive(s)" class="tabular-nums">CPU（占整机）{{ cpuText(s) }}，内存 {{ memText(s) }}</p>
            </TooltipContent>
          </Tooltip>
          <Tooltip v-if="staleVersion(s)">
            <TooltipTrigger as-child><span class="pill pill-warn" tabindex="0">需重启</span></TooltipTrigger>
            <TooltipContent>{{ staleTip(s) }}</TooltipContent>
          </Tooltip>
        </div>
        <span class="shard-actions">
          <template v-if="isLive(s)">
            <IconAction variant="destructive" show-label="sm" icon="stop" label="停止" :disabled="rooms.starting" @click="proc.stop(s.shard)" />
            <IconAction variant="outline" show-label="sm" icon="restart" label="重启" :disabled="rooms.starting || system.installing" @click="proc.restart(s.shard)" />
          </template>
          <IconAction v-else variant="outline" show-label="sm" :icon="system.installed ? 'play' : 'download'" :label="rooms.starting ? '请稍候…' : (system.installed ? '启动' : '安装并启动')" :disabled="rooms.starting" @click="proc.start(s.shard)" />
        </span>
      </div>
      <WorldState :snapshot="s.snapshot" :loading="loadingSnapshots" :error="snapshotError" />
    </div>
  </section>
</template>
