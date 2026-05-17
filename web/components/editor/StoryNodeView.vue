<!--
  Story node view — two editable paragraphs (headline + body) + a delete
  affordance + a source-URL link. Regenerate button arrives in slice #21.
-->
<script setup lang="ts">
import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from "@tiptap/vue-3";

const props = defineProps<NodeViewProps>();

function onDelete() {
  // Use the editor's transaction helpers to remove this node.
  const pos = props.getPos();
  if (pos === undefined) return;
  props.editor.chain().focus().deleteRange({ from: pos, to: pos + props.node.nodeSize }).run();
}
</script>

<template>
  <NodeViewWrapper>
    <article
      data-block="story"
      :data-story-id="node.attrs.storyId"
      :data-source-url="node.attrs.sourceUrl"
      class="group relative my-4 rounded border border-gray-200 bg-white p-4"
    >
      <header class="mb-2 flex items-start justify-between gap-2 text-xs text-gray-500">
        <a
          v-if="node.attrs.sourceUrl"
          :href="node.attrs.sourceUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="truncate hover:underline"
        >
          source ↗
        </a>
        <button
          type="button"
          class="invisible flex-shrink-0 rounded border border-red-300 px-2 py-0.5 text-red-700 hover:bg-red-50 group-hover:visible"
          contenteditable="false"
          @click="onDelete"
        >
          delete
        </button>
      </header>
      <NodeViewContent class="story-content prose prose-sm max-w-none" />
    </article>
  </NodeViewWrapper>
</template>

<style scoped>
.story-content :deep([data-attr-role="headline"]),
.story-content :deep(p:first-child) {
  font-weight: 600;
  font-size: 1.05rem;
  margin-bottom: 0.4rem;
}
</style>
