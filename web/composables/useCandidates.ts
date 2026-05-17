export interface CandidateItem {
  id: string;
  publication_id: string;
  source_id: string;
  source_type: string;
  source_identifier: string;
  url: string;
  title: string;
  fetched_at: string;
  expires_at: string;
}

export interface CandidatesListResp {
  items: CandidateItem[];
  next_cursor: string | null;
}

export function useCandidates(publicationId: string) {
  const api = useApi();
  return {
    list: (opts: { cursor?: string; limit?: number } = {}) =>
      api.get<CandidatesListResp>(`/publications/${publicationId}/candidates`, {
        query: opts,
      }),
  };
}
