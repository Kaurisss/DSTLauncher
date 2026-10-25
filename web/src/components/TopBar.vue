<script setup>
import { computed } from 'vue';
import iconUrl from '@/assets/icon.png';
import { HugeiconsIcon } from '@hugeicons/vue';
import { ComputerIcon, Moon02Icon, Sun03Icon } from '@hugeicons/core-free-icons';

import { Button } from '@/components/ui/button';
import { Field, FieldLabel } from '@/components/ui/field';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { SidebarTrigger } from '@/components/ui/sidebar';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { useTheme } from '@/composables/useTheme.js';
import { toast } from '@/lib/toast.js';
import { GAME_MODE_LABEL } from '@/lib/format.js';

const rooms = useRoomsStore();
const ui = useUiStore();
const theme = useTheme();
const pageTitle = computed(() => ui.tabs.find(t => t.key === ui.activeTab)?.label || '控制台');
const pageSub = computed(() => {
  const room = rooms.current;
  if (!room) return '还没有房间';
  return `${room.key} · ${GAME_MODE_LABEL[room.gameMode] || room.gameMode} · ${room.maxPlayers} 人`;
});
const themeIcon = computed(() => ({ auto: ComputerIcon, light: Sun03Icon, dark: Moon02Icon })[theme.mode.value]);
const themeTitle = computed(() => {
  const shown = theme.resolved.value === 'dark' ? '深色' : '浅色';
  return `当前：${theme.label.value}（显示为${shown}）\n点击切换为：${theme.nextLabel.value}`;
});
function onTheme() {
  theme.cycle();
  toast(`主题：${theme.label.value}`);
}
</script>

<template>
  <header class="sticky top-0 z-20 flex h-14 w-full min-w-0 shrink-0 items-center bg-sidebar text-sidebar-foreground">
    <!-- 左侧：与侧边栏图标列严格对齐的触发按钮区域 -->
    <div class="flex h-14 w-11 sm:w-12 shrink-0 items-center justify-center">
      <Tooltip>
        <TooltipTrigger as-child>
          <SidebarTrigger aria-controls="app-navigation" aria-label="切换导航" size="icon" />
        </TooltipTrigger>
        <TooltipContent>展开或收起导航</TooltipContent>
      </Tooltip>
    </div>

    <!-- 品牌标识与标题 -->
    <div class="flex min-w-0 items-center gap-2 pr-2 sm:gap-2.5 sm:pr-3">
      <img
        :src="iconUrl"
        alt="DST"
        width="32"
        height="32"
        class="size-7 sm:size-8 shrink-0 rounded-lg object-cover shadow-xs"
      />
      <div class="min-w-0">
        <p class="truncate text-sm font-semibold leading-tight text-sidebar-foreground">DST 开服控制台</p>
      </div>
    </div>

    <!-- 中间：当前页面 -->
    <div class="hidden md:flex min-w-0 items-center gap-2 pl-1">
      <Separator orientation="vertical" class="h-4 bg-sidebar-border" />
      <h1 class="shrink-0 text-sm font-medium text-sidebar-foreground">{{ pageTitle }}</h1>
      <Tooltip>
        <TooltipTrigger as-child>
          <span class="text-xs text-muted-foreground truncate" tabindex="0">· {{ pageSub }}</span>
        </TooltipTrigger>
        <TooltipContent>{{ pageSub }}</TooltipContent>
      </Tooltip>
    </div>

    <!-- 右侧：房间 + 主题 -->
    <div class="ml-auto flex min-w-0 shrink-0 items-center gap-1.5 pr-3 sm:gap-2 sm:pr-4 md:pr-6">
      <Field orientation="horizontal" class="w-auto min-w-0 *:data-[slot=field-label]:flex-none">
        <FieldLabel for="roomSelect" class="hidden sm:block text-sidebar-foreground">房间</FieldLabel>
        <Select
          :model-value="rooms.currentId ? String(rooms.currentId) : ''"
          :disabled="!rooms.list.length"
          @update:model-value="(val) => rooms.select(val)"
        >
          <SelectTrigger id="roomSelect" class="w-32 max-w-[38vw] sm:w-48 sm:max-w-none bg-background/80">
            <SelectValue :placeholder="rooms.list.length ? '选择房间' : '（暂无房间）'" />
          </SelectTrigger>
          <SelectContent position="popper">
            <SelectGroup>
              <SelectItem v-for="room in rooms.list" :key="room.id" :value="String(room.id)">
                {{ room.name }}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </Field>
      <Tooltip>
        <TooltipTrigger as-child>
          <Button variant="outline" size="sm" class="shrink-0 px-2 sm:pl-2 sm:pr-2.5 bg-background/80" aria-label="切换主题" @click="onTheme">
            <HugeiconsIcon :icon="themeIcon" aria-hidden="true" />
            <span class="hidden sm:inline">{{ theme.label.value }}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent class="whitespace-pre-line">{{ themeTitle }}</TooltipContent>
      </Tooltip>
    </div>
  </header>
</template>
