<script setup>
import { computed, onUnmounted, ref, watch } from 'vue';
import { PhCaretLeft, PhCaretRight, PhCopy } from '@phosphor-icons/vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import { Skeleton } from '@/components/ui/skeleton';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import UiToolbarFilters from '@/components/ui/UiToolbarFilters.vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import RoomAdmins from '@/components/rooms/RoomAdmins.vue';
import RoomPlayerLists from '@/components/rooms/RoomPlayerLists.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { logApi, playerListApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { CHARACTERS, roomShardLabel } from '@/lib/format.js';
import { playerLogRefresh } from '@/lib/playerLogRefresh.js';
import { guard, toast } from '@/lib/toast.js';

const rooms = useRoomsStore();
const ui = useUiStore();
const activeSection = ref('logs');
const sections = [
  { key: 'logs', label: '玩家日志' },
  { key: 'lists', label: '管理员与准入名单' },
];
const shard = ref('');
const kind = ref('all');
const search = ref('');
const result = ref(null);
const error = ref('');
const loading = ref(false);
const streaming = ref(false);
const expectedCount = ref(0);
const page = ref(1);
const pageSize = ref('10');
const blocking = ref('');
const blocked = ref(new Set());
const selected = ref(new Set());
const deleting = ref(false);
let context = 0;
let generation = 0;
let timer;
let disposed = false;
let activeController = null;

const shards = computed(() => rooms.current?.shards || []);
const labels = { all: '全部类型', chat: '聊天', whisper: '私聊', join: '进入', leave: '离开', death: '死亡', resurrect: '复活', announcement: '公告' };
const eventClasses = {
  chat: 'border-amber-500/20 bg-amber-500/10 text-amber-800 dark:text-amber-300',
  whisper: 'border-sky-500/20 bg-sky-500/10 text-sky-800 dark:text-sky-300',
  join: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-800 dark:text-emerald-300',
  leave: 'bg-muted text-muted-foreground',
  death: 'border-red-500/20 bg-red-500/10 text-red-800 dark:text-red-300',
  resurrect: 'border-teal-500/20 bg-teal-500/10 text-teal-800 dark:text-teal-300',
  announcement: 'border-indigo-500/20 bg-indigo-500/10 text-indigo-800 dark:text-indigo-300',
};
const entries = computed(() => result.value?.entries || []);
const size = computed(() => Number(pageSize.value));
const total = computed(() => result.value?.total || 0);
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / size.value)));
const start = computed(() => (page.value - 1) * size.value);
const initialSkeletonRows = computed(() => Math.min(size.value, 12));
const SKELETON_MSG_WIDTHS = ['w-28', 'w-56', 'w-72', 'w-64', 'w-40', 'w-60', 'w-52', 'w-36', 'w-64', 'w-28'];
const pendingSkeletonCount = computed(() => {
  if (!streaming.value) return 0;
  return Math.min(Math.max(expectedCount.value - entries.value.length, 0), 12);
});

function abortActive() {
  if (activeController) {
    activeController.abort();
    activeController = null;
  }
}

function recordedTimeParts(iso) {
  const date = new Date(iso);
  return [
    date.toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' }),
    date.toLocaleTimeString('zh-CN', { hour12: false }),
  ];
}

watch(() => rooms.currentId, () => {
  const list = shards.value;
  shard.value = list.find(item => item.isMaster)?.name || list[0]?.name || '';
  search.value = '';
  blocked.value = new Set();
}, { immediate: true });

watch(() => shards.value.map(item => item.name).join(','), () => {
  if (!shards.value.some(item => item.name === shard.value)) {
    shard.value = shards.value.find(item => item.isMaster)?.name || shards.value[0]?.name || '';
  }
});

watch(() => [rooms.currentId, shard.value, kind.value, search.value, pageSize.value], () => {
  page.value = 1;
}, { flush: 'sync' });
watch(playerLogRefresh, enabled => {
  if (enabled) refresh();
  else if (!loading.value) clearTimeout(timer);
});
watch(() => [rooms.currentId, shard.value, kind.value, search.value, page.value, pageSize.value], () => {
  clearTimeout(timer);
  abortActive();
  const ticket = ++generation;
  context++;
  selected.value = new Set();
  result.value = null;
  error.value = '';
  streaming.value = false;
  expectedCount.value = 0;
  loading.value = !!rooms.currentId && !!shard.value;
  timer = setTimeout(() => load(ticket), 250);
}, { immediate: true });

