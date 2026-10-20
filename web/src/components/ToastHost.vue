<script setup>
import { Toaster } from '@/components/ui/sonner';
import { useTheme } from '@/composables/useTheme.js';

const { resolved } = useTheme();
</script>

<template>
  <Toaster :theme="resolved" position="bottom-right" close-button />
</template>
