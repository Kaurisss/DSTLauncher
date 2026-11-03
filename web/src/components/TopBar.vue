<script setup>
import { computed } from 'vue';
import iconUrl from '@/assets/icon.png';
import { HugeiconsIcon } from '@hugeicons/vue';
import { ComputerIcon, Moon02Icon, Sun03Icon } from '@hugeicons/core-free-icons';

import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { SidebarTrigger, useSidebar } from '@/components/ui/sidebar';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { useTheme } from '@/composables/useTheme.js';
import { toast } from '@/lib/toast.js';
import { GAME_MODE_LABEL } from '@/lib/format.js';

const rooms = useRoomsStore();
const ui = useUiStore();
const theme = useTheme();
const { state } = useSidebar();

const pageTitle = computed(() => ui.tabs.find(t => t.key === ui.activeTab)?.label || '控制台');
const roomDetails = computed(() => {
  const room = rooms.current;
  if (!room) return '';
  const mode = GAME_MODE_LABEL[room.gameMode] || room.gameMode || '';
  return [room.key, mode.split(' ')[0], room.maxPlayers != null ? room.maxPlayers + '人' : ''].filter(Boolean).join(' · ');
});
const themeIcon = computed(() => ({ auto: ComputerIcon, light: Sun03Icon, dark: Moon02Icon })[theme.mode.value]);
const themeTitle = computed(() => '切换主题：' + theme.label.value);

function onTheme() {
  theme.cycle();
  toast(`主题：${theme.label.value}`);
}
</script>

<template>
  <header data-slot="app-header" class="relative z-20 flex h-14 w-full min-w-0 shrink-0 items-center justify-between bg-sidebar text-sidebar-foreground">
    <div class="flex min-w-0 flex-1 items-center">
      <!-- 品牌与触发区域：宽度与侧边栏严格同步（展开 220px，收起 48px） -->
      <div
        class="flex h-14 shrink-0 items-center overflow-hidden transition-[width] duration-180 ease-[cubic-bezier(0.2,0,0,1)]"
        :class="state === 'expanded' ? 'w-[220px]' : 'w-12'"
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

        <!-- 品牌标识：展开时展示 -->
        <div class="flex min-w-0 flex-1 items-center gap-2 pr-3">
          <img :src="iconUrl" alt="DST" width="28" height="28" class="size-7 shrink-0 rounded-lg object-cover" />
          <span class="truncate text-sm font-semibold tracking-tight text-sidebar-foreground">DST 开服控制台</span>
        </div>
      </div>

      <!-- 当前页面标题：从侧边栏分界处起排，无缝无分割线 -->
      <h1 class="truncate text-sm font-semibold sm:text-base text-sidebar-foreground pl-3">{{ pageTitle }}</h1>
    </div>

    <!-- 右侧：房间选择 + 切换主题 -->
    <div class="flex min-w-0 shrink-0 items-center gap-2 pr-3 sm:pr-4">
      <Select :model-value="rooms.currentId ? String(rooms.currentId) : ''" :disabled="!rooms.list.length" @update:model-value="rooms.select">
        <SelectTrigger id="roomSelect" aria-label="房间" class="w-auto min-w-[140px] max-w-[200px] sm:max-w-[340px] md:max-w-[420px] lg:max-w-[500px] bg-card data-[size=default]:h-9 px-3">
          <SelectValue as-child>
            <span class="flex min-w-0 flex-1 items-center gap-2">
              <span class="truncate text-sm font-medium">{{ rooms.current?.name || (rooms.list.length ? '选择房间' : '（暂无房间）') }}</span>
              <span v-if="roomDetails" class="hidden sm:inline-flex shrink-0 items-center rounded-md bg-muted/60 px-1.5 py-0.5 text-xs text-muted-foreground tabular-nums">{{ roomDetails }}</span>
            </span>
          </SelectValue>
        </SelectTrigger>
        <SelectContent position="popper" align="end">
          <SelectGroup>
            <SelectItem v-for="room in rooms.list" :key="room.id" :value="String(room.id)">{{ room.name }}</SelectItem>
          </SelectGroup>
        </SelectContent>
      </Select>

      <Tooltip>
        <TooltipTrigger as-child>
          <Button variant="outline" size="icon" class="size-9 shrink-0" aria-label="切换主题" @click="onTheme">
            <HugeiconsIcon :icon="themeIcon" aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ themeTitle }}</TooltipContent>
      </Tooltip>
    </div>
  </header>
</template>
