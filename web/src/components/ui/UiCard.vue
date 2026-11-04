<script setup>
// 标题行与房间编辑工具栏同一套：左标题、右操作，不再用整行分隔线。
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';

defineProps({
  flush: { type: Boolean, default: false },
  fill: { type: Boolean, default: false },
});
</script>

<template>
  <Card :class="cn('min-h-0 gap-0 py-0', fill ? 'flex-1 overflow-hidden' : 'overflow-visible')">
    <CardHeader v-if="$slots.title || $slots.actions" class="flex flex-row flex-wrap items-center justify-between gap-3 bg-card p-3">
      <div v-if="$slots.title || $slots.hint" class="flex min-w-0 flex-col gap-0.5">
        <CardTitle v-if="$slots.title" role="heading" aria-level="2" class="text-sm font-semibold"><slot name="title" /></CardTitle>
        <CardDescription v-if="$slots.hint"><slot name="hint" /></CardDescription>
      </div>
      <div v-if="$slots.actions" class="ml-auto flex flex-wrap items-center gap-2"><slot name="actions" /></div>
    </CardHeader>
    <CardContent :class="cn('min-h-0 flex-1', fill && 'flex flex-col overflow-hidden', flush ? 'p-0' : 'px-3 pt-0 pb-3')"><slot /></CardContent>
  </Card>
</template>
