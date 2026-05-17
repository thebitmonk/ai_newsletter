<!--
  Cover node view — read-only inside the editor. Regenerate button lands in
  slice #21; here we just render the image with a hover affordance hint.
-->
<script setup lang="ts">
import { NodeViewWrapper, type NodeViewProps } from "@tiptap/vue-3";

defineProps<NodeViewProps>();
</script>

<template>
  <NodeViewWrapper>
    <div data-block="cover" class="relative overflow-hidden rounded border border-gray-200 bg-gray-50">
      <img
        v-if="node.attrs.src"
        :src="node.attrs.src"
        :alt="node.attrs.alt"
        class="block w-full"
      />
      <div v-else class="flex aspect-[2/1] items-center justify-center text-sm text-gray-400">
        (no cover image)
      </div>
    </div>
  </NodeViewWrapper>
</template>
