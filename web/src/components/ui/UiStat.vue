<script setup>
// 指标卡：大数字 + 说明。
import { Skeleton } from '@/components/ui/skeleton';

defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], required: true },
  sub: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  /** '' | ok | warn | danger | info */
  tone: { type: String, default: '' },
});
</script>

<template>
  <div class="stat" :class="tone ? 'is-' + tone : ''">
    <div class="stat-body flex flex-1 flex-col">
      <div class="stat-label">{{ label }}</div>
      <div v-if="loading" role="status" aria-label="读取中" class="mt-1">
        <div class="stat-value flex items-center" style="height: 1.3em">
          <Skeleton class="h-4.5 w-16" />
        </div>
        <div class="stat-sub mt-0.5 flex items-center" style="height: 1.35em">
          <Skeleton class="h-3 w-28" />
        </div>
        <slot name="loading" />
      </div>
      <template v-else>
        <div class="stat-value mt-1">
          <slot name="value">{{ value }}</slot>
        </div>
        <div v-if="sub" class="stat-sub mt-0.5">{{ sub }}</div>
        <slot />
      </template>
    </div>
  </div>
</template>
