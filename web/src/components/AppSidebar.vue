<script setup>
import { computed, onMounted, ref } from 'vue';
import iconUrl from '@/assets/icon.png';
import {
  PhArchive, PhCaretUpDown, PhDesktop, PhFolder, PhGear, PhGlobe, PhHardDrives,
  PhMoon, PhPackage, PhShieldCheck, PhSignOut, PhSquaresFour, PhStack, PhSun,
  PhTerminalWindow, PhUser, PhUsers,
} from '@phosphor-icons/vue';
import { Badge } from '@/components/ui/badge';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Separator } from '@/components/ui/separator';
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel,
  SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarSeparator, SidebarTrigger,
  useSidebar,
} from '@/components/ui/sidebar';
import { useAuthStore } from '@/stores/auth.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useUiStore } from '@/stores/ui.js';
import { useTheme } from '@/composables/useTheme.js';
import { guard, toast } from '@/lib/toast.js';

const auth = useAuthStore();
const rooms = useRoomsStore();
const ui = useUiStore();
const system = useSystemStore();
const theme = useTheme();
const { isMobile, setOpenMobile, state } = useSidebar();

const menuOpen = ref(false);
const loggingOut = ref(false);

const icons = {
  dashboard: PhSquaresFour, terminal: PhTerminalWindow, server: PhStack, globe: PhGlobe,
  package: PhPackage, users: PhUsers, archive: PhArchive, folder: PhFolder,
  settings: PhGear,
};

const themeOptions = [
  { key: 'auto', label: '自动', icon: PhDesktop },
  { key: 'light', label: '浅色', icon: PhSun },
  { key: 'dark', label: '深色', icon: PhMoon },
];

const displayName = computed(() => auth.username?.trim() || 'admin');
const avatarLetter = computed(() => displayName.value.slice(0, 1).toUpperCase());
const securityStatusText = computed(() => {
  if (auth.envManaged) return '启动参数鉴权';
  return auth.enabled ? '已开启密码保护' : '本地免密模式';
});
const panelVersion = computed(() => rooms.meta?.panelVersion || '');

onMounted(() => guard(() => system.refresh()));

function navigate(key) {
  ui.select(key);
  setOpenMobile(false);
}

function openSettingsSection(section) {
  menuOpen.value = false;
  ui.openSettings(section);
  setOpenMobile(false);
}

function selectTheme(mode) {
  theme.set(mode);
  toast(`主题：${theme.label.value}`);
}

