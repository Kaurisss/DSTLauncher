<script setup>
import { computed, ref, watch } from 'vue';
import { PhCaretRight } from '@phosphor-icons/vue';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import IconAction from '@/components/ui/IconAction.vue';
import ConfigPreviewDialog from '@/components/config/ConfigPreviewDialog.vue';
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
    <DialogTrigger as-child><IconAction icon="diagnostics" variant="outline" show-label label="故障诊断" :disabled="!rooms.current" /></DialogTrigger>
    <DialogContent class="max-h-[85dvh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>故障诊断</DialogTitle>
        <DialogDescription>检查当前配置、各分片最近 400 行日志及共享模组下载记录。日志问题可能来自此前的启动。</DialogDescription>
      </DialogHeader>
      <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
        <span class="min-w-0 truncate font-medium">{{ rooms.current?.name }}</span>
        <div class="flex shrink-0 items-center gap-2">
          <ConfigPreviewDialog show-label="sm" />
          <IconAction icon="refresh" variant="outline" show-label :label="busy ? '诊断中…' : '重新诊断'" :disabled="busy" @click="refresh" />
        </div>
      </div>
      <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
      <div v-else-if="busy && !report" role="status" aria-label="正在读取最近日志" class="grid gap-3">
        <div v-for="i in 3" :key="i" class="grid gap-2 border-t pt-3">
          <Skeleton class="h-4 w-24" />
          <Skeleton class="h-14 w-full rounded-lg" />
        </div>
      </div>
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
          <Collapsible class="min-w-0 text-sm">
            <CollapsibleTrigger class="group flex cursor-pointer items-center gap-1 text-muted-foreground hover:text-foreground">
              <PhCaretRight class="size-3.5 transition-transform group-data-[state=open]:rotate-90" aria-hidden="true" />
              <span>原始日志依据</span>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <pre class="mt-2 whitespace-pre-wrap break-all font-mono text-xs">{{ finding.evidence.join('\n') }}</pre>
            </CollapsibleContent>
          </Collapsible>
        </article>
      </section>
    </DialogContent>
  </Dialog>
</template>
