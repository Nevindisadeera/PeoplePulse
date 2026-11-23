"use client";

import { useState } from "react";
import { Button, Card, Empty, Field, inputClass, Notice, PageHeader, Pagination, Skeleton } from "@/components/ui";
import { formatDateTime, humanize } from "@/lib/format";
import type { AuditEntry, Page } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const PAGE_SIZE = 25;

// Areas offered in the filter; the value is the audit entry's record type.
const AREAS = [
  ["", "Everything"],
  ["user", "Users and sign-ins"],
  ["role", "Roles"],
  ["employee", "Employees"],
  ["import", "Imports"],
  ["department", "Departments"],
  ["separation", "Separations"],
  ["clearance", "Clearance"],
  ["exit_interview", "Exit interviews"],
  ["organization", "Settings"],
  ["report", "Report downloads"],
];

// "separation.last_working_day_changed" -> "Separation: last working day changed"
function actionLabel(action: string): string {
  const [area, ...rest] = action.split(".");
  return rest.length ? `${humanize(area)}: ${rest.join(" ").replace(/_/g, " ")}` : humanize(action);
}

// summarize turns the stored change details into one readable line.
function summarize(changes: Record<string, unknown> | undefined): string {
  if (!changes) return "";
  return Object.entries(changes)
    .map(([key, value]) => {
      if (value && typeof value === "object" && !Array.isArray(value) && "from" in value && "to" in value) {
        const v = value as { from: unknown; to: unknown };
        return `${key}: ${format(v.from)} → ${format(v.to)}`;
      }
      return `${key}: ${format(value)}`;
    })
    .join(" · ");
}

function format(value: unknown): string {
  if (value === null || value === undefined || value === "") return "none";
  if (Array.isArray(value)) return value.length > 4 ? `${value.length} items` : value.map(format).join(", ");
  if (typeof value === "object") return `${Object.keys(value).length} fields`;
  const text = String(value);
  // Record IDs are long hexadecimal strings that mean nothing to a reader.
  return /^[0-9a-f]{24}$/.test(text) ? `#${text.slice(-6)}` : text;
}

export default function AuditPage() {
  const [area, setArea] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [page, setPage] = useState(1);

  const params = new URLSearchParams({ page: String(page), pageSize: String(PAGE_SIZE) });
  if (area) params.set("entityType", area);
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const list = useApi<Page<AuditEntry>>(`/audit-logs?${params}`);
  const filtered = area !== "" || from !== "" || to !== "";
  const change = (setter: (value: string) => void) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
    setter(e.target.value);
    setPage(1);
  };

  return (
    <>
      <PageHeader title="Audit log" />
      <Card>
        <div className="grid items-end gap-4 sm:grid-cols-4">
          <Field label="Area">
            <select value={area} onChange={change(setArea)} className={inputClass}>
              {AREAS.map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="From">
            <input type="date" value={from} max={to || undefined} onChange={change(setFrom)} className={inputClass} />
          </Field>
          <Field label="To">
            <input type="date" value={to} min={from || undefined} onChange={change(setTo)} className={inputClass} />
          </Field>
          {filtered && (
            <Button
              variant="secondary"
              className="h-11"
              onClick={() => {
                setArea("");
                setFrom("");
                setTo("");
                setPage(1);
              }}
            >
              Clear filters
            </Button>
          )}
        </div>
      </Card>

      <Card>
        {list.error && <Notice tone="danger">The audit log could not be loaded. {list.error.message}</Notice>}
        {list.loading && <Skeleton className="h-72" />}
        {list.data && list.data.total === 0 && <Empty>{filtered ? "Nothing was recorded for these filters." : "Nothing has been recorded yet."}</Empty>}
        {list.data && list.data.total > 0 && (
          <ul className="flex flex-col gap-1.5">
            {list.data.data.map((entry) => (
              <li key={entry.id} className="grid gap-x-4 gap-y-1 rounded-2xl bg-raised px-4 py-3 text-[13px] sm:grid-cols-[10rem_11rem_minmax(0,1fr)]">
                <span className="whitespace-nowrap text-xs tabular-nums text-muted">{formatDateTime(entry.at)}</span>
                <span className="truncate font-semibold">{entry.actorName ?? (entry.actorType === "user" ? "Unknown user" : humanize(entry.actorType))}</span>
                <div className="min-w-0">
                  <p className="font-semibold">{actionLabel(entry.action)}</p>
                  {entry.changes && <p className="break-words text-xs text-muted">{summarize(entry.changes)}</p>}
                </div>
              </li>
            ))}
          </ul>
        )}
        {list.data && <Pagination page={page} pageSize={PAGE_SIZE} total={list.data.total} onPage={setPage} />}
        <p className="text-[11px] text-muted">Entries are written by the system and cannot be edited or deleted here. Times are shown in your time zone.</p>
      </Card>
    </>
  );
}
