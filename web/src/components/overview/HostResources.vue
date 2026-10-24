<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue';

import UiStat from '@/components/ui/UiStat.vue';
import { hostApi } from '@/lib/api.js';
import { fmtSize } from '@/lib/format.js';

const snapshot = ref(null);
const error = ref('');
let disposed = false;
let timer;

const memory = computed(() => snapshot.value?.memory);
const disk = computed(() => snapshot.value?.disk);
const memoryUsed = computed(() => memory.value
  ? memory.value.totalBytes - memory.value.availableBytes : 0);
const memoryLow = computed(() => !!memory.value
  && memoryUsed.value / memory.value.totalBytes >= 0.9);
const diskLow = computed(() => !!disk.value
  && (disk.value.availableBytes / disk.value.totalBytes < 0.1
    || disk.value.availableBytes < 5 * 1024 ** 3));
const warnings = computed(() => [
  memoryLow.value ? '宿主机内存使用已达 90%，请减少运行分片或释放内存。' : '',
  diskLow.value ? '存档盘空间不足，请清理旧备份或扩容。' : '',
].filter(Boolean));
const errors = computed(() => [
  error.value,
  snapshot.value?.memoryError ? '内存读取失败：' + snapshot.value.memoryError : '',
  snapshot.value?.diskError ? '存档盘读取失败：' + snapshot.value.diskError : '',
].filter(Boolean));

async function refresh() {
  try {
    const data = await hostApi.stats();
    if (disposed) return;
    snapshot.value = data;
    error.value = '';
  } catch (err) {
    if (disposed) return;
    snapshot.value = null;
    error.value = '宿主机资源读取失败：' + err.message;
  } finally {
    // Schedule after completion: slow requests must not overlap.
    if (!disposed) timer = setTimeout(refresh, 5000);
  }
}

onMounted(refresh);
onUnmounted(() => {
  disposed = true;
  clearTimeout(timer);
});
</script>

<template>
  <section aria-label="宿主机资源" class="mb-4 grid gap-3">
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <UiStat
        label="宿主机内存"
        :value="memory ? ((memoryUsed / memory.totalBytes) * 100).toFixed(1) + '%' : '—'"
        :sub="memory ? '已用 ' + fmtSize(memoryUsed) + ' / ' + fmtSize(memory.totalBytes) : (errors.length ? '读取失败' : '读取中…')"
        icon="cpu"
        :tone="memoryLow ? 'warn' : ''"
      />
      <UiStat
        label="存档盘可用空间"
        :value="disk ? fmtSize(disk.availableBytes) : '—'"
        :sub="disk ? '总容量 ' + fmtSize(disk.totalBytes) : (errors.length ? '读取失败' : '读取中…')"
        :title="snapshot?.diskPath"
        icon="folder"
        :tone="diskLow ? 'warn' : ''"
      />
    </div>
    <div v-if="warnings.length || errors.length" role="status" class="grid gap-1 break-words text-sm text-amber-700 dark:text-amber-400">
      <p v-for="message in [...warnings, ...errors]" :key="message">{{ message }}</p>
    </div>
  </section>
</template>
