<script setup>
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue';
import { useElementSize } from '@vueuse/core';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
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
    <Popover v-if="compact" v-model:open="open">
      <PopoverTrigger as-child>
        <IconAction icon="sliders" :label="label" :variant="active ? 'secondary' : 'outline'" />
      </PopoverTrigger>
      <PopoverContent :aria-label="label" align="start" :side-offset="8" :collision-padding="16" class="toolbar-filters-popover grid w-64 gap-3 rounded-xl p-4">
        <slot :compact="true" />
      </PopoverContent>
    </Popover>
    <div v-else class="flex items-center gap-2"><slot :compact="false" /></div>
  </div>
</template>
