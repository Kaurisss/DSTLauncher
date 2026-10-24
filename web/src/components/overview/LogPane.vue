<script setup>
// 单个分片的日志面板：独立缓冲区、独立滚动位置、独立命令输入。
//
// 因为 Master 与 Caves 是两个独立进程，所以在哪个面板里敲命令，
// 就发给哪个分片——不需要再选一次目标。

import { computed, onMounted, ref, watch } from 'vue';

import { Switch } from '@/components/ui/switch';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import Icon from '@/components/ui/Icon.vue';
import { useLogs } from '@/composables/useLogs.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard } from '@/lib/toast.js';
import { shardLabel } from '@/lib/format.js';

const props = defineProps({
  /** 分片名：Master / Caves */
  shard: { type: String, required: true },
});

const rooms = useRoomsStore();
const { channels, clear, appendCmd } = useLogs();

/** 通道尚未建立时的占位。必须是稳定引用，否则 computed 会反复触发。 */
const FALLBACK = { lines: [], connected: false };
const state = computed(() => channels[props.shard] || FALLBACK);
const lines = computed(() => state.value.lines);
const connected = computed(() => !!state.value.connected);

/** 分片显示名：按存档 location 优先，与玩家列表、世界配置页一致。 */
const label = computed(() => shardLabel(props.shard, rooms.current?.locations?.[props.shard]));

/** 只有运行中的分片才接受控制台命令，其余情况禁用输入并说明原因。 */
const running = computed(() => {
  const st = rooms.statuses.find(s => s.shard === props.shard);
  return !!st && st.state === 'running';
});

const box = ref(null);
const input = ref('');
/** 是否自动跟随最新日志。关掉后可以安静地往上翻历史。 */
const autoscroll = ref(true);

function onScroll() {
  // 用户手动滚到底部时重新打开跟随，省得再去点开关
  const el = box.value;
  if (!el) return;
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 30 && !autoscroll.value) {
    autoscroll.value = true;
  }
}

/**
 * 滚动到底部，用 requestAnimationFrame 合并。
 *
 * 设置 scrollTop 会强制浏览器做一次完整布局，而日志区可能有几千个节点。
 * 高频日志下若每行都滚一次，布局计算会把主线程打满——这正是之前
 * "启动阶段整个页面近乎卡死"的根因之一。rAF 保证一帧最多定位一次，
 * 与 useLogs 的批量落账配合后，布局次数与日志量彻底解耦。
 */
let scrollQueued = false;

function scrollToEnd() {
  if (scrollQueued) return;
  scrollQueued = true;
  requestAnimationFrame(() => {
    scrollQueued = false;
    const el = box.value;
    if (el) el.scrollTop = el.scrollHeight;
  });
}

watch(lines, () => {
  if (autoscroll.value) scrollToEnd();
});

onMounted(scrollToEnd);

async function submit() {
  const cmd = input.value.trim();
  if (!cmd) return;
  const roomId = rooms.currentId;
  const { ok } = await guard(() => rooms.sendCmd(props.shard, cmd));
  if (ok && roomId === rooms.currentId) {
    appendCmd(props.shard, cmd);
    input.value = '';
    autoscroll.value = true;
    scrollToEnd();
  }
}
</script>

<template>
  <section class="log-pane" :aria-label="`${label}（${shard}）日志`">
    <header class="log-pane-head">
      <div class="log-pane-title">
        <Icon name="terminal" :size="15" />
        <span class="log-pane-name">{{ label }}</span>
        <span class="log-pane-sub">{{ shard }}</span>
        <span class="log-connection" :class="connected ? 'is-connected' : ''">
          {{ connected ? '已连接' : '重连中' }}
        </span>
      </div>

      <div class="log-pane-tools">
        <label class="log-autoscroll" title="关闭后可以安静地往上翻历史，滚回底部会自动重新打开">
          <Switch v-model="autoscroll" :aria-label="`${label} 自动滚动`" />
          <span>自动滚动</span>
        </label>
        <Button variant="ghost" size="sm" @click="clear(shard)">清屏</Button>
      </div>
    </header>

    <div
      ref="box"
      class="console"
      tabindex="0"
      role="region"
      :aria-label="`${label} 日志输出`"
      @scroll="onScroll"
    >
      <div v-if="!lines.length" class="console-empty">
        <p>暂无 {{ label }} 的日志输出。</p>
        <p>启动该分片后，这里的输出会实时刷新。</p>
      </div>
      <div v-for="l in lines" :key="l.id" class="log-line" :class="'k-' + l.kind">
        <span class="log-text">{{ l.text }}</span>
      </div>
    </div>

    <form class="console-form" @submit.prevent="submit">
      <span class="console-prompt" aria-hidden="true">$</span>
      <Input
        v-model="input"
        class="flex-1 min-w-0"
        autocomplete="off"
        :aria-label="`${label} 控制台命令`"
        :disabled="!running"
        :placeholder="running ? '控制台命令，如 c_save() / c_listallplayers()' : label + ' 未运行'"
      />
      <Button type="submit" size="sm" :disabled="!running">执行</Button>
    </form>
  </section>
</template>
