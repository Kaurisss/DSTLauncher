<script setup>
import { cn } from "cn";

const props = defineProps({
  class: {
    type: [Boolean, null, String, Object, Array],
    required: false,
    skipCheck: true,
  },
});
</script>

<template>
  <div
    data-slot="skeleton"
    :class="cn('bg-foreground/10 rounded-md animate-pulse', props.class)"
  />
</template>
