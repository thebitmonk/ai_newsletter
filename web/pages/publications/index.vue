<script setup lang="ts">
const pubs = usePublications();
const { data, error, refresh } = await useAsyncData("pubs-list", () =>
  pubs.list({ limit: 100 }),
);

const items = computed(() => data.value?.items ?? []);

function cadenceSummary(rule: string | null, tz: string): string {
  if (!rule) return "Ad-hoc only";
  return `${rule} · ${tz}`;
}

function leadHours(d: string): string {
  // Go formats durations as "24h0m0s" / "30m0s" etc. Just show the prefix
  // for the dashboard summary.
  const match = d.match(/^(\d+)h/);
  return match ? `${match[1]}h lead` : d;
}
</script>

<template>
  <section class="space-y-6">
    <header class="flex items-center justify-between">
      <h1 class="text-2xl font-semibold">Publications</h1>
      <NuxtLink
        to="/publications/new"
        class="rounded bg-gray-900 px-3 py-2 text-sm text-white"
      >
        New publication
      </NuxtLink>
    </header>

    <div v-if="error" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
      Failed to load publications: {{ error.message }}
      <button class="ml-2 underline" @click="refresh()">retry</button>
    </div>

    <div v-else-if="items.length === 0" class="rounded border border-dashed border-gray-300 p-8 text-center">
      <p class="text-gray-600">No publications yet.</p>
      <NuxtLink to="/publications/new" class="mt-3 inline-block underline">
        Create your first publication
      </NuxtLink>
    </div>

    <ul v-else class="space-y-3">
      <li
        v-for="p in items"
        :key="p.id"
        class="rounded border border-gray-200 bg-white p-4"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <h2 class="font-semibold">{{ p.name }}</h2>
            <p class="mt-1 line-clamp-2 text-sm text-gray-500">
              {{ p.brief || "(no brief yet)" }}
            </p>
            <p class="mt-1 text-xs text-gray-400">
              {{ cadenceSummary(p.cadence_rule, p.timezone) }}
              · {{ leadHours(p.curation_lead_time) }}
              · {{ p.stories_per_issue_min }}–{{ p.stories_per_issue_max }} stories
              <span v-if="p.approval_gate_enabled"> · approval-gated</span>
            </p>
          </div>
          <div class="flex flex-shrink-0 gap-2 text-sm">
            <NuxtLink :to="`/publications/${p.id}/sources`" class="underline">
              Sources
            </NuxtLink>
            <NuxtLink :to="`/publications/${p.id}/calendar`" class="underline">
              Calendar
            </NuxtLink>
            <NuxtLink :to="`/publications/${p.id}/settings`" class="underline">
              Settings
            </NuxtLink>
          </div>
        </div>
      </li>
    </ul>
  </section>
</template>
