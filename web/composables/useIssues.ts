import type { IssueListResp, IssueSummary } from "~/types/api";

export interface IssueListFilter {
  state?: IssueSummary["state"][];
  scheduledAfter?: Date;
  scheduledBefore?: Date;
  limit?: number;
  cursor?: string;
}

function buildQuery(f: IssueListFilter): Record<string, unknown> {
  const q: Record<string, unknown> = {};
  if (f.state?.length) q.state = f.state;
  if (f.scheduledAfter) q.scheduled_after = f.scheduledAfter.toISOString();
  if (f.scheduledBefore) q.scheduled_before = f.scheduledBefore.toISOString();
  if (f.limit) q.limit = f.limit;
  if (f.cursor) q.cursor = f.cursor;
  return q;
}

export function useIssues(publicationId: string) {
  const api = useApi();
  return {
    list: (filter: IssueListFilter = {}) =>
      api.get<IssueListResp>(`/publications/${publicationId}/issues`, {
        query: buildQuery(filter),
      }),
    get: <T = unknown>(issueId: string) => api.get<T>(`/issues/${issueId}`),
  };
}
