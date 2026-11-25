<script setup>
import { computed, ref } from 'vue';
import iconUrl from '@/assets/icon.png';
import { PhDesktop, PhMoon, PhSignOut, PhSun } from '@phosphor-icons/vue';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { SidebarTrigger, useSidebar } from '@/components/ui/sidebar';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useAuthStore } from '@/stores/auth.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { useTheme } from '@/composables/useTheme.js';
import { guard, toast } from '@/lib/toast.js';
import { GAME_MODE_LABEL } from '@/lib/format.js';

const auth = useAuthStore();
const rooms = useRoomsStore();
const ui = useUiStore();
const theme = useTheme();
const { state } = useSidebar();
const loggingOut = ref(false);

const pageTitle = computed(() => ui.activeTabMeta?.label || '控制台');
const roomDetails = computed(() => {
  const room = rooms.current;
  if (!room) return '';
  const mode = GAME_MODE_LABEL[room.gameMode] || room.gameMode || '';
  return [room.key, mode.split(' ')[0], room.maxPlayers != null ? room.maxPlayers + '人' : ''].filter(Boolean).join(' ');
});
const themeIcon = computed(() => ({ auto: PhDesktop, light: PhSun, dark: PhMoon })[theme.mode.value]);
const themeTitle = computed(() => '切换主题：' + theme.label.value);

function isRoomRunning(room) {
  if (!room) return false;
  if (room.id === rooms.currentId && rooms.statuses.length) return rooms.running;
  if (typeof room.running === 'boolean') return room.running;
  return (room.statuses || []).some(s => s.state === 'running' || s.state === 'starting');
}

function onTheme() {
  theme.cycle();
  toast(`主题：${theme.label.value}`);
}

async function onLogout() {
  if (loggingOut.value) return;
  loggingOut.value = true;
  try {
    await guard(() => auth.logout(), { success: '已退出登录' });
  } finally {
    loggingOut.value = false;
  }
}
</script>

<template>
  <header data-slot="app-header" class="relative z-20 flex h-14 w-full min-w-0 shrink-0 items-center justify-between bg-sidebar text-sidebar-foreground">
    <div class="flex min-w-0 flex-1 items-center">
      <!-- 品牌与触发区域：桌面端宽度与侧边栏严格同步（展开 220px，收起 48px），移动端仅保留触发按钮 -->
      <div
        class="flex h-14 shrink-0 items-center overflow-hidden transition-[width] duration-180 ease-[cubic-bezier(0.2,0,0,1)]"
        :class="state === 'expanded' ? 'w-12 md:w-[220px]' : 'w-12'"
      >
        <!-- 侧边栏折叠按钮：固定在左侧 48px 区域内居中，中心点 x=24px 与下方导航图标严格共线 -->
        <div class="flex h-14 w-12 shrink-0 items-center justify-center">
          <Tooltip>
            <TooltipTrigger as-child>
              <SidebarTrigger aria-controls="app-navigation" aria-label="切换导航" size="icon-sm" class="shrink-0 text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground" />
            </TooltipTrigger>
            <TooltipContent>展开或收起导航</TooltipContent>
          </Tooltip>
        </div>

        <!-- 品牌标识：仅在桌面端展开时展示 -->
        <div class="hidden md:flex min-w-0 flex-1 items-center gap-2 pr-3">
          <img :src="iconUrl" alt="DST" width="28" height="28" class="size-7 shrink-0 rounded-lg object-cover" />
          <span class="truncate text-sm font-semibold tracking-tight text-sidebar-foreground">DST 开服控制台</span>
        </div>
      </div>

      <!-- 当前页面标题：移动端紧随触发按钮，桌面端从侧边栏分界处起排 -->
      <div class="flex min-w-0 items-center gap-2 pl-1 md:pl-3">
        <h1 class="truncate text-sm font-semibold sm:text-base text-sidebar-foreground">{{ pageTitle }}</h1>
        <Badge v-if="ui.isGlobalTab" variant="outline" class="hidden shrink-0 text-[11px] font-normal text-muted-foreground sm:inline-flex">全局</Badge>
      </div>
    </div>

    <!-- 右侧：房间选择（仅当前房间作用域显示）+ 切换主题 + 退出登录（启用密码时显示） -->
    <div class="flex min-w-0 shrink-0 items-center gap-2 pr-3 sm:pr-4">
      <Select v-if="!ui.isGlobalTab" :model-value="rooms.currentId ? String(rooms.currentId) : ''" :disabled="!rooms.list.length" @update:model-value="rooms.select">
        <SelectTrigger id="roomSelect" aria-label="房间" size="sm" class="w-auto min-w-[140px] max-w-[200px] sm:max-w-[340px] md:max-w-[420px] lg:max-w-[500px] bg-card h-7 px-2.5 text-xs">
          <SelectValue as-child>
            <span class="flex min-w-0 flex-1 items-center gap-1.5">
              <span
                v-if="rooms.current"
                aria-hidden="true"
                :data-running="isRoomRunning(rooms.current) ? 'true' : 'false'"
                :class="['size-2 shrink-0 rounded-full', isRoomRunning(rooms.current) ? 'bg-emerald-500' : 'bg-muted-foreground/35']"
              />
              <span class="truncate text-xs font-medium">{{ rooms.current?.name || (rooms.list.length ? '选择房间' : '（暂无房间）') }}</span>
              <span v-if="roomDetails" class="hidden sm:inline-flex shrink-0 items-center rounded bg-muted/60 px-1.5 py-0.5 text-[11px] text-muted-foreground tabular-nums leading-none">{{ roomDetails }}</span>
            </span>
          </SelectValue>
        </SelectTrigger>
        <SelectContent position="popper" align="end">
          <SelectGroup>
            <SelectItem v-for="room in rooms.list" :key="room.id" :value="String(room.id)">
              <span class="flex min-w-0 items-center gap-2">
                <span
                  aria-hidden="true"
                  :data-running="isRoomRunning(room) ? 'true' : 'false'"
                  :class="['size-2 shrink-0 rounded-full', isRoomRunning(room) ? 'bg-emerald-500' : 'bg-muted-foreground/35']"
                />
                <span class="truncate">{{ room.name }}</span>
              </span>
            </SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>

      <Tooltip>
        <TooltipTrigger as-child>
          <Button variant="outline" size="icon-sm" class="size-7 shrink-0" aria-label="切换主题" @click="onTheme">
            <component :is="themeIcon" class="size-3.5" aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ themeTitle }}</TooltipContent>
      </Tooltip>

      <Tooltip v-if="auth.enabled">
        <TooltipTrigger as-child>
          <Button variant="outline" size="sm" class="h-7 shrink-0 gap-1.5 px-2 sm:px-2.5 text-xs" aria-label="退出登录" :disabled="loggingOut" @click="onLogout">
            <PhSignOut class="size-3.5" aria-hidden="true" />
            <span class="hidden sm:inline">退出登录</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent>退出登录</TooltipContent>
      </Tooltip>
    </div>
  </header>
</template>
