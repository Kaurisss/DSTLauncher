<script setup>
import { HugeiconsIcon } from '@hugeicons/vue';
import { LayoutLeftIcon } from '@hugeicons/core-free-icons';
import { cn } from "cn";
import { Button } from '@/components/ui/button';
import { useSidebar } from "./utils";

const props = defineProps({
  class: {
    type: [Boolean, null, String, Object, Array],
    required: false,
    skipCheck: true,
  },
});

const { toggleSidebar, isMobile, openMobile, open } = useSidebar();
</script>

<template>
  <Button
    data-sidebar="trigger"
    data-slot="sidebar-trigger"
    variant="ghost"
    size="icon-sm"
    :class="cn('', props.class)"
    :aria-expanded="isMobile ? openMobile : open"
    @click="toggleSidebar"
  >
    <HugeiconsIcon :icon="LayoutLeftIcon" class="cn-rtl-flip" aria-hidden="true" />
    <span class="sr-only">切换导航</span>
  </Button>
</template>
