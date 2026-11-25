<script setup>
import { computed } from 'vue';
import { PhArrowCounterClockwise } from '@phosphor-icons/vue';

import OptionField from '@/components/world/OptionField.vue';

const props = defineProps({
  group: { type: Object, required: true },
  valueOf: { type: Function, required: true },
  isChanged: { type: Function, required: true },
  disabled: { type: Boolean, default: false },
});

const emit = defineEmits(['change', 'reset-group']);

const changedInGroup = computed(() =>
  props.group.options.filter(option => props.isChanged(option)).length);
</script>

<template>
  <section class="opt-group" aria-labelledby="world-group-title">
    <div class="opt-group-head">
      <div class="opt-group-meta">
        <h3 id="world-group-title" class="opt-group-label">{{ group.label || group.id }}</h3>
        <span class="opt-group-count">{{ group.options.length }} 项</span>
        <span v-if="changedInGroup" class="opt-group-changed">· {{ changedInGroup }} 项已改</span>
      </div>
      <button
        v-if="changedInGroup && !disabled"
        type="button"
        class="opt-group-reset"
        @click="emit('reset-group', group)"
      >
        <PhArrowCounterClockwise class="size-3.5" aria-hidden="true" />
        <span>恢复本组默认</span>
      </button>
    </div>
    <div class="opt-group-body">
      <OptionField
        v-for="o in group.options"
        :key="o.name"
        :option="o"
        :value="valueOf(o)"
        :changed="isChanged(o)"
        :disabled="disabled"
        @change="v => emit('change', o, v)"
      />
    </div>
  </section>
</template>
