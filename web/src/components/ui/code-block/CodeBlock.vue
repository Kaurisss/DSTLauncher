<script setup>
import { cn } from "cn";
import { codeBlockVariants } from ".";

const props = defineProps({
  variant: { type: null, required: false, default: "default" },
  size: { type: String, required: false, default: "default" },
  class: {
    type: [Boolean, null, String, Object, Array],
    required: false,
    skipCheck: true,
  },
});
</script>

<template>
  <div
    data-slot="code-block"
    :data-variant="variant"
    :data-size="size"
    :class="cn(codeBlockVariants({ variant }), props.class)"
  >
    <slot />
  </div>
</template>
