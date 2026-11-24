<script setup>
// 空态：默认文案走插槽，便于写多行说明。
defineProps({
  text: { type: String, default: '' },
});
</script>

<template>
  <div class="empty">
    <span><slot>{{ text }}</slot></span>
  </div>
</template>
