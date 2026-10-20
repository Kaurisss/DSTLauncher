<script setup>
// 骨架屏：按真实测量尺寸（Playwright Audit）提供结构化占位，彻底消除布局跳跃（CLS）。
import { Skeleton } from '@/components/ui/skeleton';

defineProps({
  /** 行数 / 卡片数 */
  rows: { type: Number, default: 3 },
  /**
   * 骨架结构类型：
   * - 'default' | 'text': 通用文本行
   * - 'list' | 'mod': 模组/分片等列表项（实测 58px 高，含开关/名称/徽章/操作按钮）
   * - 'table': 备份/存档表格（实测 thead 40px + 每行 48px，含表头与操作按钮）
   * - 'options' | 'world': 世界配置项网格（复用真实选项的响应式布局）
   * - 'code' | 'editor': 配置文件/代码预览块（实测 313px 高）
   * - 'card': 房间卡片（实测 353x299px 卡片）
   * - 'form': 表单选项（实测标签 + 32px 输入框）
   */
  variant: { type: String, default: 'default' },
});

const WIDTHS = ['64%', '90%', '48%', '76%', '56%'];
const widthOf = i => WIDTHS[(i - 1) % WIDTHS.length];
</script>

<template>
  <!-- 模组 / 进程列表骨架：精确匹配 58px 高度的卡片项 -->
  <div v-if="variant === 'list' || variant === 'mod'" class="space-y-2.5">
    <div
      v-for="i in rows"
      :key="i"
      class="flex h-[58px] items-center gap-3 rounded-lg border border-border/50 bg-card/40 px-4 py-3 shadow-2xs"
    >
      <Skeleton class="h-5 w-9 shrink-0 rounded-full" />
      <Skeleton class="h-5 w-36 md:w-52" />
      <Skeleton class="h-5 w-14 rounded-full" />
      <Skeleton class="hidden h-5 w-14 rounded-full sm:block" />
      <div class="ml-auto flex items-center gap-2">
        <Skeleton class="h-7 w-16 rounded-md" />
        <Skeleton class="hidden h-7 w-14 rounded-md sm:block" />
      </div>
    </div>
  </div>

  <!-- 表格骨架（如备份、存档）：精确匹配 thead(40px) + tr(48px) -->
  <div v-else-if="variant === 'table'" class="w-full overflow-hidden rounded-lg border border-border/50 bg-card/30">
    <div class="flex h-10 items-center gap-4 border-b border-border/60 bg-muted/40 px-4">
      <Skeleton class="h-4 w-28" />
      <Skeleton class="h-4 w-20" />
      <Skeleton class="ml-auto h-4 w-24" />
    </div>
    <div
      v-for="i in rows"
      :key="i"
      class="flex h-12 items-center gap-4 border-b border-border/30 px-4 last:border-b-0"
    >
      <Skeleton class="h-4 w-36" />
      <Skeleton class="h-4 w-16" />
      <div class="ml-auto flex items-center gap-2">
        <Skeleton class="h-7 w-12 rounded-md" />
        <Skeleton class="h-7 w-12 rounded-md" />
        <Skeleton class="h-7 w-12 rounded-md" />
      </div>
    </div>
  </div>

  <!-- 复用真实选项的网格、方形图标位与窄屏尺寸，避免切换时布局跳动。 -->
  <div v-else-if="variant === 'options' || variant === 'world'" class="opt-group-body">
    <div v-for="i in rows" :key="i" class="opt-row">
      <Skeleton class="opt-row-icon rounded-none bg-border" />
      <div class="opt-row-main">
        <Skeleton class="h-4 w-28 max-w-full bg-border" />
        <Skeleton class="h-8 w-full max-w-40 rounded-lg bg-border" />
      </div>
    </div>
  </div>

  <!-- 代码/配置文件预览骨架：精确匹配 313px 高度文件预览卡片 -->
  <div v-else-if="variant === 'code' || variant === 'editor'" class="space-y-3">
    <div class="flex items-center gap-2 border-b border-border/50 pb-2">
      <Skeleton class="h-5 w-44" />
      <Skeleton class="h-5 w-16 rounded-full" />
    </div>
    <div class="space-y-2 rounded-lg border border-border/40 bg-muted/20 p-4 min-h-[260px]">
      <Skeleton class="h-4 w-3/4" />
      <Skeleton class="h-4 w-1/2" />
      <Skeleton class="h-4 w-5/6" />
      <Skeleton class="h-4 w-2/3" />
      <Skeleton class="h-4 w-4/5" />
      <Skeleton class="h-4 w-3/5" />
      <Skeleton class="h-4 w-2/4" />
    </div>
  </div>

  <!-- 房间卡片骨架：精确匹配 353x299px 卡片 -->
  <div v-else-if="variant === 'card'" class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
    <div
      v-for="i in rows"
      :key="i"
      class="flex h-[299px] flex-col justify-between rounded-xl border border-border/50 bg-card p-4 shadow-xs"
    >
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <Skeleton class="h-6 w-32" />
          <Skeleton class="h-5 w-16 rounded-full" />
        </div>
        <Skeleton class="h-4 w-48" />
        <Skeleton class="h-16 w-full rounded-lg" />
      </div>
      <div class="flex items-center gap-2 pt-4">
        <Skeleton class="h-8 flex-1 rounded-lg" />
        <Skeleton class="h-8 w-20 rounded-lg" />
      </div>
    </div>
  </div>

  <!-- 动态表单字段骨架（如模组配置）：标签 + 输入框 -->
  <div v-else-if="variant === 'form'" class="space-y-3">
    <div v-for="i in rows" :key="i" class="flex items-center justify-between gap-4 py-1">
      <Skeleton class="h-4 w-28" />
      <Skeleton class="h-8 w-44 rounded-lg" />
    </div>
  </div>

  <!-- 默认通用骨架：真实的行高 -->
  <div v-else class="space-y-3 py-1">
    <div v-for="i in rows" :key="i" class="flex items-center gap-3">
      <Skeleton class="size-2 rounded-full" />
      <Skeleton class="h-4 rounded-md" :style="{ width: widthOf(i) }" />
    </div>
  </div>
</template>
