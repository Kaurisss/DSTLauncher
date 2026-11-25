<script setup>
import { computed, useAttrs, useTemplateRef } from 'vue';
import { Button } from '@/components/ui/button';
import Icon from '@/components/ui/Icon.vue';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

defineOptions({ inheritAttrs: false });

const props = defineProps({
  label: { type: String, required: true },
  text: { type: String, default: '' },
  showLabel: { type: [Boolean, String], default: false },
  icon: { type: String, required: true },
  tip: { type: String, default: '' },
  iconClass: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  variant: { type: String, default: undefined },
  type: { type: String, default: 'button' },
  as: { type: String, default: 'button' },
});

const RESPONSIVE_BUTTON = {
  sm: 'w-7 px-0 sm:w-auto sm:px-2.5',
  md: 'w-7 px-0 md:w-auto md:px-2.5',
  lg: 'w-7 px-0 lg:w-auto lg:px-2.5',
  xl: 'w-7 px-0 xl:w-auto xl:px-2.5',
};

const RESPONSIVE_TEXT = {
  sm: 'hidden sm:inline',
  md: 'hidden md:inline',
  lg: 'hidden lg:inline',
  xl: 'hidden xl:inline',
};

const attrs = useAttrs();
const buttonAttrs = computed(() => {
  const { class: _class, ...rest } = attrs;
  return rest;
});
const displayText = computed(() => props.text || props.label);
const hasLabel = computed(() => props.showLabel === true || props.showLabel in RESPONSIVE_BUTTON);
const buttonSize = computed(() => (hasLabel.value ? 'sm' : 'icon-sm'));
const responsiveButtonClass = computed(() => (typeof props.showLabel === 'string' ? RESPONSIVE_BUTTON[props.showLabel] || '' : ''));
const labelClass = computed(() => (typeof props.showLabel === 'string' ? RESPONSIVE_TEXT[props.showLabel] || '' : ''));
const showTooltip = computed(() => props.showLabel !== true || !!props.tip || (!!props.text && props.text !== props.label));
const button = useTemplateRef('button');

function focus() {
  button.value?.$el?.focus?.();
}

defineExpose({ focus });
</script>

<template>
  <TooltipProvider v-if="showTooltip">
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
          :size="buttonSize"
          :class="responsiveButtonClass"
          :aria-label="label"
          :disabled="disabled"
          v-bind="buttonAttrs"
        >
          <Icon :name="icon" :size="hasLabel ? 14 : 16" :class="iconClass" />
          <span v-if="hasLabel" :class="labelClass">{{ displayText }}</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{{ tip || label }}</TooltipContent>
    </Tooltip>
  </TooltipProvider>
  <Button
    v-else
    ref="button"
    :as="as"
    :type="as === 'button' ? type : undefined"
    :variant="variant"
    :size="buttonSize"
    :class="attrs.class"
    :aria-label="label"
    :disabled="disabled"
    v-bind="buttonAttrs"
  >
    <Icon :name="icon" :size="14" :class="iconClass" />
    <span>{{ displayText }}</span>
  </Button>
</template>
