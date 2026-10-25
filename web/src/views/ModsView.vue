<script setup>
// 模组页。
//
// 模组实体的存放是跨房间共享的（-ugc_directory 指向同一份），
// 因此「更新模组」是全局动作：把所有房间启用模组的并集补齐到本地，
// 任意一个房间点效果都一样。服务器启动时带 -skip_update_server_mods，
// 只加载不下载，所以启动前必须先把模组更新到位。

import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  DialogRoot, DialogPortal, DialogOverlay, DialogContent,
  DialogTitle, DialogDescription, DialogClose,
} from 'reka-ui';

import Icon from '@/components/ui/Icon.vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiPill from '@/components/ui/UiPill.vue';
import UiSkeleton from '@/components/ui/UiSkeleton.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select';
import ModItem from '@/components/mods/ModItem.vue';
import WorkshopPanel from '@/components/mods/WorkshopPanel.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { modApi, modUpdateApi, workshopApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

const rooms = useRoomsStore();

/** 服务器运行中冻结：模组改动重启后才生效 */
const frozen = computed(() => rooms.running);

const mods = ref([]);
const loading = ref(false);
const loadError = ref('');
const workshopDetails = ref({});
const workshopError = ref('');
const pendingDetails = new Set();
const search = ref('');
const filter = ref('all');
const enabledCount = computed(() => mods.value.filter(mod => mod.enabled).length);
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
let loadRequest = 0;

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
  const room = rooms.current;
  if (!room) {
    mods.value = [];
    loadError.value = '';
    return;
  }
  loading.value = true;
  loadError.value = '';
  try {
    const list = (await modApi.list(room.id)) || [];
    if (disposed || request !== loadRequest) return;
    mods.value = list;
    loadWorkshopDetails(list.filter(mod => !mod.iconUrl || !mod.name || mod.name === mod.id));
  } catch (err) {
    if (!disposed && request === loadRequest) loadError.value = err.message;
  } finally {
    if (request === loadRequest) loading.value = false;
  }
}

/* ---------- 更新检查 ---------- */

const checking = ref(false);
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

