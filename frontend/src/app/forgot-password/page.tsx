"use client";

import Link from "next/link";
import { useState } from "react";
import { AuthCard, authButton, authField } from "@/components/set-password";
import { Notice } from "@/components/ui";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = await fetch("/api/v1/auth/forgot-password", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ email: email.trim() }) });
      if (res.ok) {
        setSent(true);
      } else {
        const body = await res.json().catch(() => null);
        setError(body?.error?.fields?.email ? `Email ${body.error.fields.email}.` : (body?.error?.message ?? "The request could not be sent."));
      }
    } catch {
      setError("The server could not be reached. Try again.");
    }
    setBusy(false);
  }

  if (sent) {
    return (
      <AuthCard>
        <div className="flex flex-col gap-2">
          <h1 className="display text-4xl font-bold">Check your email</h1>
          {/* The same message whether or not the address has an account, so accounts cannot be discovered here. */}
          <p className="text-sm text-muted">
            If {email.trim()} belongs to an active account, a reset link has been sent to it. The link works once and expires in one hour.
          </p>
          <p className="text-sm text-muted">Nothing arrived? Your administrator can create a reset link for you from the Users page.</p>
        </div>
        <Link href="/login" className={authButton}>
          Back to sign in
        </Link>
      </AuthCard>
    );
  }

  return (
    <AuthCard>
      <div className="flex flex-col gap-2">
        <h1 className="display text-4xl font-bold">Forgot password</h1>
        <p className="text-sm text-muted">Enter the email address you sign in with and we will send a link to reset your password.</p>
      </div>
      <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
        <label className="flex flex-col gap-2 text-xs font-semibold text-muted">
          Email
          <input id="email" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@company.com" className={authField} />
        </label>
        {error && <Notice tone="danger">{error}</Notice>}
        <button type="submit" disabled={busy || !email.trim()} className={authButton}>
          {busy ? "Sending…" : "Send reset link"}
        </button>
      </form>
      <Link href="/login" className="text-center text-xs font-semibold text-muted hover:text-ink">
        Back to sign in
      </Link>
    </AuthCard>
  );
}
