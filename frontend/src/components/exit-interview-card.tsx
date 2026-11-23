"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, errorMessage } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatDateTime } from "@/lib/format";
import type { ExitInterview, SeparationDetail } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { Badge, Button, Card, Notice } from "./ui";

// ExitInterviewCard sits on the separation page. HR can start the interview
// and open it; the departing employee sees it once HR has started it and
// self-service is on. Everyone else sees nothing.
export function ExitInterviewCard({ separation }: { separation: SeparationDetail }) {
  const auth = useAuth();
  const router = useRouter();
  const isHR = auth.can("exit_interview.read") || auth.can("exit_interview.manage");
  const own = auth.status === "signedIn" && auth.user.employeeId === separation.employeeId;
  const interview = useApi<ExitInterview>(isHR || own ? `/separations/${separation.id}/exit-interview` : null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!isHR && !own) return null;
  if (interview.loading) return null;
  const exists = !!interview.data;
  // For the employee, "not found" covers both "not started yet" and "self-service is off".
  if (!exists && !isHR) return null;
  const canStart = auth.can("exit_interview.manage") && (separation.status === "approved" || separation.status === "completed");

  async function start() {
    setBusy(true);
    setError(null);
    try {
      const created = await api<ExitInterview>(`/separations/${separation.id}/exit-interview`, { json: {} });
      router.push(`/exit-interviews/${created.id}`);
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <Card title="Exit interview" action={exists ? <Badge tone={interview.data!.status === "submitted" ? "lime" : "orange"}>{interview.data!.status === "submitted" ? "Submitted" : "In progress"}</Badge> : undefined}>
      {exists ? (
        <>
          <p className="text-sm text-muted">
            {interview.data!.status === "submitted"
              ? `Submitted ${formatDateTime(interview.data!.submittedAt)}.`
              : own && !isHR
                ? "Your feedback form is ready. Your answers are seen by HR only."
                : `${interview.data!.answers.length} of ${interview.data!.questions.length} questions answered so far.`}
          </p>
          <Link href={`/exit-interviews/${interview.data!.id}`} className="inline-flex h-10 w-fit items-center rounded-full bg-lime px-5 text-[13px] font-bold text-ground hover:opacity-90">
            {interview.data!.status === "submitted" ? "View" : own && !isHR ? "Fill in the form" : "Open"}
          </Link>
        </>
      ) : (
        <>
          <p className="text-sm text-muted">
            {canStart ? "No exit interview has been started for this separation." : "An exit interview can be started once the separation is approved."}
          </p>
          {error && <Notice tone="danger">{error}</Notice>}
          {canStart && (
            <Button className="w-fit" disabled={busy} onClick={start}>
              {busy ? "Starting…" : "Start exit interview"}
            </Button>
          )}
        </>
      )}
    </Card>
  );
}
