<script setup>
import { onUnmounted, ref, watch } from 'vue';
import UiCard from '@/components/ui/UiCard.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { worldStateApi } from '@/lib/api.js';
import { shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();
const items = ref([]);
const error = ref('');
const loading = ref(false);
const seasons = { spring: '春季', summer: '夏季', autumn: '秋季', winter: '冬季', mild: '温和季', wet: '飓风季', hurricane: '飓风季', dry: '旱季', green: '绿季', lush: '繁茂季', temperate: '温和季' };
let timer;
let generation = 0;
async function refresh(id, requestGeneration) {
  try {
    const data = await worldStateApi.get(id);
    if (requestGeneration !== generation) return;
    items.value = data || [];
    error.value = '';
  } catch (err) {
    if (requestGeneration !== generation) return;
    items.value = [];
    error.value = err.message;
  } finally {
    if (requestGeneration === generation) {
      loading.value = false;
      timer = setTimeout(() => refresh(id, requestGeneration), 15000);
    }
  }
}
watch(() => rooms.current?.id, id => {
  generation++;
  clearTimeout(timer);
  items.value = [];
  error.value = '';
  loading.value = !!id;
  if (id) refresh(id, generation);
}, { immediate: true });
onUnmounted(() => { generation++; clearTimeout(timer); });
</script>

<template>
  <UiCard v-if="rooms.current">
    <template #title>世界状态</template>
    <section aria-label="世界状态" class="grid gap-3 sm:grid-cols-2">
      <p v-if="loading" role="status" class="text-sm text-muted-foreground">读取存档中…</p>
      <p v-else-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
      <p v-else-if="!items.length" class="text-sm text-muted-foreground">尚无世界快照</p>
      <div v-for="item in items" :key="item.shard" class="grid gap-1.5 rounded-lg border border-border/80 bg-muted/30 p-3 text-sm">
        <div class="flex flex-wrap items-baseline justify-between gap-2">
          <h3 class="font-semibold">{{ shardLabel(item.shard, rooms.current?.locations?.[item.shard]) }}</h3>
          <time v-if="item.state?.savedAt" :datetime="item.state.savedAt" class="text-xs text-muted-foreground">存档于 {{ new Date(item.state.savedAt).toLocaleString() }}</time>
        </div>
        <p v-if="item.error" role="status" class="text-xs text-destructive">读取失败：{{ item.error }}</p>
        <template v-else-if="item.state">
          <p class="flex flex-wrap items-center gap-x-2.5 gap-y-1">
            <span class="font-medium">{{ item.state.day == null ? '天数未知' : `第 ${item.state.day} 天` }}</span>
            <span class="rounded bg-background px-1.5 py-0.5 text-xs font-medium text-muted-foreground shadow-2xs">{{ seasons[item.state.season] || item.state.season || '季节未知' }}</span>
            <span v-if="item.state.remainingDays != null" class="text-xs text-muted-foreground">本季剩余 {{ item.state.remainingDays }} 天</span>
          </p>
        </template>
        <p v-else class="text-xs text-muted-foreground">尚无世界快照</p>
      </div>
    </section>
  </UiCard>
</template>
