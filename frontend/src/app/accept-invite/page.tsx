"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { Logo, Notice } from "@/components/ui";
import { ApiError } from "@/lib/api";

// The page an invited person lands on from their invitation link. It is
// public: the single-use token in the link is what proves who they are.
function AcceptInvite() {
  const token = useSearchParams().get("token") ?? "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const mismatch = confirm.length > 0 && confirm !== password;

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/v1/auth/accept-invite", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token, password }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        const fields = body?.error?.fields as Record<string, string> | undefined;
        throw new ApiError(res.status, body?.error?.code ?? "ERROR", fields?.password ? `Password ${fields.password}.` : (body?.error?.message ?? "The password could not be set."));
      }
      setDone(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The server could not be reached. Try again.");
    }
    setBusy(false);
  }

  const field = "h-12 w-full rounded-full bg-raised px-5 text-sm text-ink placeholder:text-muted";

  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-10">
      <div className="flex w-full max-w-sm flex-col gap-8 rounded-card bg-surface p-8">
        <div className="flex items-center gap-3">
          <Logo />
          <span className="display text-xl font-semibold">PeoplePulse</span>
        </div>
        {done ? (
          <>
            <div className="flex flex-col gap-2">
              <h1 className="display text-4xl font-bold">You are set</h1>
              <p className="text-sm text-muted">Your password is saved. Sign in with your email address and the password you just chose.</p>
            </div>
            <Link href="/login" className="flex h-12 items-center justify-center rounded-full bg-lime text-sm font-bold text-ground hover:opacity-90">
              Go to sign in
            </Link>
          </>
        ) : (
          <>
            <div className="flex flex-col gap-2">
              <h1 className="display text-4xl font-bold">Choose a password</h1>
              <p className="text-sm text-muted">You have been invited to PeoplePulse. Set a password to activate your account.</p>
            </div>
            {!token && <Notice tone="danger">This link is incomplete. Open the invitation link exactly as you received it.</Notice>}
            <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
              <label className="flex flex-col gap-2 text-xs font-semibold text-muted">
                New password
                <input id="password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} className={field} />
                <span className="font-normal">At least 12 characters.</span>
              </label>
              <label className="flex flex-col gap-2 text-xs font-semibold text-muted">
                Repeat password
                <input id="confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} className={field} />
                {mismatch && <span className="font-medium text-danger">The two passwords are different.</span>}
              </label>
              {error && <Notice tone="danger">{error}</Notice>}
              <button
                type="submit"
                disabled={busy || !token || password.length === 0 || password !== confirm}
                className="mt-2 h-12 rounded-full bg-lime text-sm font-bold text-ground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {busy ? "Saving…" : "Set password"}
              </button>
            </form>
          </>
        )}
      </div>
    </main>
  );
}

export default function AcceptInvitePage() {
  // useSearchParams needs a Suspense boundary so the page can be prerendered.
  return (
    <Suspense>
      <AcceptInvite />
    </Suspense>
  );
}