async function onLogout() {
  if (loggingOut.value) return;
  loggingOut.value = true;
  try {
    menuOpen.value = false;
    await guard(() => auth.logout(), { success: '已退出登录' });
    setOpenMobile(false);
  } finally {
    loggingOut.value = false;
  }
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
      <Popover v-model:open="menuOpen">
        <PopoverTrigger as-child>
          <button
            type="button"
            class="group/user flex w-full cursor-pointer items-center gap-2 rounded-xl border border-sidebar-border/85 bg-background/95 px-2 py-2 text-left shadow-2xs transition-colors hover:bg-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring data-[state=open]:bg-background dark:bg-card/85 dark:hover:bg-card dark:data-[state=open]:bg-card group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:rounded-lg group-data-[collapsible=icon]:p-0"
            :aria-label="`当前用户：${displayName}（${securityStatusText}）`"
            :title="state === 'collapsed' && !isMobile ? `${displayName} · ${securityStatusText}` : undefined"
          >
            <span
              class="relative flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/12 text-xs font-semibold text-primary transition-colors group-data-[collapsible=icon]:size-full group-data-[collapsible=icon]:rounded-md"
            >
              <span v-if="avatarLetter" aria-hidden="true">{{ avatarLetter }}</span>
              <PhUser v-else class="size-4" aria-hidden="true" />
              <span
                v-if="auth.enabled"
                aria-hidden="true"
                class="absolute -top-0.5 -right-0.5 hidden size-2 rounded-full bg-emerald-500 ring-2 ring-background dark:ring-card group-data-[collapsible=icon]:block"
              />
            </span>
            <span class="min-w-0 flex-1 group-data-[collapsible=icon]:hidden">
              <span class="block truncate text-xs font-medium leading-tight text-foreground">
                {{ displayName }}
              </span>
              <span class="mt-0.5 block truncate text-[11px] leading-tight text-muted-foreground">
                {{ securityStatusText }}
              </span>
            </span>
            <PhCaretUpDown
              aria-hidden="true"
              class="size-3.5 shrink-0 text-muted-foreground/65 transition-colors group-hover/user:text-foreground group-data-[collapsible=icon]:hidden"
            />
          </button>
        </PopoverTrigger>

        <PopoverContent
          :side="state === 'collapsed' && !isMobile ? 'right' : 'top'"
          :align="state === 'collapsed' && !isMobile ? 'end' : 'start'"
          :side-offset="8"
          class="w-56 gap-0 rounded-xl p-1.5"
        >
          <div class="flex items-center gap-2.5 px-2 py-1.5">
            <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/12 text-xs font-semibold text-primary">
              <span v-if="avatarLetter" aria-hidden="true">{{ avatarLetter }}</span>
              <PhUser v-else class="size-4" aria-hidden="true" />
            </span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-1.5">
                <span class="truncate text-xs font-semibold text-foreground">{{ displayName }}</span>
                <Badge v-if="panelVersion" variant="secondary" class="h-4 px-1.5 font-mono text-[10px] font-normal">
                  {{ panelVersion }}
                </Badge>
              </div>
              <p class="mt-0.5 truncate text-[11px] text-muted-foreground">
                {{ securityStatusText }}
              </p>
            </div>
          </div>

          <Separator class="my-1.5" />

          <!-- 外观主题快切 -->
          <div class="px-2 py-1">
            <div class="mb-1.5 text-[11px] font-medium text-muted-foreground">外观主题</div>
            <div class="grid grid-cols-3 gap-1 rounded-lg bg-muted/70 p-0.5" role="group" aria-label="外观主题">
              <button
                v-for="opt in themeOptions"
                :key="opt.key"
                type="button"
                :aria-pressed="theme.mode.value === opt.key"
                class="flex cursor-pointer items-center justify-center gap-1 rounded-md py-1 text-[11px] font-medium transition-colors"
                :class="theme.mode.value === opt.key
                  ? 'bg-background text-foreground shadow-2xs dark:bg-card'
                  : 'text-muted-foreground hover:text-foreground'"
                @click="selectTheme(opt.key)"
              >
                <component :is="opt.icon" class="size-3 shrink-0" aria-hidden="true" />
                <span>{{ opt.label }}</span>
              </button>
            </div>
          </div>

          <Separator class="my-1.5" />

          <div role="menu" aria-label="用户功能菜单" class="grid gap-0.5">
            <button
              type="button"
              role="menuitem"
              class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none"
              @click="openSettingsSection('system')"
            >
              <PhShieldCheck class="size-3.5 text-muted-foreground" aria-hidden="true" />
              <span>{{ auth.enabled ? '账号与密码安全' : '开启密码保护' }}</span>
            </button>
            <button
              type="button"
              role="menuitem"
              class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none"
              @click="openSettingsSection('server')"
            >
              <PhHardDrives class="size-3.5 text-muted-foreground" aria-hidden="true" />
              <span>服务器与创意工坊</span>
            </button>
            <button
              type="button"
              role="menuitem"
              class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs text-popover-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:outline-none"
              @click="openSettingsSection('maintenance')"
            >
              <PhGear class="size-3.5 text-muted-foreground" aria-hidden="true" />
              <span>自动维护与系统设置</span>
            </button>

            <template v-if="auth.enabled">
              <Separator class="my-1" />
              <button
                type="button"
                role="menuitem"
                :disabled="loggingOut"
                class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-left text-xs text-destructive transition-colors hover:bg-destructive/10 focus-visible:bg-destructive/10 focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50"
                @click="onLogout"
              >
                <PhSignOut class="size-3.5" aria-hidden="true" />
                <span>{{ loggingOut ? '正在退出…' : '退出登录' }}</span>
              </button>
            </template>
          </div>
        </PopoverContent>
      </Popover>
    </SidebarFooter>
  </Sidebar>
</template>
