<script setup>
import { Badge } from '@/components/ui/badge';
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
      <CodeBlockHeader class="flex-wrap border-b border-border/75 bg-muted/35 px-3 py-2 group-data-[size=sm]/code-block:px-3 group-data-[size=sm]/code-block:pt-2">
        <CodeBlockTitle class="break-all whitespace-normal">{{ file.path }}</CodeBlockTitle>
        <Badge :variant="file.onDisk ? 'ok' : 'warn'">{{ file.onDisk ? '已落盘' : '未生成' }}</Badge>
        <Badge v-if="file.redacted" variant="muted">令牌已隐藏</Badge>
        <CodeBlockActions><CodeBlockCopyButton :value="file.content" /></CodeBlockActions>
      </CodeBlockHeader>
      <CodeBlockContent wrap line-numbers class="max-h-90" role="region" :aria-label="file.path">{{ file.content }}</CodeBlockContent>
    </CodeBlock>
  </div>
</template>
