"use client";

import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { Badge, Button, Card, Field, Notice, Skeleton } from "@/components/ui";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatDateTime } from "@/lib/format";
import type { ExitInterview, InterviewAnswer, InterviewQuestion } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { useSubmit } from "@/lib/use-submit";

const textareaClass = "min-h-24 w-full rounded-2xl bg-raised px-4 py-3 text-sm text-ink placeholder:text-muted disabled:opacity-70";
const RATING_WORDS = ["Very poor", "Poor", "Fair", "Good", "Very good"];

function Question({ q, answer, onChange, disabled, error }: { q: InterviewQuestion; answer: InterviewAnswer; onChange: (a: InterviewAnswer) => void; disabled: boolean; error?: string }) {
  const option = (selected: boolean) =>
    `inline-flex h-10 items-center rounded-full px-4 text-[13px] font-semibold transition-colors disabled:cursor-not-allowed ${selected ? "bg-lime text-ground" : "bg-raised text-ink hover:bg-line disabled:hover:bg-raised"}`;
  return (
    <fieldset className="flex flex-col gap-3 rounded-2xl bg-raised/40 p-4">
      <legend className="float-left mb-1 w-full text-sm font-semibold">
        {q.text}
        {q.required && <span className="ml-2 text-xs font-normal text-muted">Required</span>}
      </legend>
      {q.type === "rating" && (
        <div className="flex flex-wrap gap-2" role="radiogroup" aria-label={q.text}>
          {[1, 2, 3, 4, 5].map((n) => (
            <button key={n} type="button" role="radio" aria-checked={answer.rating === n} disabled={disabled} title={RATING_WORDS[n - 1]} onClick={() => onChange({ questionId: q.id, rating: answer.rating === n ? undefined : n })} className={option(answer.rating === n)}>
              {n}
            </button>
          ))}
          <span className="self-center text-xs text-muted">{answer.rating ? RATING_WORDS[answer.rating - 1] : "1 is very poor, 5 is very good"}</span>
        </div>
      )}
      {q.type === "choice" && (
        <div className="flex flex-wrap gap-2" role="radiogroup" aria-label={q.text}>
          {(q.options ?? []).map((o) => (
            <button key={o} type="button" role="radio" aria-checked={answer.choice === o} disabled={disabled} onClick={() => onChange({ questionId: q.id, choice: answer.choice === o ? undefined : o })} className={option(answer.choice === o)}>
              {o}
            </button>
          ))}
        </div>
      )}
      {q.type === "text" && <textarea aria-label={q.text} value={answer.text ?? ""} disabled={disabled} maxLength={2000} onChange={(e) => onChange({ questionId: q.id, text: e.target.value })} className={textareaClass} />}
      {error && <p className="text-xs font-medium text-danger">This question {error}.</p>}
    </fieldset>
  );
}

