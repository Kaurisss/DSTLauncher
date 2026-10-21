<script setup>
// 骨架屏：直接复用真实内容的 CSS 类（.mod / .table / .opt / .file-body），
// 内边距、行高、圆角、行距都由真实样式决定，避免两处尺寸各改各的。
// 模组封面固定 72px，条目高度随操作区换行；通用列表行 56.5px，表头 34px、
// 表格行 52px，配置表单行 32px 高 / 行距 43px，文件预览体 313px 高。
import { Skeleton } from '@/components/ui/skeleton';

defineProps({
  /** 行数 / 卡片数 */
  rows: { type: Number, default: 3 },
  /**
   * 骨架结构类型：
   * - 'default' | 'text': 通用文本行
   * - 'list': 通用列表项（复用 .mod，实测 56.3px 高、行距 10px）
   * - 'mod': 模组封面列表（72px 封面 + 名称/状态/ID，窄屏操作区换行）
   * - 'table': 备份/存档表格（复用 .table，实测表头 34px + 每行 52px）
   * - 'code' | 'editor': 配置文件/代码预览块（复用 .file-body，实测 313px 高）
   * - 'card': 房间卡片（实测 343x375px 卡片）
   * - 'form': 表单选项（复用 .opt，实测控件 32px 高、行距 43px）
   */
  variant: { type: String, default: 'default' },
  /** 表格列宽（启用 variant='table' 时生效）。末列右对齐放操作按钮。 */
  cols: { type: Array, default: () => ['28%', '20%', 'auto'] },
});

const WIDTHS = ['64%', '90%', '48%', '76%', '56%'];
const widthOf = i => WIDTHS[(i - 1) % WIDTHS.length];
</script>

<template>
  <!-- 与 ModItem 一样由 72px 封面撑高，640px 以下操作区独占一行。 -->
  <div v-if="variant === 'mod'" class="mod-skeleton" aria-hidden="true">
    <div v-for="i in rows" :key="i" class="mod">
      <div class="flex flex-wrap items-center gap-4">
        <div class="flex min-w-0 flex-[1_1_280px] items-center gap-3.5 max-[640px]:basis-full">
          <Skeleton class="size-[72px] shrink-0 rounded-lg bg-border" />
          <div class="min-w-0 flex-1">
            <Skeleton class="h-[21px] w-52 max-w-full bg-border" />
            <div class="my-[5px] flex gap-1.5">
              <Skeleton class="h-[22px] w-[55px] rounded-full bg-border" />
              <Skeleton class="h-[22px] w-[55px] rounded-full bg-border" />
            </div>
            <Skeleton class="h-[17px] w-36 max-w-full bg-border" />
          </div>
        </div>
        <div class="ml-auto flex flex-wrap items-center gap-1.5 max-[640px]:w-full max-[640px]:justify-end">
          <div class="mr-2 flex items-center gap-2 max-[640px]:mr-auto">
            <Skeleton class="h-5 w-9 rounded-full bg-border" />
            <Skeleton class="h-5 w-7 bg-border" />
          </div>
          <Skeleton class="h-[29px] w-[69px] rounded-md bg-border" />
          <Skeleton class="h-[29px] w-[48px] rounded-md bg-border" />
          <Skeleton class="h-[29px] w-[48px] rounded-md bg-border" />
        </div>
      </div>
    </div>
  </div>

  <!-- 通用列表保留紧凑尺寸，不影响其它页面。 -->
  <div v-else-if="variant === 'list'">
    <div v-for="i in rows" :key="i" class="mod">
      <div class="mod-head">
        <Skeleton class="h-5 w-9 shrink-0 rounded-full bg-border" />
        <Skeleton class="h-5 w-36 bg-border md:w-52" />
        <Skeleton class="h-[22px] w-[55px] rounded-full bg-border" />
        <Skeleton class="hidden h-[22px] w-[55px] rounded-full bg-border sm:block" />
        <span class="mod-actions">
          <Skeleton class="h-[29px] w-[69px] rounded-md bg-border" />
          <Skeleton class="hidden h-[29px] w-[48px] rounded-md bg-border sm:block" />
        </span>
      </div>
    </div>
  </div>

  <!-- 表格（如备份、存档）：直接铺一张 .table，表头 34px、每行 52px -->
  <div v-else-if="variant === 'table'" class="table-wrap">
    <table class="table">
      <thead>
        <tr>
          <th
            v-for="(w, j) in cols"
            :key="j"
            :class="{ right: j === cols.length - 1 }"
            :style="{ width: w }"
          >
            <Skeleton class="h-4 w-4/5" />
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="i in rows" :key="i">
          <td
            v-for="(w, j) in cols"
            :key="j"
            class="skeleton-cell"
            :class="{ right: j === cols.length - 1 }"
          >
            <Skeleton v-if="j === cols.length - 1" class="ml-auto h-[29px] w-[48px] rounded-md" />
            <Skeleton v-else class="h-4 w-3/5" />
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <!-- 代码 / 配置文件预览：套用 .file-head + .file-body（内容区 313px 高） -->
  <div v-else-if="variant === 'code' || variant === 'editor'" class="file-card">
    <div class="file-head">
      <Skeleton class="h-4 w-44" />
      <Skeleton class="h-[22px] w-[55px] rounded-full" />
    </div>
    <div class="file-body min-h-[313px] space-y-1.5">
      <Skeleton class="h-3.5 w-3/4" />
      <Skeleton class="h-3.5 w-1/2" />
      <Skeleton class="h-3.5 w-5/6" />
      <Skeleton class="h-3.5 w-2/3" />
      <Skeleton class="h-3.5 w-4/5" />
      <Skeleton class="h-3.5 w-3/5" />
      <Skeleton class="h-3.5 w-2/4" />
    </div>
  </div>

  <!-- 房间卡片：直接复用 .room-grid / .room-card，min-h 补到实测的 375px -->
  <div v-else-if="variant === 'card'" class="room-grid">
    <div v-for="i in rows" :key="i" class="room-card min-h-[375px]">
      <div class="flex items-center justify-between">
        <div class="space-y-1">
          <Skeleton class="h-[21px] w-28" />
          <Skeleton class="h-[17px] w-20" />
        </div>
        <Skeleton class="h-[22px] w-[55px] rounded-full" />
      </div>
      <Skeleton class="h-[17px] w-2/3" />
      <Skeleton class="h-[22px] w-1/2" />
      <Skeleton class="h-[76px] w-full rounded-lg" />
      <Skeleton class="h-[17px] w-24" />
      <div class="flex items-center gap-2">
        <Skeleton class="h-8 flex-1 rounded-lg" />
        <Skeleton class="h-8 w-20 rounded-lg" />
      </div>
    </div>
  </div>

  <!-- 动态表单字段（如模组配置）：套用 .opt 的 190px 标签列 + 32px 控件 -->
  <div v-else-if="variant === 'form'">
    <div v-for="i in rows" :key="i" class="opt">
      <Skeleton class="h-5 w-32 justify-self-end" />
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

<style scoped>
/* 真实表格行内容高 29px（按钮决定），加内边距后每行 52px；
   骨架条只有 16px，用行高把总高补到与真实一致。 */
.skeleton-cell { height: 51px; }
</style>
