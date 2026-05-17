<script setup lang="ts">
import type { ApiError } from "~/composables/useApi";
import type { PublicationUpdateInput } from "~/types/api";

const route = useRoute();
const id = route.params.id as string;
const pubs = usePublications();
const current = useCurrentPublication();

const { data: pub, error: loadError } = await useAsyncData(`pub-${id}`, () => pubs.get(id));

const form = ref<{ setError: (err: ApiError) => void } | null>(null);
const confirmingDelete = ref(false);
const deleting = ref(false);
const savedAt = ref<Date | null>(null);

async function onSubmit(payload: PublicationUpdateInput) {
  try {
    pub.value = await pubs.update(id, payload);
    savedAt.value = new Date();
  } catch (e) {
    form.value?.setError(e as ApiError);
  }
}

async function onDelete() {
  if (!confirmingDelete.value) {
    confirmingDelete.value = true;
    return;
  }
  deleting.value = true;
  try {
    await pubs.remove(id);
    if (current.id === id) current.set(null);
    await navigateTo("/publications");
  } finally {
    deleting.value = false;
  }
}
</script>

<template>
  <section class="mx-auto max-w-xl">
    <PublicationTabs v-if="pub" :publication="pub" />

    <div v-if="savedAt" class="mb-3 text-right text-xs text-emerald-600">
      Saved {{ savedAt.toLocaleTimeString() }}
    </div>

    <div v-if="loadError" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
      Couldn't load this publication: {{ loadError.message }}
    </div>

    <template v-else-if="pub">
      <PublicationForm
        ref="form"
        :initial="pub"
        submit-label="Save changes"
        @submit="onSubmit"
      />

      <div class="border-t border-gray-200 pt-4">
        <h2 class="text-sm font-semibold text-red-700">Danger zone</h2>
        <p class="mt-1 text-sm text-gray-600">
          Deleting this publication also removes its Sources, Candidates, and Issues.
        </p>
        <button
          type="button"
          :disabled="deleting"
          class="mt-3 rounded border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-700 hover:bg-red-100 disabled:opacity-50"
          @click="onDelete"
        >
          {{ deleting ? "Deleting…" : confirmingDelete ? "Click again to confirm delete" : "Delete publication" }}
        </button>
      </div>
    </template>
  </section>
</template>
