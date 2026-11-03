<script setup>
import { cn } from '@/lib/utils';

defineProps({ leadingClass: { type: String, default: '' } });
</script>

<template>
  <div role="region" class="ui-toolbar relative z-10 min-w-0 shrink-0 rounded-t-xl bg-card p-3">
    <div class="toolbar-layout flex min-w-0 flex-wrap items-center justify-between gap-3">
      <div :class="cn('toolbar-leading flex min-w-0 flex-1 items-center gap-2', leadingClass)"><slot /></div>
      <div v-if="$slots.actions" class="ml-auto flex shrink-0 flex-wrap items-center gap-2"><slot name="actions" /></div>
    </div>
  </div>
</template>
