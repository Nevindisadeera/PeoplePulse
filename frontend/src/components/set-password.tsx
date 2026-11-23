"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useState } from "react";
import { Logo, Notice } from "./ui";

// AuthCard is the centred panel used by the signed-out pages.
export function AuthCard({ children }: { children: React.ReactNode }) {
  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-10">
      <div className="flex w-full max-w-sm flex-col gap-8 rounded-card bg-surface p-8">
        <div className="flex items-center gap-3">
          <Logo />
          <span className="display text-xl font-semibold">PeoplePulse</span>
        </div>
        {children}
      </div>
    </main>
  );
}

export const authField = "h-12 w-full rounded-full bg-raised px-5 text-sm text-ink placeholder:text-muted";
export const authButton = "mt-2 flex h-12 items-center justify-center rounded-full bg-lime text-sm font-bold text-ground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50";

// SetPassword is the form behind an invitation link and a password reset
// link. Both are public pages where the single-use token in the link proves
// who the person is.
export function SetPassword({ endpoint, title, intro, doneText }: { endpoint: string; title: string; intro: string; doneText: string }) {
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
      const res = await fetch(`/api/v1${endpoint}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token, password }) });
      if (!res.ok) {
        const body = await res.json().catch(() => null);
        const fields = body?.error?.fields as Record<string, string> | undefined;
        setError(fields?.password ? `Password ${fields.password}.` : (body?.error?.message ?? "The password could not be set."));
      } else {
        setDone(true);
      }
    } catch {
      setError("The server could not be reached. Try again.");
    }
    setBusy(false);
  }

  if (done) {
    return (
      <AuthCard>
        <div className="flex flex-col gap-2">
          <h1 className="display text-4xl font-bold">You are set</h1>
          <p className="text-sm text-muted">{doneText}</p>
        </div>
        <Link href="/login" className={authButton}>
          Go to sign in
        </Link>
      </AuthCard>
    );
  }

  return (
    <AuthCard>
      <div className="flex flex-col gap-2">
        <h1 className="display text-4xl font-bold">{title}</h1>
        <p className="text-sm text-muted">{intro}</p>
      </div>
      {!token && <Notice tone="danger">This link is incomplete. Open the link exactly as you received it.</Notice>}
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <label className="flex flex-col gap-2 text-xs font-semibold text-muted">
          New password
          <input id="password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} className={authField} />
          <span className="font-normal">At least 12 characters.</span>
        </label>
        <label className="flex flex-col gap-2 text-xs font-semibold text-muted">
          Repeat password
          <input id="confirm" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} className={authField} />
          {mismatch && <span className="font-medium text-danger">The two passwords are different.</span>}
        </label>
        {error && <Notice tone="danger">{error}</Notice>}
        <button type="submit" disabled={busy || !token || password.length === 0 || password !== confirm} className={authButton}>
          {busy ? "Saving…" : "Set password"}
        </button>
      </form>
    </AuthCard>
  );
}
