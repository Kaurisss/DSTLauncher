<script setup>
import { cn } from "cn";

const props = defineProps({
  class: { type: null, required: false },
});
</script>

<template>
  <p
    data-slot="empty-description"
    :class="
      cn(
        'text-sm/relaxed text-muted-foreground [&>a]:underline [&>a]:underline-offset-4 [&>a:hover]:text-primary',
        props.class,
      )
    "
  >
    <slot />
  </p>
</template>
