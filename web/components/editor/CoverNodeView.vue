<script setup lang="ts">
import { inject } from "vue";
import { NodeViewWrapper, type NodeViewProps } from "@tiptap/vue-3";

import { COVER_PENDING_ID, RegenerateKey } from "~/composables/editorContext";

defineProps<NodeViewProps>();

const regen = inject(RegenerateKey, null);
const isPending = computed(() => regen?.pendingId.value === COVER_PENDING_ID);
const pendingOther = computed(() => regen?.status.value === "pending" && !isPending.value);

async function onRegenerate() {
  if (!regen) return;
  await regen.call("image", COVER_PENDING_ID);
}
</script>

<template>
  <NodeViewWrapper>
    <div
      data-block="cover"
      class="group relative overflow-hidden rounded border border-gray-200 bg-gray-50 transition"
      :class="{ 'opacity-50': isPending }"
    >
      <img
        v-if="node.attrs.src"
        :src="node.attrs.src"
        :alt="node.attrs.alt"
        class="block w-full"
      />
      <div v-else class="flex aspect-[2/1] items-center justify-center text-sm text-gray-400">
        (no cover image)
      </div>
      <button
        v-if="regen"
        type="button"
        :disabled="isPending || pendingOther"
        title="Re-generates the cover image. Costs ~$0.04 per call."
        class="invisible absolute right-2 top-2 rounded border border-gray-300 bg-white/90 px-2 py-1 text-xs shadow-sm hover:bg-white disabled:opacity-50 group-hover:visible"
        contenteditable="false"
        @click="onRegenerate"
      >
        {{ isPending ? "regenerating…" : "regenerate cover" }}
      </button>
    </div>
  </NodeViewWrapper>
</template>
