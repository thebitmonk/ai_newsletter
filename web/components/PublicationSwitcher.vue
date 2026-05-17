<!--
  Topbar Publication switcher. Always reflects the current-Account list of
  Publications; the "+ New publication" entry routes to the create page.
  Selecting an item updates the Pinia store AND navigates to that
  Publication's calendar (the most useful default per-Publication view).
-->
<script setup lang="ts">
import type { Publication } from "~/types/api";

const route = useRoute();
const router = useRouter();
const current = useCurrentPublication();
current.hydrateFromStorage();

const pubs = usePublications();
const { data, refresh } = await useAsyncData("pubs-switcher", () => pubs.list({ limit: 100 }));

const list = computed<Publication[]>(() => data.value?.items ?? []);

const selected = computed(() =>
  list.value.find((p) => p.id === current.id) ?? null,
);

function onSelect(e: Event) {
  const v = (e.target as HTMLSelectElement).value;
  if (v === "__new__") {
    router.push("/publications/new");
    return;
  }
  current.set(v);
  router.push(`/publications/${v}/calendar`);
}

// Keep the store synced with the URL: if you land on a publication page
// directly (e.g. via bookmark) the switcher should highlight that pub.
watchEffect(() => {
  const m = route.path.match(/^\/publications\/([0-9a-f-]{36})/);
  if (m) current.set(m[1]);
});

// Refresh the list when we return to /publications/new after creating one.
const onFocusRefresh = () => refresh();
onMounted(() => window.addEventListener("focus", onFocusRefresh));
onBeforeUnmount(() => window.removeEventListener("focus", onFocusRefresh));
</script>

<template>
  <select
    class="rounded border border-gray-300 px-2 py-1 text-sm"
    :value="selected?.id ?? ''"
    @change="onSelect"
  >
    <option value="" disabled>Choose a publication…</option>
    <option v-for="p in list" :key="p.id" :value="p.id">{{ p.name }}</option>
    <option value="__new__">＋ New publication</option>
  </select>
</template>
