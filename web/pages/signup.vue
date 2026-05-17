<script setup lang="ts">
const { signUpWithEmail, signInWithGoogle, sendVerificationEmail } = useFirebaseAuth();

const email = ref("");
const password = ref("");
const error = ref<string | null>(null);
const submitting = ref(false);

async function onSubmit() {
  error.value = null;
  if (password.value.length < 8) {
    error.value = "Password must be at least 8 characters.";
    return;
  }
  submitting.value = true;
  try {
    await signUpWithEmail(email.value, password.value);
    await sendVerificationEmail();
    await navigateTo("/verify-email");
  } catch (e) {
    error.value = friendlyFirebaseError(e);
  } finally {
    submitting.value = false;
  }
}

async function onGoogle() {
  error.value = null;
  submitting.value = true;
  try {
    await signInWithGoogle();
    await navigateTo("/");
  } catch (e) {
    error.value = friendlyFirebaseError(e);
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <section class="mx-auto max-w-sm space-y-5">
    <h1 class="text-2xl font-semibold">Create an account</h1>

    <form class="space-y-3" @submit.prevent="onSubmit">
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
      <label class="block">
        <span class="text-sm text-gray-700">Password (8+ chars)</span>
        <input
          v-model="password"
          type="password"
          required
          minlength="8"
          autocomplete="new-password"
          class="mt-1 w-full rounded border border-gray-300 px-3 py-2"
        />
      </label>
      <button
        type="submit"
        :disabled="submitting"
        class="w-full rounded bg-gray-900 px-3 py-2 text-white disabled:opacity-50"
      >
        {{ submitting ? "Creating account…" : "Sign up" }}
      </button>
    </form>

    <div class="flex items-center gap-3 text-xs text-gray-400">
      <hr class="flex-1" /> or <hr class="flex-1" />
    </div>

    <button
      type="button"
      :disabled="submitting"
      class="w-full rounded border border-gray-300 px-3 py-2 hover:bg-gray-50 disabled:opacity-50"
      @click="onGoogle"
    >
      Continue with Google
    </button>

    <div v-if="error" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">
      {{ error }}
    </div>

    <p class="text-center text-sm text-gray-500">
      Already have an account?
      <NuxtLink to="/login" class="text-gray-900 underline">Sign in</NuxtLink>
    </p>
  </section>
</template>
