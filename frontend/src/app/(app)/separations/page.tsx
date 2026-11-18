"use client";

import { Plus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { NewSeparationDialog } from "@/components/separation-dialogs";
import { Badge, Button, Card, Empty, Notice, PageHeader, Pagination, Pill, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { formatDate, SEPARATION_STATUS, SEPARATION_TYPES } from "@/lib/format";
import type { Page, Separation } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const PAGE_SIZE = 20;

export default function SeparationsPage() {
  const auth = useAuth();
  const router = useRouter();
  const canApprove = auth.can("separation.approve");
  const canManage = auth.can("separation.manage");
  const canSubmit = canManage || (auth.can("separation.submit") && auth.status === "signedIn" && !!auth.user.employeeId);
  // Approvers land on their own queue; everyone else on the full list.
  const [filter, setFilter] = useState(canApprove && !canManage ? "awaitingMe" : "");
  const [page, setPage] = useState(1);
  const [creating, setCreating] = useState(false);

  const filters = [
    ...(canApprove ? [{ value: "awaitingMe", label: "Awaiting me" }] : []),
    { value: "", label: "All" },
    { value: "pending_approval", label: "Awaiting approval" },
    { value: "approved", label: "Serving notice" },
    { value: "completed", label: "Completed" },
  ];
  const params = new URLSearchParams({ page: String(page), pageSize: String(PAGE_SIZE) });
  if (filter === "awaitingMe") params.set("awaitingMe", "true");
  else if (filter) params.set("status", filter);
  const list = useApi<Page<Separation>>(`/separations?${params}`);

  return (
    <>
      <PageHeader title="Separations">
        {canSubmit && (
          <Button onClick={() => setCreating(true)}>
            <Plus size={16} strokeWidth={2.6} aria-hidden="true" />
            {canManage ? "Record separation" : "Submit resignation"}
          </Button>
        )}
      </PageHeader>

      <div className="flex flex-wrap items-center gap-2">
        {filters.map((f) => (
          <Pill
            key={f.value}
            active={filter === f.value}
            aria-pressed={filter === f.value}
            onClick={() => {
              setFilter(f.value);
              setPage(1);
            }}
          >
            {f.label}
          </Pill>
        ))}
      </div>

      {auth.can("separation.submit") && !canManage && auth.status === "signedIn" && !auth.user.employeeId && (
        <Notice>Your account is not linked to an employee record, so you cannot submit a resignation here. Ask HR to link it or to submit it for you.</Notice>
      )}

      <Card>
        {list.error && <Notice tone="danger">Separations could not be loaded. {list.error.message}</Notice>}
        {list.loading && <Skeleton className="h-64" />}
        {list.data && list.data.total === 0 && (
          <Empty>{filter === "awaitingMe" ? "Nothing is waiting for your decision." : filter ? "No separations with this status." : "No separations have been recorded."}</Empty>
        )}
        {list.data && list.data.total > 0 && (
          <div className="-mx-2 overflow-x-auto">
            <table className="w-full min-w-[620px] border-separate border-spacing-y-1.5 text-left text-[13px]">
              <thead>
                <tr className="text-[11px] uppercase tracking-wide text-muted">
                  <th className="px-4 py-1 font-semibold">Employee</th>
                  <th className="px-4 py-1 font-semibold">Type</th>
                  <th className="px-4 py-1 font-semibold">Submitted</th>
                  <th className="px-4 py-1 font-semibold">Last working day</th>
                  <th className="px-4 py-1 font-semibold">Status</th>
                </tr>
              </thead>
              <tbody>
                {list.data.data.map((s) => {
                  const status = SEPARATION_STATUS[s.status] ?? { label: s.status, tone: "muted" as const };
                  return (
                    <tr key={s.id} className="group">
                      <td className="rounded-l-2xl bg-raised px-4 py-3 group-hover:bg-line">
                        <Link href={`/separations/${s.id}`} className="font-semibold hover:underline">
                          {s.employee?.fullName ?? "Employee"}
                        </Link>
                        <div className="text-[11px] text-muted">{s.employee?.employeeNumber}</div>
                      </td>
                      <td className="bg-raised px-4 py-3 group-hover:bg-line">{SEPARATION_TYPES[s.type] ?? s.type}</td>
                      <td className="whitespace-nowrap bg-raised px-4 py-3 tabular-nums text-muted group-hover:bg-line">{formatDate(s.submissionDate)}</td>
                      <td className="whitespace-nowrap bg-raised px-4 py-3 tabular-nums group-hover:bg-line">{formatDate(s.lastWorkingDay)}</td>
                      <td className="rounded-r-2xl bg-raised px-4 py-3 group-hover:bg-line">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge tone={status.tone}>{status.label}</Badge>
                          {s.canAct && <Badge tone="lime">Your turn</Badge>}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        {list.data && <Pagination page={page} pageSize={PAGE_SIZE} total={list.data.total} onPage={setPage} />}
      </Card>

      {creating && <NewSeparationDialog onClose={() => setCreating(false)} onSaved={(s) => router.push(`/separations/${s.id}`)} />}
    </>
  );
}
