<script setup>
import { computed, ref, watch } from 'vue';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import IconAction from '@/components/ui/IconAction.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { diagnosticsApi } from '@/lib/api.js';
import { shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const open = ref(false);
const busy = ref(false);
const report = ref(null);
const error = ref('');
const groups = computed(() => report.value ? [
  { label: '配置检查', findings: report.value.checks },
  ...report.value.shards.map(item => ({ ...item, label: shardLabel(item.shard, rooms.current?.locations?.[item.shard]) })),
  { ...report.value.downloads, label: '共享模组下载' },
] : []);

watch(() => rooms.currentId, () => { open.value = false; report.value = null; });
watch(open, value => { if (value) refresh(); });

async function refresh() {
  if (busy.value || !rooms.currentId) return;
  const room = rooms.currentId;
  busy.value = true;
  error.value = '';
  try {
    const result = await diagnosticsApi.get(room);
    if (rooms.currentId === room) report.value = result;
  } catch (err) {
    if (rooms.currentId === room) error.value = err.message;
  } finally { busy.value = false; }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child><IconAction icon="activity" variant="outline" label="故障诊断" :disabled="!rooms.current" /></DialogTrigger>
    <DialogContent class="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>故障诊断</DialogTitle>
        <DialogDescription>检查当前配置、各分片最近 400 行日志及共享模组下载记录。日志问题可能来自此前的启动。</DialogDescription>
      </DialogHeader>
      <div class="flex items-center justify-between gap-3 text-sm">
        <span>{{ rooms.current?.name }}</span>
        <IconAction icon="refresh" variant="outline" :label="busy ? '诊断中…' : '重新诊断'" :disabled="busy" @click="refresh" />
      </div>
      <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
      <p v-else-if="busy && !report" role="status" class="text-sm text-muted-foreground">正在读取最近日志…</p>
      <section v-for="group in groups" :key="group.label" class="grid gap-3 border-t pt-3" :aria-label="group.label">
        <h3 class="text-sm font-semibold">{{ group.label }}</h3>
        <p v-if="group.error" role="alert" class="text-sm text-destructive">{{ group.error }}</p>
        <p v-else-if="!group.findings.length" class="text-sm text-muted-foreground">未发现已识别问题</p>
        <article v-for="(finding, index) in group.findings" :key="`${finding.code}-${index}`" class="grid min-w-0 gap-2 rounded-lg bg-muted/50 p-4">
          <div class="flex flex-wrap items-center gap-2"><Badge variant="outline">{{ finding.stage }}</Badge><h4 class="text-sm font-medium">{{ finding.title }}</h4></div>
          <p class="text-sm">{{ finding.suggestion }}</p>
          <div v-if="finding.mods.length" class="flex flex-wrap gap-2">
            <a v-for="mod in finding.mods" :key="mod.id" :href="`https://steamcommunity.com/sharedfiles/filedetails/?id=${mod.id.slice(9)}`" target="_blank" rel="noopener noreferrer" class="text-sm text-primary underline underline-offset-4">{{ mod.name || mod.id }}</a>
          </div>
          <details class="min-w-0 text-sm"><summary class="cursor-pointer text-muted-foreground">原始日志依据</summary><pre class="mt-2 whitespace-pre-wrap break-all font-mono text-xs">{{ finding.evidence.join('\n') }}</pre></details>
        </article>
      </section>
    </DialogContent>
  </Dialog>
</template>
