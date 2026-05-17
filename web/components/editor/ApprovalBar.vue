<!--
  Sticky approve action for drafted/approved Issues when the parent
  Publication has approval_gate_enabled. Shows a live countdown to the
  send time and disables the approve action inside the 60s freeze window
  per ADR-0007.
-->
<script setup lang="ts">
import type { ApiError } from "~/composables/useApi";

const props = defineProps<{
  issueId: string;
  state: string;
  scheduledAt: string;
}>();

const emit = defineEmits<{
  (e: "approved", body: unknown): void;
}>();

const api = useApi();
const showConfirm = ref(false);
const submitting = ref(false);
const error = ref<string | null>(null);

const FREEZE_WINDOW_MS = 60_000;

const now = ref(Date.now());
let timer: ReturnType<typeof setInterval> | null = null;
onMounted(() => {
  timer = setInterval(() => { now.value = Date.now(); }, 1000);
});
onBeforeUnmount(() => { if (timer) clearInterval(timer); });

const msToSend = computed(() => new Date(props.scheduledAt).getTime() - now.value);
const inFreezeWindow = computed(() => msToSend.value <= FREEZE_WINDOW_MS);

const countdownText = computed(() => {
  const ms = msToSend.value;
  if (ms <= 0) return "Sending now";
  const totalSec = Math.floor(ms / 1000);
  const days = Math.floor(totalSec / 86400);
  const hours = Math.floor((totalSec % 86400) / 3600);
  const mins = Math.floor((totalSec % 3600) / 60);
  const parts: string[] = [];
  if (days) parts.push(`${days}d`);
  if (hours) parts.push(`${hours}h`);
  if (mins || (!days && !hours)) parts.push(`${mins}m`);
  return parts.join(" ");
});

async function onConfirmApprove() {
  showConfirm.value = false;
  submitting.value = true;
  error.value = null;
  try {
    const body = await api.post(`/issues/${props.issueId}/approve`);
    emit("approved", body);
  } catch (e) {
    const err = e as ApiError;
    error.value =
      err.code === "approval_window_closed"
        ? "The approval window just closed. Refresh to see the issue's final state."
        : err.code === "wrong_state"
        ? "This issue is no longer in drafted state — it may have been approved elsewhere."
        : err.message;
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div
    class="sticky bottom-4 z-10 mx-auto flex max-w-3xl items-center justify-between gap-3 rounded-lg border border-gray-300 bg-white px-4 py-3 shadow-lg"
  >
    <div class="text-sm">
      <template v-if="state === 'drafted'">
        <span class="font-semibold">Approval required</span>
        ·
        <span class="text-gray-600">Sends in {{ countdownText }}</span>
      </template>
      <template v-else-if="state === 'approved'">
        <span class="text-emerald-700">✓ Approved</span>
        ·
        <span class="text-gray-600">Sends in {{ countdownText }}</span>
      </template>
    </div>

    <div v-if="state === 'drafted'" class="flex items-center gap-2">
      <button
        v-if="!showConfirm"
        type="button"
        :disabled="inFreezeWindow || submitting"
        :title="inFreezeWindow ? 'Approval window closed (within 60s of send time)' : ''"
        class="rounded bg-emerald-600 px-3 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:cursor-not-allowed disabled:bg-gray-400"
        @click="showConfirm = true"
      >
        Approve &amp; send
      </button>
      <template v-else>
        <span class="text-xs text-gray-500">Send this issue?</span>
        <button
          type="button"
          class="rounded border border-gray-300 px-2 py-1 text-xs"
          @click="showConfirm = false"
        >
          cancel
        </button>
        <button
          type="button"
          :disabled="submitting"
          class="rounded bg-emerald-600 px-3 py-1 text-xs font-medium text-white"
          @click="onConfirmApprove"
        >
          {{ submitting ? "approving…" : "yes, approve" }}
        </button>
      </template>
    </div>
  </div>

  <div v-if="error" class="mt-2 rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">
    {{ error }}
  </div>
</template>
