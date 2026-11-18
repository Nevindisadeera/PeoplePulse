"use client";

import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { ClearanceChecklist } from "@/components/clearance-checklist";
import { ActionDialog } from "@/components/separation-dialogs";
import { Badge, Button, Card, Notice, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { formatDate, formatDateTime, humanize, SEPARATION_STATUS, SEPARATION_TYPES, todayISO } from "@/lib/format";
import type { ApprovalStep, Clearance, Organization, SeparationDetail, SeparationSettings } from "@/lib/types";
import { useApi } from "@/lib/use-api";

type Action = "approve" | "reject" | "withdraw" | "last-working-day" | "complete";

const STEP_OWNER: Record<ApprovalStep["approverType"], string> = { line_manager: "Line manager", role: "Role", user: "Named approver" };
const DECISION = { pending: { label: "Waiting", tone: "orange" }, approved: { label: "Approved", tone: "lime" }, rejected: { label: "Not approved", tone: "muted" } } as const;

export default function SeparationPage() {
  const { id } = useParams<{ id: string }>();
  const auth = useAuth();
  const separation = useApi<SeparationDetail>(`/separations/${id}`);
  const org = useApi<Organization & { settings: SeparationSettings }>("/organization");
  const clearanceId = separation.data?.clearance?.id;
  const clearance = useApi<Clearance>(clearanceId ? `/clearances/${clearanceId}` : null);
  const [action, setAction] = useState<Action | null>(null);

  const back = (
    <Link href="/separations" className="inline-flex w-fit items-center gap-2 rounded-full text-[13px] font-semibold text-muted hover:text-ink">
      <ArrowLeft size={15} aria-hidden="true" />
      Separations
    </Link>
  );
  if (separation.error) {
    return (
      <>
        {back}
        <Notice tone="danger">{separation.error.status === 404 ? "This separation does not exist, or you are not involved in it." : separation.error.message}</Notice>
      </>
    );
  }
  if (!separation.data) {
    return (
      <>
        {back}
        <Skeleton className="h-96 rounded-card" />
      </>
    );
  }

  const s = separation.data;
  const status = SEPARATION_STATUS[s.status] ?? { label: s.status, tone: "muted" as const };
  const reason = org.data?.settings.separationReasons.find((r) => r.code === s.reasonCode)?.name ?? humanize(s.reasonCode);
  const manage = auth.can("separation.manage");
  const own = auth.status === "signedIn" && auth.user.employeeId === s.employeeId;
  const open = s.status === "pending_approval" || s.status === "approved";
  const canWithdraw = s.status === "pending_approval" ? manage || own : s.status === "approved" && manage;
  const lastDayReached = s.lastWorkingDay <= todayISO();
  const clearanceReady = !s.clearance || s.clearance.status === "completed" || s.clearance.overridden;
  const reload = () => {
    setAction(null);
    separation.reload();
    clearance.reload();
  };

  const facts: [string, string][] = [
    ["Type", SEPARATION_TYPES[s.type] ?? s.type],
    ["Reason", `${reason} · ${s.category}`],
    ["Submitted", formatDate(s.submissionDate)],
    ["Last working day", formatDate(s.lastWorkingDay) + (s.requestedLastWorkingDay !== s.lastWorkingDay ? ` (requested ${formatDate(s.requestedLastWorkingDay)})` : "")],
    ...(s.tenureDaysAtExit !== undefined ? ([["Length of service", `${(s.tenureDaysAtExit / 365.25).toFixed(1)} years`]] as [string, string][]) : []),
  ];

  return (
    <>
      {back}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-2">
          <h1 className="display text-[2.6rem] font-bold">{s.employee?.fullName ?? "Separation"}</h1>
          <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted">
            <span className="tabular-nums">{s.employee?.employeeNumber}</span>
            <Badge tone={status.tone}>{status.label}</Badge>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {s.canAct && (
            <>
              <Button variant="secondary" onClick={() => setAction("reject")}>
                Do not approve
              </Button>
              <Button onClick={() => setAction("approve")}>Approve</Button>
            </>
          )}
          {manage && open && (
            <Button variant="secondary" onClick={() => setAction("last-working-day")}>
              Change last working day
            </Button>
          )}
          {canWithdraw && (
            <Button variant="secondary" onClick={() => setAction("withdraw")}>
              Withdraw
            </Button>
          )}
          {manage && s.status === "approved" && (
            <Button
              disabled={!lastDayReached || !clearanceReady}
              title={!lastDayReached ? "Available on the last working day" : !clearanceReady ? "Close the clearance items first, or record an override" : undefined}
              onClick={() => setAction("complete")}
            >
              Complete
            </Button>
          )}
        </div>
      </div>

      {manage && s.status === "approved" && (!lastDayReached || !clearanceReady) && (
        <Notice>
          {!lastDayReached
            ? `This separation can be completed on or after ${formatDate(s.lastWorkingDay)}.`
            : `${s.clearance?.openItems} clearance item(s) are still open. Close them, or record an override, before completing.`}
        </Notice>
      )}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
        <div className="flex min-w-0 flex-col gap-6">
          <Card title="Summary">
            <dl className="grid gap-x-6 gap-y-5 sm:grid-cols-2">
              {facts.map(([label, value]) => (
                <div key={label} className="min-w-0">
                  <dt className="text-[11px] font-semibold uppercase tracking-wide text-muted">{label}</dt>
                  <dd className="mt-1 text-sm">{value}</dd>
                </div>
              ))}
            </dl>
            {s.reasonDetails && (
              <div>
                <p className="text-[11px] font-semibold uppercase tracking-wide text-muted">Details</p>
                <p className="mt-1 whitespace-pre-wrap text-sm">{s.reasonDetails}</p>
              </div>
            )}
          </Card>
          {clearance.data && <ClearanceChecklist clearance={clearance.data} locked={s.status !== "approved"} onChanged={reload} />}
          {clearanceId && clearance.error && <Notice>The clearance checklist is visible to HR, the employee and the people assigned to its items.</Notice>}
        </div>

        <div className="flex min-w-0 flex-col gap-6">
          <Card title="Approval">
            {s.approvalSteps.length === 0 && <p className="text-sm text-muted">No approval steps: this separation was recorded by HR, or the organization has no approval chain.</p>}
            <ol className="flex flex-col gap-2">
              {s.approvalSteps.map((step) => (
                <li key={step.order} className="rounded-2xl bg-raised px-4 py-3">
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-[13px] font-semibold">
                      {step.order}. {step.fallback ? "HR (no approver found)" : STEP_OWNER[step.approverType]}
                    </span>
                    <Badge tone={DECISION[step.decision].tone}>{DECISION[step.decision].label}</Badge>
                  </div>
                  {step.decidedAt && <p className="mt-1 text-[11px] text-muted">{formatDateTime(step.decidedAt)}</p>}
                  {step.comment && <p className="mt-1 text-xs text-muted">“{step.comment}”</p>}
                </li>
              ))}
            </ol>
          </Card>
          <Card title="History">
            <ol className="flex flex-col">
              {s.statusHistory.map((h, i, all) => (
                <li key={i} className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3">
                  <div className="flex flex-col items-center">
                    <span className="mt-1 size-3 rounded-full bg-lime" aria-hidden="true" />
                    {i < all.length - 1 && <span className="w-px flex-1 bg-line" aria-hidden="true" />}
                  </div>
                  <div className="pb-4">
                    <p className="text-sm font-semibold">{h.note?.startsWith("Last working day changed") ? "Last working day changed" : (SEPARATION_STATUS[h.status]?.label ?? humanize(h.status))}</p>
                    {h.note && <p className="text-xs text-muted">{h.note}</p>}
                    <p className="text-[11px] tabular-nums text-muted">{formatDateTime(h.at)}</p>
                  </div>
                </li>
              ))}
            </ol>
          </Card>
        </div>
      </div>

      {action && <ActionDialog action={action} separation={s} onClose={() => setAction(null)} onSaved={reload} />}
    </>
  );
}
