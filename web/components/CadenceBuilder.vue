<!--
  Frequency picker + time picker + per-frequency widgets that emit a single
  reactive RRULE string. Provides round-trip support so the settings page
  populates the right UI shape from an existing rule.
-->
<script setup lang="ts">
import {
  DAYS,
  buildRRULE,
  defaultCadence,
  describe,
  parseRRULE,
  type CadenceState,
  type DayCode,
  type Frequency,
} from "~/utils/cadence";

const props = defineProps<{
  modelValue: string | null;
  timezone?: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: string | null): void;
}>();

// Hydrate from the incoming RRULE on mount; subsequent updates flow through
// emits so this stays as a pure controlled component.
const state = ref<CadenceState>(parseRRULE(props.modelValue));

// Re-parse if the parent swaps the value externally (e.g. settings page
// loads an existing publication).
watch(
  () => props.modelValue,
  (next, prev) => {
    if (next === prev) return;
    // Only re-parse when the incoming value isn't what we just emitted,
    // otherwise we'd reset user-in-progress edits on every keystroke.
    if (buildRRULE(state.value) === next) return;
    state.value = parseRRULE(next);
  },
);

watch(
  state,
  (s) => emit("update:modelValue", buildRRULE(s)),
  { deep: true },
);

const FREQUENCIES: { value: Frequency; label: string }[] = [
  { value: "none",    label: "No automatic schedule (ad-hoc only)" },
  { value: "daily",   label: "Every day" },
  { value: "weekly",  label: "Every week" },
  { value: "monthly", label: "Every month" },
  { value: "custom",  label: "Custom (advanced — raw RRULE)" },
];

const timeString = computed({
  get: () => `${String(state.value.hour).padStart(2, "0")}:${String(state.value.minute).padStart(2, "0")}`,
  set: (v: string) => {
    const [h, m] = v.split(":").map(Number);
    if (!Number.isNaN(h)) state.value.hour = h;
    if (!Number.isNaN(m)) state.value.minute = m;
  },
});

function toggleDay(day: DayCode) {
  const has = state.value.byDay.includes(day);
  state.value.byDay = has
    ? state.value.byDay.filter((d) => d !== day)
    : [...state.value.byDay, day].sort((a, b) =>
        DAYS.findIndex((x) => x.value === a) - DAYS.findIndex((x) => x.value === b),
      );
}

const preview = computed(() => describe(state.value, props.timezone ?? "UTC"));

// Convenience: when user picks Custom and the field is empty, seed it with
// the current friendly rule so they can edit a real example.
watch(
  () => state.value.frequency,
  (next, prev) => {
    if (next === "custom" && prev !== "custom" && !state.value.customRule) {
      const built = buildRRULE({ ...state.value, frequency: prev as Frequency });
      if (built) state.value.customRule = built;
    }
  },
);
</script>

<template>
  <div class="space-y-3 rounded border border-gray-200 bg-gray-50 p-3">
    <label class="block">
      <span class="text-sm text-gray-700">Schedule</span>
      <select
        v-model="state.frequency"
        class="mt-1 w-full rounded border border-gray-300 bg-white px-3 py-2 text-sm"
      >
        <option v-for="f in FREQUENCIES" :key="f.value" :value="f.value">
          {{ f.label }}
        </option>
      </select>
    </label>

    <template v-if="state.frequency === 'daily' || state.frequency === 'weekly' || state.frequency === 'monthly'">
      <label class="block">
        <span class="text-sm text-gray-700">Time of day (in the publication's timezone)</span>
        <input
          v-model="timeString"
          type="time"
          class="mt-1 w-32 rounded border border-gray-300 bg-white px-3 py-2 text-sm"
        />
      </label>
    </template>

    <div v-if="state.frequency === 'weekly'">
      <span class="text-sm text-gray-700">Days of week</span>
      <div class="mt-1 flex flex-wrap gap-2">
        <button
          v-for="day in DAYS"
          :key="day.value"
          type="button"
          class="rounded-full border px-3 py-1 text-xs"
          :class="state.byDay.includes(day.value)
            ? 'border-gray-900 bg-gray-900 text-white'
            : 'border-gray-300 bg-white text-gray-700'"
          @click="toggleDay(day.value)"
        >
          {{ day.short }}
        </button>
      </div>
    </div>

    <label v-if="state.frequency === 'monthly'" class="block">
      <span class="text-sm text-gray-700">Day of month</span>
      <select
        v-model.number="state.monthDay"
        class="mt-1 w-32 rounded border border-gray-300 bg-white px-3 py-2 text-sm"
      >
        <option v-for="n in 31" :key="n" :value="n">{{ n }}</option>
      </select>
      <span class="mt-1 block text-xs text-gray-500">
        If the day doesn't exist in a given month (e.g. 31st in February), the schedule skips that month.
      </span>
    </label>

    <label v-if="state.frequency === 'custom'" class="block">
      <span class="text-sm text-gray-700">Raw RRULE</span>
      <textarea
        v-model="state.customRule"
        rows="2"
        class="mt-1 w-full rounded border border-gray-300 bg-white px-3 py-2 font-mono text-xs"
        placeholder="FREQ=WEEKLY;BYDAY=MO,WE,FR;BYHOUR=9;BYMINUTE=0;BYSECOND=0"
      />
      <span class="mt-1 block text-xs text-gray-500">
        Full
        <a class="underline" target="_blank" href="https://tools.ietf.org/html/rfc5545#section-3.3.10">RFC 5545</a>
        RRULE syntax — use when the friendly options above don't cover your case.
      </span>
    </label>

    <p class="text-xs text-gray-600">{{ preview }}</p>
  </div>
</template>
