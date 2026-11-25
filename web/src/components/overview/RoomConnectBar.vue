<script setup>
// 概览左上大卡：房间概览信息 + 支持按分片切换的直连代码（默认自动探测外网 IP，可按分片自定义映射端口）。

import { computed, onMounted, reactive, ref, watch } from 'vue';
import {
  PhCheck, PhCopy, PhEye, PhEyeSlash, PhLockSimple,
  PhPencilSimple, PhSlidersHorizontal, PhTerminalWindow, PhWifiHigh,
} from '@phosphor-icons/vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import IconAction from '@/components/ui/IconAction.vue';
import UiField from '@/components/ui/UiField.vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { networkApi } from '@/lib/api.js';
import { GAME_MODE_LABEL, roomShardLabel } from '@/lib/format.js';
import { guard, toast } from '@/lib/toast.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useUiStore } from '@/stores/ui.js';

const rooms = useRoomsStore();
const system = useSystemStore();
const ui = useUiStore();

const ipInfo = ref({ ip: '', source: '', error: '' });
const ipLoading = ref(false);
const selectedShard = ref('');
const showPassword = ref(false);
const copied = ref(false);
const customOpen = ref(false);
const customIp = ref('');
const customPorts = reactive({});
const saving = ref(false);
const saveError = ref('');
let copyTimer = null;

const room = computed(() => rooms.current);
const enabledShards = computed(() => (room.value?.shards || []).filter(s => !s.disabled));
const shardTotal = computed(() => enabledShards.value.length);
const shardRunning = computed(() => rooms.statuses.filter(s => s.state === 'running').length);

const shardDotClass = computed(() => {
  if (!shardTotal.value || !shardRunning.value) return 'bg-muted-foreground/35';
  return shardRunning.value === shardTotal.value ? 'bg-emerald-500' : 'bg-amber-500';
});

const shardSub = computed(() => {
  if (!room.value || !shardTotal.value) return '未启用分片';
  if (rooms.starting) return '正在准备开服…';
  if (shardRunning.value === shardTotal.value) return '全部运行中';
  return shardRunning.value > 0 ? '部分运行中' : '全部已停止';
});

const activeShard = computed(() => (
  enabledShards.value.find(s => s.name === selectedShard.value)
  || enabledShards.value.find(s => s.isMaster)
  || enabledShards.value[0]
  || null
));

watch(
  [() => room.value?.id, () => enabledShards.value.map(s => s.name).join(',')],
  () => {
    if (!enabledShards.value.some(s => s.name === selectedShard.value)) {
      selectedShard.value = enabledShards.value.find(s => s.isMaster)?.name || enabledShards.value[0]?.name || '';
    }
  },
  { immediate: true },
);

function shardDisplayName(shard) {
  return roomShardLabel(room.value, shard);
}

function customPortFor(shard) {
  if (!room.value || !shard) return 0;
  const mapped = room.value.connectPorts?.[shard.name];
  if (mapped > 0) return mapped;
  if (shard.isMaster && room.value.connectPort > 0) return room.value.connectPort;
  return 0;
}

const isAnyCustom = computed(() => {
  if (!room.value) return false;
  if (room.value.connectIp || room.value.connectPort > 0) return true;
  return Object.values(room.value.connectPorts || {}).some(v => v > 0);
});

const isActiveCustom = computed(() => {
  if (!room.value || !activeShard.value) return false;
  return !!(room.value.connectIp || customPortFor(activeShard.value) > 0);
});

const effectiveIp = computed(() => (room.value?.connectIp || '').trim() || ipInfo.value.ip || '');
const effectivePort = computed(() => {
  const shard = activeShard.value;
  if (!shard) return 0;
  return customPortFor(shard) || shard.serverPort || 10999;
});

const modeLabel = computed(() => {
  const mode = room.value?.gameMode;
  return (GAME_MODE_LABEL[mode] || mode || '').split(' ')[0];
});
const networkLabel = computed(() => {
  if (!room.value) return '';
  if (room.value.offline) return '离线';
  if (room.value.lanOnly) return '局域网';
  return '在线';
});

