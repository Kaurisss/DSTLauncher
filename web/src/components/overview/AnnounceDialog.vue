<script setup>
import { computed, ref, watch } from 'vue';
import {
  Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
} from '@/components/ui/dialog';
import IconAction from '@/components/ui/IconAction.vue';
import { Textarea } from '@/components/ui/textarea';
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field';
import { useRoomsStore } from '@/stores/rooms.js';
import { procApi } from '@/lib/api.js';
import { toast } from '@/lib/toast.js';

const rooms = useRoomsStore();
const open = ref(false);
const message = ref('');
const busy = ref(false);
const error = ref('');
let generation = 0;

const master = computed(() => rooms.current?.shards?.find(shard => shard.isMaster && !shard.disabled));
const masterRunning = computed(() => master.value && rooms.statuses.some(status => status.shard === master.value.name && status.state === 'running'));
const canAnnounce = computed(() => !!rooms.current && !!masterRunning.value && !rooms.starting);

const validMessage = computed(() => {
  const text = message.value.trim();
  return text.length > 0 && Array.from(text).length <= 500 && Array.from(text).every(char => {
    const code = char.codePointAt(0);
    return code >= 32 && (code < 127 || code > 159);
  });
});

watch(() => rooms.currentId, () => {
  generation++;
  open.value = false;
  message.value = '';
  error.value = '';
});

watch(open, val => {
  if (val) error.value = '';
});

async function submit() {
  if (busy.value || !rooms.current || !validMessage.value) return;
  const room = rooms.current;
  const roomId = room.id;
  const ticket = generation;
  const text = message.value.trim();
  busy.value = true;
  error.value = '';
  try {
    await procApi.quick(roomId, { action: 'announce', message: text });
    if (ticket !== generation) return;
    toast(`「${room.name}」公告已交给游戏广播`);
    message.value = '';
    open.value = false;
  } catch (err) {
    if (ticket === generation) error.value = err.message;
    toast(`「${room.name}」${err.message}`, 'err');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <IconAction
        variant="outline"
        show-label
        icon="megaphone"
        :label="busy ? '正在发送…' : '全服公告'"
        :disabled="!canAnnounce"
      />
    </DialogTrigger>
    <DialogContent class="sm:max-w-md" :close-disabled="busy">
      <DialogHeader>
        <DialogTitle>发送全服公告</DialogTitle>
        <DialogDescription>
          向「{{ rooms.current?.name }}」的所有在线玩家广播公告（通过主世界控制台）。
        </DialogDescription>
      </DialogHeader>
      <form class="grid gap-4" @submit.prevent="submit">
        <Field>
          <FieldLabel for="announce-message">公告内容</FieldLabel>
          <Textarea
            id="announce-message"
            v-model="message"
            class="min-h-24 w-full resize-y"
            aria-label="全服公告"
            maxlength="500"
            :disabled="busy"
            @keydown.ctrl.enter="submit"
            @keydown.meta.enter="submit"
          />
          <FieldDescription class="text-xs">最多 500 字，按 Ctrl+Enter 发送。</FieldDescription>
        </Field>
        <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
        <DialogFooter>
          <DialogClose as-child>
            <IconAction type="button" variant="outline" show-label icon="x" label="取消" :disabled="busy" />
          </DialogClose>
          <IconAction type="submit" show-label icon="send" :label="busy ? '发送中…' : '发送全服公告'" :text="busy ? '发送中…' : '发送公告'" :disabled="busy || !validMessage" />
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
