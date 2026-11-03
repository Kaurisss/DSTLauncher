<script setup>
import { computed, ref } from 'vue';
import Icon from '@/components/ui/Icon.vue';
import { Badge } from '@/components/ui/badge';
import IconAction from '@/components/ui/IconAction.vue';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { workshopApi } from '@/lib/api.js';
import { fmtSize } from '@/lib/format.js';

const props = defineProps({
  open: { type: Boolean, default: false },
  mods: { type: Array, default: () => [] },
  busyIds: { type: Array, default: () => [] },
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(['update:open', 'download', 'enable', 'closed']);
const keyword = ref('');
const searching = ref(false);
const results = ref([]);
const searched = ref(false);
const error = ref('');
const page = ref(1);
const queried = ref('');
const idQuery = ref(false);
const showKeyForm = ref(false);
const apiKeyInput = ref('');
const savingKey = ref(false);
const keyError = ref('');
const keySaved = ref(false);
const currentMods = computed(() => new Map(props.mods.map(mod => [mod.id.replace(/^workshop-/, ''), mod])));
const looksLikeId = computed(() => /^(?:\d[\d\s,]*|workshop-\d+)$|[?&]id=\d+/.test(keyword.value.trim()));

async function saveKey() {
  if (savingKey.value) return;
  savingKey.value = true;
  keyError.value = '';
  try {
    await workshopApi.saveKey(apiKeyInput.value.trim());
    apiKeyInput.value = '';
    showKeyForm.value = false;
    keySaved.value = true;
    if (keyword.value.trim() && /key/i.test(error.value)) await query();
  } catch (err) {
    keyError.value = err.message;
  } finally {
    savingKey.value = false;
  }
}

async function query(nextPage) {
  const fresh = nextPage === undefined;
  const targetPage = nextPage ?? 1;
  const value = fresh ? keyword.value.trim() : queried.value;
  if (!value || searching.value) return;
  searching.value = true;
  error.value = '';
  try {
    const byId = fresh ? looksLikeId.value : idQuery.value;
    const list = byId ? await workshopApi.details(value) : await workshopApi.search(value, targetPage);
    results.value = list || [];
    queried.value = value;
    idQuery.value = byId;
    page.value = targetPage;
    searched.value = true;
  } catch (err) {
    error.value = err.message;
    if (!looksLikeId.value && /key/i.test(err.message)) showKeyForm.value = true;
  } finally {
    searching.value = false;
  }
}
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="flex min-h-0 flex-col overflow-hidden p-0 sm:max-w-3xl" :aria-describedby="undefined" @close-auto-focus.prevent="emit('closed')">
      <DialogHeader class="shrink-0 border-b p-4 pr-14 sm:p-6">
        <DialogTitle>添加模组</DialogTitle>
      </DialogHeader>
      <div class="min-h-0 flex-1 overflow-y-auto p-4 sm:px-6">
        <form class="flex gap-2" @submit.prevent="query()">
          <Input v-model="keyword" class="min-w-0 flex-1" aria-label="搜索创意工坊" placeholder="模组名称、ID 或创意工坊链接" />
          <IconAction type="submit" icon="search" :label="searching ? '查询中…' : looksLikeId ? '查询' : '搜索'" :disabled="searching || !keyword.trim()" />
        </form>
        <div class="my-3 flex items-center justify-between gap-2">
          <IconAction variant="ghost" icon="key" label="Steam API Key" :aria-expanded="showKeyForm" @click="showKeyForm = !showKeyForm" />
          <span v-if="keySaved" role="status" class="text-sm text-muted-foreground">API Key 已保存</span>
        </div>
        <form v-if="showKeyForm" class="mb-4 grid gap-2 rounded-lg border bg-muted/30 p-3" @submit.prevent="saveKey">
          <Label for="workshop-api-key">Steam Web API Key</Label>
          <div class="flex gap-2">
            <Input id="workshop-api-key" v-model="apiKeyInput" type="password" autocomplete="off" class="min-w-0 flex-1" placeholder="输入新的 API Key，留空可清除" />
            <IconAction variant="outline" type="submit" icon="save" :label="savingKey ? '保存中…' : '保存'" :disabled="savingKey" />
          </div>
          <p v-if="keyError" role="alert" class="text-sm text-destructive">{{ keyError }}</p>
        </form>
        <p v-if="error" role="alert" class="mb-4 break-words text-sm text-destructive">{{ error }}</p>
        <div v-if="searching" role="status" class="py-8 text-center text-sm text-muted-foreground">正在查询创意工坊…</div>
        <div v-else-if="results.length && !error" class="grid gap-3">
          <article v-for="item in results" :key="item.id" class="workshop-item flex min-w-0 items-center gap-3 rounded-lg border bg-background p-3">
            <div class="grid size-14 shrink-0 place-items-center overflow-hidden rounded-lg bg-muted" aria-hidden="true">
              <img v-if="item.previewUrl" :src="item.previewUrl" class="size-full object-cover" loading="lazy" decoding="async" alt="" @error="$event.target.style.display = 'none'" />
              <Icon v-else name="package" :size="24" />
            </div>
            <div class="min-w-0 flex-1">
              <a :href="`https://steamcommunity.com/sharedfiles/filedetails/?id=${item.id}`" target="_blank" rel="noopener noreferrer" class="break-words text-sm font-medium text-foreground underline-offset-4 hover:underline">{{ item.title || item.id }}</a>
              <p v-if="item.shortDesc" class="mt-1 line-clamp-2 break-words text-xs text-muted-foreground">{{ item.shortDesc }}</p>
              <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                <code>{{ item.id }}</code>
                <span v-if="item.fileSize">{{ fmtSize(item.fileSize) }}</span>
                <Badge v-if="currentMods.get(item.id)?.enabled" variant="secondary">已启用</Badge>
              </div>
            </div>
            <IconAction
              v-if="currentMods.get(item.id)?.installed || item.installed"
              variant="outline"
              icon="check"
              :label="currentMods.get(item.id)?.enabled ? '已启用' : '启用'"
              :disabled="disabled || busyIds.includes('workshop-' + item.id) || currentMods.get(item.id)?.enabled"
              @click="emit('enable', 'workshop-' + item.id)"
            />
            <IconAction v-else icon="download" :label="busyIds.includes('workshop-' + item.id) ? '已加入队列' : '下载'" :disabled="disabled || busyIds.includes('workshop-' + item.id)" @click="emit('download', { id: 'workshop-' + item.id, name: item.title, installed: false })" />
          </article>
        </div>
        <div v-else-if="searched && !error" class="py-10 text-center text-sm text-muted-foreground">没有查到结果</div>
        <div v-else-if="!error" class="grid justify-items-center gap-3 py-10 text-muted-foreground" aria-hidden="true"><Icon name="package" :size="32" /></div>
        <nav v-if="searched && !idQuery && !error && (page > 1 || results.length === 20)" aria-label="工坊分页" class="mt-4 flex items-center justify-end gap-2">
          <span class="mr-2 text-sm text-muted-foreground">第 {{ page }} 页</span>
          <IconAction variant="outline" icon="chevron" icon-class="rotate-180" label="上一页" :disabled="searching || page === 1" @click="query(page - 1)" />
          <IconAction variant="outline" icon="chevron" label="下一页" :disabled="searching || results.length < 20" @click="query(page + 1)" />
        </nav>
      </div>
      <div class="flex shrink-0 justify-end border-t p-4 sm:px-6"><IconAction variant="outline" icon="check" label="完成" @click="emit('update:open', false)" /></div>
    </DialogContent>
  </Dialog>
</template>
