<script setup>
import { Spinner } from '@/components/ui/spinner';
</script>

<template>
  <div role="status" aria-label="加载中" class="flex min-h-32 items-center justify-center text-muted-foreground">
    <Spinner class="size-5 motion-reduce:animate-none" aria-hidden="true" />
  </div>
</template>
