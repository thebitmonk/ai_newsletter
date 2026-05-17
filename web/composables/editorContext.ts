// Provide/inject keys so node views can request regenerate without each
// view re-implementing the API call. The editor page wires the
// implementation; node views just call.

import type { InjectionKey } from "vue";

export type RegenerateFn = (kind: "summary" | "image", storyId: string) => Promise<void>;

export const RegenerateKey: InjectionKey<{
  call: RegenerateFn;
  status: Ref<"idle" | "pending" | "error">;
  pendingId: Ref<string | null>; // story id currently being regenerated (or "__cover__")
  errorMessage: Ref<string | null>;
}> = Symbol("Regenerate");

export const COVER_PENDING_ID = "__cover__";
