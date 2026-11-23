"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Card, Empty, Field, inputClass, Notice, PageHeader, Pagination, Pill, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { formatDate, formatDateTime, periodStart, todayISO } from "@/lib/format";
import type { ExitInterview, FeedbackSummary, Page } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const PAGE_SIZE = 20;

function ListTab() {
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const list = useApi<Page<ExitInterview>>(`/exit-interviews?page=${page}&pageSize=${PAGE_SIZE}${status ? `&status=${status}` : ""}`);
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {[
          ["", "All"],
          ["draft", "In progress"],
          ["submitted", "Submitted"],
        ].map(([value, label]) => (
          <Pill key={value} active={status === value} aria-pressed={status === value} onClick={() => { setStatus(value); setPage(1); }}>
            {label}
          </Pill>
        ))}
      </div>
      <Card>
        {list.error && <Notice tone="danger">Exit interviews could not be loaded. {list.error.message}</Notice>}
        {list.loading && <Skeleton className="h-48" />}
        {list.data?.total === 0 && <Empty>{status ? "No interviews with this status." : "No exit interviews yet. Start one from an approved separation."}</Empty>}
        <ul className="flex flex-col gap-2">
          {list.data?.data.map((iv) => (
            <li key={iv.id}>
              <Link href={`/exit-interviews/${iv.id}`} className="flex flex-wrap items-center justify-between gap-3 rounded-2xl bg-raised px-4 py-3 hover:bg-line">
                <div className="min-w-0">
                  <p className="truncate text-[13px] font-semibold">{iv.employee?.fullName ?? "Employee"}</p>
                  <p className="text-[11px] text-muted">
                    {iv.employee?.employeeNumber} · {iv.answers.length} of {iv.questions.length} answered
                  </p>
                </div>
                <div className="flex items-center gap-3">
                  {iv.submittedAt && <span className="text-xs tabular-nums text-muted">{formatDateTime(iv.submittedAt)}</span>}
                  <Badge tone={iv.status === "submitted" ? "lime" : "orange"}>{iv.status === "submitted" ? "Submitted" : "In progress"}</Badge>
                </div>
              </Link>
            </li>
          ))}
        </ul>
        {list.data && <Pagination page={page} pageSize={PAGE_SIZE} total={list.data.total} onPage={setPage} />}
      </Card>
    </>
  );
}

function FeedbackTab() {
  const [from, setFrom] = useState(periodStart(12));
  const [to, setTo] = useState(todayISO());
  const valid = from !== "" && to !== "" && from <= to;
  const feedback = useApi<FeedbackSummary>(valid ? `/analytics/exit-feedback?from=${from}&to=${to}` : null);
  const s = feedback.data?.summary;

  return (
    <>
      <Card>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="Submitted from">
            <input type="date" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} className={inputClass} />
          </Field>
          <Field label="To">
            <input type="date" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} className={inputClass} />
          </Field>
        </div>
        <p className="text-xs text-muted">Combined results only. Individual answers and free text are never shown here.</p>
      </Card>
      {feedback.loading && <Skeleton className="h-64 rounded-card" />}
      {feedback.error && <Notice tone="danger">The summary could not be loaded. {feedback.error.message}</Notice>}
      {s && s.suppressed && (
        <Card>
          <Empty>
            {s.interviews} interview{s.interviews === 1 ? " was" : "s were"} submitted between {formatDate(from)} and {formatDate(to)}. Results are shown from {s.threshold} responses, so that
            individuals are harder to identify.
          </Empty>
        </Card>
      )}
      {s && !s.suppressed && (
        <div className="grid gap-6 lg:grid-cols-2">
          {s.questions.map((q) => {
            const entries = Object.entries(q.distribution ?? {});
            const order = q.type === "rating" ? ["5", "4", "3", "2", "1"] : entries.sort((a, b) => b[1] - a[1]).map(([k]) => k);
            const largest = Math.max(1, ...entries.map(([, n]) => n));
            return (
              <Card key={q.questionId} title={q.type === "rating" ? "Rating" : q.type === "choice" ? "Choice" : "Free text"} action={<span className="text-xs text-muted">{q.responses} responses</span>}>
                <p className="-mt-2 text-sm font-semibold">{q.text}</p>
                {q.suppressed && <Empty>Fewer than {s.threshold} people answered this question, so its results are not shown.</Empty>}
                {!q.suppressed && q.type === "rating" && q.average !== undefined && (
                  <p className="display text-5xl font-bold tabular-nums text-lime">
                    {q.average.toFixed(1)}
                    <span className="ml-2 text-base font-medium text-muted">out of 5</span>
                  </p>
                )}
                {!q.suppressed && q.type !== "text" && (
                  <ul className="flex flex-col gap-2">
                    {order.map((key) => {
                      const n = q.distribution?.[key] ?? 0;
                      return (
                        <li key={key} className="grid grid-cols-[5rem_minmax(0,1fr)_2rem] items-center gap-3 text-xs">
                          <span className="truncate font-semibold">{q.type === "rating" ? `${key} of 5` : key}</span>
                          <div className="h-5 rounded-full bg-raised">{n > 0 && <div className="h-5 rounded-full bg-lime" style={{ width: `${(n / largest) * 100}%` }} />}</div>
                          <span className="text-right font-bold tabular-nums">{n}</span>
                        </li>
                      );
                    })}
                  </ul>
                )}
                {!q.suppressed && q.type === "text" && <p className="text-sm text-muted">Written answers are read in each interview. They are not combined or shown here.</p>}
              </Card>
            );
          })}
        </div>
      )}
    </>
  );
}

export default function ExitInterviewsPage() {
  const auth = useAuth();
  const canSummary = auth.can("exit_interview.read");
  const [tab, setTab] = useState<"list" | "feedback">("list");
  if (!auth.can("exit_interview.read") && !auth.can("exit_interview.manage")) return <Notice tone="danger">You do not have permission to see exit interviews.</Notice>;
  return (
    <>
      <PageHeader title="Exit interviews">
        <Pill active={tab === "list"} aria-pressed={tab === "list"} onClick={() => setTab("list")}>
          Interviews
        </Pill>
        {canSummary && (
          <Pill active={tab === "feedback"} aria-pressed={tab === "feedback"} onClick={() => setTab("feedback")}>
            Feedback summary
          </Pill>
        )}
      </PageHeader>
      {tab === "list" ? <ListTab /> : <FeedbackTab />}
    </>
  );
}
