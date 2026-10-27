<script setup>
// 表单一项：标签 + 控件（默认插槽）。默认标签在左、控件在右；
// block 让标签叠在控件上方（常规表单布局，所有控件的左边缘因此严格对齐）。
// hint 是标签右侧的灰色副标题，用来放一行说明，避免说明文字挤占标签列。
//
// 注意：属性 id 对应的 prop 必须叫 forId 而不是 for——
// for 是 JS 保留字，写成 :for="for" 模板表达式无法解析。
defineProps({
  label: { type: String, default: '' },
  forId: { type: String, default: '' },
  hint: { type: String, default: '' },
  tip: { type: String, default: '' },
  wide: { type: Boolean, default: false },
  block: { type: Boolean, default: false },
});
</script>

<template>
  <div class="field" :class="{ 'field-wide': wide, 'field-block': block }">
    <div v-if="label || hint" class="field-label-row">
      <label v-if="label" class="field-label" :for="forId">{{ label }}</label>
      <span v-if="hint" class="field-hint">{{ hint }}</span>
    </div>
    <slot />
    <p v-if="tip" class="field-tip">{{ tip }}</p>
  </div>
</template>
