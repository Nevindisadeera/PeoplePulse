"use client";

import { ArrowLeft, ArrowRightLeft, Pencil } from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { EmployeeForm, TransferDialog } from "@/components/employee-dialogs";
import { Badge, Button, Card, Empty, Notice, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { EMPLOYEE_STATUS, formatDate } from "@/lib/format";
import type { Department, Employee, Organization, TimelineEvent } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const EVENT_TEXT: Record<TimelineEvent["type"], (e: TimelineEvent) => string> = {
  joined: (e) => `Joined${e.departmentName ? ` in ${e.departmentName}` : ""}`,
  transferred: (e) => `Transferred from ${e.fromDepartmentName ?? "another department"} to ${e.departmentName ?? "a new department"}`,
  separated: () => "Last working day",
};

export default function EmployeePage() {
  const { id } = useParams<{ id: string }>();
  const auth = useAuth();
  const employee = useApi<Employee>(`/employees/${id}`);
  const timeline = useApi<{ data: TimelineEvent[] }>(`/employees/${id}/timeline`);
  const departments = useApi<{ data: Department[] }>(auth.can("department.read") ? "/departments" : null);
  const org = useApi<Organization>("/organization");
  const managerId = employee.data?.managerId;
  const manager = useApi<Employee>(managerId ? `/employees/${managerId}` : null);
  const [dialog, setDialog] = useState<"edit" | "transfer" | null>(null);

  const back = (
    <Link href="/employees" className="inline-flex w-fit items-center gap-2 rounded-full text-[13px] font-semibold text-muted hover:text-ink">
      <ArrowLeft size={15} aria-hidden="true" />
      Employees
    </Link>
  );

  if (employee.error) {
    return (
      <>
        {back}
        <Notice tone="danger">{employee.error.status === 404 ? "This employee does not exist, or you do not have access to their record." : employee.error.message}</Notice>
      </>
    );
  }
  if (!employee.data) {
    return (
      <>
        {back}
        <Skeleton className="h-96 rounded-card" />
      </>
    );
  }

  const e = employee.data;
  const status = EMPLOYEE_STATUS[e.status] ?? { label: e.status, tone: "muted" as const };
  const department = departments.data?.data.find((d) => d.id === e.departmentId)?.name;
  const type = org.data?.settings.employmentTypes.find((t) => t.code === e.employmentType)?.name ?? e.employmentType;
  const managerLabel = manager.data ? `${manager.data.fullName} (${manager.data.employeeNumber})` : undefined;
  const canManage = auth.can("employee.manage");
  const saved = () => {
    setDialog(null);
    employee.reload();
    timeline.reload();
  };

  const facts: [string, React.ReactNode][] = [
    ["Department", department ?? "—"],
    ["Job title", e.jobTitle || "—"],
    ["Employment type", type],
    ["Line manager", e.managerId ? (managerLabel ?? (manager.error ? "Outside the departments you can see" : "…")) : "—"],
    ["Work email", e.workEmail || "—"],
    ["Phone", e.phone || "—"],
    ["Join date", formatDate(e.joinDate)],
    ["End date", e.endDate ? formatDate(e.endDate) : "—"],
  ];

  return (
    <>
      {back}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-2">
          <h1 className="display text-[2.6rem] font-bold">{e.fullName}</h1>
          <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted">
            <span className="tabular-nums">{e.employeeNumber}</span>
            <Badge tone={status.tone}>{status.label}</Badge>
          </div>
        </div>
        {canManage && (
          <div className="flex flex-wrap gap-2">
            {e.status !== "separated" && (
              <Button variant="secondary" onClick={() => setDialog("transfer")}>
                <ArrowRightLeft size={15} strokeWidth={2.4} aria-hidden="true" />
                Transfer
              </Button>
            )}
            <Button onClick={() => setDialog("edit")}>
              <Pencil size={14} strokeWidth={2.6} aria-hidden="true" />
              Edit
            </Button>
          </div>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
        <Card title="Details">
          <dl className="grid gap-x-6 gap-y-5 sm:grid-cols-2">
            {facts.map(([label, value]) => (
              <div key={label} className="min-w-0">
                <dt className="text-[11px] font-semibold uppercase tracking-wide text-muted">{label}</dt>
                <dd className="mt-1 break-words text-sm">{value}</dd>
              </div>
            ))}
          </dl>
        </Card>
        <Card title="Timeline">
          {timeline.loading && <Skeleton className="h-32" />}
          {timeline.data?.data.length === 0 && <Empty>No events recorded.</Empty>}
          <ol className="flex flex-col">
            {timeline.data?.data.map((event, i, all) => (
              <li key={i} className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3">
                <div className="flex flex-col items-center">
                  <span className={`mt-1 size-3 rounded-full ${event.type === "separated" ? "bg-orange" : "bg-lime"}`} aria-hidden="true" />
                  {i < all.length - 1 && <span className="w-px flex-1 bg-line" aria-hidden="true" />}
                </div>
                <div className="pb-5">
                  <p className="text-sm font-semibold">{EVENT_TEXT[event.type](event)}</p>
                  <p className="text-xs tabular-nums text-muted">{formatDate(event.date)}</p>
                </div>
              </li>
            ))}
          </ol>
        </Card>
      </div>

      {dialog === "edit" && <EmployeeForm employee={e} managerLabel={managerLabel} onClose={() => setDialog(null)} onSaved={saved} />}
      {dialog === "transfer" && <TransferDialog employee={e} onClose={() => setDialog(null)} onSaved={saved} />}
    </>
  );
}
