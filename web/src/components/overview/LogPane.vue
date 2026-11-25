<script setup>
// KeepAlive 保留各分片的命令草稿、命令历史、滚动位置和是否跟随最新日志。
import { computed, onActivated, ref, watch } from 'vue';
import { TabsContent } from 'reka-ui';

import { Input } from '@/components/ui/input';
import { CodeBlockContent, CodeBlockLine } from '@/components/ui/code-block';
import Icon from '@/components/ui/Icon.vue';
import IconAction from '@/components/ui/IconAction.vue';
import QuickCommandsDialog from '@/components/overview/QuickCommandsDialog.vue';
import { useLogs } from '@/composables/useLogs.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard } from '@/lib/toast.js';
import { shardLabel } from '@/lib/format.js';

const props = defineProps({
  /** 分片名：Master / Caves */
  shard: { type: String, required: true },
  /** 关键词过滤 */
  filter: { type: String, default: '' },
  /** 日志级别：all | issues */
  level: { type: String, default: 'all' },
  /** 是否铺满独立控制台页高度 */
  fullHeight: { type: Boolean, default: false },
});

const rooms = useRoomsStore();
const { channels, appendCmd } = useLogs();

/** 通道尚未建立时的占位。必须是稳定引用，否则 computed 会反复触发。 */
const FALLBACK = { lines: [] };
const state = computed(() => channels[props.shard] || FALLBACK);
const rawLines = computed(() => state.value.lines);

const ISSUE_RE = /\b(error|exception|fatal|crash|failed|warning|warn|lua error|stack traceback)\b/i;

const lines = computed(() => {
  const q = props.filter.trim().toLowerCase();
  const issuesOnly = props.level === 'issues';
  if (!q && !issuesOnly) return rawLines.value;
  return rawLines.value.filter(l => {
    if (issuesOnly) {
      const isIssueKind = l.kind === 'warn' || l.kind === 'crash' || l.kind === 'restart';
      if (!isIssueKind && !ISSUE_RE.test(l.text)) return false;
    }
    if (q && !l.text.toLowerCase().includes(q)) return false;
    return true;
  });
});

/** 分片显示名：按存档 location 优先，与玩家列表、世界配置页一致。 */
const label = computed(() => shardLabel(props.shard, rooms.current?.locations?.[props.shard]));

/** 只有运行中的分片才接受控制台命令，其余情况禁用输入并说明原因。 */
const running = computed(() => {
  const st = rooms.statuses.find(s => s.shard === props.shard);
  return !!st && st.state === 'running';
});

const box = ref(null);
const input = ref('');
const history = ref([]);
const historyIndex = ref(-1);
let draftBeforeHistory = '';
/** 贴在底部时跟随新日志。用户离开底部后暂停，直到回到底部或点悬浮按钮。 */
const autoscroll = ref(true);
const scrollTop = ref(0);
let adjusting = false;
let settling = false;

function atBottom(el) {
  return el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
}

function onScroll() {
  const el = box.value?.$el;
  if (!el?.isConnected) return;
  const moved = el.scrollTop !== scrollTop.value;
  scrollTop.value = el.scrollTop;
  if (adjusting) return;
  if (atBottom(el)) {
    autoscroll.value = true;
    settling = false;
  } else if (moved && !settling) autoscroll.value = false;
}

function stopSettle() {
  if (!settling) return;
  settling = false;
  const el = box.value?.$el;
  if (!el?.isConnected) return;
  scrollGen += 1;
  adjusting = true;
  el.scrollTo({ top: el.scrollTop, behavior: 'auto' });
  scrollTop.value = el.scrollTop;
  adjusting = false;
}

/**
 * 滚动到底部，用 requestAnimationFrame 合并。
 */
let scrollQueued = false;
let scrollGen = 0;

function restoreScroll() {
  if (scrollQueued) return;
  scrollQueued = true;
  const gen = scrollGen;
  requestAnimationFrame(() => {
    scrollQueued = false;
    if (gen !== scrollGen) return;
    const el = box.value?.$el;
    if (!el?.isConnected) return;
    if (settling && autoscroll.value) {
      el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
      return;
    }
    adjusting = true;
    el.scrollTop = autoscroll.value ? el.scrollHeight : scrollTop.value;
    adjusting = false;
  });
}

