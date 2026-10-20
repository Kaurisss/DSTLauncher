<script setup>
import { RadioGroupIndicator, RadioGroupItem, useForwardProps } from "reka-ui";
import { cn } from "cn";

const props = defineProps({
  value: { type: [String, Number, Boolean], required: true },
  disabled: { type: Boolean, required: false },
  required: { type: Boolean, required: false },
  name: { type: String, required: false },
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false },
  class: { type: [Boolean, null, String, Object, Array], required: false },
});

const forwarded = useForwardProps(props);
</script>

<template>
  <RadioGroupItem
    v-bind="forwarded"
    data-slot="radio-group-item"
    :class="
      cn(
        'border-input text-primary focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive dark:aria-invalid:border-destructive/50 aspect-square size-4 shrink-0 rounded-full border shadow-xs transition-shadow focus-visible:ring-3 aria-invalid:ring-3 outline-none disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  >
    <RadioGroupIndicator
      data-slot="radio-group-indicator"
      class="relative flex items-center justify-center after:block after:size-2 after:rounded-full after:bg-primary"
    />
  </RadioGroupItem>
</template>
