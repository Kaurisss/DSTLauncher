<script setup>
import { computed } from 'vue';
import { HugeiconsIcon } from '@hugeicons/vue';
import { ComputerIcon, Moon02Icon, Sun03Icon } from '@hugeicons/core-free-icons';

import { Button } from '@/components/ui/button';
import { Field, FieldLabel } from '@/components/ui/field';
import { NativeSelect } from '@/components/ui/native-select';
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
  <header class="flex shrink-0 flex-wrap items-center gap-3 border-b bg-background p-4 md:sticky md:top-0 md:z-20 md:px-6">
    <SidebarTrigger aria-controls="app-navigation" aria-label="切换导航" title="展开或收起导航" />
    <Separator orientation="vertical" class="h-4" />
    <div class="min-w-0 flex-1">
      <h1 class="text-base font-semibold">{{ pageTitle }}</h1>
      <p class="truncate text-xs text-muted-foreground" :title="pageSub">{{ pageSub }}</p>
    </div>

    <div class="ml-auto flex w-full min-w-0 items-center gap-3 sm:w-auto">
      <Field orientation="horizontal" class="min-w-0 flex-1 sm:w-auto sm:flex-none *:data-[slot=field-label]:flex-none">
        <FieldLabel for="roomSelect">房间</FieldLabel>
        <NativeSelect
          id="roomSelect"
          class="min-w-0 flex-1 sm:w-48 sm:flex-none"
          :model-value="rooms.currentId ?? ''"
          :disabled="!rooms.list.length"
          @update:model-value="rooms.select"
        >
          <option v-if="!rooms.list.length" value="">（暂无房间）</option>
          <option v-for="room in rooms.list" :key="room.id" :value="room.id">{{ room.name }}</option>
        </NativeSelect>
      </Field>
      <Button variant="outline" size="sm" :title="themeTitle" aria-label="切换主题" @click="onTheme">
        <HugeiconsIcon :icon="themeIcon" data-icon="inline-start" aria-hidden="true" />
        {{ theme.label.value }}
      </Button>
    </div>
  </header>
</template>
