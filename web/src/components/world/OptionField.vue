<script setup>
import { computed, ref, watch } from 'vue';
import { PhArrowCounterClockwise, PhCaretLeft, PhCaretRight, PhSliders } from '@phosphor-icons/vue';

import { iconOf } from '@/lib/worldIcons.js';
import { Input } from '@/components/ui/input';

const props = defineProps({
  option: { type: Object, required: true },
  value: { type: String, default: '' },
  changed: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
});

const emit = defineEmits(['change']);

const inputId = computed(() => 'opt-' + props.option.name);
const label = computed(() => props.option.label || props.option.name);
const icon = computed(() => iconOf(props.option.name));
const iconFailed = ref(false);
watch(icon, () => { iconFailed.value = false; });
const values = computed(() => props.option.values || []);
const hasValues = computed(() => values.value.length > 0);
const index = computed(() => values.value.findIndex(v => v.data === props.value));
const currentLabel = computed(() => {
  const hit = values.value[index.value];
  return hit ? hit.label || hit.data : props.value || '—';
});
const showStepTrack = computed(() => hasValues.value && values.value.length >= 2 && values.value.length <= 7);

function step(delta) {
  if (props.disabled || !hasValues.value) return;
  const n = values.value.length;
  const from = index.value < 0 ? 0 : index.value;
  emit('change', values.value[(((from + delta) % n) + n) % n].data);
}

function onStepKeydown(event) {
  if (event.key === 'ArrowLeft') {
    event.preventDefault();
    step(-1);
  } else if (event.key === 'ArrowRight') {
    event.preventDefault();
    step(1);
  }
}

function onInput(value) {
  if (props.disabled) return;
  emit('change', value);
}
</script>

<template>
  <div class="opt-row" :class="{ 'is-changed': changed, 'is-readonly': disabled }">
    <span class="opt-row-icon" :class="{ 'is-fallback': !icon || iconFailed }">
      <img
        v-if="icon && !iconFailed"
        :src="icon"
        alt=""
        width="56"
        height="56"
        decoding="sync"
        @error="iconFailed = true"
      />
      <PhSliders v-else class="opt-row-fallback-icon" aria-hidden="true" />
    </span>

    <template v-if="hasValues">
      <div class="opt-row-main">
        <span :id="inputId" class="opt-row-label" :title="label">{{ label }}</span>
        <span class="opt-row-value" :title="currentLabel">{{ currentLabel }}</span>
        <span v-if="showStepTrack" class="opt-step-track" aria-hidden="true">
          <span
            v-for="(item, idx) in values"
            :key="item.data"
            class="opt-step-pip"
            :class="{ 'is-current': idx === index }"
          />
        </span>
      </div>
      <div class="opt-step-controls" @keydown="onStepKeydown">
        <button
          v-if="changed"
          class="opt-row-revert"
          type="button"
          :disabled="disabled"
          :aria-label="label + ' 恢复默认'"
          title="恢复默认"
          @click="onInput(option.default)"
        >
          <PhArrowCounterClockwise aria-hidden="true" />
        </button>
        <button type="button" class="opt-arrow" :disabled="disabled" :aria-label="label + ' 上一项'" @click="step(-1)">
          <PhCaretLeft aria-hidden="true" />
        </button>
        <button type="button" class="opt-arrow" :disabled="disabled" :aria-label="label + ' 下一项'" @click="step(1)">
          <PhCaretRight aria-hidden="true" />
        </button>
      </div>
    </template>

    <template v-else>
      <div class="opt-row-main">
        <label class="opt-row-label" :for="inputId" :title="label">{{ label }}</label>
        <Input :id="inputId" class="mono opt-row-input" :model-value="value" :disabled="disabled" autocomplete="off" placeholder="按游戏内的名称填写" @update:model-value="onInput" />
      </div>
      <button
        v-if="changed"
        class="opt-row-revert opt-row-revert-standalone"
        type="button"
        :disabled="disabled"
        :aria-label="label + ' 恢复默认'"
        title="恢复默认"
        @click="onInput(option.default)"
      >
        <PhArrowCounterClockwise aria-hidden="true" />
      </button>
    </template>
  </div>
</template>
