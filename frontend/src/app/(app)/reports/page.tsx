"use client";

import { Download } from "lucide-react";
import { useState } from "react";
import { Button, Card, Empty, Field, inputClass, Notice, PageHeader, Pill, Skeleton } from "@/components/ui";
import { download, errorMessage } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatDateTime, periodStart, todayISO } from "@/lib/format";
import type { Department, Report } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const PREVIEW_ROWS = 50;

export default function ReportsPage() {
  const auth = useAuth();
  const reports = [
    { type: "employees", label: "Employee register", period: false, show: true },
    { type: "separations", label: "Separation register", period: true, show: true },
    { type: "clearance", label: "Clearance status", period: false, show: auth.can("clearance.read") || auth.can("clearance.manage") },
    { type: "turnover-summary", label: "Turnover summary", period: true, show: auth.can("analytics.read") },
  ].filter((r) => r.show);

  const [type, setType] = useState(reports[0].type);
  const [from, setFrom] = useState(periodStart(12));
  const [to, setTo] = useState(todayISO());
  const [department, setDepartment] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const current = reports.find((r) => r.type === type) ?? reports[0];

  const params = new URLSearchParams();
  if (current.period) {
    params.set("from", from);
    params.set("to", to);
  }
  if (department) params.set("departmentId", department);
  const query = params.toString();
  const validPeriod = !current.period || (from !== "" && to !== "" && from <= to);
  const preview = useApi<Report>(validPeriod ? `/reports/${type}${query ? `?${query}` : ""}` : null);
  const departments = useApi<{ data: Department[] }>(auth.can("department.read") ? "/departments" : null);

  async function exportCsv() {
    setBusy(true);
    setError(null);
    try {
      const file = await download(`/reports/${type}?format=csv${query ? `&${query}` : ""}`);
      const url = URL.createObjectURL(file.blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = file.filename;
      link.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(errorMessage(err));
    }
    setBusy(false);
  }

  const data = preview.data?.report;

  return (
    <>
      <PageHeader title="Reports">
        <Button onClick={exportCsv} disabled={busy || !data || data.rows.length === 0}>
          <Download size={15} strokeWidth={2.4} aria-hidden="true" />
          {busy ? "Preparing…" : "Download CSV"}
        </Button>
      </PageHeader>

      <div className="flex flex-wrap gap-2">
        {reports.map((r) => (
          <Pill key={r.type} active={type === r.type} aria-pressed={type === r.type} onClick={() => setType(r.type)}>
            {r.label}
          </Pill>
        ))}
      </div>

      <Card>
        <div className="grid gap-4 sm:grid-cols-3">
          {current.period && (
            <>
              <Field label="From">
                <input type="date" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} className={inputClass} />
              </Field>
              <Field label="To">
                <input type="date" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} className={inputClass} />
              </Field>
            </>
          )}
          {departments.data && (
            <Field label="Department">
              <select value={department} onChange={(e) => setDepartment(e.target.value)} className={inputClass}>
                <option value="">All departments</option>
                {departments.data.data.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </select>
            </Field>
          )}
        </div>
        {!validPeriod && <Notice tone="danger">Choose a period where the first date is not after the second.</Notice>}
        {error && <Notice tone="danger">The file could not be downloaded. {error}</Notice>}
      </Card>

      <Card
        title={data?.title ?? current.label}
        action={data && <span className="text-xs text-muted">{data.rows.length} rows · generated {formatDateTime(preview.data?.generatedAt)}</span>}
      >
        {preview.loading && <Skeleton className="h-64" />}
        {preview.error && <Notice tone="danger">The report could not be loaded. {preview.error.message}</Notice>}
        {data && data.notes.length > 0 && (
          <ul className="flex flex-col gap-1 text-xs text-muted">
            {data.notes.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
        )}
        {data && data.rows.length === 0 && <Empty>No rows for these filters.</Empty>}
        {data && data.rows.length > 0 && (
          <>
            <div className="-mx-2 overflow-x-auto">
              <table className="w-full border-separate border-spacing-y-1 text-left text-xs">
                <thead>
                  <tr className="text-[10px] uppercase tracking-wide text-muted">
                    {data.columns.map((c) => (
                      <th key={c} className="whitespace-nowrap px-3 py-1 font-semibold">
                        {c}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {data.rows.slice(0, PREVIEW_ROWS).map((row, i) => (
                    <tr key={i}>
                      {row.map((cell, j) => (
                        <td key={j} className="whitespace-nowrap bg-raised px-3 py-2 tabular-nums first:rounded-l-xl last:rounded-r-xl">
                          {cell || "—"}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {data.rows.length > PREVIEW_ROWS && (
              <p className="text-xs text-muted">
                Showing the first {PREVIEW_ROWS} of {data.rows.length} rows. The CSV file has all of them.
              </p>
            )}
          </>
        )}
        <p className="text-[11px] text-muted">Each download is recorded in the audit log.</p>
      </Card>
    </>
  );
}
