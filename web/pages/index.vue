<!--
  Placeholder home page. Proves the chain
    Firebase ID token → useApi → backend /whoami → JSON back
  works end-to-end. Real dashboard lands in slice #17.
-->
<script setup lang="ts">
const api = useApi();

interface WhoamiResponse {
  user_id: string;
  account_id: string;
  email: string;
  email_verified: boolean;
}

const { data, error, refresh } = await useAsyncData("whoami", () =>
  api.get<WhoamiResponse>("/whoami"),
);
</script>

<template>
  <section class="space-y-4">
    <h1 class="text-2xl font-semibold">You're signed in</h1>
    <p class="text-sm text-gray-500">
      This placeholder proves the Firebase token → backend chain works.
      Publications dashboard lands in slice #17.
    </p>

    <div v-if="error" class="rounded border border-red-300 bg-red-50 p-3 text-sm">
      <div class="font-medium text-red-800">Failed to reach backend</div>
      <pre class="mt-1 whitespace-pre-wrap text-red-700">{{ error.message }}</pre>
      <button class="mt-2 underline" @click="refresh()">Retry</button>
    </div>

    <dl v-else-if="data" class="rounded border border-gray-200 bg-white p-4 text-sm">
      <div class="flex gap-3 py-1">
        <dt class="w-32 text-gray-500">account_id</dt>
        <dd class="font-mono">{{ data.account_id }}</dd>
      </div>
      <div class="flex gap-3 py-1">
        <dt class="w-32 text-gray-500">user_id</dt>
        <dd class="font-mono">{{ data.user_id }}</dd>
      </div>
      <div class="flex gap-3 py-1">
        <dt class="w-32 text-gray-500">email</dt>
        <dd>{{ data.email }}</dd>
      </div>
      <div class="flex gap-3 py-1">
        <dt class="w-32 text-gray-500">verified</dt>
        <dd>{{ data.email_verified ? "✓" : "✗" }}</dd>
      </div>
    </dl>
  </section>
</template>
