<script setup>
import { Skeleton } from '@/components/ui/skeleton';
import { fmtTime } from '@/lib/format.js';

defineProps({
  snapshot: { type: Object, default: null },
  loading: { type: Boolean, default: false },
  error: { type: String, default: '' },
});

const seasons = { spring: '春季', summer: '夏季', autumn: '秋季', winter: '冬季', mild: '温和季', wet: '飓风季', hurricane: '飓风季', dry: '旱季', green: '绿季', lush: '繁茂季', temperate: '温和季' };
</script>

<template>
  <div class="flex min-w-0 flex-col gap-0.5 text-xs">
    <div v-if="loading" role="status" aria-label="读取存档中" class="flex flex-col gap-1.5 pt-0.5">
      <Skeleton class="h-3.5 w-28" />
      <Skeleton class="h-3 w-36" />
    </div>
    <p v-else-if="error || snapshot?.error" role="alert" class="break-words text-destructive">读取失败：{{ error || snapshot.error }}</p>
    <template v-else-if="snapshot?.state">
      <p class="flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 tabular-nums">
        <span class="font-medium text-foreground">{{ snapshot.state.day == null ? '天数未知' : `第 ${snapshot.state.day} 天` }}</span>
        <span class="text-muted-foreground">{{ seasons[snapshot.state.season] || snapshot.state.season || '季节未知' }}</span>
        <span v-if="snapshot.state.remainingDays != null" class="text-[11.5px] text-muted-foreground">(剩 {{ snapshot.state.remainingDays }} 天)</span>
      </p>
      <time v-if="snapshot.state.savedAt" :datetime="snapshot.state.savedAt" class="truncate text-[11.5px] text-muted-foreground tabular-nums">
        存档于 {{ fmtTime(snapshot.state.savedAt) }}
      </time>
    </template>
    <p v-else class="text-muted-foreground">尚无世界快照</p>
  </div>
</template>
