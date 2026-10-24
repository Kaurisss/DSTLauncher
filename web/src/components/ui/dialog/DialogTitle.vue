<script setup>
import { reactiveOmit } from "@vueuse/core";
import { cn } from "cn";
import { DialogTitle } from "reka-ui";

const props = defineProps({
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false },
  class: {
    type: [Boolean, null, String, Object, Array],
    required: false,
    skipCheck: true,
  },
});

const delegatedProps = reactiveOmit(props, "class");
</script>

<template>
  <DialogTitle
    data-slot="dialog-title"
    v-bind="delegatedProps"
    :class="cn('text-base font-medium leading-none cn-font-heading', props.class)"
  >
    <slot />
  </DialogTitle>
</template>