function follow() {
  autoscroll.value = true;
  if (!window.matchMedia('(prefers-reduced-motion: reduce)').matches) settling = true;
  restoreScroll();
}

watch(lines, () => {
  if (autoscroll.value) restoreScroll();
});

onActivated(restoreScroll);

function recordHistory(cmd) {
  const next = history.value.filter(item => item !== cmd);
  next.push(cmd);
  if (next.length > 50) next.splice(0, next.length - 50);
  history.value = next;
  historyIndex.value = -1;
  draftBeforeHistory = '';
}

function onInputKeydown(event) {
  if (!history.value.length) return;
  if (event.key === 'ArrowUp') {
    event.preventDefault();
    if (historyIndex.value === -1) {
      draftBeforeHistory = input.value;
      historyIndex.value = history.value.length - 1;
    } else if (historyIndex.value > 0) {
      historyIndex.value -= 1;
    }
    input.value = history.value[historyIndex.value];
  } else if (event.key === 'ArrowDown' && historyIndex.value !== -1) {
    event.preventDefault();
    if (historyIndex.value < history.value.length - 1) {
      historyIndex.value += 1;
      input.value = history.value[historyIndex.value];
    } else {
      historyIndex.value = -1;
      input.value = draftBeforeHistory;
    }
  }
}

async function runCommand(cmdText) {
  const cmd = String(cmdText || '').trim();
  if (!cmd) return;
  const roomId = rooms.currentId;
  const { ok } = await guard(() => rooms.sendCmd(props.shard, cmd));
  if (ok && roomId === rooms.currentId) {
    appendCmd(props.shard, cmd);
    recordHistory(cmd);
    input.value = '';
    follow();
  }
}

async function submit() {
  await runCommand(input.value);
}

function fillCommand(cmd) {
  input.value = cmd;
  historyIndex.value = -1;
}

defineExpose({
  getVisibleText: () => lines.value.map(l => l.text).join('\n'),
  counts: computed(() => ({ visible: lines.value.length, total: rawLines.value.length })),
});
</script>

<template>
  <TabsContent
    :value="shard"
    force-mount
    :class="['min-w-0 outline-none', fullHeight && 'flex min-h-0 flex-1 flex-col']"
  >
    <div :class="['console-view', fullHeight && 'flex min-h-0 flex-1 flex-col']">
      <CodeBlockContent
        ref="box"
        wrap
        :class="['console', fullHeight && 'is-full']"
        tabindex="0"
        role="region"
        :aria-label="`${label} 日志输出`"
        @scroll="onScroll"
        @wheel="stopSettle"
        @pointerdown="stopSettle"
        @keydown="stopSettle"
      >
        <CodeBlockLine v-if="!rawLines.length" class="text-muted-foreground">暂无 {{ label }} 的日志输出。</CodeBlockLine>
        <CodeBlockLine v-else-if="!lines.length" class="text-muted-foreground">当前筛选条件下没有匹配的日志（共 {{ rawLines.length }} 行）。</CodeBlockLine>
        <CodeBlockLine v-for="l in lines" :key="l.id" class="log-line" :class="'k-' + l.kind">
          <span class="log-text">{{ l.text }}</span>
        </CodeBlockLine>
      </CodeBlockContent>
      <Transition name="console-latest">
        <button v-if="!autoscroll" type="button" class="console-latest" :aria-label="`${label} 回到最新日志`" @click="follow">
          <Icon name="chevron" class="rotate-90" size="16" />
        </button>
      </Transition>
    </div>

    <form class="console-form shrink-0" @submit.prevent="submit">
      <QuickCommandsDialog :shard="shard" :disabled="!running" @fill="fillCommand" @execute="runCommand" />
      <span class="console-prompt" aria-hidden="true">$</span>
      <Input
        v-model="input"
        class="min-w-0 flex-1 font-mono text-xs"
        autocomplete="off"
        :aria-label="`${label} 控制台命令`"
        :disabled="!running"
        :placeholder="running ? '输入 Lua 控制台命令（支持 ↑ / ↓ 回溯历史），如 c_save()' : label + ' 未运行'"
        @keydown="onInputKeydown"
      />
      <IconAction type="submit" show-label="sm" icon="terminal" label="执行" :disabled="!running || !input.trim()" />
    </form>
  </TabsContent>
</template>
