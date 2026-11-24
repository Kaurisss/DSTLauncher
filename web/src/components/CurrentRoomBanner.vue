<script setup>
import { computed } from 'vue';
import { Badge } from '@/components/ui/badge';
import { useRoomsStore } from '@/stores/rooms.js';
import { GAME_MODE_LABEL } from '@/lib/format.js';

const rooms = useRoomsStore();
const room = computed(() => rooms.current);
const mode = computed(() => GAME_MODE_LABEL[room.value?.gameMode] || room.value?.gameMode || '');
const state = computed(() => !rooms.statusLoaded ? '等待状态' : (rooms.running ? '运行中' : '已停止'));
</script>

<template>
  <section v-if="room" aria-label="当前房间" aria-live="polite" class="flex min-w-0 flex-wrap items-center gap-3 border-b bg-muted/50 px-4 py-3 text-sm md:px-6">
    <div class="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-3 gap-y-1">
      <span class="text-xs text-muted-foreground">当前房间</span>
      <strong class="min-w-0 break-all font-semibold">{{ room.name }}</strong>
      <span class="break-all text-xs text-muted-foreground">{{ room.key }}<template v-if="mode"> {{ mode }}</template></span>
    </div>
    <Badge :variant="rooms.running ? 'default' : 'secondary'">{{ state }}</Badge>
  </section>
</template>
