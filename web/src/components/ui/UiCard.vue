<script setup>
// 保留旧页面的插槽与 flush 契约，内部使用新主题的 Card。
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { cn } from '@/lib/utils';

defineProps({
  flush: { type: Boolean, default: false },
  fill: { type: Boolean, default: false },
});
</script>

<template>
  <Card :class="cn('min-h-0 gap-0 py-0', fill ? 'flex-1 overflow-hidden' : 'overflow-visible')">
    <CardHeader v-if="$slots.title || $slots.actions" class="flex flex-wrap items-center gap-4 py-4">
      <div v-if="$slots.title || $slots.hint" class="flex min-w-0 flex-col gap-1">
        <CardTitle v-if="$slots.title" role="heading" aria-level="2"><slot name="title" /></CardTitle>
        <CardDescription v-if="$slots.hint"><slot name="hint" /></CardDescription>
      </div>
      <div v-if="$slots.actions" class="ml-auto flex flex-wrap items-center gap-2"><slot name="actions" /></div>
    </CardHeader>
    <Separator v-if="$slots.title || $slots.actions" />
    <CardContent :class="cn('min-h-0 flex-1', fill && 'flex flex-col overflow-hidden', flush ? 'p-0' : 'p-4 sm:p-6')"><slot /></CardContent>
  </Card>
</template>
