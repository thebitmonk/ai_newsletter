import type { Source, SourceListResp } from "~/types/api";

export interface SourceCreateInput {
  type: Source["type"];
  identifier: string;
  poll_interval?: string;
  enabled?: boolean;
}

export interface SourceUpdateInput {
  identifier?: string;
  poll_interval?: string;
  enabled?: boolean;
}

export function useSources(publicationId: string) {
  const api = useApi();
  const base = `/publications/${publicationId}/sources`;
  return {
    list: () => api.get<SourceListResp>(base),
    create: (input: SourceCreateInput) => api.post<Source>(base, input),
    update: (sourceId: string, patch: SourceUpdateInput) =>
      api.patch<Source>(`${base}/${sourceId}`, patch),
    remove: (sourceId: string) => api.del(`${base}/${sourceId}`),
  };
}
