<script setup>
import { onMounted, onUnmounted, ref } from 'vue';
import UiCard from '@/components/ui/UiCard.vue';
import IconAction from '@/components/ui/IconAction.vue';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Checkbox } from '@/components/ui/checkbox';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { backupRemoteApi } from '@/lib/api.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard } from '@/lib/toast.js';
import { fmtTime } from '@/lib/format.js';

const rooms = useRoomsStore();
const draft = ref(null);
const snapshot = ref(null);
const password = ref('');
const clearPassword = ref(false);
const busy = ref(false);
const error = ref('');
const labels = { queued: '排队中', syncing: '同步中', synced: '已同步', failed: '同步失败' };
let timer, disposed = false;

async function refresh() {
  clearTimeout(timer);
  try {
    const data = await backupRemoteApi.get();
    if (disposed) return;
    snapshot.value = data;
    if (!draft.value) draft.value = { enabled: !!data?.enabled, kind: data?.kind || 'directory', directory: data?.directory || '', url: data?.url || '', username: data?.username || '', keep: data?.keep ?? 3 };
    error.value = '';
  } catch (err) { if (!disposed) error.value = err.message; }
  finally { if (!disposed) timer = setTimeout(refresh, 3000); }
}
onMounted(refresh);
onUnmounted(() => { disposed = true; clearTimeout(timer); });

async function save() {
  if (busy.value) return;
  busy.value = true;
  try {
    const config = { ...draft.value };
    if (clearPassword.value) config.password = '';
    else if (password.value) config.password = password.value;
    const result = await guard(() => backupRemoteApi.save(config), { success: '异地备份设置已保存' });
    if (result.ok) { snapshot.value = result.data; password.value = ''; clearPassword.value = false; }
  } finally { busy.value = false; }
}
async function test() {
  if (busy.value) return;
  busy.value = true;
  try { await guard(() => backupRemoteApi.test(), { success: '目标可写，测试文件已清理' }); }
  finally { busy.value = false; }
}
async function retry(item) {
  if (busy.value) return;
  busy.value = true;
  try {
    await guard(() => backupRemoteApi.sync(item.room, item.name), { success: '已加入同步队列' });
    await refresh();
  } finally { busy.value = false; }
}
</script>

<template>
  <UiCard>
    <template #title>异地备份</template>
    <template #actions>
      <IconAction icon="plug" variant="outline" show-label="sm" label="测试已保存的备份目标" text="测试连接" :disabled="busy || !snapshot || (snapshot.kind === 'directory' ? !snapshot.directory : !snapshot.url)" @click="test" />
      <IconAction v-if="draft" type="submit" form="backup-remote-settings" show-label icon="save" :label="busy ? '处理中…' : '保存异地备份设置'" :text="busy ? '处理中…' : '保存设置'" :disabled="busy" />
    </template>
    <p v-if="error" role="alert" class="mb-4 text-sm text-destructive">{{ error }}</p>
    <form v-if="draft" id="backup-remote-settings" @submit.prevent="save">
      <FieldGroup>
        <Field orientation="horizontal"><FieldLabel for="remote-enabled">自动同步新备份</FieldLabel><Switch id="remote-enabled" v-model="draft.enabled" :disabled="busy" /></Field>
        <Field>
          <FieldLabel for="remote-kind">备份目标</FieldLabel>
          <Select v-model="draft.kind" :disabled="busy"><SelectTrigger id="remote-kind" class="w-full" aria-label="备份目标"><SelectValue /></SelectTrigger><SelectContent position="popper" align="start"><SelectGroup><SelectItem value="directory">目录 / NAS</SelectItem><SelectItem value="webdav">WebDAV</SelectItem></SelectGroup></SelectContent></Select>
        </Field>
        <Field v-if="draft.kind === 'directory'"><FieldLabel for="remote-directory">目录或 NAS 路径</FieldLabel><Input id="remote-directory" v-model="draft.directory" placeholder="D:\Backups 或 \\NAS\Backups" :required="draft.enabled" :disabled="busy" /></Field>
        <template v-else>
          <Field><FieldLabel for="remote-url">WebDAV 目录地址</FieldLabel><Input id="remote-url" v-model="draft.url" type="url" placeholder="https://example.com/dav/backups/" :required="draft.enabled" :disabled="busy" /><FieldDescription>填写已存在的 WebDAV 目录，面板会在其中创建独立子目录。</FieldDescription></Field>
          <Field><FieldLabel for="remote-username">用户名</FieldLabel><Input id="remote-username" v-model="draft.username" autocomplete="username" :disabled="busy" /></Field>
          <Field><FieldLabel for="remote-password">密码或应用密码</FieldLabel><Input id="remote-password" v-model="password" type="password" autocomplete="new-password" :placeholder="snapshot.passwordSet ? '已保存，留空保留' : '可选'" :disabled="busy || clearPassword" /></Field>
          <label v-if="snapshot.passwordSet" class="flex items-center gap-2 text-sm"><Checkbox v-model="clearPassword" :disabled="busy" />清除已保存密码</label>
        </template>
        <Field><FieldLabel for="remote-keep">每个房间保留份数</FieldLabel><Input id="remote-keep" v-model.number="draft.keep" type="number" min="1" max="100" required :disabled="busy" /></Field>
        <p class="text-sm text-muted-foreground">手动和自动备份成功后在后台同步。目标下使用本面板的独立子目录，失败时保留本地备份，可在备份页重试。</p>
      </FieldGroup>
    </form>
    <div v-else-if="!error" role="status" aria-label="正在读取异地备份设置" class="grid gap-4">
      <div class="flex items-center justify-between gap-4">
        <Skeleton class="h-4 w-28" />
        <Skeleton class="h-5 w-9 rounded-full" />
      </div>
      <div v-for="i in 3" :key="i" class="grid gap-2">
        <Skeleton class="h-4 w-28" />
        <Skeleton class="h-9 w-full" />
      </div>
    </div>
    <section v-if="snapshot?.results?.length" class="mt-4 border-t pt-4" aria-label="最近备份同步">
      <h3 class="mb-3 text-sm font-medium">最近同步</h3>
      <ul class="grid gap-3">
        <li v-for="item in snapshot.results" :key="item.id" class="grid min-w-0 gap-2 border-t pt-3 first:border-t-0 first:pt-0">
          <div class="flex flex-wrap items-center justify-between gap-2 text-sm"><span>{{ rooms.list.find(room => room.id === item.room)?.name || `房间 ${item.room}` }}</span><Badge :variant="item.state === 'failed' ? 'destructive' : 'secondary'">{{ labels[item.state] }}</Badge></div>
          <span class="break-all text-xs text-muted-foreground">{{ item.name }}</span>
          <div class="flex flex-wrap items-center justify-between gap-2"><time class="text-xs text-muted-foreground">{{ fmtTime(item.time) }}</time><Button v-if="item.state === 'failed'" size="sm" variant="outline" :disabled="busy || !snapshot.enabled" @click="retry(item)">重试同步</Button></div>
          <p v-if="item.error" class="break-words text-sm text-destructive">{{ item.error }}</p>
        </li>
      </ul>
    </section>
  </UiCard>
</template>
