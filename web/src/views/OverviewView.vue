<script setup>
// 概览页：顶部「房间概览与直连 / 资源监控」两大卡 + 底部「分片管理 / 在线玩家」两大卡（2×2 平衡布局）。

import { computed, ref } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import IconAction from '@/components/ui/IconAction.vue';
import UiProgress from '@/components/ui/UiProgress.vue';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import RoomConnectBar from '@/components/overview/RoomConnectBar.vue';
import ShardList from '@/components/overview/ShardList.vue';
import HostResources from '@/components/overview/HostResources.vue';
import PlayersPanel from '@/components/overview/PlayersPanel.vue';
import RollbackDialog from '@/components/overview/RollbackDialog.vue';
import AnnounceDialog from '@/components/overview/AnnounceDialog.vue';
import DiagnosticsDialog from '@/components/overview/DiagnosticsDialog.vue';
import SavedMapDialog from '@/components/rooms/SavedMapDialog.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useUiStore } from '@/stores/ui.js';
import { useProcessControl } from '@/composables/useProcessControl.js';
import { procApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { fmtSize, roomShardLabel } from '@/lib/format.js';
import { guard, toast } from '@/lib/toast.js';

const rooms = useRoomsStore();
const system = useSystemStore();
const ui = useUiStore();
const proc = useProcessControl();

const quickBusy = ref('');

const master = computed(() => rooms.current?.shards?.find(shard => shard.isMaster && !shard.disabled));
const masterRunning = computed(() => master.value && rooms.statuses.some(status => status.shard === master.value.name && status.state === 'running'));
const anyRunning = computed(() => rooms.statuses.some(status => status.state === 'running'));

/** 运行中分片的资源合计。优先展示内存占用（DST 更易受内存限制），副标题展示 CPU 占比。 */
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
  return r.mem ? fmtSize(r.mem) : '—';
});

const resSub = computed(() => {
  const r = res.value;
  if (!r.count) return '未运行';
  const cpuText = !r.cpu ? '—' : (r.cpu < 0.05 ? '< 0.1%' : r.cpu.toFixed(1) + '%');
  return `CPU ${cpuText}`;
});

const installPercent = computed(() => {
  const match = /([\d.]+)\s*%/.exec(system.progress || '');
  if (!match) return -1;
  const value = Number(match[1]);
  return Number.isFinite(value) && value >= 0 && value <= 100 ? value : -1;
});
const installStage = computed(() => (system.progress || '准备中…').replace(/\s*[\d.]+\s*%/, ''));

function cancelInstall() {
  guard(() => system.cancelInstall(), { success: '已请求取消安装' });
}

function resumeInstall() {
  guard(() => system.startInstall(), { success: '已继续服务器安装' });
}

async function runQuick(action) {
  if (quickBusy.value || !rooms.current) return;
  const room = rooms.current;
  const roomId = room.id;
  quickBusy.value = action;
  try {
    if (action === 'skipday') {
      if (!await confirmAction(`让「${room.name}」的主世界快进一天？\n\n会改变主世界的时间与实体状态，不能撤销；建议先保存并备份。该操作执行后一分钟内不能再次快进、回档或重置。`)) return;
    } else if (action === 'regenerate') {
      if (!await confirmAction({
        title: `永久重置「${room.name}」的世界？`,
        description: '将执行 c_regenerateworld() 清空当前全部世界的地形、建筑与天数并重新生成新世界，无法撤销！建议先前往「备份与回档」保留一份完整备份。',
        confirmText: '确认重置世界',
        destructive: true,
      })) return;
    }
    const labels = new Map(room.shards.map(shard => [shard.name, roomShardLabel(room, shard)]));
    const result = await procApi.quick(roomId, { action });
    const failures = Object.entries(result.errors || {}).map(([shard, reason]) => `${labels.get(shard) || shard}：${reason}`);
    const actionName = action === 'save' ? '保存' : action === 'skipday' ? '快进' : '重置世界';
    if (failures.length) toast(`「${room.name}」部分分片${actionName}失败：${failures.join('、')}`, 'err');
    else if (action === 'save') toast(`「${room.name}」已确认存档保存`);
    else if (action === 'skipday') toast(`「${room.name}」主世界已执行快进`);
    else toast(`「${room.name}」世界重置指令已发出，正在重新生成世界，可在控制台查看进度`);
  } catch (err) {
    toast(`「${room.name}」${err.message}`, 'err');
  } finally {
    quickBusy.value = '';
  }
}
</script>

