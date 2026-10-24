<script setup>
// 分片状态列表与启停按钮。

import { computed } from 'vue';

import UiEmpty from '@/components/ui/UiEmpty.vue';
import { Button } from '@/components/ui/button';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useProcessControl } from '@/composables/useProcessControl.js';
import { STATE_LABEL, fmtSize, shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const system = useSystemStore();
const proc = useProcessControl();

// 后端只返回「有进程记录」的分片（已启动过、被停止过都算），
// 从未启动过的分片没有记录。
//
// 所以不能「有记录就整份用记录」：只启动一个分片时，
// 其它分片的行会跟着消失，用户看不到它们的启动按钮。
// 按房间启用中的分片列表对齐，缺的分片补成 stopped 占位行。
const rows = computed(() => {
  const byShard = new Map(rooms.statuses.map(s => [s.shard, s]));
  return rooms.shards.map(name =>
    byShard.get(name) || { shard: name, state: 'stopped', placeholder: true });
});

const isLive = s => s.state === 'running' || s.state === 'starting';

function label(s) {
  return shardLabel(s.shard, rooms.current?.locations?.[s.shard]);
}

function meta(s) {
  if (s.placeholder) return '未启动';
  return [
    STATE_LABEL[s.state] || s.state,
    s.uptime,
    s.pid ? 'PID ' + s.pid : '',
    s.port ? '端口 ' + s.port : '',
    s.restarts ? '重启 ' + s.restarts + ' 次' : '',
  ].filter(Boolean).join(' · ');
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
</script>

<template>
  <UiEmpty v-if="!rooms.current" icon="server" text="请先在「房间」页新建一个房间" />

  <template v-else>
    <div v-for="s in rows" :key="s.shard" class="shard-row">
      <span class="dot" :class="s.state" />
      <span class="shard-name">{{ label(s) }}</span>
      <span v-if="staleVersion(s)" class="pill pill-warn" :title="staleTip(s)">需重启</span>
      <span class="shard-meta">{{ meta(s) }}</span>
      <!-- 占用率按「占整机」口径，与任务管理器一致 -->
      <span v-if="isLive(s)" class="shard-res" title="CPU 按占整机计算（任务管理器口径）">
        <span>CPU <span class="res-val">{{ cpuText(s) }}</span></span>
        <span>内存 <span class="res-val">{{ memText(s) }}</span></span>
      </span>
      <span class="shard-actions">
        <template v-if="isLive(s)">
          <Button variant="destructive" size="sm" :disabled="rooms.starting" @click="proc.stop(s.shard)">停止</Button>
          <Button variant="outline" size="sm" :disabled="rooms.starting || system.installing" @click="proc.restart(s.shard)">重启</Button>
        </template>
        <Button v-else size="sm" :disabled="rooms.starting" @click="proc.start(s.shard)">
          {{ rooms.starting ? '请稍候…' : (system.installed ? '启动' : '安装并启动') }}
        </Button>
      </span>
    </div>
  </template>
</template>
