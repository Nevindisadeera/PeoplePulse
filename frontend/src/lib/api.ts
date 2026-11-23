// Client for the PeoplePulse REST API. Requests go to /api/v1 on the same
// origin; next.config.ts forwards them to the Go backend, which keeps the
// refresh cookie first-party.

import type { Session } from "./types";

const BASE = "/api/v1";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields?: Record<string, string>,
    // The full response body, for endpoints that return more than the error envelope.
    public body?: unknown,
  ) {
    super(message);
  }
}

// errorMessage turns anything thrown by a request into text for the user.
export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return "The server could not be reached. Check your connection and try again.";
}

// The access token lives in memory only. After a page reload it is obtained
// again from the HttpOnly refresh cookie.
let accessToken: string | null = null;
let refreshInFlight: Promise<Session | null> | null = null;
let onSessionLost: (() => void) | null = null;

export function setSessionLostHandler(handler: (() => void) | null) {
  onSessionLost = handler;
}

async function parse(res: Response) {
  if (res.status === 204) return null;
  return res.json().catch(() => null);
}

function toError(res: Response, body: unknown): ApiError {
  const err = (body as { error?: { code?: string; message?: string; fields?: Record<string, string> } } | null)?.error;
  return new ApiError(res.status, err?.code ?? "ERROR", err?.message ?? "The request could not be completed.", err?.fields, body);
}

// refreshSession exchanges the refresh cookie for a new access token. Refresh
// tokens are single-use, so concurrent callers must share one request: a
// second request with the same cookie would look like token theft to the API
// and end the session.
export function refreshSession(): Promise<Session | null> {
  if (!refreshInFlight) {
    refreshInFlight = fetch(`${BASE}/auth/refresh`, { method: "POST", credentials: "include" })
      .then(async (res) => {
        if (!res.ok) {
          accessToken = null;
          return null;
        }
        const session = (await res.json()) as Session;
        accessToken = session.accessToken;
        return session;
      })
      .catch(() => null)
      .finally(() => {
        refreshInFlight = null;
      });
  }
  return refreshInFlight;
}

export async function login(email: string, password: string): Promise<Session> {
  const res = await fetch(`${BASE}/auth/login`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  const body = await parse(res);
  if (!res.ok) throw toError(res, body);
  accessToken = (body as Session).accessToken;
  return body as Session;
}

export async function logout(): Promise<void> {
  accessToken = null;
  await fetch(`${BASE}/auth/logout`, { method: "POST", credentials: "include" }).catch(() => undefined);
}

// json sends a JSON body; form sends multipart data (file uploads).
type Options = { method?: string; json?: unknown; form?: FormData; signal?: AbortSignal };

// api calls an authenticated endpoint. When the access token has expired it
// refreshes once and repeats the request.
export async function api<T>(path: string, options: Options = {}): Promise<T> {
  const send = () =>
    fetch(BASE + path, {
      method: options.method ?? (options.json === undefined && !options.form ? "GET" : "POST"),
      credentials: "include",
      signal: options.signal,
      headers: {
        ...(options.json === undefined ? {} : { "Content-Type": "application/json" }),
        ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      },
      body: options.form ?? (options.json === undefined ? undefined : JSON.stringify(options.json)),
    });

  let res = await send();
  if (res.status === 401) {
    if (await refreshSession()) {
      res = await send();
    } else {
      onSessionLost?.();
    }
  }
  const body = await parse(res);
  if (!res.ok) throw toError(res, body);
  return body as T;
}

// download fetches a file from an authenticated endpoint. A plain link cannot
// be used because the access token travels in a header, not a cookie.
export async function download(path: string): Promise<{ blob: Blob; filename: string }> {
  const send = () => fetch(BASE + path, { credentials: "include", headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {} });
  let res = await send();
  if (res.status === 401) {
    if (await refreshSession()) res = await send();
    else onSessionLost?.();
  }
  if (!res.ok) throw toError(res, await parse(res));
  const name = /filename="([^"]+)"/.exec(res.headers.get("Content-Disposition") ?? "")?.[1] ?? "download";
  return { blob: await res.blob(), filename: name };
}
