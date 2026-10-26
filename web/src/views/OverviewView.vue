<script setup>
// 概览页：核心指标 + 房间状态 + 服务器文件 + 实时日志。

import { computed } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import { Button } from '@/components/ui/button';
import UiStat from '@/components/ui/UiStat.vue';
import ShardList from '@/components/overview/ShardList.vue';
import HostResources from '@/components/overview/HostResources.vue';
import PlayersPanel from '@/components/overview/PlayersPanel.vue';
import InstallPanel from '@/components/overview/InstallPanel.vue';
import RollbackDialog from '@/components/overview/RollbackDialog.vue';
import WorldState from '@/components/overview/WorldState.vue';
import LogConsole from '@/components/overview/LogConsole.vue';
import QuickActions from '@/components/overview/QuickActions.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useProcessControl } from '@/composables/useProcessControl.js';
import { GAME_MODE_LABEL, fmtSize } from '@/lib/format.js';

const rooms = useRoomsStore();
const system = useSystemStore();
const proc = useProcessControl();

const shardTotal = computed(() => rooms.shards.length);
const shardRunning = computed(() => rooms.statuses.filter(s => s.state === 'running').length);

const shardTone = computed(() => {
  if (!shardTotal.value) return '';
  if (shardRunning.value === shardTotal.value) return 'ok';
  return shardRunning.value > 0 ? 'warn' : '';
});

const shardSub = computed(() => {
  if (!rooms.current || !shardTotal.value) return '未启用分片';
  if (rooms.starting) return '正在准备开服…';
  if (shardRunning.value === shardTotal.value) return '全部运行中';
  return shardRunning.value > 0 ? '部分运行中' : '全部已停止';
});

/** 运行中分片的资源合计。CPU 为各分片之和，口径是「占整机」。 */
const res = computed(() => {
  const live = rooms.statuses.filter(s => s.state === 'running');
  return {
    count: live.length,
    cpu: live.reduce((n, s) => n + (s.cpuPct || 0), 0),
    mem: live.reduce((n, s) => n + (s.memBytes || 0), 0),
  };
});

const resValue = computed(() => {
  const r = res.value;
  if (!r.count) return '—';
  // 首次采样没有基准，后端给 0；此时显示「—」而不是「0.0%」
  if (!r.cpu) return '—';
  return r.cpu < 0.05 ? '< 0.1%' : r.cpu.toFixed(1) + '%';
});

const resSub = computed(() => {
  const r = res.value;
  if (!r.count) return '未运行';
  return `内存 ${fmtSize(r.mem)}`;
});

const resTone = computed(() => {
  if (!res.value.count) return '';
  return res.value.cpu > 50 ? 'warn' : 'info';
});

const roomSub = computed(() => {
  const r = rooms.current;
  if (!r) return '还没有房间';
  const mode = GAME_MODE_LABEL[r.gameMode] || r.gameMode;
  return r.maxPlayers ? `${mode} · ${r.maxPlayers} 人` : mode;
});

const serverValue = computed(() => {
  if (system.installing) return '安装中';
  if (!system.installed) return '未安装';
  return system.version || '已安装';
});

const serverSub = computed(() => {
  if (system.installing) return system.progress || '准备中…';
  if (system.installed && system.external) return '由 Steam 更新';
  return system.installed ? '可检查更新' : '首次启动会自动安装服务器文件';
});

const serverTone = computed(() => {
  if (system.installing) return 'info';
  return system.installed ? 'ok' : 'warn';
});
</script>

<template>
  <!-- 必须保持单一根节点：App.vue 用 <Transition> 包裹视图组件，
       而 Transition 只接受「恰好一个」根元素。多根（fragment）会让切换动画失效，
       表现为来回点导航后右侧内容整个空白。 -->
  <div class="overview-page">
    <div class="stat-row">
      <UiStat
        label="运行分片"
        :value="`${shardRunning} / ${shardTotal}`"
        :sub="shardSub"
        icon="server"
        :tone="shardTone"
      />
      <UiStat
        label="当前房间"
        :value="rooms.current ? rooms.current.name : '—'"
        :sub="roomSub"
        icon="globe"
        tone="info"
      />
      <UiStat
        label="资源占用"
        :value="resValue"
        :sub="resSub"
        icon="cpu"
        :tone="resTone"
      />
      <UiStat
        label="服务器文件"
        :value="serverValue"
        :sub="serverSub"
        icon="package"
        :tone="serverTone"
      />
    </div>

    <!-- 第一层主控工作台：左侧管分片进程与回档，右侧管游戏内即时指令（保存/快进/公告），
         所有高频开服与控服操作集中在首屏黄金区域。 -->
    <div class="overview-split">
      <UiCard>
        <template #title>房间状态</template>
        <template #actions>
          <RollbackDialog />
          <Button size="sm" :disabled="!rooms.current || rooms.starting" @click="proc.start('')">
            {{ rooms.starting ? '准备开服中…' : (system.installed ? '全部启动' : '安装并启动') }}
          </Button>
          <Button variant="destructive" size="sm" :disabled="!rooms.current || rooms.starting" @click="proc.stop('')">
            全部停止
          </Button>
        </template>
        <ShardList />
      </UiCard>

      <QuickActions />
    </div>

    <!-- 第二层详情与环境：左栏聚焦游戏内状态（世界进度 + 在线玩家），
         右栏聚焦宿主机资源与服务器文件，语义分区清晰且左右高度均衡。 -->
    <div class="overview-split">
      <div class="overview-col">
        <WorldState />
        <PlayersPanel />
      </div>

      <div class="overview-col">
        <HostResources />
        <UiCard class="flex-1">
          <template #title>服务器文件</template>
          <InstallPanel />
        </UiCard>
      </div>
    </div>

    <LogConsole />
  </div>
</template>
