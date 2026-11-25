<script setup>
// 模组页。
//
// 模组实体的存放是跨房间共享的（-ugc_directory 指向同一份），
// 因此「更新模组」是全局动作：把所有房间启用模组的并集补齐到本地，
// 任意一个房间点效果都一样。服务器启动时带 -skip_update_server_mods，
// 只加载不下载，所以启动前必须先把模组更新到位。

import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import {
  DialogRoot, DialogPortal, DialogOverlay, DialogContent,
  DialogTitle, DialogDescription, DialogClose,
} from 'reka-ui';

import { PhMagnifyingGlass } from '@phosphor-icons/vue';
import Icon from '@/components/ui/Icon.vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiPill from '@/components/ui/UiPill.vue';
import UiToolbarFilters from '@/components/ui/UiToolbarFilters.vue';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import IconAction from '@/components/ui/IconAction.vue';
import { CodeBlock, CodeBlockActions, CodeBlockContent, CodeBlockCopyButton } from '@/components/ui/code-block';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import ModItem from '@/components/mods/ModItem.vue';
import WorkshopPanel from '@/components/mods/WorkshopPanel.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { useUiStore } from '@/stores/ui.js';
import { modApi, modUpdateApi, workshopApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';
import { shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const ui = useUiStore();
const shard = ref('');
const shards = computed(() => rooms.current?.shards || []);
const configOpen = ref(false);
const busyItems = ref([]);
function setItemBusy(id, busy) {
  busyItems.value = busy ? [...new Set([...busyItems.value, id])] : busyItems.value.filter(value => value !== id);
}
function labelOf(item) {
  return `${shardLabel(item.name, rooms.current?.locations?.[item.name] || item.worldType)}${item.disabled ? '（已停用）' : ''}`;
}

/** 服务器运行中冻结：模组改动重启后才生效 */
const frozen = computed(() => rooms.starting || rooms.current?.running
  || rooms.statuses.some(status => ['running', 'starting', 'stopping'].includes(status.state)));

const INITIAL_CARD_SLOTS = 8;
const mods = ref([]);
const loading = ref(false);
const streaming = ref(false);
const expectedTotal = ref(0);
const loadError = ref('');
const workshopDetails = ref({});
const workshopError = ref('');
const pendingDetails = reactive(new Set());
const queuedWorkshopIds = reactive(new Set());
const search = ref('');
const filter = ref('all');
const slotEpoch = ref(0);
const slotKeyByModId = reactive(new Map());
const enabledCount = computed(() => mods.value.filter(mod => mod.enabled).length);
const isInitialLoading = computed(() => !rooms.loaded || (loading.value && !mods.value.length));
const pendingSlotCount = computed(() => {
  if (!rooms.loaded) return INITIAL_CARD_SLOTS;
  if (loading.value && !mods.value.length) {
    return expectedTotal.value > 0 ? Math.min(expectedTotal.value, 12) : INITIAL_CARD_SLOTS;
  }
  if (!streaming.value) return 0;
  // 保留首屏卡片壳不闪退：已有条目继续就地填充，超出后仅预留少量尾卡壳
  const target = expectedTotal.value > 0
    ? Math.min(expectedTotal.value, Math.max(INITIAL_CARD_SLOTS, mods.value.length + 3))
    : Math.max(INITIAL_CARD_SLOTS, mods.value.length + 1);
  return Math.max(0, target - mods.value.length);
});
const visibleMods = computed(() => {
  const query = search.value.trim().toLowerCase();
  return displayMods.value.filter(mod => {
    if (query && !(mod.searchText || `${mod.name} ${mod.id}`).toLowerCase().includes(query)) return false;
    if (filter.value === 'enabled') return mod.enabled;
    if (filter.value === 'disabled') return !mod.enabled;
    if (filter.value === 'missing') return !mod.installed;
    if (filter.value === 'updates') return !!updateMap.value[mod.id];
    return true;
  });
});
const cardSlots = computed(() => {
  const epoch = slotEpoch.value;
  const rows = visibleMods.value.map(mod => ({
    key: slotKeyByModId.get(mod.id) || `mod:${epoch}:${mod.id}`,
    mod,
  }));
  // 仅在未加筛选时渲染尾部占位卡壳，避免搜索/筛选时出现空白占位卡
  if (!search.value.trim() && filter.value === 'all') {
    const start = mods.value.length;
    for (let i = 0; i < pendingSlotCount.value; i++) {
      rows.push({ key: `slot:${epoch}:${start + i}`, mod: null });
    }
  }
  return rows;
});
function isWorkshopPending(mod) {
  if (!mod || !/^workshop-\d+$/.test(mod.id)) return false;
  const id = mod.id.slice('workshop-'.length);
  return !workshopDetails.value[id] && (pendingDetails.has(id) || queuedWorkshopIds.has(id));
}
let loadRequest = 0;
let loadController = null;
let workshopBatch = [];
let workshopBatchTimer = null;

function flushWorkshopBatch() {
  clearTimeout(workshopBatchTimer);
  workshopBatchTimer = null;
  if (!workshopBatch.length) return;
  const batch = workshopBatch;
  workshopBatch = [];
  for (const mod of batch) {
    if (/^workshop-\d+$/.test(mod.id)) queuedWorkshopIds.delete(mod.id.slice('workshop-'.length));
  }
  loadWorkshopDetails(batch);
}

function queueWorkshopCheck(mod) {
  if (!mod || (mod.iconUrl && mod.name && mod.name !== mod.id)) return;
  if (/^workshop-\d+$/.test(mod.id)) {
    const wid = mod.id.slice('workshop-'.length);
    if (!workshopDetails.value[wid]) queuedWorkshopIds.add(wid);
  }
  workshopBatch.push(mod);
  if (workshopBatch.length >= 12) {
    flushWorkshopBatch();
  } else if (!workshopBatchTimer) {
    workshopBatchTimer = setTimeout(flushWorkshopBatch, 60);
  }
}

const hasCJK = s => /[\u4e00-\u9fff]/.test(s || '');

const displayMods = computed(() => mods.value.map(mod => {
  const detail = workshopDetails.value[mod.id.slice('workshop-'.length)];
  if (!detail || !mod.id.startsWith('workshop-')) return mod;
  const preferLocal = hasCJK(mod.name) && !hasCJK(detail.title);
  return {
    ...mod,
    name: (preferLocal ? mod.name : detail.title) || mod.name,
    searchText: `${mod.name} ${detail.title || ''} ${mod.id}`,
    previewUrl: detail.previewUrl || '',
  };
}));

// 工坊信息独立加载并在本页缓存；Steam 不可达时仍能管理本地模组。
async function loadWorkshopDetails(list) {
  const ids = list.filter(mod => /^workshop-\d+$/.test(mod.id))
    .map(mod => mod.id.slice('workshop-'.length))
    .filter(id => !workshopDetails.value[id] && !pendingDetails.has(id));
  if (!ids.length) return;
  ids.forEach(id => pendingDetails.add(id));
  try {
    const details = await workshopApi.details(ids.join(','));
    if (disposed) return;
    for (const id of ids) workshopDetails.value[id] = {};
    for (const detail of details || []) workshopDetails.value[detail.id] = detail;
    workshopError.value = '';
  } catch (err) {
    if (!disposed) workshopError.value = err.message;
  } finally {
    ids.forEach(id => pendingDetails.delete(id));
  }
}

/** 已启用但本地还没有的模组数量 */
const missingCount = computed(() =>
  mods.value.filter(m => m.enabled && !m.installed).length);

async function load() {
  const request = ++loadRequest;
  loadController?.abort();
  loadController = null;
  clearTimeout(workshopBatchTimer);
  workshopBatchTimer = null;
  workshopBatch = [];
  queuedWorkshopIds.clear();

  const room = rooms.current;
  const target = shard.value;
  if (!room || !target) {
    mods.value = [];
    loading.value = false;
    streaming.value = false;
    expectedTotal.value = 0;
    loadError.value = '';
    return;
  }

  const controller = new AbortController();
  loadController = controller;
  const progressive = mods.value.length === 0;
  const buffered = [];
  const epoch = slotEpoch.value;
  const isCurrent = () => !disposed && request === loadRequest && shard.value === target && rooms.currentId === room.id;

  loading.value = true;
  streaming.value = progressive;
  if (progressive) expectedTotal.value = 0;
  loadError.value = '';

  let completed = false;
  try {
    await modApi.listStream(room.id, target, {
      signal: controller.signal,
      onMeta(total) {
        if (!isCurrent()) return;
        if (progressive) expectedTotal.value = total;
      },
      onMod(mod) {
        if (!isCurrent()) return;
        if (progressive) {
          const idx = mods.value.length;
          if (!slotKeyByModId.has(mod.id)) slotKeyByModId.set(mod.id, `slot:${epoch}:${idx}`);
          mods.value.push(mod);
          queueWorkshopCheck(mod);
        } else {
          buffered.push(mod);
        }
      },
      onDone() {
        if (!isCurrent()) return;
        if (progressive) {
          flushWorkshopBatch();
        } else {
          buffered.forEach((mod, idx) => {
            if (!slotKeyByModId.has(mod.id)) slotKeyByModId.set(mod.id, `mod:${epoch}:${mod.id}:${idx}`);
          });
          mods.value = buffered;
          loadWorkshopDetails(buffered.filter(mod => !mod.iconUrl || !mod.name || mod.name === mod.id));
        }
      },
    });
    completed = true;
  } catch (err) {
    if (err?.name !== 'AbortError' && !disposed && request === loadRequest) {
      loadError.value = err.message;
    }
  } finally {
    if (loadController === controller) loadController = null;
    if (request === loadRequest) {
      if (completed) flushWorkshopBatch();
      loading.value = false;
      streaming.value = false;
    }
  }
}

/* ---------- 更新检查 ---------- */

const checking = ref(false);
let checkRequest = 0;
/** { checked, updates, missing }；null 表示还没查过 */
const checkResult = ref(null);

/** 有更新的模组，按 id 索引，供列表项打标 */
const updateMap = computed(() => {
  const map = {};
  for (const u of (checkResult.value && checkResult.value.updates) || []) {
    map[u.id] = u;
  }
  return map;
});

const updatesCount = computed(() => (checkResult.value?.updates || []).length);
const statusFilters = computed(() => {
  const missing = mods.value.filter(mod => !mod.installed).length;
  return [
    { value: 'all', label: '全部', count: mods.value.length },
    { value: 'enabled', label: '已启用', count: enabledCount.value },
    { value: 'disabled', label: '未启用', count: mods.value.length - enabledCount.value },
    ...(missing || filter.value === 'missing' ? [{ value: 'missing', label: '未下载', count: missing }] : []),
    ...(updatesCount.value || filter.value === 'updates' ? [{ value: 'updates', label: '可更新', count: updatesCount.value }] : []),
  ];
});

async function checkUpdates() {
  const room = rooms.current;
  if (!room || checking.value) return;
  checking.value = true;
  const target = shard.value;
  const request = ++checkRequest;
  try {
    const result = await modUpdateApi.check(room.id, target);
    if (!disposed && request === checkRequest && rooms.currentId === room.id && shard.value === target) checkResult.value = result;
    return result;
  } catch (err) {
    if (!disposed && request === checkRequest) toast('检查更新失败：' + err.message, 'err');
  } finally {
    if (request === checkRequest) checking.value = false;
  }
}

/* ---------- 模组预更新 ---------- */

const updating = ref(false);
const updateOpen = ref(false);
const updateLogButton = ref(null);
const addModButton = ref(null);
/** 创意工坊面板的展开状态 */
const workshopOpen = ref(false);
const progress = ref('');
const updateError = ref('');
const output = ref([]);
/** 排队等待下载的模组数量（后端串行消化，不并行） */
const queuedCount = ref(0);
/** 当前页面提交过的下载任务，避免重复加入同一会话。 */
const queuedIds = ref([]);
const pendingIds = ref([]);
const busyIds = computed(() => [...queuedIds.value, ...pendingIds.value]);
const queueing = ref(false);
const stopping = ref(false);
const pollError = ref('');
let pollTimer = null;
let polling = false;
let disposed = false;

function restoreUpdateFocus() {
  nextTick(() => updateLogButton.value?.focus());
}

function restoreWorkshopFocus() {
  nextTick(() => addModButton.value?.focus());
}

function stopPoll() {
  if (pollTimer) {
    clearTimeout(pollTimer);
    pollTimer = null;
  }
}

async function pollUpdate() {
  if (polling || disposed) return;
  polling = true;
  try {
    let st;
    try {
      st = await modUpdateApi.status();
    } catch (err) {
      if (!disposed) pollError.value = err.message;
      return;
    }
    if (disposed) return;
    pollError.value = '';
    const wasUpdating = updating.value;
    updating.value = !!st.running;
    progress.value = st.progress || '';
    updateError.value = st.error || '';
    output.value = st.output || [];
    queuedCount.value = st.queued || 0;

    if (!st.running) {
      stopPoll();
      queuedIds.value = [];
      if (!wasUpdating) return; // 初次查询空闲状态不代表刚完成更新
      // 引擎可能退出码为 0 却没下载成功，以本地文件清点结果为准。
      await load();
      checkResult.value = null;
      const missing = mods.value.filter(m => m.enabled && !m.installed).length;
      const nothingTodo = st.output && st.output.length <= 2;
      if (st.error) toast('模组更新失败：' + st.error, 'err');
      else if (missing > 0) {
        updateError.value = `仍有 ${missing} 个已启用模组未下载`;
        toast(`更新结束，但仍有 ${missing} 个模组未下载，请查看更新日志后重试`, 'warn');
      } else if (nothingTodo) {
        toast('全部模组均已在本地，无需更新');
      } else {
        toast('模组更新完成');
      }
    }
  } finally {
    polling = false;
  }
}

function startPoll() {
  if (disposed) return;
  stopPoll();
  pollTimer = setTimeout(async () => {
    await pollUpdate();
    if (!disposed && updating.value) startPoll();
  }, 2000);
}

// 检查与下载分开：确认后才修改跨房间共享的模组文件。
async function updateAll() {
  const result = checkResult.value;
  const target = shard.value;
  const roomId = rooms.currentId;
  if (!result) return;
  const todo = new Set();
  for (const u of result.updates || []) todo.add(u.id);
  for (const id of result.notInstalled || []) todo.add(id);

  if (!todo.size) {
    toast('已启用的模组都是最新版，无需更新');
    return;
  }

  if (!await confirmAction({
    title: `更新 ${todo.size} 个模组？`,
    description: '模组文件由所有房间及 Steam 客户端共享，请先退出游戏并暂停 Steam 的模组更新。',
    confirmText: '开始更新',
  })) return;

  if (target === shard.value && roomId === rooms.currentId) await enqueue([...todo]);
}

async function enqueue(ids) {
  const room = rooms.current;
  if (!room || frozen.value || queueing.value) return;
  queueing.value = true;
  let added = 0;
  try {
    for (const id of ids) {
      if (disposed || rooms.currentId !== room.id || frozen.value) break;
      if (queuedIds.value.includes(id)) continue;
      queuedIds.value = [...queuedIds.value, id];
      const { ok } = await guard(() => modUpdateApi.start(room.id, null, id));
      if (ok) added++;
      else queuedIds.value = queuedIds.value.filter(value => value !== id);
    }
    if (added && !disposed) {
      toast(`${added} 个模组已加入下载队列`);
      updating.value = true;
      startPoll();
      pollUpdate();
    }
  } finally {
    queueing.value = false;
  }
}

async function startOne(mod) {
  const id = /^\d+$/.test(mod.id) ? `workshop-${mod.id}` : mod.id;
  if (frozen.value || queueing.value || busyIds.value.includes(id)) return;
  pendingIds.value = [...pendingIds.value, id];
  try {
    if (mod.installed && !await confirmAction(
      `重新下载模组「${mod.name || id}」？\n\n`
      + '模组文件与其它房间及 Steam 客户端共享。将先备份本地文件，下载失败时恢复；请先退出游戏并暂停 Steam 的模组更新。',
    )) return;
    await enqueue([id]);
  } finally {
    pendingIds.value = pendingIds.value.filter(value => value !== id);
  }
}

function onToggled(id, enabled) {
  const mod = mods.value.find(item => item.id === id);
  if (mod) mod.enabled = enabled;
}

async function enableOne(id) {
  const room = rooms.current;
  const target = shard.value;
  if (!room || frozen.value || pendingIds.value.includes(id)) return;
  pendingIds.value = [...pendingIds.value, id];
  try {
    const { ok } = await guard(() => modApi.toggle(room.id, id, true, target), { success: '模组已启用' });
    if (ok && !disposed && room.id === rooms.currentId && shard.value === target) await load();
  } finally {
    pendingIds.value = pendingIds.value.filter(value => value !== id);
  }
}

async function stopUpdate() {
  if (stopping.value) return;
  stopping.value = true;
  try {
    await guard(() => modUpdateApi.stop(), { success: '已请求中止更新' });
  } finally {
    stopping.value = false;
  }
}

onMounted(async () => {
  if (disposed) return;
  // 面板重启后若上次更新还没结束，恢复轮询而不是丢掉进度
  await pollUpdate();
  if (updating.value) startPoll();
});
onUnmounted(() => {
  disposed = true;
  loadController?.abort();
  clearTimeout(workshopBatchTimer);
  stopPoll();
});
// 用 current 而不是 currentId 做触发：刷新页面时 currentId 从 localStorage
// 恢复、值不变，watch 不会触发，列表就一直空着；而 current 要等房间列表
// 加载完才从 null 变成房间对象，正好在数据就绪时拉一次模组列表
watch([() => rooms.current?.id, () => shards.value.map(item => item.name).join('\0')], ([id], [previousId] = []) => {
  if (id !== previousId || !shards.value.some(item => item.name === shard.value)) {
    shard.value = (shards.value.find(item => item.isMaster && !item.disabled)
      || shards.value.find(item => item.isMaster) || shards.value[0])?.name || '';
  }
}, { immediate: true });
watch([() => rooms.current, shard], () => {
  loadRequest++;
  checkRequest++;
  checking.value = false;
  checkResult.value = null;
  mods.value = [];
  slotEpoch.value++;
  slotKeyByModId.clear();
  configOpen.value = false;
  busyItems.value = [];
  load();
}, { immediate: true });
</script>

<template>
  <DialogRoot v-model:open="updateOpen">
    <UiCard flush fill class="mods-workspace">
      <div class="mods-toolbar z-10 flex shrink-0 items-center justify-between gap-3" role="region" aria-label="模组工具栏">
        <div class="mods-toolbar-left">
          <Select v-if="rooms.current" v-model="shard" :disabled="configOpen || busyItems.length > 0 || workshopOpen || queueing || pendingIds.length > 0">
            <SelectTrigger aria-label="模组分片" class="mods-shard w-20 shrink-0 sm:w-28">
              <SelectValue placeholder="选择分片" />
            </SelectTrigger>
            <SelectContent position="popper" align="start">
              <SelectGroup>
                <SelectItem v-for="item in shards" :key="item.name" :value="item.name">{{ labelOf(item) }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <UiToolbarFilters v-slot="{ compact }" label="模组筛选" :active="filter !== 'all'">
            <Select v-if="compact" v-model="filter">
              <SelectTrigger aria-label="模组状态" class="w-full"><SelectValue /></SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectGroup><SelectItem v-for="item in statusFilters" :key="item.value" :value="item.value">{{ item.label }} ({{ item.count }})</SelectItem></SelectGroup>
              </SelectContent>
            </Select>
            <div v-else class="seg mods-filters" role="group" aria-label="模组状态">
              <button v-for="item in statusFilters" :key="item.value" type="button" class="seg-btn" :class="{ 'is-active': filter === item.value }" :aria-pressed="filter === item.value" @click="filter = item.value">
                {{ item.label }} <span class="tabular-nums">({{ item.count }})</span>
              </button>
            </div>
          </UiToolbarFilters>
          <div class="mods-search">
            <PhMagnifyingGlass class="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            <Input v-model="search" class="w-full pl-8" aria-label="搜索模组" placeholder="搜索模组名称或 ID" />
          </div>
        </div>
        <div class="mods-toolbar-actions">
          <IconAction variant="outline" show-label="lg" icon="cloudCheck" :icon-class="checking ? 'animate-pulse motion-reduce:animate-none' : ''" :label="checking ? '检查中…' : '检查更新'" :disabled="!rooms.current || updating || checking || queueing" @click="checkUpdates" />
          <IconAction ref="addModButton" show-label="lg" icon="plus" label="添加模组" :disabled="!rooms.current" aria-haspopup="dialog" @click="workshopOpen = true" />
          <IconAction v-if="updating || output.length || updateError || pollError" ref="updateLogButton" variant="outline" show-label="lg" icon="fileCode" label="更新日志" aria-haspopup="dialog" :aria-expanded="updateOpen" @click="updateOpen = true" />
          <Separator orientation="vertical" class="mx-0.5 h-4 self-center" />
          <Tooltip>
            <TooltipTrigger as="span" :as-child="!(loading || !rooms.current)" class="inline-flex" :tabindex="loading || !rooms.current ? 0 : undefined">
              <Button variant="ghost" size="icon-sm" class="text-muted-foreground" :disabled="loading || !rooms.current" aria-label="刷新" @click="load"><Icon name="refresh" size="16" /></Button>
            </TooltipTrigger>
            <TooltipContent>刷新本地模组列表</TooltipContent>
          </Tooltip>
        </div>
      </div>

      <div data-slot="page-scroll" class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-[var(--panel-padding)]" tabindex="0" role="region" aria-label="模组列表" :aria-busy="loading">
        <UiEmpty v-if="rooms.loaded && !rooms.current">
          <span>请先在「房间管理」页新建一个房间</span>
          <Button size="sm" variant="outline" class="mt-2" @click="ui.openRoomEditor()">前往新建房间</Button>
        </UiEmpty>
        <UiEmpty v-else-if="loadError" :text="'读取模组失败：' + loadError" />

        <template v-else>
          <span v-if="isInitialLoading || streaming" class="sr-only" role="status">{{ isInitialLoading ? '正在加载模组…' : '正在继续加载模组…' }}</span>
          <div v-if="frozen" class="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm" role="status">
            <Icon name="info" size="16" />
            <span>服务器运行中，模组修改已冻结。</span>
            <Button size="sm" variant="outline" class="ml-auto h-7 px-2.5 text-xs" @click="ui.select('overview')">前往概览</Button>
          </div>
          <div v-else-if="missingCount" class="mb-4 flex flex-wrap items-center gap-3 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm">
            <Icon name="download" size="16" />
            <span>{{ missingCount }} 个已启用模组未下载</span>
            <IconAction class="ml-auto" variant="outline" show-label icon="download" label="下载缺失" :disabled="queueing || updating" @click="enqueue(mods.filter(mod => mod.enabled && !mod.installed).map(mod => mod.id))" />
          </div>

          <!-- 检查结果：明确指出是哪个模组旧了，客户端只会笼统提示「有旧版本 mod」 -->
          <div v-if="checkResult" class="mb-4 flex flex-wrap items-center gap-3 rounded-lg border bg-muted/30 p-3 text-sm" role="status">
            <span>{{ updatesCount ? `${updatesCount} 个模组有新版本` : '没有可更新的模组' }}</span>
            <IconAction v-if="updatesCount" class="ml-auto" show-label icon="download" label="更新全部" :disabled="frozen || updating || queueing" @click="updateAll" />
          </div>
          <p
            v-if="checkResult && (checkResult.missing || []).length"
            class="field-tip"
            style="margin-bottom: 16px"
          >
            工坊查不到以下模组（可能已下架或 ID 有误）：{{ (checkResult.missing || []).join('、') }}
          </p>

          <div v-if="updating || updateError || pollError" class="mb-4 flex flex-wrap items-center gap-3 rounded-lg border bg-muted/30 p-3 text-sm" role="status">
            <Icon :name="updateError || pollError ? 'alert' : 'download'" size="16" />
            <span class="min-w-0 flex-1 break-words">{{ pollError ? '无法读取更新进度：' + pollError : updateError || progress || '正在下载模组…' }}</span>
            <span v-if="updating && queuedCount" class="text-muted-foreground">排队 {{ queuedCount }} 个</span>
            <IconAction v-if="pollError" variant="outline" show-label icon="refresh" label="重试" @click="pollUpdate" />
            <IconAction variant="outline" show-label icon="activity" label="查看进度" @click="updateOpen = true" />
          </div>

          <Alert v-if="workshopError" class="mb-3">
            <Icon name="alert" size="16" />
            <AlertTitle>创意工坊信息暂未加载</AlertTitle>
            <AlertDescription class="break-words">
              {{ workshopError }}。当前显示本地名称或 ID，可点击「刷新」重试。
            </AlertDescription>
          </Alert>
          <template v-if="cardSlots.length">
            <ModItem
              v-for="(slot, index) in cardSlots"
              :key="slot.key"
              :mod="slot.mod"
              :shard="shard"
              :enter-index="index"
              :workshop-loading="isWorkshopPending(slot.mod)"
              :update-info="slot.mod ? (updateMap[slot.mod.id] || null) : null"
              :downloading="slot.mod ? busyIds.includes(slot.mod.id) : false"
              :updating="updating || queueing"
              @changed="load"
              @update="startOne"
              @toggled="enabled => slot.mod && onToggled(slot.mod.id, enabled)"
              @cover-needed="slot.mod && loadWorkshopDetails([slot.mod])"
              @config-open="configOpen = $event"
              @busy="slot.mod && setItemBusy(slot.mod.id, $event)"
            />
          </template>
          <div v-else class="grid justify-items-center gap-3 rounded-lg border border-dashed p-10">
            <UiEmpty :text="mods.length ? '没有匹配的模组' : '还没有模组'" />
            <IconAction v-if="mods.length" variant="outline" show-label icon="clean" label="清除筛选" @click="search = ''; filter = 'all'" />
            <IconAction v-else show-label icon="plus" label="添加模组" @click="workshopOpen = true" />
          </div>
        </template>
      </div>
    </UiCard>

    <WorkshopPanel
      v-model:open="workshopOpen"
      :mods="mods" :busy-ids="busyIds" :disabled="frozen || queueing"
      @download="startOne" @enable="enableOne" @closed="restoreWorkshopFocus"
    />

    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-50 bg-black/20" />
      <DialogContent class="mod-update-dialog" @close-auto-focus.prevent="restoreUpdateFocus">
        <div class="flex flex-wrap items-center gap-2">
          <DialogTitle class="text-lg font-semibold">模组更新</DialogTitle>
          <UiPill :tone="pollError ? 'warn' : updating ? 'info' : (updateError ? 'danger' : 'ok')" role="status">
            {{ pollError ? '进度暂不可用' : updating ? (progress || '进行中') : (updateError ? '失败' : '完成') }}
          </UiPill>
        </div>
        <DialogDescription class="sr-only">模组下载任务的进度与日志</DialogDescription>
        <p v-if="queuedCount" class="field-tip">队列中还有 {{ queuedCount }} 个待下载</p>
        <p v-if="updateError || pollError" class="text-sm text-destructive" role="alert">{{ pollError ? '无法读取更新进度：' + pollError : updateError }}</p>
        <CodeBlock size="sm">
          <CodeBlockActions><CodeBlockCopyButton :value="output.join('\n')" :disabled="!output.length" /></CodeBlockActions>
          <CodeBlockContent wrap class="mod-terminal" role="region" aria-label="模组更新输出">{{ output.join('\n') || (updating ? '正在等待更新输出…' : '暂无更新输出') }}</CodeBlockContent>
        </CodeBlock>
        <div class="flex justify-end gap-2">
          <IconAction v-if="pollError" variant="outline" show-label icon="refresh" label="重试" @click="pollUpdate" />
          <IconAction v-if="updating" variant="destructive" show-label icon="stop" :label="stopping ? '正在中止…' : '中止'" :disabled="stopping" @click="stopUpdate" />
          <DialogClose as-child>
            <IconAction variant="outline" show-label icon="x" label="关闭" />
          </DialogClose>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.mods-workspace { min-width: 0; container-type: inline-size; }
.mods-toolbar { padding: var(--panel-padding); border-radius: var(--radius-lg) var(--radius-lg) 0 0; background: var(--card); }
.mods-toolbar-left { display: flex; flex: 1; align-items: center; gap: 12px; min-width: 0; }
.mods-shard { width: 112px; min-width: 0; flex-shrink: 0; }
.mods-search { position: relative; flex: 1; min-width: 0; max-width: 256px; }
.mods-filters { flex-shrink: 0; border-radius: 9999px; }
.mods-filters .seg-btn { flex-shrink: 0; padding: 4px 8px; border-radius: 9999px; white-space: nowrap; font-size: 12px; }
.mods-toolbar-actions { display: flex; flex-shrink: 0; align-items: center; gap: 8px; margin-left: auto; }
@container (max-width: 640px) {
  .mods-toolbar-left { gap: 8px; }
  .mods-shard { width: 96px; }
  .mods-toolbar-actions { gap: 4px; }
}
.mod-update-dialog {
  position: fixed;
  top: 50%;
  left: 50%;
  z-index: 50;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  gap: 16px;
  width: min(880px, calc(100vw - 32px));
  max-height: calc(100dvh - 32px);
  padding: 20px;
  overflow: auto;
  overscroll-behavior: contain;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text);
  box-shadow: var(--shadow-lg);
}
.mod-terminal { min-height: 160px; height: min(50dvh, 420px); max-height: 55dvh; overscroll-behavior: contain; }
.mod-terminal:focus-visible { outline: 2px solid var(--ring); outline-offset: 2px; }
</style>
