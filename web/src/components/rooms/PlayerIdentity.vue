<script setup>
import { computed } from 'vue';
import { CHARACTERS, characterName, lookupPlayer } from '@/lib/format.js';

const props = defineProps({
  id: { type: String, required: true },
  seen: { type: Object, default: () => ({}) },
});

const face = computed(() => lookupPlayer(props.seen, props.id) || {});
const role = computed(() => characterName(face.value.prefab));
const avatar = computed(() => {
  const prefab = face.value.prefab;
  if (!prefab) return '';
  const file = Object.hasOwn(CHARACTERS, prefab) ? prefab : 'unknown';
  return `/character-avatars/${file}.png`;
});
</script>

<template>
  <div class="flex min-w-0 items-center gap-2">
    <img v-if="avatar" :src="avatar" :alt="role" width="32" height="32" class="size-8 shrink-0 object-contain" />
    <div class="min-w-0">
      <div class="truncate text-sm font-medium" :class="{ 'font-mono': !face.name }">{{ face.name || id }}</div>
      <div v-if="face.name || role" class="truncate text-xs text-muted-foreground">
        <span class="font-mono">{{ id }}</span><template v-if="role"> ({{ role }})</template>
      </div>
    </div>
  </div>
</template>
