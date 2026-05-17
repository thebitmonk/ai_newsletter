import type {
  Publication,
  PublicationCreateInput,
  PublicationListResp,
  PublicationUpdateInput,
} from "~/types/api";

export function usePublications() {
  const api = useApi();
  return {
    list: (opts: { cursor?: string; limit?: number } = {}) =>
      api.get<PublicationListResp>("/publications", { query: opts }),
    get: (id: string) => api.get<Publication>(`/publications/${id}`),
    create: (input: PublicationCreateInput) =>
      api.post<Publication>("/publications", input),
    update: (id: string, patch: PublicationUpdateInput) =>
      api.patch<Publication>(`/publications/${id}`, patch),
    remove: (id: string) => api.del(`/publications/${id}`),
  };
}
