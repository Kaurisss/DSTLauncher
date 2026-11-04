<script setup>
import { onMounted } from 'vue';
import iconUrl from '@/assets/icon.png';
import {
  PhArchive, PhChatCircle, PhCode, PhFolder, PhGear, PhGlobe, PhPackage, PhSquaresFour, PhStack,
} from '@phosphor-icons/vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Sidebar, SidebarContent, SidebarGroup, SidebarGroupContent,
  SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarHeader, SidebarTrigger,
  useSidebar,
} from '@/components/ui/sidebar';
import { useUiStore } from '@/stores/ui.js';
import { useSystemStore } from '@/stores/system.js';
import { guard } from '@/lib/toast.js';

const ui = useUiStore();
const system = useSystemStore();
const { setOpenMobile } = useSidebar();
const icons = {
  dashboard: PhSquaresFour, server: PhStack, globe: PhGlobe,
  package: PhPackage, message: PhChatCircle, archive: PhArchive, folder: PhFolder, fileCode: PhCode,
  settings: PhGear,
};
onMounted(() => guard(() => system.refresh()));

function navigate(key) {
  ui.select(key);
  setOpenMobile(false);
}
</script>

<template>
  <Sidebar collapsible="icon" class="shrink-0 border-none">
    <SidebarHeader class="h-16 shrink-0 justify-center px-3 md:hidden group-data-[collapsible=icon]:h-12 group-data-[collapsible=icon]:justify-end group-data-[collapsible=icon]:px-2 group-data-[collapsible=icon]:py-0">
      <div class="flex min-w-0 items-center gap-2">
        <img :src="iconUrl" alt="DST" width="32" height="32" class="size-8 shrink-0 rounded-lg object-cover group-data-[collapsible=icon]:hidden" />
        <div class="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
          <p class="truncate text-[13px] font-semibold leading-tight">DST 开服控制台</p>
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
                  <component :is="icons[tab.icon]" aria-hidden="true" />
                  <span>{{ tab.label }}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </nav>
        </SidebarGroupContent>
      </SidebarGroup>
    </SidebarContent>
  </Sidebar>
</template>
