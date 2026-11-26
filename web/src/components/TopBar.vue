<script setup>
import { computed } from 'vue';
import { PhDesktop, PhMoon, PhSun } from '@phosphor-icons/vue';

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

const pageTitle = computed(() => ui.activeTabMeta?.label || '控制台');
const groupLabel = computed(() => (ui.isGlobalTab ? '全局管理' : '当前房间'));
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
</script>

<template>
  <header
    data-slot="app-header"
    class="relative z-20 flex h-12 w-full min-w-0 shrink-0 items-center justify-between gap-3 border-b border-border/75 bg-background px-3 text-foreground"
  >
    <div class="flex min-w-0 flex-1 items-center gap-2">
      <Tooltip>
        <TooltipTrigger as-child>
          <SidebarTrigger
            aria-controls="app-navigation"
            aria-label="切换导航"
            size="icon-sm"
            class="size-7 shrink-0 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
          />
        </TooltipTrigger>
        <TooltipContent>展开或收起导航 (Ctrl+B)</TooltipContent>
      </Tooltip>

      <span aria-hidden="true" class="h-4 w-px shrink-0 bg-border/80" />

      <div class="flex min-w-0 items-center gap-1.5">
        <span class="hidden shrink-0 text-xs font-medium text-muted-foreground sm:inline">{{ groupLabel }}</span>
        <span aria-hidden="true" class="hidden shrink-0 select-none text-xs text-muted-foreground/45 sm:inline">/</span>
        <h1 class="truncate text-sm font-semibold text-foreground">{{ pageTitle }}</h1>
      </div>
    </div>

    <div class="flex min-w-0 shrink-0 items-center gap-1.5">
      <Select
        v-if="!ui.isGlobalTab"
        :model-value="rooms.currentId ? String(rooms.currentId) : ''"
        :disabled="!rooms.list.length"
        @update:model-value="rooms.select"
      >
        <SelectTrigger
          id="roomSelect"
          aria-label="房间"
          size="sm"
          class="h-7.5 w-auto min-w-[140px] max-w-[200px] rounded-lg border-border/80 bg-background px-2.5 text-xs shadow-none hover:bg-muted/50 sm:max-w-[340px] md:max-w-[420px] lg:max-w-[480px]"
        >
          <SelectValue as-child>
            <span class="flex min-w-0 flex-1 items-center gap-1.5">
              <span
                v-if="rooms.current"
                aria-hidden="true"
                :data-running="isRoomRunning(rooms.current) ? 'true' : 'false'"
                :class="['size-2 shrink-0 rounded-full', isRoomRunning(rooms.current) ? 'bg-emerald-500' : 'bg-muted-foreground/35']"
              />
              <span class="truncate text-xs font-medium">{{ rooms.current?.name || (rooms.list.length ? '选择房间' : '（暂无房间）') }}</span>
              <span
                v-if="roomDetails"
                class="hidden shrink-0 items-center rounded bg-muted/70 px-1.5 py-0.5 text-[11px] leading-none text-muted-foreground tabular-nums sm:inline-flex"
              >
                {{ roomDetails }}
              </span>
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
          <Button
            variant="outline"
            size="icon-sm"
            class="size-7.5 shrink-0 rounded-lg border-border/80 bg-background text-muted-foreground shadow-none hover:bg-muted/60 hover:text-foreground"
            aria-label="切换主题"
            @click="onTheme"
          >
            <component :is="themeIcon" class="size-3.5" aria-hidden="true" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{{ themeTitle }}</TooltipContent>
      </Tooltip>
    </div>
  </header>
</template>