async function checkUpdates() {
  const room = rooms.current;
  if (!room || checking.value) return;
  checking.value = true;
  try {
    const result = await modUpdateApi.check(room.id);
    if (!disposed && rooms.currentId === room.id) checkResult.value = result;
    return result;
  } catch (err) {
    toast('检查更新失败：' + err.message, 'err');
  } finally {
    checking.value = false;
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
  nextTick(() => updateLogButton.value?.$el?.focus());
}

function restoreWorkshopFocus() {
  nextTick(() => addModButton.value?.$el?.focus());
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

  await enqueue([...todo]);
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
  if (!room || frozen.value || pendingIds.value.includes(id)) return;
  pendingIds.value = [...pendingIds.value, id];
  try {
    const { ok } = await guard(() => modApi.toggle(room.id, id, true), { success: '模组已启用' });
    if (ok && !disposed && room.id === rooms.currentId) await load();
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
  await load();
  if (disposed) return;
  // 面板重启后若上次更新还没结束，恢复轮询而不是丢掉进度
  await pollUpdate();
  if (updating.value) startPoll();
});
onUnmounted(() => {
  disposed = true;
  stopPoll();
});
// 用 current 而不是 currentId 做触发：刷新页面时 currentId 从 localStorage
// 恢复、值不变，watch 不会触发，列表就一直空着；而 current 要等房间列表
// 加载完才从 null 变成房间对象，正好在数据就绪时拉一次模组列表
watch(() => rooms.current, () => {
  checkResult.value = null;
  load();
});
</script>

<template>
  <DialogRoot v-model:open="updateOpen">
    <UiCard>
      <template #title>模组管理</template>
      <template #actions>
        <Button
          variant="outline" size="sm"
          :disabled="!rooms.current || updating || checking || queueing"
          @click="checkUpdates"
        >
          <Icon name="refresh" size="14" />{{ checking ? '检查中…' : '检查更新' }}
        </Button>
        <Button
          ref="addModButton"
          size="sm"
          :disabled="!rooms.current"
          aria-haspopup="dialog"
          @click="workshopOpen = true"
        >
          <Icon name="plus" size="14" />添加模组
        </Button>
        <Button
          v-if="updating || output.length || updateError || pollError"
          ref="updateLogButton"
          variant="outline" size="sm"
          aria-haspopup="dialog"
          :aria-expanded="updateOpen"
          @click="updateOpen = true"
        >
          更新日志
        </Button>
        <Button variant="ghost" size="icon-sm" :disabled="loading" aria-label="刷新" title="刷新模组列表" @click="load"><Icon name="refresh" size="16" /></Button>
      </template>

      <UiSkeleton v-if="!rooms.loaded" variant="mod" :rows="4" />
      <UiEmpty v-else-if="!rooms.current" icon="package" text="请先在「房间」页新建一个房间" />
      <!-- 骨架屏只给首屏：后台刷新（每 15 秒的房间轮询触发的 load）不能
         把整个内容区（含创意工坊搜索结果、更新进度块）拆掉重建，
         否则表现为整页周期性闪烁、搜索结果周期性消失 -->
      <UiSkeleton v-else-if="loading && !mods.length" variant="mod" :rows="4" />
      <UiEmpty v-else-if="loadError" icon="alert" :text="'读取模组失败：' + loadError" />

      <template v-else>
        <div v-if="frozen" class="mb-4 flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm" role="status">
          <Icon name="info" size="16" />
          <span>服务器运行中，模组修改已冻结。</span>
        </div>
        <div v-else-if="missingCount" class="mb-4 flex flex-wrap items-center gap-3 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm">
          <Icon name="download" size="16" />
          <span>{{ missingCount }} 个已启用模组未下载</span>
          <Button class="ml-auto" variant="outline" size="sm" :disabled="queueing || updating" @click="enqueue(mods.filter(mod => mod.enabled && !mod.installed).map(mod => mod.id))">下载缺失</Button>
        </div>

        <!-- 检查结果：明确指出是哪个模组旧了，客户端只会笼统提示「有旧版本 mod」 -->
        <div v-if="checkResult" class="mb-4 flex flex-wrap items-center gap-3 rounded-lg border bg-muted/30 p-3 text-sm" role="status">
          <span>{{ updatesCount ? `${updatesCount} 个模组有新版本` : '没有可更新的模组' }}</span>
          <Button v-if="updatesCount" class="ml-auto" size="sm" :disabled="frozen || updating || queueing" @click="updateAll">更新全部</Button>
        </div>
        <p
          v-if="checkResult && (checkResult.missing || []).length"
          class="field-tip"
          style="margin-bottom: 14px"
        >
          工坊查不到以下模组（可能已下架或 ID 有误）：{{ (checkResult.missing || []).join('、') }}
        </p>

        <div v-if="updating || updateError || pollError" class="mb-4 flex flex-wrap items-center gap-3 rounded-lg border bg-muted/30 p-3 text-sm" role="status">
          <Icon :name="updateError || pollError ? 'alert' : 'download'" size="16" />
          <span class="min-w-0 flex-1 break-words">{{ pollError ? '无法读取更新进度：' + pollError : updateError || progress || '正在下载模组…' }}</span>
          <span v-if="updating && queuedCount" class="text-muted-foreground">排队 {{ queuedCount }} 个</span>
          <Button v-if="pollError" variant="outline" size="sm" @click="pollUpdate">重试</Button>
          <Button variant="outline" size="sm" @click="updateOpen = true">查看进度</Button>
        </div>

        <p v-if="workshopError" class="field-tip mb-3" role="status">
          创意工坊信息暂未加载：{{ workshopError }}。当前显示本地名称或 ID，可点击「刷新」重试。
        </p>
        <div v-if="mods.length" class="mb-4 flex flex-wrap items-center gap-3">
          <Input v-model="search" class="min-w-0 flex-1 sm:max-w-sm" aria-label="搜索模组" placeholder="搜索模组名称或 ID" />
          <NativeSelect v-model="filter" aria-label="模组状态" class="w-full sm:w-40">
            <NativeSelectOption value="all">全部 {{ mods.length }}</NativeSelectOption>
            <NativeSelectOption value="enabled">已启用 {{ enabledCount }}</NativeSelectOption>
            <NativeSelectOption value="disabled">已禁用 {{ mods.length - enabledCount }}</NativeSelectOption>
            <NativeSelectOption value="missing">未下载 {{ mods.filter(mod => !mod.installed).length }}</NativeSelectOption>
            <NativeSelectOption value="updates">有更新 {{ updatesCount }}</NativeSelectOption>
          </NativeSelect>
          <span class="ml-auto text-sm tabular-nums text-muted-foreground">已启用 {{ enabledCount }} / {{ mods.length }}</span>
        </div>
        <template v-if="visibleMods.length">
          <ModItem
            v-for="m in visibleMods"
            :key="m.id"
            :mod="m"
            :update-info="updateMap[m.id] || null"
            :downloading="busyIds.includes(m.id)"
            :updating="updating || queueing"
            @changed="load"
            @update="startOne"
            @toggled="enabled => onToggled(m.id, enabled)"
            @cover-needed="loadWorkshopDetails([m])"
          />
        </template>
        <div v-else class="grid justify-items-center gap-3 rounded-lg border border-dashed p-10">
          <UiEmpty icon="package" :text="mods.length ? '没有匹配的模组' : '还没有模组'" />
          <Button v-if="mods.length" variant="outline" size="sm" @click="search = ''; filter = 'all'">清除筛选</Button>
          <Button v-else size="sm" @click="workshopOpen = true">添加模组</Button>
        </div>
      </template>
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
        <pre class="mod-update-log mod-terminal" tabindex="0" role="region" aria-label="模组更新输出">{{ output.join('\n') || (updating ? '正在等待更新输出…' : '暂无更新输出') }}</pre>
        <div class="flex justify-end gap-2">
          <Button v-if="pollError" variant="outline" @click="pollUpdate">重试</Button>
          <Button v-if="updating" variant="destructive" :disabled="stopping" @click="stopUpdate">{{ stopping ? '正在中止…' : '中止' }}</Button>
          <DialogClose as-child>
            <Button variant="outline">关闭</Button>
          </DialogClose>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
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
