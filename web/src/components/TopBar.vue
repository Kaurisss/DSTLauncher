<script setup>
import { computed } from 'vue';
import { HugeiconsIcon } from '@hugeicons/vue';
import { ComputerIcon, Moon02Icon, Sun03Icon } from '@hugeicons/core-free-icons';

import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
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
  <header class="flex h-16 min-w-0 shrink-0 items-center gap-3 border-b border-border bg-background px-4 text-foreground md:px-6">
    <div class="flex min-w-0 flex-1 items-center gap-2">
      <Tooltip>
        <TooltipTrigger as-child>
          <SidebarTrigger class="md:hidden" aria-controls="app-navigation" aria-label="切换导航" size="icon-sm" />
        </TooltipTrigger>
        <TooltipContent>展开或收起导航</TooltipContent>
      </Tooltip>
      <h1 class="truncate text-sm font-semibold sm:text-base">{{ pageTitle }}</h1>
    </div>

    <div class="flex min-w-0 shrink-0 items-center gap-2">
      <Select :model-value="rooms.currentId ? String(rooms.currentId) : ''" :disabled="!rooms.list.length" @update:model-value="rooms.select">
        <SelectTrigger id="roomSelect" aria-label="房间" class="w-36 min-w-0 max-w-[42vw] bg-card data-[size=default]:h-10 sm:w-64 sm:max-w-none lg:w-auto lg:max-w-[55vw]">
          <SelectValue as-child>
            <span class="flex min-w-0 flex-1 flex-col items-start gap-0.5 lg:flex-row lg:items-center lg:gap-2">
              <span class="block max-w-full truncate text-xs sm:text-sm lg:max-w-72">{{ rooms.current?.name || (rooms.list.length ? '选择房间' : '（暂无房间）') }}</span>
              <span v-if="roomDetails" class="max-w-full truncate text-[10px] text-muted-foreground lg:rounded-md lg:bg-muted lg:px-1.5 lg:py-0.5 lg:text-xs">{{ roomDetails }}</span>
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
          <Button variant="outline" size="icon" class="shrink-0" aria-label="切换主题" @click="onTheme">
            <HugeiconsIcon :icon="themeIcon" aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ themeTitle }}</TooltipContent>
      </Tooltip>
    </div>
  </header>
</template>
