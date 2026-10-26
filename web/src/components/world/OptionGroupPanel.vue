<script setup>
import OptionField from '@/components/world/OptionField.vue';

defineProps({
  group: { type: Object, required: true },
  valueOf: { type: Function, required: true },
  isChanged: { type: Function, required: true },
  disabled: { type: Boolean, default: false },
});

const emit = defineEmits(['change']);
</script>

<template>
  <section class="opt-group" aria-labelledby="world-group-title">
    <div class="opt-group-head">
      <h3 id="world-group-title" class="opt-group-label">{{ group.label || group.id }}</h3>
      <span class="opt-group-count">{{ group.options.length }} 项</span>
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
