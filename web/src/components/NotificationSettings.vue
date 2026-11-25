<script setup>
import { onMounted, ref } from 'vue';
import UiCard from '@/components/ui/UiCard.vue';
import IconAction from '@/components/ui/IconAction.vue';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Checkbox } from '@/components/ui/checkbox';
import { notificationApi } from '@/lib/api.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard } from '@/lib/toast.js';

const rooms = useRoomsStore();
const draft = ref(null);
const snapshot = ref(null);
const secret = ref('');
const clearSecret = ref(false);
const busy = ref(false);
const error = ref('');
const labels = {
  crash_limit: '连续崩溃达到重启上限', maintenance_failed: '自动维护失败',
  backup_failed: '备份失败', update_failed: '模组更新失败', start_failed: '分片启动失败', started: '分片启动完成', stopped: '分片停止',
};

async function load() {
  try {
    const data = await notificationApi.get();
    snapshot.value = data;
    draft.value = { enabled: data.enabled, url: data.url, events: [...data.events], rooms: [...data.rooms] };
    error.value = '';
  } catch (err) { error.value = err.message; }
}
onMounted(load);

function toggle(field, value, checked) {
  draft.value[field] = checked ? [...draft.value[field], value] : draft.value[field].filter(item => item !== value);
}

async function save() {
  if (busy.value) return;
  busy.value = true;
  try {
    const data = { ...draft.value };
    if (clearSecret.value) data.secret = '';
    else if (secret.value) data.secret = secret.value;
    const result = await guard(() => notificationApi.save(data), { success: '通知设置已保存' });
    if (result.ok) { secret.value = ''; clearSecret.value = false; await load(); }
  } finally { busy.value = false; }
}

async function test() {
  if (busy.value) return;
  busy.value = true;
  try {
    await guard(() => notificationApi.test(), { success: '测试通知已送达' });
    const data = await notificationApi.get();
    snapshot.value = data;
  } catch (err) { error.value = err.message; }
  finally { busy.value = false; }
}
</script>

<template>
  <UiCard>
    <template #title>事件通知</template>
    <template #actions>
      <IconAction icon="send" variant="outline" show-label="sm" label="测试已保存的通知设置" text="测试通知" :disabled="busy || !snapshot?.url" @click="test" />
      <IconAction v-if="draft" type="submit" form="notification-settings" show-label icon="save" :label="busy ? '处理中…' : '保存通知设置'" :text="busy ? '处理中…' : '保存设置'" :disabled="busy" />
    </template>
    <p v-if="error" role="alert" class="mb-4 text-sm text-destructive">{{ error }}</p>
    <form v-if="draft" id="notification-settings" @submit.prevent="save">
      <FieldGroup>
        <Field orientation="horizontal"><FieldLabel for="notification-enabled">启用通知</FieldLabel><Switch id="notification-enabled" v-model="draft.enabled" :disabled="busy" /></Field>
        <Field><FieldLabel for="notification-url">Webhook 地址</FieldLabel><Input id="notification-url" v-model="draft.url" type="url" placeholder="https://example.com/webhook" :required="draft.enabled" :disabled="busy" /></Field>
        <Field><FieldLabel for="notification-secret">签名密钥</FieldLabel><Input id="notification-secret" v-model="secret" type="password" autocomplete="new-password" :placeholder="snapshot.secretSet ? '已保存，留空保留' : '可选'" :disabled="busy || clearSecret" /></Field>
        <label v-if="snapshot.secretSet" class="flex items-center gap-2 text-sm"><Checkbox v-model="clearSecret" :disabled="busy" />清除已保存密钥</label>
        <fieldset class="grid gap-3" :disabled="busy">
          <legend class="mb-3 text-sm font-medium">通知事件</legend>
          <label v-for="(label, key) in labels" :key="key" class="flex items-center gap-2 text-sm"><Checkbox :model-value="draft.events.includes(key)" @update:model-value="toggle('events', key, $event)" />{{ label }}</label>
        </fieldset>
        <fieldset class="grid gap-3" :disabled="busy">
          <legend class="mb-3 text-sm font-medium">通知房间</legend>
          <p class="text-sm text-muted-foreground">不勾选时通知全部房间。自动维护和共享模组更新属于全局事件。</p>
          <label v-for="room in rooms.list" :key="room.id" class="flex items-center gap-2 text-sm"><Checkbox :model-value="draft.rooms.includes(room.id)" @update:model-value="toggle('rooms', room.id, $event)" />{{ room.name }}</label>
        </fieldset>
        <p class="text-sm text-muted-foreground">向接收端发送 JSON。测试使用已保存的设置；签名接入方式见项目说明。</p>
      </FieldGroup>
    </form>
    <div v-else-if="!error" role="status" aria-label="正在读取通知设置" class="grid gap-4">
      <div class="flex items-center justify-between gap-4">
        <Skeleton class="h-4 w-20" />
        <Skeleton class="h-5 w-9 rounded-full" />
      </div>
      <div v-for="i in 2" :key="i" class="grid gap-2">
        <Skeleton class="h-4 w-24" />
        <Skeleton class="h-9 w-full" />
      </div>
      <div class="grid gap-2 pt-1">
        <Skeleton class="h-4 w-20" />
        <Skeleton v-for="i in 4" :key="i" class="h-4 w-40" />
      </div>
    </div>
    <section v-if="snapshot?.results.length" class="mt-4 border-t pt-4" aria-label="最近通知结果">
      <h3 class="mb-3 text-sm font-medium">最近投递结果</h3>
      <ul class="grid gap-3">
        <li v-for="(item, index) in snapshot.results" :key="index" class="flex flex-wrap justify-between gap-2 text-sm">
          <span>{{ labels[item.event] || '测试通知' }} <time class="text-xs text-muted-foreground">{{ new Date(item.time).toLocaleString() }}</time></span>
          <span :class="item.error ? 'text-destructive' : 'text-muted-foreground'">{{ item.error || `已送达（HTTP ${item.status}）` }}</span>
        </li>
      </ul>
    </section>
  </UiCard>
</template>
