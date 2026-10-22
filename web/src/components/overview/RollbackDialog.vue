<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import {
  DialogRoot, DialogTrigger, DialogPortal, DialogOverlay, DialogContent,
  DialogTitle, DialogDescription, DialogClose,
} from 'reka-ui';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import UiField from '@/components/ui/UiField.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard } from '@/lib/toast.js';

const rooms = useRoomsStore();
const open = ref(false);
const count = ref(1);
const now = ref(Date.now());
let timer;
onMounted(() => { timer = setInterval(() => { now.value = Date.now(); }, 1000); });
onUnmounted(() => clearInterval(timer));

const remaining = computed(() => Math.max(0, Math.ceil(((rooms.rollbackDeadlines[rooms.currentId] || 0) - now.value) / 1000)));
const maxCount = computed(() => Math.max(0, Math.min(99, (rooms.current?.maxSnapshots || 10) - 1)));
const allRunning = computed(() => rooms.shards.length > 0 && rooms.shards.every(shard =>
  rooms.statuses.some(status => status.shard === shard && status.state === 'running')));
const valid = computed(() => Number.isInteger(Number(count.value)) && Number(count.value) >= 1 && Number(count.value) <= maxCount.value);
const unavailable = computed(() => {
  if (rooms.starting) return '正在准备开服，请等待世界加载完成。';
  if (!allRunning.value) return '请先在概览页启动房间所有启用的世界，等待各世界加载并连接完成。';
  if (!maxCount.value) return '房间仅保留一个存档点，请先增加回档点数并等待产生新存档。';
  return '';
});

watch(() => rooms.currentId, () => { open.value = false; count.value = 1; });
watch(open, value => { if (value) count.value = 1; });

async function submit() {
  if (!valid.value || unavailable.value || rooms.rollingBack || remaining.value) return;
  const name = rooms.current.name;
  const { ok } = await guard(() => rooms.rollback(Number(count.value)), {
    success: `「${name}」回档请求已发送，正在重新加载世界，请查看实时日志确认。`,
  });
  if (ok) {
    now.value = Date.now();
    open.value = false;
    rooms.refreshStatus().catch(() => {});
  }
}

function preventWhileBusy(event) {
  if (rooms.rollingBack) event.preventDefault();
}
</script>

<template>
  <DialogRoot v-model:open="open">
    <DialogTrigger as-child>
      <Button variant="outline" size="sm" :disabled="!rooms.current || rooms.rollingBack || remaining > 0">
        {{ rooms.rollingBack ? '正在请求回档…' : remaining > 0 ? `回档中（${remaining}秒）` : '回档' }}
      </Button>
    </DialogTrigger>
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-50 bg-black/20" />
      <DialogContent
        class="fixed left-1/2 top-1/2 z-50 grid w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 rounded-xl border bg-popover p-6 text-popover-foreground shadow-lg"
        @escape-key-down="preventWhileBusy" @interact-outside="preventWhileBusy"
      >
        <DialogTitle class="text-lg font-semibold">游戏存档回档</DialogTitle>
        <DialogDescription class="text-sm text-muted-foreground">
          回档房间「{{ rooms.current?.name }}」的全部启用世界，各世界将同步重新加载。
        </DialogDescription>
        <form class="grid gap-4" @submit.prevent="submit">
          <UiField label="回退存档次数" for-id="rollback-count" block tip="1 表示回到最新存档之前的一个存档点。存档点通常每天产生，手动保存也会产生存档点。">
            <Input id="rollback-count" v-model="count" type="number" min="1" :max="maxCount || 1" step="1" :disabled="rooms.rollingBack" />
          </UiField>
          <p class="text-sm text-destructive">当前未保存进度及目标之后的存档点将被丢弃。建议先到备份页保留一份整房间备份。</p>
          <p v-if="unavailable" role="status" class="text-sm text-muted-foreground">{{ unavailable }}</p>
          <p v-else-if="!valid" role="alert" class="text-sm text-destructive">请输入 1–{{ maxCount }} 之间的整数；实际可回退数量由游戏中保留的存档决定。</p>
          <div class="flex justify-end gap-2">
            <DialogClose as-child>
              <Button type="button" variant="outline" :disabled="rooms.rollingBack">取消</Button>
            </DialogClose>
            <Button type="submit" variant="destructive" :disabled="!valid || !!unavailable || rooms.rollingBack || remaining > 0">
              {{ rooms.rollingBack ? '正在请求回档…' : '确认回档' }}
            </Button>
          </div>
        </form>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