<template>
  <!-- 必须保持单一根节点：App.vue 用 <Transition> 包裹视图组件，
       而 Transition 只接受「恰好一个」根元素。多根（fragment）会让切换动画失效，
       表现为来回点导航后右侧内容整个空白。 -->
  <div class="overview-page grid gap-3">
    <UiCard v-if="system.installing" role="status" aria-label="服务器安装进度">
      <template #title>
        <span class="inline-flex items-center gap-2">
          <span>正在安装服务器</span>
          <Badge variant="secondary">{{ system.canceling ? '正在取消' : (installPercent >= 0 ? `${installPercent}%` : '进行中') }}</Badge>
        </span>
      </template>
      <template #actions>
        <IconAction
          variant="destructive"
          show-label
          icon="stop"
          :label="system.canceling ? '正在取消…' : '取消安装'"
          :disabled="system.canceling"
          @click="cancelInstall"
        />
      </template>
      <div class="grid gap-2">
        <p class="text-sm font-medium">{{ installStage }}</p>
        <UiProgress :value="installPercent" :aria-label="installStage" />
        <p class="text-xs text-muted-foreground">
          {{ rooms.starting ? '安装完成后将自动开服。取消会保留已下载的文件，之后可随时继续。' : '取消会保留已下载的文件，之后可随时继续。' }}
        </p>
      </div>
    </UiCard>

    <Alert v-else-if="system.canceled || system.error" :variant="system.canceled ? 'default' : 'destructive'">
      <AlertTitle>{{ system.canceled ? '服务器安装已暂停' : '服务器安装或更新失败' }}</AlertTitle>
      <AlertDescription class="flex flex-wrap items-center justify-between gap-2">
        <span>{{ system.canceled ? '已下载的分块已保留，随时可以继续续传。' : system.error }}</span>
        <span class="flex shrink-0 items-center gap-2">
          <Button size="sm" variant="outline" @click="ui.openSettings('server')">服务器设置</Button>
          <Button size="sm" :disabled="anyRunning || rooms.starting" @click="resumeInstall">{{ system.canceled ? '继续安装' : '重试安装' }}</Button>
        </span>
      </AlertDescription>
    </Alert>

    <Alert v-if="rooms.current?.needToken" class="border-amber-500/40 bg-amber-500/5">
      <AlertTitle>当前房间缺少 Klei 服务器令牌</AlertTitle>
      <AlertDescription class="flex flex-wrap items-center justify-between gap-2">
        <span>在线模式开服需要配置 <code>cluster_token.txt</code> 令牌，局域网或离线模式可在房间设置中关闭在线校验。</span>
        <Button size="sm" variant="outline" @click="ui.openRoomEditor(rooms.current.id, 'network')">前往配置令牌</Button>
      </AlertDescription>
    </Alert>

    <!-- 顶行两大卡：房间概览与按分片直连 | 资源监控 -->
    <div class="overview-workspace">
      <RoomConnectBar />
      <HostResources
        :process-value="resValue"
        :process-sub="resSub"
        :process-cpu="res.cpu"
        :process-running="res.count > 0"
      />
    </div>

    <!-- 底行两大卡：分片管理与快捷运维 | 在线玩家 -->
    <div class="overview-workspace">
      <UiCard class="h-full" fill>
        <template #title>分片管理</template>
        <template #actions>
          <DiagnosticsDialog />
          <Separator orientation="vertical" class="hidden h-4 self-center sm:block" />
          <IconAction
            :variant="anyRunning ? 'outline' : 'default'"
            show-label
            :icon="system.installed ? 'play' : 'download'"
            :label="rooms.starting ? '准备开服中…' : (system.installed ? '全部启动' : '安装并启动')"
            :disabled="!rooms.current || rooms.starting"
            @click="proc.start('')"
          />
          <IconAction
            variant="outline"
            show-label="sm"
            icon="restart"
            label="全部重启"
            :disabled="!rooms.current || !anyRunning || rooms.starting || system.installing"
            @click="proc.restart('')"
          />
          <IconAction
            variant="destructive"
            show-label
            icon="stop"
            label="全部停止"
            :disabled="!rooms.current || rooms.starting"
            @click="proc.stop('')"
          />
        </template>
        <div class="flex min-h-0 flex-1 flex-col justify-between gap-3">
          <ShardList />
          <div v-if="rooms.current" role="group" aria-label="快捷运维" class="grid gap-2 border-t border-border/75 pt-3">
            <span class="text-[11.5px] font-medium text-muted-foreground">快捷运维</span>
            <div class="flex flex-wrap items-center gap-1.5">
              <IconAction
                variant="outline"
                show-label
                icon="save"
                :label="quickBusy === 'save' ? '等待保存完成…' : '立即保存'"
                :disabled="!!quickBusy || !anyRunning || rooms.starting"
                @click="runQuick('save')"
              />
              <AnnounceDialog />
              <RollbackDialog />
              <IconAction
                variant="outline"
                show-label
                icon="skip"
                :label="quickBusy === 'skipday' ? '等待游戏回执…' : '主世界快进一天'"
                :text="quickBusy === 'skipday' ? '等待回执…' : '快进一天'"
                :disabled="!!quickBusy || !masterRunning || rooms.starting"
                @click="runQuick('skipday')"
              />
              <IconAction
                variant="outline"
                show-label
                icon="undo"
                class="text-destructive hover:text-destructive"
                :label="quickBusy === 'regenerate' ? '等待世界重置…' : '重置世界'"
                :disabled="!!quickBusy || !masterRunning || rooms.starting"
                @click="runQuick('regenerate')"
              />
              <SavedMapDialog :show-label="true" />
            </div>
          </div>
        </div>
      </UiCard>

      <div class="overview-sidebar h-full">
        <PlayersPanel />
      </div>
    </div>
  </div>
</template>
