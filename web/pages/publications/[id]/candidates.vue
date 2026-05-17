<!--
  Per-publication Candidates pool view. Read-only — the pool is owned by
  the poller. Lists what's been fetched and is currently eligible for the
  next curation run, with source attribution and TTL info.
-->
<script setup lang="ts">
import type { CandidateItem } from "~/composables/useCandidates";

const route = useRoute();
const id = route.params.id as string;
const pubs = usePublications();
const cands = useCandidates(id);

const { data: pub, error: pubError } = await useAsyncData(`pub-${id}`, () => pubs.get(id));

const items = ref<CandidateItem[]>([]);
const nextCursor = ref<string | null>(null);
const loadingMore = ref(false);
const listError = ref<string | null>(null);

async function load(reset = false) {
  if (reset) {
    items.value = [];
    nextCursor.value = null;
  }
  loadingMore.value = true;
  try {
    const resp = await cands.list({ cursor: nextCursor.value ?? undefined, limit: 25 });
    items.value.push(...resp.items);
    nextCursor.value = resp.next_cursor;
  } catch (e) {
    listError.value = (e as Error).message;
  } finally {
    loadingMore.value = false;
  }
}
await load(true);

function relativeFetched(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

function relativeExpires(iso: string): string {
  const diff = new Date(iso).getTime() - Date.now();
  if (diff <= 0) return "expired";
  const h = Math.floor(diff / 3_600_000);
  if (h < 24) return `expires in ${h}h`;
  return `expires in ${Math.floor(h / 24)}d`;
}
</script>

<template>
  <section>
    <PublicationTabs v-if="pub" :publication="pub" />

    <div v-if="pubError" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
      Couldn't load this publication: {{ pubError.message }}
    </div>

    <template v-else>
      <p class="mb-3 text-sm text-gray-600">
        Items the source poller has fetched and that haven't expired. The
        curator picks Stories from this pool against the Publication's Brief.
      </p>

      <div v-if="listError" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
        {{ listError }}
        <button class="ml-2 underline" @click="load(true)">retry</button>
      </div>

      <div
        v-else-if="items.length === 0"
        class="rounded border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500"
      >
        Pool is empty. Either no sources have been polled yet, or items
        have expired. Check the Sources tab for last-poll timestamps.
      </div>

      <ul v-else class="divide-y divide-gray-200 rounded border border-gray-200 bg-white">
        <li v-for="c in items" :key="c.id" class="p-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <a
                :href="c.url"
                target="_blank"
                rel="noopener noreferrer"
                class="block truncate font-medium hover:underline"
              >
                {{ c.title || c.url }}
              </a>
              <p class="mt-0.5 truncate text-xs text-gray-500">{{ c.url }}</p>
              <p class="mt-1 text-xs text-gray-500">
                <span class="rounded bg-gray-100 px-1.5 py-0.5 uppercase tracking-wide">
                  {{ c.source_type.replace("_", " ") }}
                </span>
                ·
                <span class="font-mono">{{ c.source_identifier }}</span>
                ·
                fetched {{ relativeFetched(c.fetched_at) }}
                ·
                {{ relativeExpires(c.expires_at) }}
              </p>
            </div>
          </div>
        </li>
      </ul>

      <div v-if="nextCursor" class="mt-4 text-center">
        <button
          type="button"
          :disabled="loadingMore"
          class="rounded border border-gray-300 px-3 py-2 text-sm hover:bg-gray-50 disabled:opacity-50"
          @click="load(false)"
        >
          {{ loadingMore ? "Loading…" : "Load more" }}
        </button>
      </div>
    </template>
  </section>
</template>
