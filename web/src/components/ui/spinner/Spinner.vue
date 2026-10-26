<script setup>
import { HugeiconsIcon } from "@hugeicons/vue";
import { Loading03Icon } from "@hugeicons/core-free-icons";
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
  <HugeiconsIcon
    :icon="Loading03Icon"
    role="status"
    aria-label="Loading"
    :class="cn('size-4 animate-spin', props.class)"
  />
</template>
