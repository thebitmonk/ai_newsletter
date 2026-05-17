<script setup lang="ts">
import { inject } from "vue";
import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from "@tiptap/vue-3";

import { RegenerateKey } from "~/composables/editorContext";

const props = defineProps<NodeViewProps>();

const regen = inject(RegenerateKey, null);
const storyId = computed(() => String(props.node.attrs.storyId ?? ""));
const isPendingThis = computed(() => regen?.pendingId.value === storyId.value);
const pendingOther = computed(() => regen?.status.value === "pending" && !isPendingThis.value);

function onDelete() {
  const pos = props.getPos();
  if (pos === undefined) return;
  props.editor.chain().focus().deleteRange({ from: pos, to: pos + props.node.nodeSize }).run();
}

async function onRegenerate() {
  if (!regen || !storyId.value) return;
  await regen.call("summary", storyId.value);
}
</script>

<template>
  <NodeViewWrapper>
    <article
      data-block="story"
      :data-story-id="node.attrs.storyId"
      :data-source-url="node.attrs.sourceUrl"
      class="group relative my-4 rounded border border-gray-200 bg-white p-4 transition"
      :class="{ 'opacity-50': isPendingThis }"
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
        <div class="flex flex-shrink-0 items-center gap-1" contenteditable="false">
          <button
            v-if="regen"
            type="button"
            :disabled="isPendingThis || pendingOther"
            title="Re-generates this story via the configured LLM. Costs ~$0.005 per call."
            class="invisible rounded border border-gray-300 px-2 py-0.5 hover:bg-gray-50 disabled:opacity-50 group-hover:visible"
            @click="onRegenerate"
          >
            {{ isPendingThis ? "regenerating…" : "regenerate" }}
          </button>
          <button
            type="button"
            class="invisible rounded border border-red-300 px-2 py-0.5 text-red-700 hover:bg-red-50 group-hover:visible"
            @click="onDelete"
          >
            delete
          </button>
        </div>
      </header>
      <NodeViewContent class="story-content prose prose-sm max-w-none" />
    </article>
  </NodeViewWrapper>
</template>

<style scoped>
.story-content :deep(p:first-child) {
  font-weight: 600;
  font-size: 1.05rem;
  margin-bottom: 0.4rem;
}
</style>
