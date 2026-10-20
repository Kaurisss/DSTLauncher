<script setup>
import { Switch } from '@/components/ui/switch';

defineProps({
  modelValue: { type: Boolean, default: false },
  label: { type: String, default: '' },
  /** 服务器运行中等场景下冻结开关 */
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue']);
</script>

<template>
  <label class="inline-flex items-center gap-2 cursor-pointer select-none text-sm text-foreground">
    <Switch
      :checked="modelValue"
      :disabled="disabled"
      @update:checked="emit('update:modelValue', $event)"
    />
    <span v-if="label">{{ label }}</span>
  </label>
</template>
