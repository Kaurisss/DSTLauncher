<script setup>
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue';
import { useElementSize } from '@vueuse/core';
import { PopoverContent, PopoverPortal, PopoverRoot, PopoverTrigger } from 'reka-ui';
import IconAction from '@/components/ui/IconAction.vue';

defineProps({
  label: { type: String, required: true },
  active: { type: Boolean, default: false },
});

const anchor = useTemplateRef('anchor');
const toolbar = ref(null);
const open = ref(false);
const { width } = useElementSize(toolbar);
const compact = computed(() => width.value < 720);

onMounted(() => { toolbar.value = anchor.value.closest('.ui-toolbar, .mods-toolbar'); });
watch(compact, value => {
  if (!value) open.value = false;
});
</script>

<template>
  <div ref="anchor" class="shrink-0">
    <PopoverRoot v-if="compact" v-model:open="open">
      <PopoverTrigger as-child>
        <IconAction icon="sliders" :label="label" :variant="active ? 'secondary' : 'outline'" />
      </PopoverTrigger>
      <PopoverPortal>
        <PopoverContent :aria-label="label" align="start" :side-offset="8" :collision-padding="16" class="toolbar-filters-popover grid w-64 gap-3 rounded-xl border border-border bg-popover p-4 text-popover-foreground shadow-md outline-none">
          <slot :compact="true" />
        </PopoverContent>
      </PopoverPortal>
    </PopoverRoot>
    <div v-else class="flex items-center gap-2"><slot :compact="false" /></div>
  </div>
</template>
