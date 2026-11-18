"use client";

import { Plus, Search, Upload } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { EmployeeForm, ImportDialog } from "@/components/employee-dialogs";
import { Badge, Button, Card, Empty, inputClass, Notice, PageHeader, Pagination, Pill, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { EMPLOYEE_STATUS, formatDate } from "@/lib/format";
import type { Department, Employee, Organization, Page } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const STATUSES = [
  { value: "", label: "All" },
  { value: "active", label: "Active" },
  { value: "serving_notice", label: "Serving notice" },
  { value: "separated", label: "Left" },
];
const PAGE_SIZE = 20;

export default function EmployeesPage() {
  const auth = useAuth();
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [department, setDepartment] = useState("");
  const [page, setPage] = useState(1);
  const [dialog, setDialog] = useState<"add" | "import" | null>(null);

  const params = new URLSearchParams({ page: String(page), pageSize: String(PAGE_SIZE) });
  if (search.trim()) params.set("q", search.trim());
  if (status) params.set("status", status);
  if (department) params.set("departmentId", department);
  const list = useApi<Page<Employee>>(`/employees?${params}`);
  const departments = useApi<{ data: Department[] }>(auth.can("department.read") ? "/departments" : null);
  const org = useApi<Organization>("/organization");

  const deptName = new Map(departments.data?.data.map((d) => [d.id, d.name]));
  const typeName = new Map(org.data?.settings.employmentTypes.map((t) => [t.code, t.name]));
  const canManage = auth.can("employee.manage");
  // Any change to a filter goes back to the first page.
  const filter = <T,>(setter: (value: T) => void) => (value: T) => {
    setter(value);
    setPage(1);
  };

  return (
    <>
      <PageHeader title="Employees">
        {canManage && (
          <>
            <Button variant="secondary" onClick={() => setDialog("import")}>
              <Upload size={15} strokeWidth={2.4} aria-hidden="true" />
              Import CSV
            </Button>
            <Button onClick={() => setDialog("add")}>
              <Plus size={16} strokeWidth={2.6} aria-hidden="true" />
              Add employee
            </Button>
          </>
        )}
      </PageHeader>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1 sm:max-w-xs">
          <Search size={15} className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-muted" aria-hidden="true" />
          <input
            type="search"
            value={search}
            onChange={(e) => filter(setSearch)(e.target.value)}
            placeholder="Search name, number or email"
            aria-label="Search employees"
            className={`${inputClass} h-10 pl-10`}
          />
        </div>
        {STATUSES.map((s) => (
          <Pill key={s.value} active={status === s.value} aria-pressed={status === s.value} onClick={() => filter(setStatus)(s.value)}>
            {s.label}
          </Pill>
        ))}
        {departments.data && (
          <select value={department} onChange={(e) => filter(setDepartment)(e.target.value)} aria-label="Department" className="h-10 rounded-full bg-raised px-4 text-[13px] font-semibold text-ink">
            <option value="">All departments</option>
            {departments.data.data.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name}
              </option>
            ))}
          </select>
        )}
      </div>

      <Card>
        {list.error && <Notice tone="danger">Employees could not be loaded. {list.error.message}</Notice>}
        {list.loading && <Skeleton className="h-72" />}
        {list.data && list.data.total === 0 && (
          <Empty>
            {search || status || department
              ? "No employees match these filters."
              : canManage
                ? "No employees yet. Add the first one, or import a CSV file."
                : "There are no employees you can see. Department heads see the people in their own departments."}
          </Empty>
        )}
        {list.data && list.data.total > 0 && (
          <div className="-mx-2 overflow-x-auto">
            <table className="w-full min-w-[640px] border-separate border-spacing-y-1.5 text-left text-[13px]">
              <thead>
                <tr className="text-[11px] font-semibold uppercase tracking-wide text-muted">
                  <th className="px-4 py-1 font-semibold">Employee</th>
                  <th className="px-4 py-1 font-semibold">Department</th>
                  <th className="px-4 py-1 font-semibold">Job title</th>
                  <th className="px-4 py-1 font-semibold">Type</th>
                  <th className="px-4 py-1 font-semibold">Joined</th>
                  <th className="px-4 py-1 font-semibold">Status</th>
                </tr>
              </thead>
              <tbody>
                {list.data.data.map((e) => {
                  const st = EMPLOYEE_STATUS[e.status] ?? { label: e.status, tone: "muted" as const };
                  return (
                    <tr key={e.id} className="group">
                      <td className="rounded-l-2xl bg-raised px-4 py-3 group-hover:bg-line">
                        <Link href={`/employees/${e.id}`} className="font-semibold hover:underline">
                          {e.fullName}
                        </Link>
                        <div className="text-[11px] text-muted">{e.employeeNumber}</div>
                      </td>
                      <td className="bg-raised px-4 py-3 group-hover:bg-line">{deptName.get(e.departmentId) ?? "—"}</td>
                      <td className="bg-raised px-4 py-3 text-muted group-hover:bg-line">{e.jobTitle || "—"}</td>
                      <td className="bg-raised px-4 py-3 text-muted group-hover:bg-line">{typeName.get(e.employmentType) ?? e.employmentType}</td>
                      <td className="whitespace-nowrap bg-raised px-4 py-3 tabular-nums text-muted group-hover:bg-line">{formatDate(e.joinDate)}</td>
                      <td className="rounded-r-2xl bg-raised px-4 py-3 group-hover:bg-line">
                        <Badge tone={st.tone}>{st.label}</Badge>
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

      {dialog === "add" && <EmployeeForm onClose={() => setDialog(null)} onSaved={(e) => router.push(`/employees/${e.id}`)} />}
      {dialog === "import" && <ImportDialog onClose={() => setDialog(null)} onImported={list.reload} />}
    </>
  );
}
