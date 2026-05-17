<script setup lang="ts">
import type { IssueSummary } from "~/types/api";

const route = useRoute();
const router = useRouter();
const id = route.params.id as string;
const issues = useIssues(id);
const pubs = usePublications();

const { data: pub } = await useAsyncData(`pub-${id}`, () => pubs.get(id));

// View mode: month (default) or week. Persisted in the URL ?view= so refresh
// keeps the same view.
const view = computed<"month" | "week">(() => (route.query.view === "week" ? "week" : "month"));
function setView(v: "month" | "week") {
  router.replace({ query: { ...route.query, view: v } });
}

// Anchor date — the date "in focus" for the current view. Defaults to today.
const anchor = ref(new Date());

function jumpToToday() {
  anchor.value = new Date();
}

function shift(direction: -1 | 1) {
  const d = new Date(anchor.value);
  if (view.value === "month") d.setMonth(d.getMonth() + direction);
  else d.setDate(d.getDate() + 7 * direction);
  anchor.value = d;
}

const visibleRange = computed(() => {
  if (view.value === "month") {
    const start = new Date(anchor.value.getFullYear(), anchor.value.getMonth(), 1);
    const end = new Date(anchor.value.getFullYear(), anchor.value.getMonth() + 1, 1);
    // Pad the grid to whole weeks for display.
    const padStart = new Date(start); padStart.setDate(start.getDate() - start.getDay());
    const padEnd = new Date(end); padEnd.setDate(end.getDate() + (6 - ((end.getDay() + 6) % 7)));
    return { start, end, padStart, padEnd };
  } else {
    const start = new Date(anchor.value); start.setDate(anchor.value.getDate() - anchor.value.getDay());
    start.setHours(0, 0, 0, 0);
    const end = new Date(start); end.setDate(start.getDate() + 7);
    return { start, end, padStart: start, padEnd: end };
  }
});

const fetchKey = computed(() => `issues-${id}-${view.value}-${visibleRange.value.padStart.toISOString()}`);
const { data, error, refresh } = await useAsyncData(fetchKey, () =>
  issues.list({
    scheduledAfter: visibleRange.value.padStart,
    scheduledBefore: visibleRange.value.padEnd,
    limit: 100,
  }),
);
watch(fetchKey, () => refresh());

const items = computed<IssueSummary[]>(() => data.value?.items ?? []);

// Group issues by yyyy-mm-dd (UTC slice from scheduled_at).
const byDay = computed(() => {
  const map: Record<string, IssueSummary[]> = {};
  for (const i of items.value) {
    const key = i.scheduled_at.slice(0, 10);
    (map[key] ??= []).push(i);
  }
  return map;
});

// Build the cell grid.
const cells = computed(() => {
  const { padStart, padEnd, start, end } = visibleRange.value;
  const out: { date: Date; iso: string; inMonth: boolean; isToday: boolean }[] = [];
  const today = new Date().toISOString().slice(0, 10);
  for (let d = new Date(padStart); d < padEnd; d.setDate(d.getDate() + 1)) {
    const iso = d.toISOString().slice(0, 10);
    out.push({
      date: new Date(d),
      iso,
      inMonth: d >= start && d < end,
      isToday: iso === today,
    });
  }
  return out;
});

const STATE_COLOR: Record<IssueSummary["state"], string> = {
  planned:  "border-gray-300 bg-white text-gray-700",
  curating: "border-blue-300 bg-blue-50 text-blue-800 animate-pulse",
  drafted:  "border-emerald-300 bg-emerald-50 text-emerald-800",
  approved: "border-emerald-500 bg-emerald-100 text-emerald-900",
  sending:  "border-amber-400 bg-amber-50 text-amber-900",
  sent:     "border-gray-200 bg-gray-50 text-gray-500",
  failed:   "border-red-400 bg-red-50 text-red-800",
  skipped:  "border-gray-200 bg-gray-50 text-gray-400 line-through",
};

function goto(iss: IssueSummary) {
  router.push(`/publications/${id}/issues/${iss.id}`);
}

const title = computed(() =>
  view.value === "month"
    ? anchor.value.toLocaleDateString(undefined, { month: "long", year: "numeric" })
    : `Week of ${visibleRange.value.start.toLocaleDateString()}`,
);
</script>

<template>
  <section class="space-y-4">
    <header class="flex flex-wrap items-center justify-between gap-2">
      <div>
        <h1 class="text-2xl font-semibold">Calendar</h1>
        <p v-if="pub" class="text-sm text-gray-500">{{ pub.name }}</p>
      </div>
      <div class="flex items-center gap-2 text-sm">
        <div class="rounded border border-gray-300 bg-white p-0.5 text-xs">
          <button
            v-for="v in ['month', 'week'] as const"
            :key="v"
            class="rounded px-2 py-1"
            :class="view === v ? 'bg-gray-900 text-white' : 'text-gray-600'"
            @click="setView(v)"
          >{{ v }}</button>
        </div>
        <button class="rounded border border-gray-300 px-2 py-1" @click="shift(-1)">‹</button>
        <button class="rounded border border-gray-300 px-2 py-1" @click="jumpToToday">today</button>
        <button class="rounded border border-gray-300 px-2 py-1" @click="shift(1)">›</button>
        <strong class="ml-2 min-w-[10ch] text-right">{{ title }}</strong>
      </div>
    </header>

    <div v-if="error" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
      Failed to load issues: {{ error.message }}
    </div>

    <div class="grid grid-cols-7 gap-px overflow-hidden rounded border border-gray-200 bg-gray-200 text-xs">
      <div
        v-for="d in ['Sun','Mon','Tue','Wed','Thu','Fri','Sat']"
        :key="d"
        class="bg-gray-50 px-2 py-1 text-center font-medium text-gray-500"
      >{{ d }}</div>
      <div
        v-for="cell in cells"
        :key="cell.iso"
        class="min-h-[6rem] bg-white p-1"
        :class="!cell.inMonth ? 'bg-gray-50 text-gray-400' : ''"
      >
        <div class="flex items-center justify-between">
          <span :class="cell.isToday ? 'rounded bg-gray-900 px-1.5 text-white' : ''">
            {{ cell.date.getDate() }}
          </span>
        </div>
        <div class="mt-1 space-y-1">
          <button
            v-for="iss in (byDay[cell.iso] ?? [])"
            :key="iss.id"
            type="button"
            class="block w-full truncate rounded border px-1.5 py-0.5 text-left text-[11px]"
            :class="STATE_COLOR[iss.state]"
            :title="(iss.subject ?? iss.state) + ' — ' + iss.state"
            @click="goto(iss)"
          >
            {{ iss.subject ?? "(" + iss.state + ")" }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="items.length === 0" class="text-sm text-gray-500">
      No issues in this range yet.
      <span v-if="pub?.cadence_rule">
        Next slot will appear when the scheduler runs.
      </span>
      <span v-else>
        This publication has no cadence — issues only appear when manually created.
      </span>
    </div>
  </section>
</template>
