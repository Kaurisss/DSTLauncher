<script setup>
// 保留 value 契约；负数由 Reka Progress 作为未知进度处理。
import { computed } from 'vue';
import { Progress } from '@/components/ui/progress';

const props = defineProps({
  value: { type: Number, default: 0 },
});
const value = computed(() => Number.isFinite(props.value) && props.value >= 0
  ? Math.min(100, props.value)
  : null);
</script>

<template>
  <Progress :model-value="value" :max="100" />
</template>
