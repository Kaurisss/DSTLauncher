<script setup>
import { computed, onMounted } from 'vue';
import iconUrl from '@/assets/icon.png';
import { HugeiconsIcon } from '@hugeicons/vue';
import {
  Archive02Icon, CodeFolderIcon, DashboardSquare01Icon, EarthIcon,
  Folder01Icon, Message01Icon, PackageIcon, ServerStack01Icon, Settings02Icon,
} from '@hugeicons/core-free-icons';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarHeader, SidebarTrigger,
  SidebarSeparator, useSidebar,
} from '@/components/ui/sidebar';
import { useUiStore } from '@/stores/ui.js';
import { useSystemStore } from '@/stores/system.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useLogs } from '@/composables/useLogs.js';
import { guard } from '@/lib/toast.js';

const ui = useUiStore();
const system = useSystemStore();
const rooms = useRoomsStore();
const { connected } = useLogs();
const { setOpenMobile } = useSidebar();
const icons = {
  dashboard: DashboardSquare01Icon, server: ServerStack01Icon, globe: EarthIcon,
  package: PackageIcon, message: Message01Icon, archive: Archive02Icon, folder: Folder01Icon, fileCode: CodeFolderIcon,
  settings: Settings02Icon,
};
const panelVersion = computed(() => rooms.meta?.panelVersion || '');
const serverText = computed(() => {
  if (system.installing) return '正在安装…';
  if (!system.installed) return '未安装服务器';
  return '游戏版本 ' + (system.version || '未知');
});
onMounted(() => guard(() => system.refresh()));

function navigate(key) {
  ui.select(key);
  setOpenMobile(false);
}
</script>

<template>
  <Sidebar collapsible="icon" class="border-r border-sidebar-border">
    <SidebarHeader class="h-16 shrink-0 justify-center px-3 group-data-[collapsible=icon]:h-12 group-data-[collapsible=icon]:justify-end group-data-[collapsible=icon]:px-2 group-data-[collapsible=icon]:py-0">
      <div class="flex min-w-0 items-center gap-2">
        <img :src="iconUrl" alt="DST" width="32" height="32" class="size-8 shrink-0 rounded-lg object-cover group-data-[collapsible=icon]:hidden" />
        <div class="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
          <p class="truncate text-[13px] font-semibold leading-tight">DST 开服控制台</p>
          <Badge v-if="panelVersion" variant="secondary" class="mt-1 px-1.5 py-0 text-[10px]">{{ panelVersion }}</Badge>
        </div>
        <Tooltip>
          <TooltipTrigger as-child>
            <SidebarTrigger class="shrink-0 group-data-[collapsible=icon]:size-8" aria-controls="app-navigation" aria-label="切换导航" size="icon-sm" />
          </TooltipTrigger>
          <TooltipContent side="right">展开或收起导航</TooltipContent>
        </Tooltip>
      </div>
    </SidebarHeader>
    <SidebarContent>
      <SidebarGroup class="group-data-[collapsible=icon]:pt-0">
        <SidebarGroupContent>
          <nav id="app-navigation" aria-label="主导航">
            <SidebarMenu>
              <SidebarMenuItem v-for="tab in ui.tabs" :key="tab.key">
                <SidebarMenuButton
                  class="transition-colors"
                  :is-active="ui.activeTab === tab.key"
                  :aria-current="ui.activeTab === tab.key ? 'page' : undefined"
                  :tooltip="tab.label"
                  @click="navigate(tab.key)"
                >
                  <HugeiconsIcon :icon="icons[tab.icon]" aria-hidden="true" />
                  <span>{{ tab.label }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </nav>
        </SidebarGroupContent>
      </SidebarGroup>
    </SidebarContent>

    <SidebarSeparator />
    <SidebarFooter class="px-3 py-3 group-data-[collapsible=icon]:hidden">
      <div class="flex min-w-0 items-center justify-between gap-2 text-[10px] text-muted-foreground">
        <span role="status" class="inline-flex shrink-0 items-center gap-1.5">
          <span class="size-1.5 shrink-0 rounded-full" :class="connected ? 'bg-emerald-600' : 'bg-muted-foreground'" aria-hidden="true" />
          日志{{ connected ? '已连接' : '未连接' }}
        </span>
        <Tooltip>
          <TooltipTrigger as-child>
            <span class="truncate" tabindex="0">{{ serverText }}</span>
          </TooltipTrigger>
          <TooltipContent>{{ serverText }}</TooltipContent>
        </Tooltip>
      </div>
    </SidebarFooter>
  </Sidebar>
</template>