onUnmounted(() => {
  disposed = true;
  generation++;
  clearTimeout(timer);
  abortActive();
});

async function load(ticket) {
  if (ticket !== generation || !rooms.currentId || !shard.value) return;
  abortActive();
  const controller = new AbortController();
  activeController = controller;
  const progressive = !result.value;
  loading.value = true;
  if (progressive) {
    streaming.value = true;
    expectedCount.value = 0;
  }

  let redirected = false;
  let bufferedMeta = null;
  const bufferedEntries = [];

  try {
    await logApi.chatStream(rooms.currentId, shard.value, {
      kind: kind.value,
      search: search.value.trim(),
      page: page.value,
      limit: size.value,
      signal: controller.signal,
      onMeta(meta) {
        if (ticket !== generation || redirected) return;
        const lastPage = Math.max(1, Math.ceil((meta.total || 0) / size.value));
        if (page.value > lastPage) {
          redirected = true;
          controller.abort();
          page.value = lastPage;
          return;
        }
        bufferedMeta = meta;
        if (progressive) {
          expectedCount.value = meta.count || 0;
          result.value = {
            entries: [],
            exists: meta.exists,
            total: meta.total || 0,
            page: meta.page || page.value,
            pageSize: meta.pageSize || size.value,
          };
        }
      },
      onEntry(entry) {
        if (ticket !== generation || redirected) return;
        if (progressive) {
          if (!result.value) {
            result.value = { entries: [entry], exists: true, total: 1, page: page.value, pageSize: size.value };
          } else {
            result.value.entries.push(entry);
          }
        } else {
          bufferedEntries.push(entry);
        }
      },
    });
    if (ticket !== generation || redirected) return;
    if (!progressive && bufferedMeta) {
      result.value = {
        entries: bufferedEntries,
        exists: bufferedMeta.exists,
        total: bufferedMeta.total || 0,
        page: bufferedMeta.page || page.value,
        pageSize: bufferedMeta.pageSize || size.value,
      };
    }
    error.value = '';
    const visible = new Set((result.value?.entries || []).map(entry => entry.id));
    for (const id of [...selected.value]) {
      if (!visible.has(id)) selected.value.delete(id);
    }
  } catch (err) {
    if (err?.name === 'AbortError' || ticket !== generation || redirected) return;
    error.value = err.message;
  } finally {
    if (activeController === controller) activeController = null;
    if (ticket === generation) {
      loading.value = false;
      streaming.value = false;
      expectedCount.value = 0;
      if (!disposed && !redirected && playerLogRefresh.value) timer = setTimeout(() => load(ticket), 5000);
    }
  }
}

function refresh() {
  clearTimeout(timer);
  abortActive();
  const ticket = ++generation;
  error.value = '';
  load(ticket);
}

function resetFilters() {
  kind.value = 'all';
  search.value = '';
}

function selectEntry(id, checked) {
  if (checked) selected.value.add(id);
  else selected.value.delete(id);
}

async function removeEntries(ids) {
  if (deleting.value || blocking.value || loading.value || !rooms.currentId || !shard.value) return;
  const room = rooms.current;
  const sourceShard = shard.value;
  const sourceContext = context;
  deleting.value = true;
  try {
    if (!await confirmAction({
      title: `删除 ${ids.length} 条玩家日志？`,
      description: `房间「${room.name}」分片「${sourceShard}」。只删面板历史、不删游戏原始日志。`,
      destructive: true,
    })) return;
    if (disposed || sourceContext !== context) return;
    await logApi.removeChat(room.id, sourceShard, ids);
    if (disposed || sourceContext !== context) return;
    selected.value = new Set();
    refresh();
  } catch (err) {
    if (!disposed && sourceContext === context) toast(err.message, 'err');
  } finally {
    deleting.value = false;
  }
}

async function copyId(id, label = 'Klei ID') {
  if (!window.navigator.clipboard) {
    toast('当前连接不支持剪贴板访问，请选中 ID 手动复制', 'err');
    return;
  }
  await guard(() => window.navigator.clipboard.writeText(id), { success: `${label} 已复制` });
}

