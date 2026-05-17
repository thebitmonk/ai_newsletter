// Nuxt 3 config. Backend API is proxied via /api/v1/* so no CORS needed in dev.
// Public runtime config is hydrated from NUXT_PUBLIC_* env vars per Nuxt
// convention; nothing here is sensitive (Firebase Web SDK config is
// intentionally public — see ADR-0016).

export default defineNuxtConfig({
  ssr: true,
  devtools: { enabled: true },
  modules: [
    "@nuxtjs/tailwindcss",
    "@pinia/nuxt",
    "@vueuse/nuxt",
  ],
  runtimeConfig: {
    public: {
      firebaseApiKey: "",
      firebaseAuthDomain: "",
      firebaseProjectId: "",
      firebaseAuthEmulator: "",
      apiBase: "/api/v1",
    },
  },
  nitro: {
    devProxy: {
      "/api/v1": {
        target: "http://localhost:8080/api/v1",
        changeOrigin: true,
      },
    },
  },
  typescript: {
    strict: true,
  },
  compatibilityDate: "2026-05-01",
});
