<script setup>
import { onMounted, onUnmounted, ref } from 'vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiField from '@/components/ui/UiField.vue';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select';
import { Switch } from '@/components/ui/switch';
import { automationApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { guard } from '@/lib/toast.js';

const snapshot = ref(null);
const draft = ref(null);
const error = ref('');
const saving = ref(false);
const jobs = { backup: '自动备份', restart: '每日重启', update: '更新检查' };
let timer;
let disposed = false;

async function refresh() {
  try {
    const data = await automationApi.status();
    if (disposed) return;
    snapshot.value = data;
    if (!draft.value) draft.value = { ...data.config };
    error.value = '';
  } catch (err) {
    if (!disposed) error.value = err.message;
  } finally {
    if (!disposed) timer = setTimeout(refresh, 5000);
  }
}
onMounted(refresh);
onUnmounted(() => { disposed = true; clearTimeout(timer); });

async function save() {
  if (saving.value) return;
  saving.value = true;
  try {
    const config = {
      ...draft.value,
      backupHours: Number(draft.value.backupHours), keep: Number(draft.value.keep),
      updateMinutes: Number(draft.value.updateMinutes),
    };
    if (!await confirmAction({
      title: '将维护计划应用于所有房间？',
      description: '自动备份成功后会按份数删除旧自动备份，手动备份不受影响。\n启用重启或自动更新后，会提前公告并保存；更新失败时保持停服，请查看执行结果。',
    })) return;
    const { ok, data } = await guard(() => automationApi.save(config), { success: '自动维护计划已保存' });
    if (ok && !disposed) { snapshot.value = data; draft.value = { ...data.config }; }
  } finally { saving.value = false; }
}

function date(value) {
  return !value || value.startsWith('0001-') ? '未安排' : new Date(value).toLocaleString();
}
</script>

<template>
  <UiCard>
    <template #title>自动维护</template>
    <template #actions>
      <IconAction v-if="draft" type="submit" form="automation-plan-form" icon="save" :label="saving ? '保存中…' : '保存计划'" :disabled="saving || !!snapshot?.running" />
    </template>
    <p v-if="error" role="alert" class="mb-4 text-sm text-destructive">{{ error }}</p>
    <form v-if="draft" id="automation-plan-form" class="grid gap-4" @submit.prevent="save">
      <fieldset class="grid gap-4 sm:grid-cols-2" :disabled="saving || !!snapshot?.running">
        <UiField label="所有房间备份间隔（小时，0 关闭）" for-id="auto-backup-hours" block>
          <Input id="auto-backup-hours" v-model="draft.backupHours" type="number" min="0" max="720" step="1" required />
        </UiField>
        <UiField label="每个房间保留自动备份份数" for-id="auto-backup-keep" block>
          <Input id="auto-backup-keep" v-model="draft.keep" type="number" min="1" max="100" step="1" required />
        </UiField>
        <UiField :label="`每日重启（${snapshot?.timezone || '宿主机时间'}）`" for-id="auto-restart-enabled" block>
          <div class="flex items-center gap-3">
            <Switch id="auto-restart-enabled" :model-value="!!draft.restartAt" @update:model-value="draft.restartAt = $event ? '04:00' : ''" />
            <Input v-if="draft.restartAt" v-model="draft.restartAt" aria-label="每日重启时间" type="time" required />
          </div>
        </UiField>
        <UiField label="更新检查间隔（分钟，0 关闭）" for-id="auto-update-minutes" block>
          <Input id="auto-update-minutes" v-model="draft.updateMinutes" type="number" min="0" max="1440" step="1" required />
        </UiField>
        <UiField label="发现服务端或已启用模组更新时" for-id="auto-update-mode" block>
          <NativeSelect id="auto-update-mode" v-model="draft.updateMode">
            <NativeSelectOption value="notify">仅检测</NativeSelectOption>
            <NativeSelectOption value="auto">自动停服更新并恢复</NativeSelectOption>
            <NativeSelectOption value="idle">所有房间无玩家时更新</NativeSelectOption>
          </NativeSelect>
        </UiField>
      </fieldset>
    </form>
    <p v-else-if="!error" role="status">读取计划中…</p>
    <div v-if="snapshot" class="mt-4 grid gap-3 text-sm" aria-label="维护执行状态">
      <p v-if="snapshot.running" role="status">正在执行：{{ jobs[snapshot.running] }}</p>
      <div v-for="(label, key) in jobs" :key="key" class="grid gap-1 border-t pt-3">
        <div class="flex flex-wrap justify-between gap-2"><span>{{ label }}</span><span>下次：{{ date(snapshot.jobs?.[key]?.next) }}</span></div>
        <p v-if="snapshot.jobs?.[key]?.result" class="break-words">{{ snapshot.jobs[key].result }}</p>
      </div>
    </div>
  </UiCard>
</template>
