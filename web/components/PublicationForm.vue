<!--
  Shared editor for create + settings. Backend validation errors surface
  inline by mapping ApiError.code → the relevant field.
-->
<script setup lang="ts">
import type { ApiError } from "~/composables/useApi";
import type { Publication, PublicationCreateInput, PublicationUpdateInput } from "~/types/api";

const props = defineProps<{
  initial?: Publication;
  submitLabel: string;
}>();

const emit = defineEmits<{
  (e: "submit", payload: PublicationCreateInput | PublicationUpdateInput): void;
}>();

const name = ref(props.initial?.name ?? "");
const brief = ref(props.initial?.brief ?? "");
const timezone = ref(props.initial?.timezone ?? guessTimezone());
const cadenceRule = ref<string | null>(props.initial?.cadence_rule ?? null);
const min = ref(props.initial?.stories_per_issue_min ?? 3);
const max = ref(props.initial?.stories_per_issue_max ?? 7);
const introEnabled = ref(props.initial?.intro_enabled ?? true);
const leadTime = ref(normaliseLead(props.initial?.curation_lead_time));
const approvalGate = ref(props.initial?.approval_gate_enabled ?? false);

const submitting = ref(false);
const errorByField = ref<Record<string, string>>({});
const generalError = ref<string | null>(null);

// Curated IANA list — covers the common cases without forcing the user to
// know IANA naming. "Other (custom)" reveals a text field for everything else.
const COMMON_TIMEZONES = [
  "UTC",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Sao_Paulo",
  "Europe/London",
  "Europe/Berlin",
  "Europe/Paris",
  "Europe/Madrid",
  "Africa/Lagos",
  "Asia/Dubai",
  "Asia/Kolkata",
  "Asia/Singapore",
  "Asia/Tokyo",
  "Asia/Shanghai",
  "Australia/Sydney",
];

function guessTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

function normaliseLead(d?: string): string {
  if (!d) return "24h";
  return d.replace(/0m0s$/, "").replace(/0s$/, "");
}

const isCustomTimezone = ref(!COMMON_TIMEZONES.includes(timezone.value));
const timezoneSelect = ref(isCustomTimezone.value ? "__other__" : timezone.value);
watch(timezoneSelect, (v) => {
  if (v === "__other__") {
    isCustomTimezone.value = true;
  } else {
    isCustomTimezone.value = false;
    timezone.value = v;
  }
});

async function onSubmit() {
  errorByField.value = {};
  generalError.value = null;
  submitting.value = true;
  const payload: PublicationCreateInput | PublicationUpdateInput = {
    name: name.value,
    brief: brief.value,
    timezone: timezone.value,
    stories_per_issue_min: min.value,
    stories_per_issue_max: max.value,
    intro_enabled: introEnabled.value,
    curation_lead_time: leadTime.value,
    approval_gate_enabled: approvalGate.value,
  };
  // The CadenceBuilder emits null when "No automatic schedule" is selected.
  if (cadenceRule.value) {
    payload.cadence_rule = cadenceRule.value;
  } else if (props.initial?.cadence_rule) {
    // Was set, now cleared → tell the backend to unset it.
    (payload as PublicationUpdateInput).unset_cadence_rule = true;
  }
  try {
    emit("submit", payload);
  } finally {
    submitting.value = false;
  }
}

defineExpose({
  setError(err: ApiError) {
    const code = err.code;
    if (code === "invalid_timezone") errorByField.value.timezone = err.message;
    else if (code === "invalid_cadence_rule") errorByField.value.cadenceRule = err.message;
    else if (code === "invalid_curation_lead_time") errorByField.value.leadTime = err.message;
    else generalError.value = err.message;
  },
});
</script>

<template>
  <form class="space-y-4" @submit.prevent="onSubmit">
    <label class="block">
      <span class="text-sm text-gray-700">Name</span>
      <input
        v-model="name"
        required
        maxlength="200"
        class="mt-1 w-full rounded border border-gray-300 px-3 py-2"
      />
    </label>

    <label class="block">
      <span class="text-sm text-gray-700">Brief — editorial voice, audience, exclusions</span>
      <textarea
        v-model="brief"
        rows="5"
        class="mt-1 w-full rounded border border-gray-300 px-3 py-2"
      />
      <span class="mt-1 block text-xs text-gray-500">
        Injected verbatim into every LLM call (ranker, summarizer, image gen). See ADR-0005.
      </span>
    </label>

    <div class="space-y-2">
      <label class="block">
        <span class="text-sm text-gray-700">Timezone</span>
        <select
          v-model="timezoneSelect"
          class="mt-1 w-full rounded border border-gray-300 bg-white px-3 py-2 text-sm"
        >
          <option v-for="tz in COMMON_TIMEZONES" :key="tz" :value="tz">{{ tz }}</option>
          <option value="__other__">Other (custom IANA name)…</option>
        </select>
      </label>
      <label v-if="isCustomTimezone" class="block">
        <span class="text-xs text-gray-500">Custom IANA timezone</span>
        <input
          v-model="timezone"
          required
          placeholder="Pacific/Auckland"
          class="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-sm"
        />
      </label>
      <span v-if="errorByField.timezone" class="block text-xs text-red-600">{{ errorByField.timezone }}</span>
    </div>

    <CadenceBuilder v-model="cadenceRule" :timezone="timezone" />
    <span v-if="errorByField.cadenceRule" class="block text-xs text-red-600">{{ errorByField.cadenceRule }}</span>

    <div class="grid grid-cols-2 gap-3">
      <label class="block">
        <span class="text-sm text-gray-700">Stories per issue — min</span>
        <input
          v-model.number="min"
          type="number"
          min="1"
          max="20"
          required
          class="mt-1 w-full rounded border border-gray-300 px-3 py-2"
        />
      </label>
      <label class="block">
        <span class="text-sm text-gray-700">max</span>
        <input
          v-model.number="max"
          type="number"
          min="1"
          max="20"
          required
          class="mt-1 w-full rounded border border-gray-300 px-3 py-2"
        />
      </label>
    </div>

    <label class="block">
      <span class="text-sm text-gray-700">Curation lead time (e.g. 24h, 12h, 6h)</span>
      <input
        v-model="leadTime"
        required
        class="mt-1 w-full rounded border border-gray-300 px-3 py-2"
      />
      <span class="mt-1 block text-xs text-gray-500">
        How long before send-time the curation pipeline runs.
      </span>
      <span v-if="errorByField.leadTime" class="mt-1 block text-xs text-red-600">{{ errorByField.leadTime }}</span>
    </label>

    <label class="flex items-center gap-2">
      <input v-model="introEnabled" type="checkbox" />
      <span class="text-sm">Generate an intro paragraph per issue</span>
    </label>

    <label class="flex items-center gap-2">
      <input v-model="approvalGate" type="checkbox" />
      <span class="text-sm">
        Require approval before send (per ADR-0002)
      </span>
    </label>

    <div v-if="generalError" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">
      {{ generalError }}
    </div>

    <button
      type="submit"
      :disabled="submitting"
      class="w-full rounded bg-gray-900 px-3 py-2 text-white disabled:opacity-50"
    >
      {{ submitting ? "Saving…" : submitLabel }}
    </button>
  </form>
</template>
