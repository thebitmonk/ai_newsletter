<script setup lang="ts">
import type { ApiError } from "~/composables/useApi";
import type { Source } from "~/types/api";

const route = useRoute();
const id = route.params.id as string;
const sources = useSources(id);
const pubs = usePublications();

const { data: pub } = await useAsyncData(`pub-${id}`, () => pubs.get(id));
const { data, error, refresh } = await useAsyncData(`sources-${id}`, () => sources.list());

const items = computed(() => data.value?.items ?? []);

interface TypeMeta {
  label: string;
  identifierLabel: string;
  placeholder: string;
  help: string;
}

const TYPE_META: Record<Source["type"], TypeMeta> = {
  rss: {
    label: "RSS / Atom feed",
    identifierLabel: "Feed URL",
    placeholder: "https://example.com/feed.xml",
    help: "Any RSS 2.0 or Atom feed URL.",
  },
  youtube_channel: {
    label: "YouTube channel",
    identifierLabel: "Channel ID",
    placeholder: "UCxxxxxxxxxxxxxxxxxxxxxx",
    help: "Find this in the channel URL after /channel/.",
  },
  x_handle: {
    label: "X / Twitter handle",
    identifierLabel: "Handle",
    placeholder: "@karpathy",
    help: "Leading @ optional — it's stripped automatically.",
  },
  substack: {
    label: "Substack publication",
    identifierLabel: "Publication URL",
    placeholder: "https://stratechery.com",
    help: "Just the publication root — /feed is appended automatically.",
  },
  web: {
    label: "Generic website / blog",
    identifierLabel: "Site URL",
    placeholder: "https://anthropic.com/news",
    help: "RSS/Atom is autodiscovered from the page's <link rel=alternate>.",
  },
};

// --- create form ---
const showForm = ref(false);
const newType = ref<Source["type"]>("rss");
const newIdentifier = ref("");
const newError = ref<string | null>(null);
const submitting = ref(false);

function normaliseIdentifier(t: Source["type"], raw: string): string {
  if (t === "x_handle") return raw.trim().replace(/^@+/, "");
  return raw.trim();
}

async function onCreate() {
  newError.value = null;
  if (!newIdentifier.value.trim()) {
    newError.value = "Identifier is required.";
    return;
  }
  submitting.value = true;
  try {
    await sources.create({
      type: newType.value,
      identifier: normaliseIdentifier(newType.value, newIdentifier.value),
    });
    newIdentifier.value = "";
    showForm.value = false;
    await refresh();
  } catch (e) {
    const err = e as ApiError;
    if (err.code === "duplicate_source") {
      newError.value = "This publication already has a source with that type and identifier.";
    } else {
      newError.value = err.message;
    }
  } finally {
    submitting.value = false;
  }
}

// --- per-row actions ---
const confirmingDelete = ref<string | null>(null);

async function toggleEnabled(src: Source) {
  await sources.update(src.id, { enabled: !src.enabled });
  await refresh();
}

async function deleteSource(src: Source) {
  if (confirmingDelete.value !== src.id) {
    confirmingDelete.value = src.id;
    return;
  }
  await sources.remove(src.id);
  confirmingDelete.value = null;
  await refresh();
}

function relativeTime(iso: string | null): string {
  if (!iso) return "never";
  const diff = Date.now() - new Date(iso).getTime();
  const m = Math.floor(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}
</script>

<template>
  <section class="mx-auto max-w-3xl">
    <PublicationTabs v-if="pub" :publication="pub" />

    <header class="mb-3 flex items-center justify-end">
      <button
        type="button"
        class="rounded bg-gray-900 px-3 py-2 text-sm text-white"
        @click="showForm = !showForm"
      >
        {{ showForm ? "Cancel" : "Add source" }}
      </button>
    </header>

    <div v-if="showForm" class="space-y-3 rounded border border-gray-200 bg-white p-4">
      <label class="block">
        <span class="text-sm text-gray-700">Type</span>
        <select v-model="newType" class="mt-1 w-full rounded border border-gray-300 px-3 py-2">
          <option v-for="(meta, t) in TYPE_META" :key="t" :value="t">{{ meta.label }}</option>
        </select>
      </label>
      <label class="block">
        <span class="text-sm text-gray-700">{{ TYPE_META[newType].identifierLabel }}</span>
        <input
          v-model="newIdentifier"
          :placeholder="TYPE_META[newType].placeholder"
          class="mt-1 w-full rounded border border-gray-300 px-3 py-2 font-mono text-sm"
        />
        <span class="mt-1 block text-xs text-gray-500">{{ TYPE_META[newType].help }}</span>
      </label>
      <div v-if="newError" class="rounded border border-red-300 bg-red-50 p-2 text-sm text-red-700">
        {{ newError }}
      </div>
      <button
        type="button"
        :disabled="submitting"
        class="rounded bg-gray-900 px-3 py-2 text-sm text-white disabled:opacity-50"
        @click="onCreate"
      >
        {{ submitting ? "Adding…" : "Add" }}
      </button>
    </div>

    <div v-if="error" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
      Failed to load sources: {{ error.message }}
    </div>

    <div
      v-else-if="items.length === 0"
      class="rounded border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500"
    >
      No sources yet. Add one above to start the curation pipeline.
    </div>

    <ul v-else class="divide-y divide-gray-200 rounded border border-gray-200 bg-white">
      <li v-for="src in items" :key="src.id" class="flex items-center gap-3 p-3">
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 text-sm">
            <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs uppercase tracking-wide text-gray-600">
              {{ src.type.replace("_", " ") }}
            </span>
            <span class="truncate font-mono">{{ src.identifier }}</span>
          </div>
          <div class="mt-0.5 text-xs text-gray-500">
            last polled {{ relativeTime(src.last_polled_at) }}
            · every {{ src.poll_interval.replace(/0m0s$/, "").replace(/0s$/, "") }}
          </div>
        </div>
        <label class="flex items-center gap-1 text-xs text-gray-600">
          <input type="checkbox" :checked="src.enabled" @change="toggleEnabled(src)" />
          enabled
        </label>
        <button
          type="button"
          class="rounded border border-red-300 px-2 py-1 text-xs text-red-700 hover:bg-red-50"
          @click="deleteSource(src)"
        >
          {{ confirmingDelete === src.id ? "confirm" : "delete" }}
        </button>
      </li>
    </ul>
  </section>
</template>
