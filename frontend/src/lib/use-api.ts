"use client";

import { useCallback, useEffect, useState } from "react";
import { api, ApiError } from "./api";

type Result<T> = { loading: boolean; data: T | null; error: ApiError | null; reload: () => void };

// useApi loads a GET endpoint and reloads when the path changes or reload()
// is called. Pass null to skip the request, for example when the user lacks
// the permission for it.
export function useApi<T>(path: string | null): Result<T> {
  const [version, setVersion] = useState(0);
  const [state, setState] = useState<{ key: string | null; data: T | null; error: ApiError | null }>({
    key: null,
    data: null,
    error: null,
  });
  const reload = useCallback(() => setVersion((v) => v + 1), []);
  const key = path === null ? null : `${path}#${version}`;

  useEffect(() => {
    if (path === null || key === null) return;
    const controller = new AbortController();
    api<T>(path, { signal: controller.signal })
      .then((data) => setState({ key, data, error: null }))
      .catch((error: unknown) => {
        if (controller.signal.aborted) return;
        const apiError = error instanceof ApiError ? error : new ApiError(0, "NETWORK", "The server could not be reached.");
        setState({ key, data: null, error: apiError });
      });
    return () => controller.abort();
  }, [path, key]);

  if (key === null) return { loading: false, data: null, error: null, reload };
  if (state.key !== key) {
    // While reloading the same path, keep showing the previous data instead of flashing a skeleton.
    const samePath = state.key?.startsWith(`${path}#`) ?? false;
    return { loading: !samePath, data: samePath ? state.data : null, error: null, reload };
  }
  return { loading: false, data: state.data, error: state.error, reload };
}
