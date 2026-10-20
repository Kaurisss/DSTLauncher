<script setup>
import { RadioGroupRoot, useForwardPropsEmits } from "reka-ui";
import { cn } from "cn";

const props = defineProps({
  defaultValue: { type: null, required: false },
  modelValue: { type: null, required: false },
  disabled: { type: Boolean, required: false },
  name: { type: String, required: false },
  required: { type: Boolean, required: false },
  orientation: { type: String, required: false },
  dir: { type: String, required: false },
  loop: { type: Boolean, required: false },
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false },
  class: { type: [Boolean, null, String, Object, Array], required: false },
});

const emits = defineEmits(["update:modelValue"]);
const forwarded = useForwardPropsEmits(props, emits);
</script>

<template>
  <RadioGroupRoot
    v-bind="forwarded"
    data-slot="radio-group"
    :class="cn('grid gap-3', props.class)"
  >
    <slot />
  </RadioGroupRoot>
</template>
