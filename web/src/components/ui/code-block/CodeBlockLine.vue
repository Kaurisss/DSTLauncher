<script setup>
import { cn } from "cn";
import { Comment, Fragment, Text, useSlots } from "vue";
import { codeBlockLineVariants } from ".";

const props = defineProps({
  variant: { type: null, required: false, default: "default" },
  class: {
    type: [Boolean, null, String, Object, Array],
    required: false,
    skipCheck: true,
  },
});

const slots = useSlots();

function hasContent(nodes) {
  return nodes.some((node) => {
    if (node.type === Comment) return false;
    if (node.type === Text) return node.children !== "";
    if (node.type === Fragment)
      return Array.isArray(node.children) && hasContent(node.children);
    return true;
  });
}

// An empty line holds a newline so copying a selection keeps blank lines.
function isEmpty() {
  return !hasContent(slots.default?.() ?? []);
}
</script>

<template>
  <span
    data-slot="code-block-line"
    :data-variant="variant"
    :class="cn(codeBlockLineVariants({ variant }), props.class)"
    ><span data-slot="code-block-line-content" class="min-w-0"
      ><template v-if="isEmpty()">{{ "\n" }}</template
      ><slot v-else /></span
  ></span>
</template>
