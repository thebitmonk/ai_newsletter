import { defineStore } from "pinia";

// Holds which Publication is currently in focus (for the Sources/Calendar/
// Editor screens that all live under a Publication). Persisted across
// reloads via localStorage so deep-link navigation stays consistent with
// the topbar selector.
export const useCurrentPublication = defineStore("currentPublication", {
  state: () => ({ id: null as string | null }),
  actions: {
    set(id: string | null) {
      this.id = id;
      if (import.meta.client) {
        if (id) localStorage.setItem("currentPublicationId", id);
        else localStorage.removeItem("currentPublicationId");
      }
    },
    hydrateFromStorage() {
      if (import.meta.client && !this.id) {
        const stored = localStorage.getItem("currentPublicationId");
        if (stored) this.id = stored;
      }
    },
  },
});
