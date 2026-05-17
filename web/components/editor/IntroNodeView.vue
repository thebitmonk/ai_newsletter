<script setup lang="ts">
import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from "@tiptap/vue-3";

defineProps<NodeViewProps>();
</script>

<template>
  <NodeViewWrapper>
    <div data-block="intro" class="italic text-gray-700">
      <NodeViewContent />
    </div>
  </NodeViewWrapper>
</template>
