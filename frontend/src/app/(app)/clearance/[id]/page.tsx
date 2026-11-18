"use client";

import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ClearanceChecklist } from "@/components/clearance-checklist";
import { Notice, Skeleton } from "@/components/ui";
import { formatDate } from "@/lib/format";
import type { Clearance } from "@/lib/types";
import { useApi } from "@/lib/use-api";

// The checklist on its own page, for assignees who are not part of the
// separation itself and so cannot open it.
export default function ClearanceChecklistPage() {
  const { id } = useParams<{ id: string }>();
  const clearance = useApi<Clearance>(`/clearances/${id}`);

  const back = (
    <Link href="/clearance" className="inline-flex w-fit items-center gap-2 rounded-full text-[13px] font-semibold text-muted hover:text-ink">
      <ArrowLeft size={15} aria-hidden="true" />
      Clearance
    </Link>
  );
  if (clearance.error) {
    return (
      <>
        {back}
        <Notice tone="danger">{clearance.error.status === 404 ? "This checklist does not exist, or none of its items are assigned to you." : clearance.error.message}</Notice>
      </>
    );
  }
  if (!clearance.data) {
    return (
      <>
        {back}
        <Skeleton className="h-80 rounded-card" />
      </>
    );
  }
  const c = clearance.data;
  return (
    <>
      {back}
      <div className="flex flex-col gap-2">
        <h1 className="display text-[2.6rem] font-bold">{c.employee?.fullName ?? "Clearance"}</h1>
        <p className="text-[13px] text-muted">
          {c.employee?.employeeNumber} · last working day {formatDate(c.lastWorkingDay)}
        </p>
      </div>
      {/* The API refuses changes once the separation is closed; a completed checklist can still be reopened before that. */}
      <ClearanceChecklist clearance={c} locked={false} onChanged={clearance.reload} />
    </>
  );
}
