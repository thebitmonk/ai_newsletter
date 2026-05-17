<script setup lang="ts">
const { sendPasswordResetEmail } = useFirebaseAuth();

const email = ref("");
const submitting = ref(false);
const sent = ref(false);
const error = ref<string | null>(null);

async function onSubmit() {
  error.value = null;
  submitting.value = true;
  try {
    await sendPasswordResetEmail(email.value);
    sent.value = true;
  } catch (e) {
    error.value = friendlyFirebaseError(e);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <section class="mx-auto max-w-sm space-y-5">
    <h1 class="text-2xl font-semibold">Reset password</h1>

    <div v-if="sent" class="rounded border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-700">
      Check your inbox at <span class="font-medium">{{ email }}</span> for a reset link.
    </div>

    <form v-else class="space-y-3" @submit.prevent="onSubmit">
      <label class="block">
        <span class="text-sm text-gray-700">Email</span>
        <input
          v-model="email"
          type="email"
          required
          autocomplete="email"
          class="mt-1 w-full rounded border border-gray-300 px-3 py-2"
        />
      </label>
      <button
        type="submit"
        :disabled="submitting"
        class="w-full rounded bg-gray-900 px-3 py-2 text-white disabled:opacity-50"
      >
        {{ submitting ? "Sending…" : "Send reset link" }}
      </button>
    </form>

    <div v-if="error" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">
      {{ error }}
    </div>

    <p class="text-center text-sm text-gray-500">
      Remembered it?
      <NuxtLink to="/login" class="text-gray-900 underline">Sign in</NuxtLink>
    </p>
  </section>
</template>
