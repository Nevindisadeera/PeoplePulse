"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { Logo, Notice } from "@/components/ui";
import { ApiError } from "@/lib/api";
import { useAuth } from "@/lib/auth";

export default function LoginPage() {
  const auth = useAuth();
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (auth.status === "signedIn") router.replace("/");
  }, [auth.status, router]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await auth.signIn(email.trim(), password);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The server could not be reached. Check that the API is running and try again.");
      setBusy(false);
    }
  }

  const field = "h-12 w-full rounded-full bg-raised px-5 text-sm text-ink placeholder:text-muted";

  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-10">
      <div className="flex w-full max-w-sm flex-col gap-8 rounded-card bg-surface p-8">
        <div className="flex items-center gap-3">
          <Logo />
          <span className="display text-xl font-semibold">PeoplePulse</span>
        </div>
        <div className="flex flex-col gap-2">
          <h1 className="display text-4xl font-bold">Sign in</h1>
          <p className="text-sm text-muted">Employee turnover and retention management.</p>
        </div>
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
          <label className="flex flex-col gap-2 text-xs font-semibold text-muted">
            Email
            <input id="email" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@company.com" className={field} />
          </label>
          <label className="flex flex-col gap-2 text-xs font-semibold text-muted">
            Password
            <input id="password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} className={field} />
          </label>
          {error && <Notice tone="danger">{error}</Notice>}
          <button
            type="submit"
            disabled={busy || !email || !password}
            className="mt-2 h-12 rounded-full bg-lime text-sm font-bold text-ground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {busy ? "Signing in…" : "Sign in"}
          </button>
        </form>
        <p className="text-xs text-muted">Accounts are created by invitation. Ask your administrator if you do not have one.</p>
      </div>
    </main>
  );
}