async function blockPlayer(entry) {
  if (blocking.value) return;
  const room = rooms.current;
  blocking.value = entry.userid;
  try {
    if (!await confirmAction({
      title: `封禁「${entry.name || entry.userid}」？`,
      description: `${entry.userid}（房间「${room.name}」）。下次重启房间后生效，不会立即踢出在线玩家。`,
      destructive: true,
    })) return;
    if (disposed || rooms.currentId !== room.id) return;
    const { ok } = await guard(() => playerListApi.add(room.id, 'blocklist', entry.userid));
    if (ok) {
      if (!disposed && rooms.currentId === room.id) blocked.value.add(entry.userid);
      toast(`已封禁，重启「${room.name}」后生效`);
    }
  } finally {
    blocking.value = '';
  }
}
</script>

<template>
  <UiCard flush fill role="region" :aria-label="activeSection === 'logs' ? '聊天与进出记录' : '管理员与准入名单'" class="min-w-0" :aria-busy="activeSection === 'logs' && loading">
    <div class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b bg-muted/20 px-[var(--panel-padding)] py-2">
      <div role="tablist" aria-label="玩家管理标签" class="inline-flex items-center gap-1 rounded-lg bg-muted/70 p-0.5">
        <button
          v-for="item in sections"
          :key="item.key"
          type="button"
          role="tab"
          :aria-selected="activeSection === item.key"
          class="rounded-md px-3 py-1 text-xs font-medium transition-colors"
          :class="activeSection === item.key ? 'bg-background text-foreground shadow-xs' : 'text-muted-foreground hover:text-foreground'"
          @click="activeSection = item.key"
        >
          {{ item.label }}
        </button>
      </div>
      <label v-if="activeSection === 'logs'" for="player-log-refresh" class="flex shrink-0 cursor-pointer select-none items-center gap-2 text-xs text-muted-foreground">
        <Switch id="player-log-refresh" v-model="playerLogRefresh" aria-label="自动刷新玩家日志" />
        <span>自动刷新</span>
      </label>
      <span v-else-if="rooms.current" class="hidden truncate text-xs text-muted-foreground sm:inline">
        当前房间：{{ rooms.current.name }}（名单改动需重启生效）
      </span>
    </div>

    <template v-if="activeSection === 'logs'">
      <UiToolbar aria-label="玩家日志工具栏" layout-class="flex-nowrap">
        <form class="flex min-w-0 flex-1 items-center gap-2" @submit.prevent="refresh">
          <Select v-model="shard" :disabled="!shards.length">
            <SelectTrigger id="chat_shard" aria-label="记录分片" class="w-20 min-w-0 shrink-0 bg-background sm:w-24"><SelectValue placeholder="选择分片" /></SelectTrigger>
            <SelectContent position="popper" align="start">
              <SelectGroup>
                <SelectItem v-for="item in shards" :key="item.name" :value="item.name">{{ roomShardLabel(rooms.current, item) }}{{ item.disabled ? '（停用）' : '' }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <UiToolbarFilters v-slot="{ compact }" label="日志筛选" :active="kind !== 'all'">
            <Select v-model="kind">
              <SelectTrigger id="chat_kind" aria-label="记录类型" :class="compact ? 'w-full' : 'w-28'" class="min-w-0 shrink-0 bg-background"><SelectValue /></SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectGroup><SelectItem v-for="(label, key) in labels" :key="key" :value="key">{{ label }}</SelectItem></SelectGroup>
              </SelectContent>
            </Select>
          </UiToolbarFilters>
          <div class="relative min-w-0 flex-1 basis-32">
            <Input id="chat_search" v-model="search" aria-label="搜索历史记录" class="bg-background pr-10" placeholder="昵称、ID、IP、角色或内容" maxlength="200" />
            <IconAction type="button" variant="ghost" class="absolute end-0 top-1/2 -translate-y-1/2" icon="x" label="重置筛选" :disabled="kind === 'all' && !search" @click="resetFilters" />
          </div>
        </form>
        <template #actions>
          <IconAction :variant="selected.size ? 'destructive' : 'ghost'" show-label="lg" icon="trash" :label="`删除所选（${selected.size}）`" :disabled="!selected.size || deleting || !!blocking || loading" @click="removeEntries([...selected])" />
          <IconAction variant="outline" icon="refresh" :icon-class="loading ? 'animate-spin motion-reduce:animate-none' : ''" label="刷新记录" :disabled="loading || !rooms.currentId || !shard" @click="refresh" />
        </template>
      </UiToolbar>

      <div data-slot="page-scroll" class="flex min-h-0 min-w-0 flex-1 flex-col overflow-y-auto overscroll-contain p-[var(--panel-padding)]" tabindex="0" role="region" aria-label="玩家日志列表">
        <p v-if="error" role="alert" class="break-words text-sm text-destructive">读取失败：{{ error }}</p>
        <div v-else-if="loading && !result" role="status" aria-label="加载中" class="log-table-frame flex min-w-0 flex-auto shrink-0 flex-col overflow-hidden rounded-lg border">
          <Table class="min-w-[820px] table-fixed">
            <TableHeader class="bg-muted/50">
              <TableRow>
                <TableHead scope="col" class="w-10 px-0"><div class="flex items-center justify-center"><Skeleton class="size-4 rounded-[4px]" /></div></TableHead>
                <TableHead scope="col" class="w-28">记录时间</TableHead>
                <TableHead scope="col" class="w-56">玩家</TableHead>
                <TableHead scope="col" class="w-44">网络标识</TableHead>
                <TableHead scope="col" class="w-20">事件</TableHead>
                <TableHead scope="col">内容</TableHead>
                <TableHead scope="col" class="w-20 pr-4 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="i in initialSkeletonRows" :key="i">
                <TableCell class="px-0"><div class="flex items-center justify-center"><Skeleton class="size-4 rounded-[4px]" /></div></TableCell>
                <TableCell class="py-2"><div class="inline-flex h-8 flex-col justify-center gap-1"><Skeleton class="h-3 w-[70px] rounded" /><Skeleton class="h-3 w-14 rounded" /></div></TableCell>
                <TableCell class="py-2">
                  <div class="flex min-h-[42px] items-center gap-2">
                    <Skeleton class="size-8 shrink-0 rounded-md" />
                    <div class="grid flex-1 gap-0.5">
                      <div class="flex h-5 items-center"><Skeleton class="h-3.5 w-16 rounded" /></div>
                      <div class="flex h-5 items-center"><Skeleton class="h-3 w-24 rounded" /></div>
                    </div>
                  </div>
                </TableCell>
                <TableCell class="py-2">
                  <div class="grid gap-0.5">
                    <div class="flex h-5 items-center"><Skeleton class="h-3 w-[124px] rounded" /></div>
                    <div class="flex h-4 items-center"><Skeleton class="h-3 w-16 rounded" /></div>
                  </div>
                </TableCell>
                <TableCell><Skeleton class="h-5 w-[42px] rounded-full" /></TableCell>
                <TableCell><Skeleton class="h-3.5 max-w-full rounded" :class="SKELETON_MSG_WIDTHS[(i - 1) % SKELETON_MSG_WIDTHS.length]" /></TableCell>
                <TableCell class="pr-4 text-right"><div class="flex justify-end"><Skeleton class="size-7 rounded-lg" /></div></TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
        <UiEmpty v-else-if="!rooms.current">
          <span>请先在「房间管理」页新建一个房间</span>
          <Button size="sm" variant="outline" class="mt-2" @click="ui.openRoomEditor()">前往新建房间</Button>
        </UiEmpty>
        <UiEmpty v-else-if="!result?.exists" text="该分片还没有聊天日志，开服并产生聊天或进出记录后显示" />
        <UiEmpty v-else-if="!entries.length && pendingSkeletonCount === 0" text="历史日志中没有匹配的记录" />
        <div v-else class="log-table-frame flex min-w-0 flex-auto shrink-0 flex-col overflow-hidden rounded-lg border">
          <Table class="min-w-[820px] table-fixed" role="region" aria-label="日志表格，可横向滚动" tabindex="0">
            <caption class="sr-only">游戏聊天记录</caption>
            <TableHeader class="bg-muted/50">
              <TableRow>
                <TableHead scope="col" class="w-10 px-0">
                  <div class="flex items-center justify-center">
                    <Checkbox aria-label="选择本页日志" :disabled="deleting || loading" :model-value="entries.length > 0 && selected.size === entries.length ? true : selected.size ? 'indeterminate' : false" @update:model-value="selected = $event ? new Set(entries.map(entry => entry.id)) : new Set()" />
                  </div>
                </TableHead>
                <TableHead scope="col" class="w-28">记录时间</TableHead>
                <TableHead scope="col" class="w-56">玩家</TableHead>
                <TableHead scope="col" class="w-44">网络标识</TableHead>
                <TableHead scope="col" class="w-20">事件</TableHead>
                <TableHead scope="col">内容</TableHead>
                <TableHead scope="col" class="w-20 pr-4 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="entry in entries" :key="entry.id">
                <TableCell class="px-0">
                  <div class="flex items-center justify-center">
                    <Checkbox :aria-label="`选择日志 ${entry.id}`" :disabled="deleting || loading" :model-value="selected.has(entry.id)" @update:model-value="selectEntry(entry.id, $event)" />
                  </div>
                </TableCell>
                <TableCell class="whitespace-normal py-2 font-mono text-xs text-muted-foreground tabular-nums">
                  <time v-if="entry.recordedAt" :datetime="entry.recordedAt" class="log-timestamp inline-flex flex-col whitespace-nowrap leading-4">
                    <span v-for="(part, index) in recordedTimeParts(entry.recordedAt)" :key="index">{{ part }}</span>
                  </time>
                  <span v-else>运行时长 {{ entry.time }}</span>
                </TableCell>
                <TableCell class="py-2">
                  <div class="flex min-h-[42px] min-w-0 items-center gap-2">
                    <Tooltip v-if="entry.prefab">
                      <TooltipTrigger as-child>
                        <img
                          :src="`/character-avatars/${Object.hasOwn(CHARACTERS, entry.prefab) ? entry.prefab : 'unknown'}.png`"
                          :alt="CHARACTERS[entry.prefab] || entry.prefab"
                          width="32" height="32" tabindex="0" class="size-8 shrink-0 object-contain"
                        />
                      </TooltipTrigger>
                      <TooltipContent>{{ CHARACTERS[entry.prefab] || entry.prefab }}</TooltipContent>
                    </Tooltip>
                    <div class="grid min-w-0 flex-1 gap-0.5">
                      <span class="truncate font-medium">{{ entry.name || '—' }}</span>
                      <div v-if="entry.userid" class="flex min-w-0 items-center gap-1">
                        <span class="min-w-0 truncate font-mono text-[11px] text-muted-foreground">{{ entry.userid }}</span>
                        <Tooltip>
                          <TooltipTrigger as-child>
                            <Button variant="ghost" size="icon-sm" class="size-5 shrink-0" :aria-label="`复制 Klei ID ${entry.userid}`" @click="copyId(entry.userid)">
                              <PhCopy class="size-3" aria-hidden="true" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>复制 Klei ID</TooltipContent>
                        </Tooltip>
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell class="py-2">
                  <div v-if="entry.steamid || entry.ip" class="grid min-w-0 gap-0.5">
                    <div v-if="entry.steamid" class="flex min-w-0 items-center gap-1">
                      <a v-if="/^\d+$/.test(entry.steamid)" :href="`https://steamcommunity.com/profiles/${entry.steamid}`" target="_blank" rel="noopener noreferrer" class="truncate font-mono text-[11px] text-emerald-700 underline underline-offset-2 dark:text-emerald-300 hover:text-foreground">{{ entry.steamid }}</a>
                      <span v-else class="truncate font-mono text-[11px]">{{ entry.steamid }}</span>
                      <Button variant="ghost" size="icon-sm" class="size-5 shrink-0" :aria-label="`复制 Steam ID ${entry.steamid}`" @click="copyId(entry.steamid, 'Steam ID')"><PhCopy class="size-3" aria-hidden="true" /></Button>
                    </div>
                    <span v-if="entry.ip" class="truncate font-mono text-[11px] text-muted-foreground">{{ entry.ip }}</span>
                  </div>
                  <span v-else class="text-muted-foreground">—</span>
                </TableCell>
                <TableCell><Badge variant="outline" :class="eventClasses[entry.kind]">{{ labels[entry.kind] || entry.kind }}</Badge></TableCell>
                <TableCell class="whitespace-pre-wrap break-words [overflow-wrap:anywhere] leading-5">{{ entry.message }}</TableCell>
                <TableCell class="pr-4 text-right">
                  <IconAction
                    v-if="/^KU_[A-Za-z0-9_-]{1,32}$/.test(entry.userid || '')" variant="destructive" icon="ban"
                    :label="blocked.has(entry.userid) ? '已封禁' : blocking === entry.userid ? '处理中…' : '封禁'"
                    :disabled="!!blocking || deleting || blocked.has(entry.userid)"
                    @click="blockPlayer(entry)"
                  />
                </TableCell>
              </TableRow>
              <TableRow v-for="n in pendingSkeletonCount" :key="`stream-skeleton-${n}`" role="status" aria-label="继续加载日志">
                <TableCell class="px-0"><div class="flex items-center justify-center"><Skeleton class="size-4 rounded-[4px]" /></div></TableCell>
                <TableCell class="py-2"><div class="inline-flex h-8 flex-col justify-center gap-1"><Skeleton class="h-3 w-[70px] rounded" /><Skeleton class="h-3 w-14 rounded" /></div></TableCell>
                <TableCell class="py-2">
                  <div class="flex min-h-[42px] items-center gap-2">
                    <Skeleton class="size-8 shrink-0 rounded-md" />
                    <div class="grid flex-1 gap-0.5">
                      <div class="flex h-5 items-center"><Skeleton class="h-3.5 w-16 rounded" /></div>
                      <div class="flex h-5 items-center"><Skeleton class="h-3 w-24 rounded" /></div>
                    </div>
                  </div>
                </TableCell>
                <TableCell class="py-2">
                  <div class="grid gap-0.5">
                    <div class="flex h-5 items-center"><Skeleton class="h-3 w-[124px] rounded" /></div>
                    <div class="flex h-4 items-center"><Skeleton class="h-3 w-16 rounded" /></div>
                  </div>
                </TableCell>
                <TableCell><Skeleton class="h-5 w-[42px] rounded-full" /></TableCell>
                <TableCell><Skeleton class="h-3.5 max-w-full rounded" :class="SKELETON_MSG_WIDTHS[(entries.length + n - 1) % SKELETON_MSG_WIDTHS.length]" /></TableCell>
                <TableCell class="pr-4 text-right"><div class="flex justify-end"><Skeleton class="size-7 rounded-lg" /></div></TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      </div>

      <nav
        v-if="rooms.current"
        aria-label="日志分页"
        class="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t bg-card px-[var(--panel-padding)] py-3 text-xs"
      >
        <p v-if="loading && !result" class="flex h-4 items-center">
          <Skeleton class="h-3.5 w-28 rounded" />
        </p>
        <p v-else-if="total > 0" class="text-muted-foreground tabular-nums">
          第 {{ start + 1 }}–{{ Math.min(start + size, total) }} 条，共 {{ total }} 条
        </p>
        <p v-else class="text-muted-foreground tabular-nums">共 0 条</p>
        <div class="flex flex-wrap items-center gap-2">
          <Select v-model="pageSize" :disabled="loading && !result">
            <SelectTrigger class="w-28" aria-label="每页日志条数"><SelectValue /></SelectTrigger>
            <SelectContent position="popper" align="end">
              <SelectGroup><SelectItem v-for="count in [10, 15, 20, 50, 100]" :key="count" :value="String(count)">{{ count }} 条 / 页</SelectItem></SelectGroup>
            </SelectContent>
          </Select>
          <Button variant="outline" size="icon-sm" aria-label="上一页" :disabled="loading || page === 1" @click="page--">
            <PhCaretLeft aria-hidden="true" />
          </Button>
          <span class="min-w-16 text-center tabular-nums">第 {{ page }} / {{ pageCount }} 页</span>
          <Button variant="outline" size="icon-sm" aria-label="下一页" :disabled="loading || page >= pageCount" @click="page++">
            <PhCaretRight aria-hidden="true" />
          </Button>
        </div>
      </nav>
    </template>

    <div
      v-else
      data-slot="page-scroll"
      class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-[var(--panel-padding)]"
      tabindex="0"
      role="region"
      aria-label="管理员与准入名单"
    >
      <UiEmpty v-if="!rooms.current">
        <span>请先在「房间管理」页新建一个房间</span>
        <Button size="sm" variant="outline" class="mt-2" @click="ui.openRoomEditor()">前往新建房间</Button>
      </UiEmpty>
      <div v-else class="grid items-start gap-4 lg:grid-cols-2">
        <RoomAdmins :key="rooms.current.id" :room="rooms.current" />
        <RoomPlayerLists :key="`lists-${rooms.current.id}`" :room="rooms.current" />
      </div>
    </div>
  </UiCard>
</template>

<style scoped>
.log-table-frame :deep([data-slot="table-container"]) {
  flex: 1 0 auto;
}

.log-timestamp {
  font-family: "Cascadia Code", Consolas, "Liberation Mono", monospace;
  font-variant-ligatures: none;
}
</style>