function InterviewForm({ interview, onChanged }: { interview: ExitInterview; onChanged: () => void }) {
  const auth = useAuth();
  const isHR = auth.can("exit_interview.manage");
  const submitted = interview.status === "submitted";
  const canAmend = submitted && auth.can("exit_interview.amend");
  const [amending, setAmending] = useState(false);
  const [reason, setReason] = useState("");
  const [answers, setAnswers] = useState<Record<string, InterviewAnswer>>(() => Object.fromEntries(interview.answers.map((a) => [a.questionId, a])));
  const [notes, setNotes] = useState(interview.notes ?? "");
  const [saved, setSaved] = useState(false);
  // The version is tracked here because a save can succeed while the submit that follows it is refused.
  const [version, setVersion] = useState(interview.version);
  const submit = useSubmit();
  const locked = submitted && !amending;

  const body = () => ({
    answers: Object.values(answers).filter((a) => a.rating !== undefined || a.choice || a.text?.trim()),
    // The employee may only send answers; notes belong to HR.
    ...(isHR ? { notes } : {}),
    ...(amending ? { amendReason: reason } : {}),
  });
  const save = (thenSubmit: boolean) =>
    submit.run(async () => {
      setSaved(false);
      const updated = await api<ExitInterview>(`/exit-interviews/${interview.id}`, { method: "PATCH", json: { ...body(), version } });
      setVersion(updated.version);
      if (thenSubmit) {
        const done = await api<ExitInterview>(`/exit-interviews/${interview.id}/submit`, { json: { version: updated.version } });
        setVersion(done.version);
      }
      setSaved(true);
      setAmending(false);
      setReason("");
      onChanged();
    });
  // The API reports answer problems as "answers.<question id>".
  const errorFor = (id: string) => submit.fields?.[`answers.${id}`];
  const answered = Object.values(answers).filter((a) => a.rating !== undefined || a.choice || a.text?.trim()).length;

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
      <Card title="Questions" action={<span className="text-xs text-muted">{answered} of {interview.questions.length} answered</span>}>
        {interview.questions.map((q) => (
          <Question
            key={q.id}
            q={q}
            answer={answers[q.id] ?? { questionId: q.id }}
            disabled={locked || submit.busy}
            error={errorFor(q.id)}
            onChange={(a) => {
              setSaved(false);
              setAnswers({ ...answers, [q.id]: a });
            }}
          />
        ))}
      </Card>

      <div className="flex min-w-0 flex-col gap-6">
        {isHR && (
          <Card title="HR notes">
            <textarea aria-label="HR notes" value={notes} disabled={locked || submit.busy} maxLength={4000} onChange={(e) => { setSaved(false); setNotes(e.target.value); }} placeholder="Notes from the conversation. Never shown to the employee." className={textareaClass} />
          </Card>
        )}
        <Card title={submitted ? "Submitted" : "Finish"}>
          {submitted && !amending && <p className="text-sm text-muted">This interview was submitted {formatDateTime(interview.submittedAt)} and is locked.</p>}
          {!submitted && <p className="text-sm text-muted">Save a draft to continue later. Submitting locks the answers. {isHR ? "" : "Your answers are seen by HR only."}</p>}
          {amending && (
            <Field label="Reason for the change" error={submit.fields?.amendReason} hint="Recorded in the audit log.">
              <textarea value={reason} onChange={(e) => setReason(e.target.value)} maxLength={500} className={textareaClass} />
            </Field>
          )}
          {submit.error && <Notice tone="danger">{submit.fields && Object.keys(submit.fields).some((k) => k.startsWith("answers.")) ? "Some questions need attention. They are marked above." : submit.error}</Notice>}
          {saved && <p className="text-xs font-semibold text-lime" role="status">Saved</p>}
          <div className="flex flex-wrap gap-2">
            {!submitted && (
              <>
                <Button variant="secondary" disabled={submit.busy} onClick={() => save(false)}>
                  Save draft
                </Button>
                <Button disabled={submit.busy} onClick={() => save(true)}>
                  {submit.busy ? "Saving…" : "Submit"}
                </Button>
              </>
            )}
            {canAmend && !amending && (
              <Button variant="secondary" onClick={() => setAmending(true)}>
                Amend
              </Button>
            )}
            {amending && (
              <>
                <Button variant="secondary" disabled={submit.busy} onClick={() => setAmending(false)}>
                  Cancel
                </Button>
                <Button disabled={submit.busy || !reason.trim()} onClick={() => save(false)}>
                  Save amendment
                </Button>
              </>
            )}
          </div>
        </Card>
      </div>
    </div>
  );
}

export default function ExitInterviewPage() {
  const { id } = useParams<{ id: string }>();
  const interview = useApi<ExitInterview>(`/exit-interviews/${id}`);

  const back = interview.data ? (
    <Link href={`/separations/${interview.data.separationId}`} className="inline-flex w-fit items-center gap-2 rounded-full text-[13px] font-semibold text-muted hover:text-ink">
      <ArrowLeft size={15} aria-hidden="true" />
      Separation
    </Link>
  ) : null;

  if (interview.error) return <Notice tone="danger">{interview.error.status === 404 ? "This exit interview does not exist, or you do not have access to it." : interview.error.message}</Notice>;
  if (!interview.data) return <Skeleton className="h-96 rounded-card" />;
  const iv = interview.data;

  return (
    <>
      {back}
      <div className="flex flex-col gap-2">
        <h1 className="display text-[2.6rem] font-bold">Exit interview</h1>
        <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted">
          <span>
            {iv.employee?.fullName} · {iv.employee?.employeeNumber}
          </span>
          <Badge tone={iv.status === "submitted" ? "lime" : "orange"}>{iv.status === "submitted" ? "Submitted" : "Draft"}</Badge>
        </div>
      </div>
      <InterviewForm interview={iv} onChanged={interview.reload} />
    </>
  );
}
