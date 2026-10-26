<script setup>
import UiPill from '@/components/ui/UiPill.vue';
import {
  CodeBlock, CodeBlockActions, CodeBlockContent, CodeBlockCopyButton,
  CodeBlockHeader, CodeBlockTitle,
} from '@/components/ui/code-block';

defineProps({
  files: { type: Array, required: true },
});
</script>

<template>
  <div class="grid min-w-0 gap-4">
    <CodeBlock v-for="file in files" :key="file.path" size="sm">
      <CodeBlockHeader class="flex-wrap">
        <CodeBlockTitle class="break-all whitespace-normal">{{ file.path }}</CodeBlockTitle>
        <UiPill :tone="file.onDisk ? 'ok' : 'warn'">{{ file.onDisk ? '已落盘' : '未生成' }}</UiPill>
        <UiPill v-if="file.redacted" tone="muted">令牌已隐藏</UiPill>
        <CodeBlockActions><CodeBlockCopyButton :value="file.content" /></CodeBlockActions>
      </CodeBlockHeader>
      <CodeBlockContent wrap line-numbers class="max-h-90" role="region" :aria-label="file.path">{{ file.content }}</CodeBlockContent>
    </CodeBlock>
  </div>
</template>
