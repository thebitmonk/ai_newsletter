<script setup lang="ts">
import { provide } from "vue";
import { Editor, EditorContent, VueNodeViewRenderer } from "@tiptap/vue-3";
import StarterKit from "@tiptap/starter-kit";
import { useDebounceFn } from "@vueuse/core";

import { CoverNode, IntroNode, IssueDocument, StoryNode } from "~/tiptap/schema";
import CoverNodeView from "~/components/editor/CoverNodeView.vue";
import IntroNodeView from "~/components/editor/IntroNodeView.vue";
import StoryNodeView from "~/components/editor/StoryNodeView.vue";

import type { ApiError } from "~/composables/useApi";
import { COVER_PENDING_ID, RegenerateKey } from "~/composables/editorContext";
import { regenErrorMessage, useIssueRegen } from "~/composables/useIssueRegen";

const route = useRoute();
const issueId = route.params.issue_id as string;
const issues = useIssues(route.params.id as string);

interface IssueDetail {
  id: string;
  state: string;
  subject: string | null;
  title: string | null;
  cover_url: string | null;
  scheduled_at: string;
  body_doc: unknown | null;
  failed_reason?: string | null;
}

const { data: iss, error: loadError } = await useAsyncData(`issue-${issueId}`, () =>
  issues.get<IssueDetail>(issueId),
);

const editor = shallowRef<Editor | null>(null);
const subject = ref("");
const title = ref("");
const saveStatus = ref<"idle" | "saving" | "saved" | "error">("idle");
const saveError = ref<string | null>(null);

const canEdit = computed(() => iss.value?.state === "drafted" || iss.value?.state === "approved");

onMounted(() => {
  if (!iss.value || !iss.value.body_doc) return;
  subject.value = iss.value.subject ?? "";
  title.value = iss.value.title ?? "";

  editor.value = new Editor({
    content: iss.value.body_doc as object,
    editable: canEdit.value,
    extensions: [
      StarterKit.configure({
        document: false, // we override with IssueDocument
        heading: false,
        codeBlock: false,
        blockquote: false,
        horizontalRule: false,
        bulletList: false,
        orderedList: false,
        listItem: false,
        hardBreak: false,
      }),
      IssueDocument,
      CoverNode.extend({ addNodeView: () => VueNodeViewRenderer(CoverNodeView) }),
      IntroNode.extend({ addNodeView: () => VueNodeViewRenderer(IntroNodeView) }),
      StoryNode.extend({ addNodeView: () => VueNodeViewRenderer(StoryNodeView) }),
    ],
    onUpdate: () => scheduleSave(),
  });
});

onBeforeUnmount(() => editor.value?.destroy());

const api = useApi();

const scheduleSave = useDebounceFn(async () => {
  if (!editor.value || !canEdit.value) return;
  saveStatus.value = "saving";
  saveError.value = null;
  try {
    const body_doc = editor.value.getJSON();
    await api.put(`/issues/${issueId}/body`, {
      subject: subject.value,
      title: title.value,
      body_doc,
    });
    saveStatus.value = "saved";
  } catch (e) {
    const err = e as ApiError;
    saveStatus.value = "error";
    saveError.value = err.message;
  }
}, 1500);

watch([subject, title], () => scheduleSave());

const statusLabel = computed(() => {
  switch (saveStatus.value) {
    case "saving": return "Saving…";
    case "saved":  return "Saved";
    case "error":  return "Save failed";
    default:        return "";
  }
});

// Regenerate wiring — story summaries + cover image. Node views inject this
// and call .call("summary"|"image", storyId).
const regen = useIssueRegen();
const regenStatus = ref<"idle" | "pending" | "error">("idle");
const regenPendingId = ref<string | null>(null);
const regenError = ref<string | null>(null);

async function callRegenerate(kind: "summary" | "image", storyId: string) {
  if (regenStatus.value === "pending") return;
  regenStatus.value = "pending";
  regenError.value = null;
  regenPendingId.value = kind === "image" ? COVER_PENDING_ID : storyId;
  try {
    const updated =
      kind === "summary"
        ? await regen.regenerateStorySummary(issueId, storyId)
        : await regen.regenerateCover(issueId, anyStoryId() ?? storyId);
    // Replace the in-editor doc with the backend's updated body_doc so the
    // mutation lands without us doing tree-walking client-side.
    if (editor.value && updated.body_doc) {
      editor.value.commands.setContent(updated.body_doc as object, false);
    }
    if (iss.value) {
      iss.value.body_doc = updated.body_doc;
      iss.value.cover_url = updated.cover_url;
    }
    regenStatus.value = "idle";
  } catch (e) {
    regenStatus.value = "error";
    regenError.value = regenErrorMessage(e);
  } finally {
    regenPendingId.value = null;
  }
}

function anyStoryId(): string | null {
  if (!editor.value) return null;
  const doc = editor.value.getJSON();
  const stories = (doc.content ?? []).filter((n) => n.type === "story") as { attrs?: { storyId?: string } }[];
  return stories[0]?.attrs?.storyId ?? null;
}

provide(RegenerateKey, {
  call: callRegenerate,
  status: regenStatus,
  pendingId: regenPendingId,
  errorMessage: regenError,
});
</script>

<template>
  <section class="space-y-4">
    <div v-if="loadError" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
      Couldn't load this issue: {{ loadError.message }}
    </div>

    <template v-else-if="iss">
      <header class="flex items-start justify-between gap-3">
        <div class="flex-1 space-y-2">
          <input
            v-model="subject"
            placeholder="Subject (the email Subject: header)"
            :disabled="!canEdit"
            class="block w-full rounded border border-gray-300 px-3 py-2 text-base disabled:bg-gray-50"
          />
          <input
            v-model="title"
            placeholder="In-issue title"
            :disabled="!canEdit"
            class="block w-full rounded border border-gray-300 px-3 py-2 text-xl font-semibold disabled:bg-gray-50"
          />
        </div>
        <div class="flex flex-col items-end gap-1 text-xs text-gray-500">
          <span class="rounded bg-gray-100 px-2 py-0.5 uppercase tracking-wide">
            {{ iss.state }}
          </span>
          <span
            :class="{
              'text-amber-600': saveStatus === 'saving',
              'text-emerald-600': saveStatus === 'saved',
              'text-red-700':    saveStatus === 'error',
            }"
          >
            {{ statusLabel }}
          </span>
          <span v-if="iss.state === 'failed' && iss.failed_reason" class="text-red-700">
            {{ iss.failed_reason }}
          </span>
        </div>
      </header>

      <div v-if="saveError" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">
        {{ saveError }}
      </div>
      <div v-if="regenError" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">
        {{ regenError }}
      </div>

      <ClientOnly>
        <div class="rounded border border-gray-200 bg-white p-4">
          <EditorContent :editor="editor" />
        </div>
      </ClientOnly>
    </template>
  </section>
</template>
