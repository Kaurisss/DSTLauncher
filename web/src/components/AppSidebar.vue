<script setup>
import { computed, onMounted } from 'vue';
import iconUrl from '@/assets/icon.png';
import {
  PhArchive, PhCaretRight, PhFolder, PhGear, PhGlobe, PhPackage, PhSquaresFour, PhStack, PhUsers,
} from '@phosphor-icons/vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel,
  SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarSeparator, SidebarTrigger,
  useSidebar,
} from '@/components/ui/sidebar';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { useSystemStore } from '@/stores/system.js';
import { guard } from '@/lib/toast.js';

const rooms = useRoomsStore();
const ui = useUiStore();
const system = useSystemStore();
const { isMobile, setOpenMobile, state } = useSidebar();

const icons = {
  dashboard: PhSquaresFour, server: PhStack, globe: PhGlobe,
  package: PhPackage, users: PhUsers, archive: PhArchive, folder: PhFolder,
  settings: PhGear,
};

const runningCount = computed(() => rooms.statuses.filter(s => s.state === 'running').length);
const shardTotal = computed(() => rooms.shards.length);
const roomStatusText = computed(() => {
  if (!rooms.current) return '点击创建或选择房间';
  if (rooms.starting) return '正在准备开服…';
  if (rooms.running) return shardTotal.value ? `运行中 · ${runningCount.value}/${shardTotal.value} 分片` : '运行中';
  if (rooms.statusLoaded) return '已停止';
  return rooms.current.key || '等待状态';
});

onMounted(() => guard(() => system.refresh()));

function navigate(key) {
  ui.select(key);
  setOpenMobile(false);
}

function onRoomCardClick() {
  navigate(ui.activeTab === 'rooms' && rooms.current ? 'overview' : 'rooms');
}
</script>

<template>
  <Sidebar collapsible="icon" class="shrink-0 border-none">
    <SidebarHeader class="h-12 shrink-0 justify-center border-b border-sidebar-border/60 px-2.5 py-0 md:mt-2 md:border-none group-data-[collapsible=icon]:px-2">
      <div class="flex min-w-0 items-center gap-2.5 group-data-[collapsible=icon]:justify-center">
        <img
          :src="iconUrl"
          alt="DST"
          width="28"
          height="28"
          class="size-7 shrink-0 rounded-lg object-cover shadow-2xs ring-1 ring-black/8 dark:ring-white/10"
        />
        <span class="truncate text-[13.5px] font-semibold tracking-tight text-sidebar-foreground group-data-[collapsible=icon]:hidden">
          DST 开服控制台
        </span>
        <SidebarTrigger
          class="ml-auto size-7 shrink-0 md:hidden"
          aria-controls="app-navigation"
          aria-label="收起导航"
          size="icon-sm"
        />
      </div>
    </SidebarHeader>

    <SidebarContent class="pt-1">
      <nav id="app-navigation" aria-label="主导航" class="flex flex-1 flex-col">
        <template v-for="(group, index) in ui.navGroups" :key="group.key">
          <SidebarSeparator v-if="index > 0" class="my-1.5 hidden group-data-[collapsible=icon]:block" />
          <SidebarGroup :class="index === 0 ? 'pt-1 pb-2' : 'pt-2 pb-2'">
            <SidebarGroupLabel class="h-7 px-2 text-[11px] font-medium tracking-wider text-muted-foreground whitespace-nowrap overflow-hidden group-data-[collapsible=icon]:hidden">
              {{ group.label }}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu class="gap-0.5">
                <SidebarMenuItem v-for="tab in group.items" :key="tab.key">
                  <SidebarMenuButton
                    class="h-8.5 gap-2.5 rounded-lg ps-2 pe-2.5 text-[13.5px] text-sidebar-foreground/75 transition-colors hover:bg-sidebar-accent/70 hover:text-sidebar-foreground data-active:bg-sidebar-accent data-active:font-medium data-active:text-sidebar-accent-foreground [&_svg]:text-sidebar-foreground/60 hover:[&_svg]:text-sidebar-foreground data-active:[&_svg]:text-sidebar-accent-foreground"
                    :is-active="ui.activeTab === tab.key"
                    :aria-current="ui.activeTab === tab.key ? 'page' : undefined"
                    :tooltip="tab.label"
                    @click="navigate(tab.key)"
                  >
                    <component :is="icons[tab.icon]" aria-hidden="true" />
                    <span>{{ tab.label }}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </template>
      </nav>
    </SidebarContent>

    <SidebarFooter class="px-2 pt-1 pb-2.5 md:pb-2 group-data-[collapsible=icon]:items-center">
      <Tooltip>
        <TooltipTrigger as-child>
          <button
            type="button"
            class="group/room flex w-full items-center gap-2 rounded-xl border border-sidebar-border/85 bg-background/95 px-2 py-2 text-left shadow-2xs transition-colors hover:bg-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring dark:bg-card/85 dark:hover:bg-card group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-lg group-data-[collapsible=icon]:p-0"
            :aria-label="rooms.current ? `当前房间：${rooms.current.name}（${roomStatusText}）` : '前往房间管理'"
            @click="onRoomCardClick"
          >
            <span
              :class="[
                'relative flex size-7 shrink-0 items-center justify-center rounded-lg transition-colors group-data-[collapsible=icon]:size-full group-data-[collapsible=icon]:rounded-md',
                rooms.running
                  ? 'bg-emerald-500/12 text-emerald-600 dark:bg-emerald-500/18 dark:text-emerald-400'
                  : 'bg-muted text-muted-foreground',
              ]"
            >
              <PhStack class="size-4" aria-hidden="true" />
              <span
                v-if="rooms.current"
                aria-hidden="true"
                :class="[
                  'absolute -top-0.5 -right-0.5 hidden size-2 rounded-full ring-2 ring-background dark:ring-card group-data-[collapsible=icon]:block',
                  rooms.running ? 'bg-emerald-500' : 'bg-muted-foreground/45',
                ]"
              />
            </span>
            <span class="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
              <span class="block truncate text-xs font-medium leading-tight text-foreground">
                {{ rooms.current?.name || '暂无房间' }}
              </span>
              <span class="mt-0.5 flex items-center gap-1.5 truncate text-[11px] leading-tight text-muted-foreground tabular-nums">
                <span
                  v-if="rooms.current"
                  aria-hidden="true"
                  :class="['size-1.5 shrink-0 rounded-full', rooms.running ? 'bg-emerald-500' : 'bg-muted-foreground/45']"
                />
                <span class="truncate">{{ roomStatusText }}</span>
              </span>
            </span>
            <PhCaretRight
              aria-hidden="true"
              class="size-3.5 shrink-0 text-muted-foreground/60 transition-transform group-hover/room:translate-x-0.5 group-hover/room:text-foreground group-data-[collapsible=icon]:hidden"
            />
          </button>
        </TooltipTrigger>
        <TooltipContent side="right" :hidden="state !== 'collapsed' || isMobile">
          {{ rooms.current ? `${rooms.current.name} · ${roomStatusText}` : '房间管理' }}
        </TooltipContent>
      </Tooltip>
    </SidebarFooter>
  </Sidebar>
</template>
