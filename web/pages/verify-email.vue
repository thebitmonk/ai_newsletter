<script setup lang="ts">
const { currentUser, sendVerificationEmail, signOut } = useFirebaseAuth();

const resending = ref(false);
const resendMessage = ref<string | null>(null);
const resendError = ref<string | null>(null);

async function onResend() {
  resendMessage.value = null;
  resendError.value = null;
  resending.value = true;
  try {
    await sendVerificationEmail();
    resendMessage.value = "Verification email sent. Check your inbox.";
  } catch (e) {
    resendError.value = friendlyFirebaseError(e);
  } finally {
    resending.value = false;
  }
}

async function onSignOut() {
  await signOut();
  await navigateTo("/login");
}

// Poll currentUser.reload() every 3s; flip when emailVerified becomes true.
onMounted(() => {
  const id = window.setInterval(async () => {
    if (!currentUser.value) return;
    try {
      await currentUser.value.reload();
    } catch {
      // network blip; try again on next tick.
      return;
    }
    if (currentUser.value.emailVerified) {
      window.clearInterval(id);
      await navigateTo("/");
    }
  }, 3000);
  onBeforeUnmount(() => window.clearInterval(id));
});
</script>

<template>
  <section class="mx-auto max-w-sm space-y-5">
    <h1 class="text-2xl font-semibold">Verify your email</h1>

    <p class="text-sm text-gray-600">
      We sent a verification link to
      <span class="font-medium">{{ currentUser?.email ?? "your inbox" }}</span>.
      Click it to continue. This page will redirect automatically once your
      email is confirmed.
    </p>

    <div class="space-y-2">
      <button
        type="button"
        :disabled="resending"
        class="w-full rounded bg-gray-900 px-3 py-2 text-white disabled:opacity-50"
        @click="onResend"
      >
        {{ resending ? "Sending…" : "Resend verification email" }}
      </button>
      <button
        type="button"
        class="w-full rounded border border-gray-300 px-3 py-2 hover:bg-gray-50"
        @click="onSignOut"
      >
        Sign out and use a different account
      </button>
    </div>

    <div v-if="resendMessage" class="rounded border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-700">
      {{ resendMessage }}
    </div>
    <div v-if="resendError" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700">
      {{ resendError }}
    </div>
  </section>
</template>
