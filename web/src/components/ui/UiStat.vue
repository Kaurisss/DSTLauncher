<script setup>
// 指标卡：大数字 + 说明。
defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], required: true },
  sub: { type: String, default: '' },
  /** '' | ok | warn | danger | info */
  tone: { type: String, default: '' },
});
</script>

<template>
  <div class="stat" :class="tone ? 'is-' + tone : ''">
    <div class="stat-body flex-1">
      <div class="stat-label">{{ label }}</div>
      <div class="stat-value">{{ value }}</div>
      <div v-if="sub" class="stat-sub">{{ sub }}</div>
      <slot />
    </div>
  </div>
</template>
