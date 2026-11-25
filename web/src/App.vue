<script setup>
// 应用外壳：布局、视图切换、轮询与日志连接的编排。
//
// 各视图自己负责加载「进入时」的数据；这里只管与时间相关的部分：
// 状态轮询、房间列表刷新、日志 SSE 连接。

import { computed, defineAsyncComponent, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';

import iconUrl from '@/assets/icon.png';

import AppSidebar from '@/components/AppSidebar.vue';
import TopBar from '@/components/TopBar.vue';
import ToastHost from '@/components/ToastHost.vue';
import ConfirmHost from '@/components/ConfirmHost.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { SidebarProvider } from '@/components/ui/sidebar';

import OverviewView from '@/views/OverviewView.vue';

import { onUnauthorized } from '@/lib/api.js';
import { useAuthStore } from '@/stores/auth.js';
import { useUiStore } from '@/stores/ui.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useLogs } from '@/composables/useLogs.js';
import { guard } from '@/lib/toast.js';

/** 标签 key -> 视图组件。加新页面时在这里登记一行。 */
const VIEWS = {
  overview: OverviewView,
  rooms: defineAsyncComponent(() => import('@/views/RoomsView.vue')),
  world: defineAsyncComponent(() => import('@/views/WorldView.vue')),
  mods: defineAsyncComponent(() => import('@/views/ModsView.vue')),
  logs: defineAsyncComponent(() => import('@/views/PlayerLogsView.vue')),
  backups: defineAsyncComponent(() => import('@/views/BackupsView.vue')),
  saves: defineAsyncComponent(() => import('@/views/SavesView.vue')),

  settings: defineAsyncComponent(() => import('@/views/SettingsView.vue')),
};

/** 兜底：万一 activeTab 与 VIEWS 不一致，也不会渲染出空白页。 */
const currentView = computed(() => VIEWS[ui.activeTab] || OverviewView);
const cardScroll = computed(() => ['rooms', 'mods', 'world', 'logs', 'backups', 'saves'].includes(ui.activeTab));
// 全局管理页保留草稿；房间业务页重建实例，隔离旧房间的异步响应和草稿。
const viewKey = computed(() => ui.isGlobalTab ? ui.activeTab : `${ui.activeTab}:${rooms.currentId}`);


const auth = useAuthStore();
const ui = useUiStore();
const rooms = useRoomsStore();
const logs = useLogs();

const loginPwd = ref('');
const loggingIn = ref(false);
const loginError = ref('');
const loginWait = ref(0);
let loginTimer;
const workspace = ref(null);
let sidebarAnimation;

async function animateSidebarChange() {
  const panel = workspace.value;
  if (!panel) return;
  const left = panel.getBoundingClientRect().left;
  sidebarAnimation?.cancel();
  await nextTick();
  if (!panel.isConnected || !window.matchMedia('(min-width: 768px) and (prefers-reduced-motion: no-preference)').matches) return;
  // 只重排一次正文；中断时从当前可见位置继续，不逐帧改变内容宽度。
  const offset = left - panel.getBoundingClientRect().left;
  sidebarAnimation = panel.animate(
    [{ transform: `translateX(${offset}px)` }, { transform: 'translateX(0)' }],
    { duration: 180, easing: 'cubic-bezier(0.2, 0, 0, 1)' },
  );
}

let statusTimer = null;
let roomsTimer = null;
let lastRoomId = undefined;
let disposed = false;

onUnauthorized(() => {
  auth.markUnauthorized();
  logs.disconnect();
});

async function submitLogin() {
  if (!loginPwd.value || loggingIn.value || loginWait.value) return;
  loggingIn.value = true;
  loginError.value = '';
  try {
    await auth.login(loginPwd.value);
    loginPwd.value = '';
    await guard(() => rooms.load());
    guard(() => rooms.loadMeta());
    logs.openRoom(rooms.currentId, rooms.shards);
    rooms.refreshStatus().catch(() => {});
  } catch (err) {
    if (err.retryAfter) startLoginCooldown(err.retryAfter);
    else loginError.value = err.message || '登录失败';
  } finally {
    loggingIn.value = false;
  }
}

function startLoginCooldown(seconds) {
  clearInterval(loginTimer);
  loginError.value = '';
  const deadline = Date.now() + seconds * 1000;
  loginWait.value = Math.ceil(seconds);
  loginTimer = setInterval(() => {
    loginWait.value = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
    if (!loginWait.value) clearInterval(loginTimer);
  }, 250);
}

// 换房间要重连日志；启停洞穴会改变分片列表，也要跟着增删对应分片的连接
watch(
  [() => rooms.currentId, () => rooms.shards.join(',')],
  () => {
    if (!auth.authenticated) return;
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
  auth.load().then(status => {
    if (status?.retryAfter) startLoginCooldown(status.retryAfter);
  });
  // 首次加载会设置 currentId，从而触发上面的 watcher 完成状态刷新与日志连接
  const { ok } = await guard(() => rooms.load());
  if (disposed) return;

  if (ok) {
    // 表单枚举值失败不影响使用，不阻塞进入面板。
    guard(() => rooms.loadMeta());
  }

  statusTimer = setInterval(() => {
    if (auth.authenticated) rooms.refreshStatus().catch(() => {});
  }, 3000);
  roomsTimer = setInterval(() => {
    if (auth.authenticated) rooms.load().catch(() => {});
  }, 15000);
});

onUnmounted(() => {
  disposed = true;
  sidebarAnimation?.cancel();
  clearInterval(statusTimer);
  clearInterval(roomsTimer);
  clearInterval(loginTimer);
  logs.disconnect();
});
</script>

<template>
  <div v-if="!auth.authenticated" class="flex h-dvh items-center justify-center overflow-y-auto bg-sidebar p-4 text-sidebar-foreground">
    <form class="grid w-full max-w-sm gap-4 rounded-xl border border-border bg-card p-6 text-card-foreground shadow-sm" @submit.prevent="submitLogin">
      <div class="flex items-center gap-3">
        <img :src="iconUrl" alt="DST" width="40" height="40" class="size-10 shrink-0 rounded-lg object-cover shadow-xs" />
        <div>
          <h1 class="text-base font-semibold leading-tight">DST 开服控制台</h1>
        </div>
      </div>
      <div class="grid gap-1.5">
        <label for="login-password" class="text-xs font-medium text-muted-foreground">面板访问密码</label>
        <Input
          id="login-password"
          v-model="loginPwd"
          type="password"
          autocomplete="current-password"
        />
      </div>
      <p v-if="loginWait" role="status" class="text-xs text-destructive">密码错误次数过多，请 {{ loginWait }} 秒后再试</p>
      <p v-else-if="loginError" role="alert" class="text-xs text-destructive">{{ loginError }}</p>
      <Button type="submit" :disabled="loggingIn || !loginPwd || loginWait > 0">
        {{ loggingIn ? '验证中…' : loginWait ? `${loginWait} 秒后重试` : '登录' }}
      </Button>
    </form>
  </div>

  <SidebarProvider v-else class="flex h-dvh min-h-0 w-screen flex-row overflow-hidden bg-sidebar" style="--sidebar-width: 224px" @update:open="animateSidebarChange">
    <a href="#page-content" class="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:p-3 focus:text-foreground focus:shadow-md">跳到内容</a>
    <AppSidebar />
    <div ref="workspace" data-slot="main-workspace" class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden md:p-2 md:pl-0">
      <div
        data-slot="app-shell-surface"
        class="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-background md:rounded-xl md:border md:border-border/85 md:shadow-[0_1px_2px_rgba(15,23,42,0.04),0_6px_20px_-8px_rgba(15,23,42,0.08)] dark:md:border-border dark:md:shadow-[0_2px_12px_rgba(0,0,0,0.45)]"
      >
        <TopBar />
        <main
          id="page-content"
          tabindex="-1"
          :class="[
            'flex min-h-0 min-w-0 flex-1 flex-col',
            cardScroll ? 'overflow-hidden' : 'overflow-y-auto'
          ]"
          :style="cardScroll ? undefined : { scrollbarGutter: 'stable' }"
        >
          <div :class="['page-view w-full max-w-[2400px] min-w-0 p-3', cardScroll ? 'flex min-h-0 flex-1 flex-col overflow-hidden' : 'shrink-0']">
            <!-- 不延迟旧视图卸载，快速导航和换房始终只保留一个业务实例。 -->
            <component :is="currentView" :key="viewKey" />
          </div>
        </main>
      </div>
    </div>
  </SidebarProvider>

  <ToastHost />
  <ConfirmHost />
</template>

<style scoped>
/* 在实际内容挂载后入场，兼容异步视图和多根节点，不延迟旧实例卸载。 */
@media (prefers-reduced-motion: no-preference) {
  .page-view > :deep(*) {
    animation: page-enter 180ms ease-out;
  }
}

@keyframes page-enter {
  from { opacity: 0; transform: translateY(6px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
