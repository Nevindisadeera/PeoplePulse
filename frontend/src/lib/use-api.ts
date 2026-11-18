"use client";

import { useEffect, useState } from "react";
import { api, ApiError } from "./api";

type Result<T> = { loading: boolean; data: T | null; error: ApiError | null };

// useApi loads a GET endpoint and reloads when the path changes. Pass null to
// skip the request, for example when the user lacks the permission for it.
export function useApi<T>(path: string | null): Result<T> {
  const [state, setState] = useState<{ path: string | null; data: T | null; error: ApiError | null }>({
    path: null,
    data: null,
    error: null,
  });

  useEffect(() => {
    if (path === null) return;
    const controller = new AbortController();
    api<T>(path, { signal: controller.signal })
      .then((data) => setState({ path, data, error: null }))
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        const apiError = error instanceof ApiError ? error : new ApiError(0, "NETWORK", "The server could not be reached.");
        setState({ path, data: null, error: apiError });
      });
    return () => controller.abort();
  }, [path]);

  if (path === null) return { loading: false, data: null, error: null };
  // Until the response for the current path arrives, the previous one is stale.
  if (state.path !== path) return { loading: true, data: null, error: null };
  return { loading: false, data: state.data, error: state.error };
}
