<script setup>
import { PhSidebarSimple } from '@phosphor-icons/vue';
import { cn } from "cn";
import { Button } from '@/components/ui/button';
import { useSidebar } from "./utils";

const props = defineProps({
  size: {
    type: String,
    required: false,
    default: "icon",
  },
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
    :size="props.size"
    :class="cn('', props.class)"
    :aria-expanded="isMobile ? openMobile : open"
    @click="toggleSidebar"
  >
    <PhSidebarSimple class="cn-rtl-flip" aria-hidden="true" />
    <span class="sr-only">切换导航</span>
  </Button>
</template>
