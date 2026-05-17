// Story/cover regenerate API wrapper. Returns the updated Issue so the
// editor page can swap in the new body_doc.

import type { ApiError } from "~/composables/useApi";

export interface RegeneratedIssue {
  id: string;
  state: string;
  subject: string | null;
  title: string | null;
  cover_url: string | null;
  body_doc: unknown;
}

export function useIssueRegen() {
  const api = useApi();
  return {
    regenerateStorySummary: (issueId: string, storyId: string) =>
      api.post<RegeneratedIssue>(
        `/issues/${issueId}/stories/${storyId}/regenerate`,
        { type: "summary" },
      ),
    regenerateCover: (issueId: string, anyStoryId: string) =>
      api.post<RegeneratedIssue>(
        // story_id is ignored for type=image but the URL still requires one;
        // pass any present story id (the editor knows at least one exists).
        `/issues/${issueId}/stories/${anyStoryId}/regenerate`,
        { type: "image" },
      ),
  };
}

// Used by buttons to format the backend error envelope into UI text.
export function regenErrorMessage(e: unknown): string {
  const err = e as ApiError;
  switch (err.code) {
    case "wrong_state":
      return "Can only regenerate while the issue is drafted or approved.";
    case "candidate_expired":
      return "The source for this story has expired from the pool. Re-poll the source and try again.";
    case "story_not_found":
      return "Story is missing from the document — refresh the page.";
    default:
      return err.message ?? "Regenerate failed.";
  }
}
