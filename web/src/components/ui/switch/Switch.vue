<script setup>
import { SwitchRoot, SwitchThumb, useForwardPropsEmits } from "reka-ui";
import { cn } from "cn";

const props = defineProps({
  defaultChecked: { type: Boolean, required: false },
  checked: { type: [Boolean, String], required: false },
  disabled: { type: Boolean, required: false },
  required: { type: Boolean, required: false },
  name: { type: String, required: false },
  value: { type: String, required: false },
  id: { type: String, required: false },
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false },
  class: { type: [Boolean, null, String, Object, Array], required: false },
});

const emits = defineEmits(["update:checked"]);
const forwarded = useForwardPropsEmits(props, emits);
</script>

<template>
  <SwitchRoot
    v-bind="forwarded"
    data-slot="switch"
    :class="
      cn(
        'peer focus-visible:border-ring focus-visible:ring-ring/50 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input/80 dark:data-[state=unchecked]:bg-input/40 focus-visible:ring-3 inline-flex h-5 w-9 shrink-0 items-center rounded-full border-2 border-transparent shadow-xs transition-colors outline-none disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer',
        props.class,
      )
    "
  >
    <SwitchThumb
      :class="
        cn(
          'bg-background pointer-events-none block size-4 rounded-full ring-0 shadow-sm transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0',
        )
      "
    />
  </SwitchRoot>
</template>
