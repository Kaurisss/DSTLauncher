<script setup>
// 单个世界设置项，按游戏内「世界设置」界面的单元格布局呈现：
// 左侧图标、中间标签与当前值、两侧箭头翻值。
//
// 有取值列表的靠左右箭头循环切换；取值由游戏运行时生成的（task_set、
// start_location 等）无法穷举，只能用输入框让用户自己填。
// 改过的项在标签旁点一个小圆点，方便在两百多项里一眼看出自己改了什么。

import { computed, ref } from 'vue';

import { iconOf } from '@/lib/worldIcons.js';
import { Input } from '@/components/ui/input';

const props = defineProps({
  option: { type: Object, required: true },
  value: { type: String, default: '' },
  changed: { type: Boolean, default: false },
  /** 服务器运行中或选项只读：控件不可操作 */
  disabled: { type: Boolean, default: false },
});

const emit = defineEmits(['change']);

const inputId = computed(() => 'opt-' + props.option.name);
const icon = computed(() => iconOf(props.option.name));
const iconFailed = ref(false);

const values = computed(() => props.option.values || []);
const hasValues = computed(() => values.value.length > 0);
const isDynamic = computed(() => !hasValues.value);

/** 当前值在列表里的位置；不在列表里（游戏更新改名）时返回 -1 */
const index = computed(() => values.value.findIndex(v => v.data === props.value));

/** 显示用的取值文字：优先中文标签，缺失时回落到原始值 */
const currentLabel = computed(() => {
  const hit = values.value.find(v => v.data === props.value);
  if (hit) return hit.label || hit.data;
  return props.value || '—';
});

/** 左右翻值。到头绕回，和游戏里的箭头一致。 */
function step(delta) {
  if (props.disabled || !hasValues.value) return;
  const n = values.value.length;
  const from = index.value < 0 ? 0 : index.value;
  const next = (((from + delta) % n) + n) % n;
  emit('change', values.value[next].data);
}

function onInput(e) {
  if (props.disabled) return;
  emit('change', e.target.value);
}
</script>

<template>
  <div class="opt-row" :class="{ 'is-changed': changed, 'is-readonly': disabled }">
    <!-- 图标位常驻：即使某张图缺失，单元格布局也不会变形 -->
    <span class="opt-row-icon">
      <img
        v-if="icon && !iconFailed"
        :src="icon"
        :alt="''"
        loading="lazy"
        @error="iconFailed = true"
      >
    </span>

    <template v-if="isDynamic">
      <div class="opt-row-main">
        <label class="opt-row-label" :for="inputId">
          {{ option.label || option.name }}
          <span v-if="changed" class="opt-row-dot" title="已修改" />
        </label>
        <Input
          :id="inputId"
          class="mono opt-row-input"
          :model-value="value"
          :disabled="disabled"
          autocomplete="off"
          placeholder="按游戏内的名称填写"
          @update:model-value="onInput"
        />
      </div>
    </template>

    <template v-else>
      <button
        type="button"
        class="opt-arrow"
        :disabled="disabled"
        :aria-label="`${option.label || option.name} 上一项`"
        @click="step(-1)"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 5l-7 7 7 7z" /></svg>
      </button>

      <div class="opt-row-main">
        <span class="opt-row-label" :id="inputId">
          {{ option.label || option.name }}
          <span v-if="changed" class="opt-row-dot" title="已修改" />
        </span>
        <span class="opt-row-value">{{ currentLabel }}</span>
      </div>

      <button
        type="button"
        class="opt-arrow"
        :disabled="disabled"
        :aria-label="`${option.label || option.name} 下一项`"
        @click="step(1)"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 5l7 7-7 7z" /></svg>
      </button>
    </template>
  </div>
</template>
