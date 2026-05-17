<!--
  Sub-nav rendered at the top of every per-publication page so owners can
  move between Calendar / Sources / Candidates / Settings without
  bouncing back to the dashboard. Also surfaces the publication name +
  brief excerpt so the context is obvious.
-->
<script setup lang="ts">
import type { Publication } from "~/types/api";

const props = defineProps<{
  publication: Publication;
}>();

const TABS = [
  { key: "calendar",   label: "Calendar",   path: "calendar" },
  { key: "sources",    label: "Sources",    path: "sources" },
  { key: "candidates", label: "Candidates", path: "candidates" },
  { key: "settings",   label: "Settings",   path: "settings" },
] as const;

const route = useRoute();
const active = computed(() => {
  // Pick the tab whose path appears in the current route.
  const m = route.path.match(/\/publications\/[^/]+\/([^/]+)/);
  return m?.[1] ?? "calendar";
});
</script>

<template>
  <header class="mb-6 space-y-3">
    <div>
      <NuxtLink to="/publications" class="text-xs text-gray-500 hover:underline">
        ← All publications
      </NuxtLink>
      <h1 class="mt-1 text-2xl font-semibold">{{ publication.name }}</h1>
      <p v-if="publication.brief" class="mt-1 line-clamp-2 text-sm text-gray-500">
        {{ publication.brief }}
      </p>
    </div>
    <nav class="flex gap-1 border-b border-gray-200 text-sm">
      <NuxtLink
        v-for="t in TABS"
        :key="t.key"
        :to="`/publications/${publication.id}/${t.path}`"
        class="-mb-px border-b-2 px-3 py-2"
        :class="active === t.key
          ? 'border-gray-900 font-medium text-gray-900'
          : 'border-transparent text-gray-500 hover:text-gray-800'"
      >
        {{ t.label }}
      </NuxtLink>
    </nav>
  </header>
</template>
