<script setup>
import { computed, onUnmounted, ref, watch } from 'vue';
import UiCard from '@/components/ui/UiCard.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useRoomsStore } from '@/stores/rooms.js';
import { procApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { roomShardLabel } from '@/lib/format.js';
import { toast } from '@/lib/toast.js';

const rooms = useRoomsStore();
const message = ref('');
const busy = ref('');
const report = ref(null);
const error = ref('');
let generation = 0;
const master = computed(() => rooms.current?.shards?.find(shard => shard.isMaster && !shard.disabled));
const masterRunning = computed(() => master.value && rooms.statuses.some(status => status.shard === master.value.name && status.state === 'running'));
const anyRunning = computed(() => rooms.statuses.some(status => status.state === 'running'));
const validMessage = computed(() => {
  const text = message.value.trim();
  return text.length > 0 && Array.from(text).length <= 500 && Array.from(text).every(char => {
    const code = char.codePointAt(0);
    return code >= 32 && (code < 127 || code > 159);
  });
});
watch(() => rooms.currentId, () => {
  generation++;
  message.value = '';
  report.value = null;
  error.value = '';
});
onUnmounted(() => { generation++; });

async function run(action) {
  if (busy.value || !rooms.current || (action === 'announce' && !validMessage.value)) return;
  const room = rooms.current;
  const roomId = room.id;
  const ticket = generation;
  const text = message.value.trim();
  const labels = new Map(room.shards.map(shard => [shard.name, roomShardLabel(room, shard)]));
  busy.value = action;
  error.value = '';
  report.value = null;
  try {
    if (action === 'skipday') {
      if (!await confirmAction(`让「${room.name}」的主世界快进一天？\n\n会改变主世界的时间与实体状态，不能撤销；建议先保存并备份。该操作执行后一分钟内不能再次快进或回档。`)) return;
    }
    if (ticket !== generation) return;
    const result = await procApi.quick(roomId, action === 'announce' ? { action, message: text } : { action });
    const failures = Object.entries(result.errors || {}).map(([shard, reason]) => `${labels.get(shard) || shard}：${reason}`);
    const completed = (result.shards || []).map(shard => labels.get(shard) || shard);
    if (ticket === generation) {
      report.value = {
        completed: completed.join('、'), failures,
        skipped: (result.skipped || []).map(shard => labels.get(shard) || shard).join('、'),
        action,
      };
      if (action === 'announce' && message.value.trim() === text) message.value = '';
    }
    if (failures.length) toast(`「${room.name}」部分分片保存失败，请查看结果`, 'err');
    else toast(`「${room.name}」${action === 'save' ? '已确认存档保存' : action === 'announce' ? '公告已交给游戏广播' : '主世界已执行快进'}`);
  } catch (err) {
    if (ticket === generation) error.value = err.message;
    toast(`「${room.name}」${err.message}`, 'err');
  } finally {
    busy.value = '';
  }
}
</script>

<template>
  <UiCard>
    <template #title>快捷操作</template>
    <section aria-label="快捷操作" class="grid min-w-0 gap-3.5">
      <div class="flex flex-wrap gap-2">
        <Button variant="outline" size="sm" :disabled="!!busy || !anyRunning || rooms.starting" @click="run('save')">{{ busy === 'save' ? '等待保存完成…' : '立即保存' }}</Button>
        <Button variant="outline" size="sm" :disabled="!!busy || !masterRunning || rooms.starting" @click="run('skipday')">{{ busy === 'skipday' ? '等待游戏回执…' : '主世界快进一天' }}</Button>
      </div>
      <form class="flex flex-col gap-2 sm:flex-row" @submit.prevent="run('announce')">
        <Input v-model="message" class="min-w-0 flex-1" aria-label="全服公告" placeholder="输入全服公告内容（最多 500 字）" :disabled="!!busy || !masterRunning || rooms.starting" />
        <Button type="submit" size="sm" :disabled="!!busy || !masterRunning || !validMessage || rooms.starting">{{ busy === 'announce' ? '等待游戏回执…' : '发送全服公告' }}</Button>
      </form>
      <p v-if="error" role="alert" class="break-words text-sm text-destructive">{{ error }}</p>
      <div v-if="report" role="status" class="space-y-1 break-words rounded-md bg-muted/50 px-3 py-2 text-xs">
        <p v-if="report.completed">{{ report.action === 'save' ? '已确认保存' : report.action === 'announce' ? '公告已提交' : '已执行快进' }}：{{ report.completed }}</p>
        <p v-if="report.skipped" class="text-muted-foreground">未运行，已跳过：{{ report.skipped }}</p>
        <p v-for="failure in report.failures" :key="failure" class="text-destructive">{{ failure }}</p>
      </div>
    </section>
  </UiCard>
</template>
