"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge, Card, Empty, Notice, PageHeader, Pill, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { formatDate } from "@/lib/format";
import type { Clearance, ClearanceTask, Page } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const STATUS = { in_progress: { label: "In progress", tone: "orange" }, completed: { label: "Completed", tone: "lime" }, cancelled: { label: "Cancelled", tone: "muted" } } as const;

export default function ClearancePage() {
  const auth = useAuth();
  const seesAll = auth.can("clearance.read") || auth.can("clearance.manage");
  const [tab, setTab] = useState<"mine" | "all">("mine");
  const tasks = useApi<{ data: ClearanceTask[] }>(tab === "mine" ? "/me/clearance-tasks" : null);
  const all = useApi<Page<Clearance>>(tab === "all" ? "/clearances?pageSize=50" : null);

  // Group the caller's tasks by the checklist they belong to.
  const groups = new Map<string, ClearanceTask[]>();
  for (const task of tasks.data?.data ?? []) groups.set(task.clearanceId, [...(groups.get(task.clearanceId) ?? []), task]);

  return (
    <>
      <PageHeader title="Clearance">
        {seesAll && (
          <>
            <Pill active={tab === "mine"} aria-pressed={tab === "mine"} onClick={() => setTab("mine")}>
              My tasks
            </Pill>
            <Pill active={tab === "all"} aria-pressed={tab === "all"} onClick={() => setTab("all")}>
              All checklists
            </Pill>
          </>
        )}
      </PageHeader>

      {tab === "mine" && (
        <>
          {tasks.loading && <Skeleton className="h-48 rounded-card" />}
          {tasks.error && <Notice tone="danger">Your tasks could not be loaded. {tasks.error.message}</Notice>}
          {tasks.data && groups.size === 0 && (
            <Card>
              <Empty>You have no open clearance tasks.</Empty>
            </Card>
          )}
          <div className="grid gap-6 lg:grid-cols-2">
            {[...groups.entries()].map(([clearanceId, items]) => (
              <Card
                key={clearanceId}
                title={items[0].employee?.fullName ?? "Employee"}
                action={
                  <Link href={`/clearance/${clearanceId}`} className="rounded-full bg-lime px-4 py-2 text-[12px] font-bold text-ground hover:opacity-90">
                    Open checklist
                  </Link>
                }
              >
                <p className="-mt-3 text-xs text-muted">
                  {items[0].employee?.employeeNumber} · last working day {formatDate(items[0].lastWorkingDay)}
                </p>
                <ul className="flex flex-col gap-2">
                  {items.map((t) => (
                    <li key={t.item.itemId} className="flex items-center justify-between gap-3 rounded-2xl bg-raised px-4 py-3">
                      <div className="min-w-0">
                        <p className="truncate text-[13px] font-semibold">{t.item.title}</p>
                        <p className="text-[11px] text-muted">
                          {t.item.category} · due {formatDate(t.item.dueDate)}
                        </p>
                      </div>
                      {t.overdue ? <Badge tone="orange">Overdue</Badge> : t.item.status === "blocked" ? <Badge tone="orange">Blocked</Badge> : <Badge tone="ink">To do</Badge>}
                    </li>
                  ))}
                </ul>
              </Card>
            ))}
          </div>
        </>
      )}

      {tab === "all" && (
        <Card>
          {all.loading && <Skeleton className="h-48" />}
          {all.error && <Notice tone="danger">Checklists could not be loaded. {all.error.message}</Notice>}
          {all.data?.total === 0 && <Empty>No clearance checklists yet. One is created when a separation is approved.</Empty>}
          <ul className="flex flex-col gap-2">
            {all.data?.data.map((c) => (
              <li key={c.id}>
                <Link href={`/clearance/${c.id}`} className="flex flex-wrap items-center justify-between gap-3 rounded-2xl bg-raised px-4 py-3 hover:bg-line">
                  <div className="min-w-0">
                    <p className="truncate text-[13px] font-semibold">{c.employee?.fullName ?? "Employee"}</p>
                    <p className="text-[11px] text-muted">
                      {c.employee?.employeeNumber} · last working day {formatDate(c.lastWorkingDay)}
                    </p>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="text-xs tabular-nums text-muted">
                      {c.totalItems - c.openItems} of {c.totalItems} closed
                    </span>
                    <Badge tone={STATUS[c.status].tone}>{STATUS[c.status].label}</Badge>
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </>
  );
}
