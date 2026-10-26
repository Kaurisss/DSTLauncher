<script setup>
import { HugeiconsIcon } from "@hugeicons/vue";
import { Tick02Icon, Copy01Icon } from "@hugeicons/core-free-icons";
import { useClipboard } from "@vueuse/core";
import { cn } from "cn";
import { Button } from '@/components/ui/button';

defineOptions({ inheritAttrs: false });

const props = defineProps({
  asChild: { type: Boolean, required: false },
  as: { type: null, required: false, default: "button" },
  value: { type: String, required: true },
  variant: { type: null, required: false, default: "ghost" },
  size: { type: null, required: false, default: "icon-xs" },
  class: {
    type: [Boolean, null, String, Object, Array],
    required: false,
    skipCheck: true,
  },
});

const { copy, copied } = useClipboard({ copiedDuring: 2000, legacy: true });
</script>

<template>
  <Button
    v-bind="$attrs"
    data-slot="code-block-copy-button"
    :data-copied="copied || undefined"
    :as="as"
    :as-child="asChild"
    :variant="variant"
    :size="size"
    :aria-label="copied ? 'Copied' : 'Copy'"
    :class="cn('', props.class)"
    @click="copy(props.value)"
  >
    <HugeiconsIcon v-if="copied" :icon="Tick02Icon" />
    <HugeiconsIcon v-else :icon="Copy01Icon" />
  </Button>
  <span aria-live="polite" class="sr-only">{{ copied ? "Copied" : "" }}</span>
</template>
