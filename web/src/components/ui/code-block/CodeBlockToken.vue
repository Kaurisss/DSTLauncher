<script setup>
defineProps({
  token: { type: null, required: true },
});
</script>

<template>
  <span
    :style="token.htmlStyle"
    class="text-(--shiki-light) dark:text-(--shiki-dark)"
    >{{ token.content }}</span
  >
</template>
