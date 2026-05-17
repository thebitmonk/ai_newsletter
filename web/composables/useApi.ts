// Backend HTTP client. Injects the Firebase ID token, retries once on 401
// after forcing a token refresh, and normalises the backend's
// {error: {code, message}} envelope into a thrown JS Error with .code + .message.

export interface ApiError extends Error {
  code: string;
  status: number;
  details?: unknown;
}

interface ApiClient {
  get: <T>(path: string, opts?: { query?: Record<string, unknown> }) => Promise<T>;
  post: <T>(path: string, body?: unknown) => Promise<T>;
  put: <T>(path: string, body?: unknown) => Promise<T>;
  patch: <T>(path: string, body?: unknown) => Promise<T>;
  del: (path: string) => Promise<void>;
}

function makeApiError(status: number, code: string, message: string, details?: unknown): ApiError {
  const e = new Error(message) as ApiError;
  e.code = code;
  e.status = status;
  if (details !== undefined) e.details = details;
  return e;
}

export function useApi(): ApiClient {
  const { idToken, refreshIdToken } = useFirebaseAuth();
  const base = useRuntimeConfig().public.apiBase;

  async function request<T>(
    method: string,
    path: string,
    opts: { body?: unknown; query?: Record<string, unknown> } = {},
  ): Promise<T> {
    const url = base + path;
    const doFetch = async (token: string | null) =>
      await $fetch.raw<T>(url, {
        method: method as never,
        body: opts.body as never,
        query: opts.query,
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
        ignoreResponseError: true,
      });

    let resp = await doFetch(idToken.value);
    if (resp.status === 401) {
      const fresh = await refreshIdToken();
      if (fresh) resp = await doFetch(fresh);
    }

    if (resp.status >= 200 && resp.status < 300) {
      return (resp._data ?? undefined) as T;
    }

    // Normalise the backend error envelope into a thrown Error.
    const body = resp._data as { error?: { code?: string; message?: string; details?: unknown } } | undefined;
    const code = body?.error?.code ?? "http_error";
    const message = body?.error?.message ?? `HTTP ${resp.status}`;
    throw makeApiError(resp.status, code, message, body?.error?.details);
  }

  return {
    get: (path, opts) => request("GET", path, { query: opts?.query }),
    post: (path, body) => request("POST", path, { body }),
    put: (path, body) => request("PUT", path, { body }),
    patch: (path, body) => request("PATCH", path, { body }),
    del: (path) => request("DELETE", path),
  };
}
