<script setup>
// KeepAlive 保留各分片的命令草稿、滚动位置和自动滚动开关。
import { computed, onActivated, ref, watch } from 'vue';
import { TabsContent } from 'reka-ui';

import { Input } from '@/components/ui/input';
import { CodeBlockContent, CodeBlockLine } from '@/components/ui/code-block';
import { Button } from '@/components/ui/button';
import { useLogs } from '@/composables/useLogs.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard } from '@/lib/toast.js';
import { shardLabel } from '@/lib/format.js';

const props = defineProps({
  /** 分片名：Master / Caves */
  shard: { type: String, required: true },
});

const rooms = useRoomsStore();
const { channels, appendCmd } = useLogs();

/** 通道尚未建立时的占位。必须是稳定引用，否则 computed 会反复触发。 */
const FALLBACK = { lines: [] };
const state = computed(() => channels[props.shard] || FALLBACK);
const lines = computed(() => state.value.lines);

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
const scrollTop = ref(0);
defineExpose({ autoscroll });

function onScroll() {
  // 用户手动滚到底部时重新打开跟随，省得再去点开关
  const el = box.value?.$el;
  if (!el?.isConnected) return;
  scrollTop.value = el.scrollTop;
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

function restoreScroll() {
  if (scrollQueued) return;
  scrollQueued = true;
  requestAnimationFrame(() => {
    scrollQueued = false;
    const el = box.value?.$el;
    if (el?.isConnected) el.scrollTop = autoscroll.value ? el.scrollHeight : scrollTop.value;
  });
}

watch(lines, () => {
  if (autoscroll.value) restoreScroll();
});

onActivated(restoreScroll);

async function submit() {
  const cmd = input.value.trim();
  if (!cmd) return;
  const roomId = rooms.currentId;
  const { ok } = await guard(() => rooms.sendCmd(props.shard, cmd));
  if (ok && roomId === rooms.currentId) {
    appendCmd(props.shard, cmd);
    input.value = '';
    autoscroll.value = true;
    restoreScroll();
  }
}
</script>

<template>
  <TabsContent :value="shard" force-mount class="min-w-0 outline-none">
    <CodeBlockContent
      ref="box"
      wrap
      class="console"
      tabindex="0"
      role="region"
      :aria-label="`${label} 日志输出`"
      @scroll="onScroll"
    >
      <CodeBlockLine v-if="!lines.length" class="text-muted-foreground">暂无 {{ label }} 的日志输出。</CodeBlockLine>
      <CodeBlockLine v-for="l in lines" :key="l.id" class="log-line" :class="'k-' + l.kind">
        <span class="log-text">{{ l.text }}</span>
      </CodeBlockLine>
    </CodeBlockContent>

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
  </TabsContent>
</template>
