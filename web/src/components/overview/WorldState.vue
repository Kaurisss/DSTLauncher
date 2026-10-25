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
  <UiCard v-if="rooms.current" class="mb-4">
    <template #title>世界状态</template>
    <section aria-label="世界状态" class="grid gap-3 sm:grid-cols-2">
      <p v-if="loading" role="status">读取存档中…</p>
      <p v-else-if="error" role="alert">{{ error }}</p>
      <div v-for="item in items" :key="item.shard" class="grid gap-1 text-sm">
        <h3 class="font-medium">{{ shardLabel(item.shard, rooms.current?.locations?.[item.shard]) }}</h3>
        <p v-if="item.error" role="status">读取失败：{{ item.error }}</p>
        <template v-else-if="item.state">
          <p class="flex flex-wrap gap-x-3">
            <span>{{ item.state.day == null ? '天数未知' : `第 ${item.state.day} 天` }}</span>
            <span>{{ seasons[item.state.season] || item.state.season || '季节未知' }}</span>
            <span v-if="item.state.remainingDays != null">本季剩余 {{ item.state.remainingDays }} 天</span>
          </p>
          <time :datetime="item.state.savedAt" class="text-muted-foreground">存档于 {{ new Date(item.state.savedAt).toLocaleString() }}</time>
        </template>
        <p v-else>尚无世界快照</p>
      </div>
    </section>
  </UiCard>
</template>
