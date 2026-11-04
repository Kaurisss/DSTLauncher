<script setup>
import { computed, onUnmounted, ref, watch } from 'vue';
import { PhCaretLeft, PhCaretRight, PhCopy } from '@phosphor-icons/vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiLoading from '@/components/ui/UiLoading.vue';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useRoomsStore } from '@/stores/rooms.js';
import { logApi, playerListApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { CHARACTERS, roomShardLabel } from '@/lib/format.js';
import { guard, toast } from '@/lib/toast.js';

const rooms = useRoomsStore();
const shard = ref('');
const kind = ref('all');
const search = ref('');
const result = ref(null);
const error = ref('');
const loading = ref(false);
const autoRefresh = ref(true);
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
watch(autoRefresh, enabled => {
  if (enabled) refresh();
  else if (!loading.value) clearTimeout(timer);
});
watch(() => [rooms.currentId, shard.value, kind.value, search.value, page.value, pageSize.value], () => {
  clearTimeout(timer);
  const ticket = ++generation;
  context++;
  selected.value = new Set();
  result.value = null;
  error.value = '';
  loading.value = !!rooms.currentId && !!shard.value;
  timer = setTimeout(() => load(ticket), 250);
}, { immediate: true });

onUnmounted(() => {
  disposed = true;
  generation++;
  clearTimeout(timer);
});

async function load(ticket) {
  if (ticket !== generation || !rooms.currentId || !shard.value) return;
  loading.value = true;
  try {
    const data = await logApi.chat(rooms.currentId, shard.value, kind.value, search.value.trim(), page.value, size.value);
    if (ticket !== generation) return;
    const lastPage = Math.max(1, Math.ceil(data.total / size.value));
    if (page.value > lastPage) {
      page.value = lastPage;
      return;
    }
    result.value = data;
    selected.value = new Set([...selected.value].filter(id => data.entries.some(entry => entry.id === id)));
    error.value = '';
  } catch (err) {
    if (ticket !== generation) return;
    result.value = null;
    error.value = err.message;
  } finally {
    if (ticket === generation) {
      loading.value = false;
      if (autoRefresh.value) timer = setTimeout(() => load(ticket), 5000);
    }
  }
}

function refresh() {
  clearTimeout(timer);
  load(++generation);
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
      description: `房间「${room.name}」· 分片「${sourceShard}」。只删面板历史、不删游戏原始日志。`,
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
      description: `${entry.userid} · 房间「${room.name}」。下次重启房间后生效，不会立即踢出在线玩家。`,
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
  <UiCard flush fill role="region" aria-label="聊天与进出记录" class="min-w-0" :aria-busy="loading">
    <UiToolbar aria-label="玩家日志工具栏" leading-class="min-w-full xl:min-w-0">
      <form class="flex min-w-0 flex-1 flex-wrap items-center gap-2 sm:flex-nowrap" @submit.prevent="refresh">
        <Select v-model="shard" :disabled="!shards.length">
          <SelectTrigger id="chat_shard" aria-label="记录分片" class="w-32 min-w-0 shrink-0 bg-background"><SelectValue placeholder="选择分片" /></SelectTrigger>
          <SelectContent position="popper" align="start">
            <SelectGroup>
              <SelectItem v-for="item in shards" :key="item.name" :value="item.name">{{ roomShardLabel(rooms.current, item) }}{{ item.disabled ? '（停用）' : '' }}</SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select v-model="kind">
          <SelectTrigger id="chat_kind" aria-label="记录类型" class="w-28 min-w-0 shrink-0 bg-background"><SelectValue /></SelectTrigger>
          <SelectContent position="popper" align="start">
            <SelectGroup><SelectItem v-for="(label, key) in labels" :key="key" :value="key">{{ label }}</SelectItem></SelectGroup>
          </SelectContent>
        </Select>
        <Input id="chat_search" v-model="search" aria-label="搜索历史记录" class="min-w-24 max-w-80 flex-1 bg-background" placeholder="昵称、ID、IP、角色或内容" maxlength="200" />
        <IconAction type="button" variant="ghost" icon="x" label="重置筛选" :disabled="kind === 'all' && !search" @click="resetFilters" />
      </form>
      <template #actions>
        <IconAction :variant="selected.size ? 'destructive' : 'ghost'" icon="trash" :label="`删除所选（${selected.size}）`" :disabled="!selected.size || deleting || !!blocking || loading" @click="removeEntries([...selected])" />
        <div class="flex items-center gap-2 whitespace-nowrap text-xs text-muted-foreground">
          <Switch id="player-log-refresh" v-model="autoRefresh" aria-label="自动刷新玩家日志" />
          <label for="player-log-refresh" class="cursor-pointer">自动刷新</label>
        </div>
        <IconAction variant="outline" icon="refresh" :icon-class="loading ? 'animate-spin motion-reduce:animate-none' : ''" label="刷新记录" :disabled="loading || !rooms.currentId || !shard" @click="refresh" />
      </template>
    </UiToolbar>

    <div data-slot="page-scroll" class="min-h-0 min-w-0 flex-1 space-y-4 overflow-y-auto overscroll-contain p-3" tabindex="0" role="region" aria-label="玩家日志列表">
      <p v-if="error" role="alert" class="break-words text-sm text-destructive">读取失败：{{ error }}</p>
      <UiLoading v-else-if="loading && !result" />
      <UiEmpty v-else-if="!rooms.current" icon="users" text="请先选择房间" />
      <UiEmpty v-else-if="!result?.exists" icon="inbox" text="该分片还没有聊天日志，开服并产生聊天或进出记录后显示" />
      <UiEmpty v-else-if="!entries.length" icon="inbox" text="历史日志中没有匹配的记录" />
      <template v-else>
        <div class="min-w-0 overflow-hidden rounded-lg border">
          <Table class="min-w-[1440px] table-fixed" role="region" aria-label="日志表格，可横向滚动" tabindex="0">
            <caption class="sr-only">游戏聊天记录</caption>
            <TableHeader class="bg-muted/50">
              <TableRow>
                <TableHead scope="col" class="w-10 px-0">
                  <div class="flex items-center justify-center">
                    <Checkbox aria-label="选择本页日志" :disabled="deleting || loading" :model-value="selected.size === entries.length ? true : selected.size ? 'indeterminate' : false" @update:model-value="selected = $event ? new Set(entries.map(entry => entry.id)) : new Set()" />
                  </div>
                </TableHead>
                <TableHead scope="col" class="w-28">记录时间</TableHead>
                <TableHead scope="col" class="w-32">玩家</TableHead>
                <TableHead scope="col" class="w-40">Klei ID</TableHead>
                <TableHead scope="col" class="w-16">角色</TableHead>
                <TableHead scope="col" class="w-48">Steam ID</TableHead>
                <TableHead scope="col" class="w-32">IP</TableHead>
                <TableHead scope="col" class="w-20">事件</TableHead>
                <TableHead scope="col">内容</TableHead>
                <TableHead scope="col" class="w-28 pr-4 text-right">操作</TableHead>
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
                <TableCell class="whitespace-normal break-words [overflow-wrap:anywhere] font-medium">{{ entry.name || '—' }}</TableCell>
                <TableCell>
                  <div v-if="entry.userid" class="flex min-w-0 items-center gap-1">
                    <span class="min-w-0 break-all whitespace-normal font-mono text-xs">{{ entry.userid }}</span>
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <Button variant="ghost" size="icon-sm" class="shrink-0" :aria-label="`复制 Klei ID ${entry.userid}`" @click="copyId(entry.userid)">
                          <PhCopy aria-hidden="true" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>复制 Klei ID</TooltipContent>
                    </Tooltip>
                  </div>
                  <span v-else class="text-muted-foreground">—</span>
                </TableCell>
                <TableCell>
                  <Tooltip v-if="entry.prefab">
                    <TooltipTrigger as-child>
                      <img
                        :src="`/character-avatars/${Object.hasOwn(CHARACTERS, entry.prefab) ? entry.prefab : 'unknown'}.png`"
                        :alt="CHARACTERS[entry.prefab] || entry.prefab"
                        width="36" height="36" tabindex="0" class="size-9 object-contain"
                      />
                    </TooltipTrigger>
                    <TooltipContent>{{ CHARACTERS[entry.prefab] || entry.prefab }}</TooltipContent>
                  </Tooltip>
                  <span v-else class="text-muted-foreground">—</span>
                </TableCell>
                <TableCell>
                  <div v-if="entry.steamid" class="flex min-w-0 items-center gap-1">
                    <a v-if="/^\d+$/.test(entry.steamid)" :href="`https://steamcommunity.com/profiles/${entry.steamid}`" target="_blank" rel="noopener noreferrer" class="break-all whitespace-normal font-mono text-xs text-emerald-700 underline underline-offset-2 dark:text-emerald-300 hover:text-foreground">{{ entry.steamid }}</a>
                    <span v-else class="break-all whitespace-normal font-mono text-xs">{{ entry.steamid }}</span>
                    <Button variant="ghost" size="icon-sm" class="shrink-0" :aria-label="`复制 Steam ID ${entry.steamid}`" @click="copyId(entry.steamid, 'Steam ID')"><PhCopy aria-hidden="true" /></Button>
                  </div>
                  <span v-else class="text-muted-foreground">—</span>
                </TableCell>
                <TableCell class="break-all whitespace-normal font-mono text-xs">{{ entry.ip || '—' }}</TableCell>
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
            </TableBody>
          </Table>
        </div>
        <nav aria-label="日志分页" class="flex flex-wrap items-center justify-between gap-3 text-xs">
          <p class="text-muted-foreground tabular-nums">第 {{ start + 1 }}–{{ Math.min(start + size, total) }} 条，共 {{ total }} 条</p>
          <div class="flex flex-wrap items-center gap-2">
            <Select v-model="pageSize">
              <SelectTrigger class="w-28" aria-label="每页日志条数"><SelectValue /></SelectTrigger>
              <SelectContent position="popper" align="end">
                <SelectGroup><SelectItem v-for="count in [10, 20, 50, 100]" :key="count" :value="String(count)">{{ count }} 条 / 页</SelectItem></SelectGroup>
              </SelectContent>
            </Select>
            <Button variant="outline" size="icon-sm" aria-label="上一页" :disabled="loading || page === 1" @click="page--">
              <PhCaretLeft aria-hidden="true" />
            </Button>
            <span class="min-w-16 text-center tabular-nums">第 {{ page }} / {{ pageCount }} 页</span>
            <Button variant="outline" size="icon-sm" aria-label="下一页" :disabled="loading || page === pageCount" @click="page++">
              <PhCaretRight aria-hidden="true" />
            </Button>
          </div>
        </nav>
      </template>
    </div>
  </UiCard>
</template>

<style scoped>
.log-timestamp {
  font-family: "Cascadia Code", Consolas, "Liberation Mono", monospace;
  font-variant-ligatures: none;
}
</style>
