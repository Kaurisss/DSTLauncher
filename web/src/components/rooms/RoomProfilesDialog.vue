<script setup>
import { computed, ref, watch } from 'vue';
import { useFileDialog } from '@vueuse/core';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogTrigger } from '@/components/ui/dialog';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import IconAction from '@/components/ui/IconAction.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { profileApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { guard } from '@/lib/toast.js';

const emit = defineEmits(['created']);
const rooms = useRoomsStore();
const open = ref(false);
const source = ref('');
const name = ref('');
const items = ref([]);
const busy = ref(false);
const error = ref('');
const { open: openImportDialog, onChange: onImportFile } = useFileDialog({
  accept: '.json,application/json',
  multiple: false,
  reset: true,
});
const sourceRoom = computed(() => rooms.list.find(room => room.id === Number(source.value)));

watch(open, async value => {
  if (!value) return;
  source.value = String(rooms.currentId || rooms.list[0]?.id || '');
  name.value = sourceRoom.value?.name || '';
  await load();
});
watch(source, () => { name.value = sourceRoom.value?.name || ''; });

async function load() {
  const result = await guard(() => profileApi.list());
  error.value = result.ok ? '' : '读取方案失败，请重试。';
  if (result.ok) items.value = result.data;
}

async function run(action) {
  if (busy.value) return;
  busy.value = true;
  try { await action(); } finally { busy.value = false; }
}

function save() {
  run(async () => {
    const result = await guard(() => profileApi.save(Number(source.value), name.value), { success: '配置方案已保存' });
    if (result.ok) await load();
  });
}

function create(profile) {
  run(async () => {
    const result = await guard(() => profile
      ? profileApi.apply(profile.id, profile.name + ' 副本')
      : profileApi.copy(Number(source.value), sourceRoom.value.name + ' 副本'), { success: '新房间已创建，请检查房间设置' });
    if (result.ok) { open.value = false; emit('created', result.data); }
  });
}

function remove(profile) {
  run(async () => {
    if (!await confirmAction(`删除配置方案「${profile.name}」？`)) return;
    const result = await guard(() => profileApi.remove(profile.id), { success: '配置方案已删除' });
    if (result.ok) await load();
  });
}

onImportFile(files => {
  const selected = files?.[0];
  if (!selected) return;
  run(async () => {
    const result = await guard(async () => {
      if (selected.size > 8 * 1024 * 1024) throw new Error('方案不能超过 8 MiB');
      return profileApi.import(JSON.parse(await selected.text()));
    }, { success: '配置方案已导入' });
    if (result.ok) await load();
  });
});
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child><IconAction icon="templates" variant="outline" show-label="sm" label="配置方案" /></DialogTrigger>
    <DialogContent class="max-h-[85dvh] overflow-y-auto sm:max-w-xl" :close-disabled="busy" @escape-key-down="busy && $event.preventDefault()" @interact-outside="busy && $event.preventDefault()">
      <DialogHeader>
        <DialogTitle>配置方案</DialogTitle>
        <DialogDescription>复用房间、各世界和模组配置。新房间使用新端口，不携带存档、令牌和房间密码。</DialogDescription>
      </DialogHeader>
      <form class="grid gap-4" @submit.prevent="save">
        <FieldGroup>
          <Field>
            <FieldLabel for="profile-source">源房间</FieldLabel>
            <Select v-model="source" :disabled="busy || !rooms.list.length">
              <SelectTrigger id="profile-source" aria-label="源房间" class="w-full"><SelectValue placeholder="选择源房间" /></SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectGroup>
                  <SelectItem v-for="room in rooms.list" :key="room.id" :value="String(room.id)">{{ room.name }}{{ room.running ? '（运行中）' : '' }}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Field><FieldLabel for="profile-name">方案名称</FieldLabel><Input id="profile-name" v-model="name" maxlength="80" required :disabled="busy" /></Field>
        </FieldGroup>
        <p v-if="sourceRoom?.running" class="text-sm text-muted-foreground">请先停止源房间，再复制或保存方案。</p>
        <div class="flex flex-wrap gap-2">
          <Button type="submit" :disabled="busy || !sourceRoom || sourceRoom.running || !name.trim()">保存为方案</Button>
          <Button type="button" variant="outline" :disabled="busy || !sourceRoom || sourceRoom.running" @click="create()">复制房间</Button>
          <Button type="button" variant="outline" :disabled="busy" @click="openImportDialog()">导入方案</Button>
        </div>
      </form>
      <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }} <Button variant="link" :disabled="busy" @click="load">重试</Button></p>
      <ul v-else class="grid gap-3" aria-label="已保存配置方案">
        <li v-for="profile in items" :key="profile.id" class="flex min-w-0 flex-wrap items-center justify-between gap-3 border-t pt-3">
          <div class="min-w-0"><p class="break-words text-sm font-medium">{{ profile.name }}</p><p class="text-xs text-muted-foreground">{{ profile.shards }} 个世界</p></div>
          <div class="flex shrink-0 items-center gap-2">
            <Button type="button" size="sm" variant="outline" :disabled="busy" :aria-label="`从${profile.name}新建房间`" @click="create(profile)">新建房间</Button>
            <IconAction as="a" icon="download" variant="ghost" :label="`导出${profile.name}`" :href="profileApi.exportUrl(profile.id)" download />
            <IconAction icon="trash" variant="ghost" :label="`删除${profile.name}`" :disabled="busy" @click="remove(profile)" />
          </div>
        </li>
        <li v-if="!items.length" class="py-4 text-sm text-muted-foreground">尚未保存配置方案</li>
      </ul>
    </DialogContent>
  </Dialog>
</template>
