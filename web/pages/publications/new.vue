<script setup lang="ts">
import type { ApiError } from "~/composables/useApi";
import type { PublicationCreateInput } from "~/types/api";

const pubs = usePublications();
const current = useCurrentPublication();
const form = ref<{ setError: (err: ApiError) => void } | null>(null);

async function onSubmit(payload: PublicationCreateInput) {
  try {
    const created = await pubs.create(payload);
    current.set(created.id);
    // Fresh publication has no Sources yet — the calendar would be empty.
    // Drop the owner on /sources so the first task (adding sources) is
    // discoverable.
    await navigateTo(`/publications/${created.id}/sources`);
  } catch (e) {
    form.value?.setError(e as ApiError);
  }
}
</script>

<template>
  <section class="mx-auto max-w-xl space-y-4">
    <h1 class="text-2xl font-semibold">New publication</h1>
    <PublicationForm ref="form" submit-label="Create" @submit="onSubmit" />
  </section>
</template>
