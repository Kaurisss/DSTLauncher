<script setup>
import { computed } from 'vue';
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
  <header class="sticky top-0 z-20 flex h-14 shrink-0 items-center bg-sidebar text-sidebar-foreground">
    <!-- 左侧：与侧边栏图标列严格对齐的触发按钮区域 -->
    <div class="flex h-14 w-12 shrink-0 items-center justify-center">
      <SidebarTrigger
        aria-controls="app-navigation"
        aria-label="切换导航"
        title="展开或收起导航"
        size="icon"
      />
    </div>

    <!-- 品牌标识与标题 -->
    <div class="flex items-center gap-2.5 pr-3">
      <img
        src="/icon.png"
        alt="DST"
        class="size-8 shrink-0 rounded-lg object-cover shadow-xs"
      />
      <div class="min-w-0">
        <p class="truncate text-sm font-semibold leading-tight text-sidebar-foreground">DST 开服控制台</p>
        <p class="text-xs text-muted-foreground leading-tight hidden sm:block">专用服务器管理</p>
      </div>
    </div>

    <!-- 中间：当前页面 -->
    <div class="hidden md:flex items-center gap-2 pl-1">
      <Separator orientation="vertical" class="h-4 bg-sidebar-border" />
      <h1 class="text-sm font-medium text-sidebar-foreground">{{ pageTitle }}</h1>
      <span class="text-xs text-muted-foreground truncate" :title="pageSub">· {{ pageSub }}</span>
    </div>

    <!-- 右侧：房间 + 主题 -->
    <div class="ml-auto flex items-center gap-2 pr-4 md:pr-6">
      <Field orientation="horizontal" class="*:data-[slot=field-label]:flex-none">
        <FieldLabel for="roomSelect" class="hidden sm:block text-sidebar-foreground">房间</FieldLabel>
        <Select
          :model-value="rooms.currentId ? String(rooms.currentId) : ''"
          :disabled="!rooms.list.length"
          @update:model-value="(val) => rooms.select(val)"
        >
          <SelectTrigger id="roomSelect" class="w-40 sm:w-48 bg-background/80">
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
      <Button variant="outline" size="sm" class="bg-background/80" :title="themeTitle" aria-label="切换主题" @click="onTheme">
        <HugeiconsIcon :icon="themeIcon" data-icon="inline-start" aria-hidden="true" />
        <span class="hidden sm:inline">{{ theme.label.value }}</span>
      </Button>
    </div>
  </header>
</template>
