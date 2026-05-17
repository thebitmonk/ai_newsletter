<script setup lang="ts">
const { currentUser, signOut } = useFirebaseAuth();

async function onSignOut() {
  await signOut();
  await navigateTo("/login");
}
</script>

<template>
  <div class="min-h-screen bg-gray-50 text-gray-900">
    <header
      v-if="currentUser"
      class="flex items-center justify-between border-b border-gray-200 bg-white px-6 py-3"
    >
      <div class="flex items-center gap-4">
        <NuxtLink to="/" class="font-semibold">ai_newsletter</NuxtLink>
        <ClientOnly>
          <PublicationSwitcher />
        </ClientOnly>
      </div>
      <div class="flex items-center gap-3 text-sm">
        <span class="text-gray-500">{{ currentUser.email }}</span>
        <button
          type="button"
          class="rounded border border-gray-300 px-2 py-1 hover:bg-gray-100"
          @click="onSignOut"
        >
          Sign out
        </button>
      </div>
    </header>
    <main class="mx-auto max-w-3xl px-6 py-8">
      <slot />
    </main>
  </div>
</template>
