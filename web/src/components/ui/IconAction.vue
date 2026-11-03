<script setup>
import { computed, useAttrs, useTemplateRef } from 'vue';
import { Button } from '@/components/ui/button';
import Icon from '@/components/ui/Icon.vue';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

defineOptions({ inheritAttrs: false });

defineProps({
  label: { type: String, required: true },
  icon: { type: String, required: true },
  tip: { type: String, default: '' },
  iconClass: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  variant: { type: String, default: undefined },
  type: { type: String, default: 'button' },
  as: { type: String, default: 'button' },
});

const attrs = useAttrs();
const buttonAttrs = computed(() => {
  const { class: _class, ...rest } = attrs;
  return rest;
});
const button = useTemplateRef('button');

function focus() {
  button.value?.$el?.focus?.();
}

defineExpose({ focus });
</script>

<template>
  <TooltipProvider>
    <Tooltip>
      <TooltipTrigger
        as="span"
        :as-child="!disabled"
        class="inline-flex"
        :class="attrs.class"
        :tabindex="disabled ? 0 : undefined"
      >
        <Button
          ref="button"
          :as="as"
          :type="as === 'button' ? type : undefined"
          :variant="variant"
          size="icon-sm"
          :aria-label="label"
          :disabled="disabled"
          v-bind="buttonAttrs"
        >
          <Icon :name="icon" :size="16" :class="iconClass" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{{ tip || label }}</TooltipContent>
    </Tooltip>
  </TooltipProvider>
</template>
