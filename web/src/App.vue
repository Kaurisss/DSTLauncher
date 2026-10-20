<script setup>
// 应用外壳：布局、视图切换、轮询与日志连接的编排。
//
// 各视图自己负责加载「进入时」的数据；这里只管与时间相关的部分：
// 状态轮询、房间列表刷新、日志 SSE 连接。

import { computed, onMounted, onUnmounted, watch } from 'vue';

import AppSidebar from '@/components/AppSidebar.vue';
import TopBar from '@/components/TopBar.vue';
import ToastHost from '@/components/ToastHost.vue';
import ConfirmHost from '@/components/ConfirmHost.vue';
import { SidebarProvider } from '@/components/ui/sidebar';

import OverviewView from '@/views/OverviewView.vue';
import RoomsView from '@/views/RoomsView.vue';
import WorldView from '@/views/WorldView.vue';
import ModsView from '@/views/ModsView.vue';
import BackupsView from '@/views/BackupsView.vue';
import SavesView from '@/views/SavesView.vue';
import ConfigView from '@/views/ConfigView.vue';

import { useUiStore } from '@/stores/ui.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useLogs } from '@/composables/useLogs.js';
import { guard } from '@/lib/toast.js';

/** 标签 key -> 视图组件。加新页面时在这里登记一行。 */
const VIEWS = {
  overview: OverviewView,
  rooms: RoomsView,
  world: WorldView,
  mods: ModsView,
  backups: BackupsView,
  saves: SavesView,
  config: ConfigView,
};

/** 兜底：万一 activeTab 与 VIEWS 不一致，也不会渲染出空白页。 */
const currentView = computed(() => VIEWS[ui.activeTab] || OverviewView);
// 全局管理页保留草稿；房间业务页重建实例，隔离旧房间的异步响应和草稿。
const viewKey = computed(() => ['rooms', 'saves'].includes(ui.activeTab)
  ? ui.activeTab : `${ui.activeTab}:${rooms.currentId}`);

const ui = useUiStore();
const rooms = useRoomsStore();
const logs = useLogs();

let statusTimer = null;
let roomsTimer = null;
let lastRoomId = undefined;
let disposed = false;

// 换房间要重连日志；启停洞穴会改变分片列表，也要跟着增删对应分片的连接
watch(
  [() => rooms.currentId, () => rooms.shards.join(',')],
  () => {
    // 只有真正换了房间才清空日志，否则切换洞穴开关会把当前房间的日志也清掉
    if (rooms.currentId !== lastRoomId) {
      logs.clearAll();
      lastRoomId = rooms.currentId;
    }
    logs.openRoom(rooms.currentId, rooms.shards);
    rooms.refreshStatus().catch(() => {});
  },
);

onMounted(async () => {
  // 首次加载会设置 currentId，从而触发上面的 watcher 完成状态刷新与日志连接
  await guard(() => rooms.load());
  if (disposed) return;

  // 表单枚举值失败不影响使用，不阻塞进入面板。
  // 安装状态由概览页的 InstallPanel 自己拉取，这里不重复请求。
  guard(() => rooms.loadMeta());

  statusTimer = setInterval(() => rooms.refreshStatus().catch(() => {}), 3000);
  roomsTimer = setInterval(() => rooms.load().catch(() => {}), 15000);
});

onUnmounted(() => {
  disposed = true;
  clearInterval(statusTimer);
  clearInterval(roomsTimer);
  logs.disconnect();
});
</script>

<template>
  <SidebarProvider class="flex h-svh max-h-svh flex-col overflow-hidden bg-sidebar">
    <a href="#page-content" class="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:p-3 focus:text-foreground focus:shadow-md">跳到内容</a>
    <TopBar />

    <div class="flex flex-1 min-h-0 w-full overflow-hidden">
      <AppSidebar />

      <main
        id="page-content"
        tabindex="-1"
        class="flex-1 min-w-0 md:rounded-tl-xl bg-background md:border-t md:border-l border-border shadow-xs flex flex-col overflow-y-auto"
      >
        <div class="w-full max-w-[2400px] min-w-0 p-4 md:p-6">
          <!-- 不延迟旧视图卸载，快速导航和换房始终只保留一个业务实例。 -->
          <component :is="currentView" :key="viewKey" />
        </div>
      </main>
    </div>
  </SidebarProvider>

  <ToastHost />
  <ConfirmHost />
</template>