function luaQuote(text) {
  return `"${String(text).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

const connectCommand = computed(() => {
  if (!effectiveIp.value || !effectivePort.value) return '';
  const host = luaQuote(effectiveIp.value);
  const pwd = room.value?.password || '';
  if (pwd) return `c_connect(${host}, ${effectivePort.value}, ${luaQuote(pwd)})`;
  return `c_connect(${host}, ${effectivePort.value})`;
});

const displayCommand = computed(() => {
  if (!effectiveIp.value || !effectivePort.value) return '';
  const host = luaQuote(effectiveIp.value);
  const pwd = room.value?.password || '';
  if (pwd && !showPassword.value) return `c_connect(${host}, ${effectivePort.value}, "••••")`;
  return connectCommand.value;
});

async function probePublicIp(refresh = false) {
  if (ipLoading.value) return;
  ipLoading.value = true;
  try {
    const data = await networkApi.publicIp(refresh);
    ipInfo.value = {
      ip: data?.ip || '',
      source: data?.source || '',
      error: data?.error || '',
    };
    if (refresh && data?.ip) toast(`已刷新外网 IP：${data.ip}`);
    else if (refresh && data?.error) toast(data.error, 'warn');
  } catch (err) {
    ipInfo.value = { ip: ipInfo.value.ip || '', source: ipInfo.value.source || '', error: err.message || '外网 IP 探测失败' };
    if (refresh) toast(err.message || '外网 IP 探测失败', 'err');
  } finally {
    ipLoading.value = false;
  }
}

async function copyConnect() {
  if (!connectCommand.value) return;
  if (!window.navigator.clipboard?.writeText) {
    toast('当前连接不支持剪贴板访问，请选中直连代码手动复制', 'warn');
    return;
  }
  const targetName = activeShard.value ? shardDisplayName(activeShard.value) : '';
  const { ok } = await guard(() => window.navigator.clipboard.writeText(connectCommand.value), {
    success: `${targetName ? `「${targetName}」` : ''}直连代码已复制，游戏内按 ~ 粘贴回车即可加入`,
  });
  if (ok) {
    copied.value = true;
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => { copied.value = false; }, 1800);
  }
}

watch(customOpen, open => {
  if (open && room.value) {
    customIp.value = room.value.connectIp || '';
    for (const k of Object.keys(customPorts)) delete customPorts[k];
    for (const sh of enabledShards.value) {
      const p = customPortFor(sh);
      customPorts[sh.name] = p > 0 ? String(p) : '';
    }
    saveError.value = '';
  }
});

watch(() => room.value?.id, () => {
  showPassword.value = false;
  customOpen.value = false;
});

async function saveCustom() {
  if (!room.value || saving.value) return;
  saveError.value = '';
  const ip = customIp.value.trim();
  const portsPayload = {};
  let masterPort = 0;

  for (const sh of enabledShards.value) {
    const raw = String(customPorts[sh.name] ?? '').trim();
    if (raw === '') continue;
    const n = Number(raw);
    if (!Number.isInteger(n) || n < 1 || n > 65535) {
      saveError.value = `「${shardDisplayName(sh)}」的映射端口必须是 1–65535 之间的整数`;
      return;
    }
    portsPayload[sh.name] = n;
    if (sh.isMaster) masterPort = n;
  }

  saving.value = true;
  try {
    const { ok, error } = await guard(() => rooms.updateConnect(room.value.id, {
      connectIp: ip,
      connectPort: masterPort,
      connectPorts: portsPayload,
    }));
    if (!ok) {
      saveError.value = error?.message || '保存失败';
      return;
    }
    const hasOverride = !!ip || Object.keys(portsPayload).length > 0;
    toast(hasOverride ? '自定义直连地址已保存' : '已恢复默认直连地址');
    customOpen.value = false;
  } finally {
    saving.value = false;
  }
}

async function clearCustom() {
  if (!room.value || saving.value) return;
  saving.value = true;
  saveError.value = '';
  try {
    const { ok, error } = await guard(() => rooms.updateConnect(room.value.id, {
      connectIp: '',
      connectPort: 0,
      connectPorts: {},
    }));
    if (!ok) {
      saveError.value = error?.message || '清除失败';
      return;
    }
    customIp.value = '';
    for (const k of Object.keys(customPorts)) customPorts[k] = '';
    toast('已清除自定义地址，恢复使用自动探测外网 IP 与分片默认端口');
    customOpen.value = false;
  } finally {
    saving.value = false;
  }
}

onMounted(() => {
  probePublicIp(false);
});
</script>

<template>
  <UiCard class="h-full" fill>
    <template #title>
      <span class="inline-flex flex-wrap items-center gap-1.5">
        <span>房间概览</span>
        <template v-if="room">
          <Badge variant="secondary" class="h-5 px-2 text-[11px] font-medium">{{ modeLabel }}</Badge>
          <Badge variant="outline" class="h-5 gap-1 px-2 text-[11px] font-normal text-muted-foreground">
            <PhWifiHigh class="size-3" aria-hidden="true" />
            <span>{{ networkLabel }}</span>
          </Badge>
          <Badge v-if="room.password" variant="outline" class="h-5 gap-1 px-2 text-[11px] font-normal text-muted-foreground">
            <PhLockSimple class="size-3" aria-hidden="true" />
            <span>有密码</span>
          </Badge>
        </template>
      </span>
    </template>
    <template #actions>
      <template v-if="room">
        <Button variant="outline" size="sm" class="h-7 gap-1.5 ps-2 pe-2.5 text-xs" @click="ui.select('console')">
          <PhTerminalWindow class="size-3.5" aria-hidden="true" />
          <span>控制台</span>
        </Button>
        <Button variant="outline" size="sm" class="h-7 gap-1.5 ps-2 pe-2.5 text-xs" @click="ui.openRoomEditor(room.id, 'general')">
          <PhPencilSimple class="size-3.5" aria-hidden="true" />
          <span>编辑房间</span>
        </Button>
      </template>
    </template>

    <UiEmpty v-if="!room" class="flex-1 justify-center">
      <span>请先在「房间管理」页新建一个房间</span>
      <Button size="sm" variant="outline" class="mt-2" @click="ui.openRoomEditor()">前往新建房间</Button>
    </UiEmpty>

    <div v-else class="flex min-h-0 flex-1 flex-col justify-between gap-3">
      <!-- 房间核心状态三联格 -->
      <div class="grid grid-cols-3 gap-2.5">
        <div class="flex min-w-0 flex-col justify-between rounded-lg border border-border/80 bg-muted/25 px-3 py-2.5">
          <span class="text-[11.5px] font-medium text-muted-foreground">运行分片</span>
          <div class="mt-1 flex items-center gap-1.5 text-lg font-semibold leading-tight tracking-tight tabular-nums">
            <span aria-hidden="true" :class="['size-2 shrink-0 rounded-full', shardDotClass]" />
            <span>{{ shardRunning }} / {{ shardTotal }}</span>
          </div>
          <span class="mt-0.5 truncate text-[11px] text-muted-foreground">{{ shardSub }}</span>
        </div>

        <button
          type="button"
          class="flex min-w-0 flex-col justify-between rounded-lg border border-border/80 bg-muted/25 px-3 py-2.5 text-left transition-colors hover:border-border hover:bg-muted/45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          @click="ui.select('mods')"
        >
          <span class="text-[11.5px] font-medium text-muted-foreground">已启用模组</span>
          <div class="mt-1 text-lg font-semibold leading-tight tracking-tight tabular-nums">
            {{ room.modCount ?? 0 }}
          </div>
          <span class="mt-0.5 truncate text-[11px] text-muted-foreground tabular-nums">上限 {{ room.maxPlayers }} 人</span>
        </button>

        <div class="flex min-w-0 flex-col justify-between rounded-lg border border-border/80 bg-muted/25 px-3 py-2.5">
          <span class="text-[11.5px] font-medium text-muted-foreground">游戏版本</span>
          <div class="mt-1 truncate font-mono text-base font-semibold leading-tight tracking-tight tabular-nums">
            {{ system.version ? `v${system.version}` : '—' }}
          </div>
          <span class="mt-0.5 truncate text-[11px] text-muted-foreground">
            {{ system.installed ? '服务端已就绪' : '未安装服务端' }}
          </span>
        </div>
      </div>

      <!-- 按分片切换的直连代码区 -->
      <div class="overview-connect-box grid gap-2 rounded-lg border border-border/80 bg-muted/25 p-2.5">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex min-w-0 flex-wrap items-center gap-1.5">
            <span class="text-[11.5px] font-medium text-muted-foreground">直连代码</span>
            <Badge
              v-if="isActiveCustom"
              variant="secondary"
              class="h-4.5 border-primary/20 bg-primary/10 px-1.5 text-[10.5px] font-medium text-primary"
            >
              自定义
            </Badge>
            <div v-if="enabledShards.length > 1" class="seg" role="group" aria-label="选择直连分片">
              <button
                v-for="sh in enabledShards"
                :key="sh.name"
                type="button"
                class="seg-btn h-5.5 px-2 text-[11.5px]"
                :class="{ 'is-active': activeShard?.name === sh.name }"
                :aria-pressed="activeShard?.name === sh.name"
                @click="selectedShard = sh.name"
              >
                {{ shardDisplayName(sh) }}
              </button>
            </div>
            <span v-else-if="activeShard" class="text-[11.5px] text-muted-foreground">
              {{ shardDisplayName(activeShard) }}
            </span>
          </div>

          <Dialog v-model:open="customOpen">
            <Tooltip>
              <TooltipTrigger as-child>
                <DialogTrigger as-child>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    class="h-6 gap-1 rounded-md px-1.5 text-[11.5px] text-muted-foreground hover:text-foreground"
                  >
                    <PhSlidersHorizontal class="size-3" aria-hidden="true" />
                    <span>自定义</span>
                  </Button>
                </DialogTrigger>
              </TooltipTrigger>
              <TooltipContent>自定义内网穿透 IP 与各分片映射端口</TooltipContent>
            </Tooltip>

            <DialogContent class="max-h-[88dvh] overflow-y-auto overscroll-contain sm:max-w-md" :close-disabled="saving">
              <DialogHeader>
                <DialogTitle>自定义直连地址</DialogTitle>
                <DialogDescription>
                  使用内网穿透（frp / ZeroTier）、DDNS 或端口映射时，可为「{{ room.name }}」自定义公网地址与各分片外部端口；留空则使用自动探测外网 IP 与分片默认游戏端口。
                </DialogDescription>
              </DialogHeader>
              <form class="grid gap-4" @submit.prevent="saveCustom">
                <UiField label="公网 IP 或域名" for-id="connect-custom-ip" block :tip="ipInfo.ip ? `当前自动探测外网 IP：${ipInfo.ip}` : '可填写公网 IPv4、域名或组网地址'">
                  <Input
                    id="connect-custom-ip"
                    v-model="customIp"
                    autocomplete="off"
                    :placeholder="ipInfo.ip || '如 play.example.com 或 1.2.3.4'"
                    :disabled="saving"
                  />
                </UiField>

                <div class="grid gap-2">
                  <div class="flex items-baseline justify-between gap-2">
                    <span class="text-xs font-medium text-foreground">各分片映射端口</span>
                    <span class="text-[11px] text-muted-foreground">留空使用默认游戏端口</span>
                  </div>
                  <div class="grid gap-2 sm:grid-cols-2">
                    <div
                      v-for="sh in enabledShards"
                      :key="sh.name"
                      class="grid gap-1 rounded-lg border border-border/80 bg-muted/25 p-2.5"
                    >
                      <label :for="`connect-port-${sh.name}`" class="flex items-center justify-between gap-1 text-xs font-medium">
                        <span class="truncate">{{ shardDisplayName(sh) }}</span>
                        <span class="font-mono text-[11px] font-normal text-muted-foreground tabular-nums">默认 {{ sh.serverPort }}</span>
                      </label>
                      <Input
                        :id="`connect-port-${sh.name}`"
                        v-model="customPorts[sh.name]"
                        type="number"
                        min="1"
                        max="65535"
                        step="1"
                        class="h-8 bg-background font-mono text-xs tabular-nums"
                        :placeholder="String(sh.serverPort)"
                        :disabled="saving"
                      />
                    </div>
                  </div>
                </div>

                <p v-if="saveError" role="alert" class="text-sm text-destructive">{{ saveError }}</p>
                <DialogFooter class="gap-2 sm:justify-between">
                  <Button
                    v-if="isAnyCustom"
                    type="button"
                    variant="ghost"
                    size="sm"
                    class="text-muted-foreground hover:text-destructive"
                    :disabled="saving"
                    @click="clearCustom"
                  >
                    恢复默认
                  </Button>
                  <div class="ml-auto flex items-center gap-2">
                    <DialogClose as-child>
                      <Button type="button" variant="outline" size="sm" :disabled="saving">取消</Button>
                    </DialogClose>
                    <Button type="submit" size="sm" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</Button>
                  </div>
                </DialogFooter>
              </form>
            </DialogContent>
          </Dialog>
        </div>

        <div class="flex min-w-0 items-center justify-between gap-1.5 rounded-md border border-border/75 bg-background px-2.5 py-1.5">
          <Skeleton v-if="ipLoading && !effectiveIp" class="h-4 w-52 rounded" />
          <code
            v-else-if="displayCommand"
            class="min-w-0 flex-1 truncate rounded bg-transparent font-mono text-xs text-foreground"
            :title="displayCommand"
          >{{ displayCommand }}</code>
          <span v-else class="min-w-0 flex-1 truncate text-xs text-muted-foreground">
            {{ ipInfo.error || '未探测到外网 IP' }}
          </span>

          <div class="flex shrink-0 items-center gap-0.5">
            <Tooltip v-if="room.password && connectCommand">
              <TooltipTrigger as-child>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  class="size-6 rounded-md text-muted-foreground hover:text-foreground"
                  :aria-label="showPassword ? '隐藏直连密码' : '显示直连密码'"
                  @click="showPassword = !showPassword"
                >
                  <component :is="showPassword ? PhEyeSlash : PhEye" class="size-3.5" aria-hidden="true" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{{ showPassword ? '隐藏密码' : '显示明文密码' }}</TooltipContent>
            </Tooltip>

            <Tooltip v-if="!room.connectIp">
              <TooltipTrigger as-child>
                <IconAction
                  variant="ghost"
                  icon="refresh"
                  class="size-6 rounded-md text-muted-foreground hover:text-foreground"
                  :icon-class="ipLoading ? 'animate-spin motion-reduce:animate-none' : ''"
                  label="重新探测外网 IP"
                  :disabled="ipLoading"
                  @click="probePublicIp(true)"
                />
              </TooltipTrigger>
              <TooltipContent>{{ ipInfo.source ? `外网 IP 来源：${ipInfo.source}（点击刷新）` : '重新探测外网 IP' }}</TooltipContent>
            </Tooltip>

            <Tooltip v-if="connectCommand">
              <TooltipTrigger as-child>
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  class="h-6 gap-1 rounded-md px-2 text-[11.5px]"
                  aria-label="复制直连代码"
                  @click="copyConnect"
                >
                  <component :is="copied ? PhCheck : PhCopy" :class="['size-3', copied && 'text-emerald-600 dark:text-emerald-400']" aria-hidden="true" />
                  <span>{{ copied ? '已复制' : '复制' }}</span>
                </Button>
              </TooltipTrigger>
              <TooltipContent>复制当前分片直连命令</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </div>
    </div>
  </UiCard>
</template>
