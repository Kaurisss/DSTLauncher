<script setup>
import { computed } from 'vue';
import { HugeiconsIcon } from '@hugeicons/vue';
import {
  Archive02Icon, CodeFolderIcon, DashboardSquare01Icon, EarthIcon,
  Folder01Icon, GameController03Icon, PackageIcon, ServerStack01Icon,
} from '@hugeicons/core-free-icons';
import { Badge } from '@/components/ui/badge';
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent,
  SidebarGroupLabel, SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem,
  SidebarSeparator, useSidebar,
} from '@/components/ui/sidebar';
import { useUiStore } from '@/stores/ui.js';
import { useSystemStore } from '@/stores/system.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useLogs } from '@/composables/useLogs.js';

const ui = useUiStore();
const system = useSystemStore();
const rooms = useRoomsStore();
const { connected } = useLogs();
const { setOpenMobile } = useSidebar();
const icons = {
  dashboard: DashboardSquare01Icon, server: ServerStack01Icon, globe: EarthIcon,
  package: PackageIcon, archive: Archive02Icon, folder: Folder01Icon, fileCode: CodeFolderIcon,
};
const panelVersion = computed(() => rooms.meta?.panelVersion || '');
const serverText = computed(() => {
  if (system.installing) return '正在安装…';
  if (!system.installed) return '未安装服务器';
  return '版本 ' + (system.version || '未知');
});
function navigate(key) {
  ui.select(key);
  setOpenMobile(false);
}
</script>

<template>
  <Sidebar collapsible="icon">
    <SidebarHeader>
      <div class="flex items-center gap-3 p-2 group-data-[collapsible=icon]:p-0">
        <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
          <HugeiconsIcon :icon="GameController03Icon" class="size-5" aria-hidden="true" />
        </span>
        <div class="min-w-0 group-data-[collapsible=icon]:hidden">
          <p class="truncate text-sm font-semibold">DST 开服控制台</p>
          <p class="text-xs text-muted-foreground">专用服务器管理</p>
        </div>
      </div>
    </SidebarHeader>

    <SidebarContent>
      <SidebarGroup>
        <SidebarGroupLabel>管理</SidebarGroupLabel>
        <SidebarGroupContent>
          <nav id="app-navigation" aria-label="主导航">
            <SidebarMenu>
              <SidebarMenuItem v-for="tab in ui.tabs" :key="tab.key">
                <SidebarMenuButton
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
    <SidebarFooter class="gap-2 p-4 group-data-[collapsible=icon]:hidden">
      <Badge variant="secondary">日志 {{ connected ? '已连接' : '未连接' }}</Badge>
      <p v-if="panelVersion" class="break-all text-xs text-muted-foreground" title="面板版本号，重建后可据此确认构建">面板 {{ panelVersion }}</p>
      <p class="text-xs text-muted-foreground">{{ serverText }}</p>
    </SidebarFooter>
  </Sidebar>
</template>
