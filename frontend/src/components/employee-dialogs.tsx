"use client";

import { useState } from "react";
import { api, ApiError, errorMessage } from "@/lib/api";
import { todayISO } from "@/lib/format";
import type { Department, Employee, ImportReport, Organization } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { EmployeePicker } from "./employee-picker";
import { Button, Field, inputClass, Modal, Notice } from "./ui";

type Fields = Record<string, string> | undefined;

// useSubmit runs a save and keeps the API's message and per-field errors.
function useSubmit() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fields, setFields] = useState<Fields>();
  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError(null);
    setFields(undefined);
    try {
      await action();
    } catch (err) {
      setFields(err instanceof ApiError ? err.fields : undefined);
      // When every problem is attached to a field, the summary line would only repeat them.
      setError(err instanceof ApiError && err.fields && err.code === "VALIDATION_FAILED" ? "Check the highlighted fields." : errorMessage(err));
      setBusy(false);
    }
  }
  return { busy, error, fields, run };
}

function useDepartments() {
  return useApi<{ data: Department[] }>("/departments?status=active");
}

// EmployeeForm creates an employee, or edits one when `employee` is given.
export function EmployeeForm({ employee, managerLabel, onClose, onSaved }: { employee?: Employee; managerLabel?: string; onClose: () => void; onSaved: (e: Employee) => void }) {
  const departments = useDepartments();
  const org = useApi<Organization>("/organization");
  const submit = useSubmit();
  const [form, setForm] = useState({
    employeeNumber: employee?.employeeNumber ?? "",
    fullName: employee?.fullName ?? "",
    workEmail: employee?.workEmail ?? "",
    phone: employee?.phone ?? "",
    departmentId: employee?.departmentId ?? "",
    jobTitle: employee?.jobTitle ?? "",
    employmentType: employee?.employmentType ?? "",
    joinDate: employee?.joinDate ?? todayISO(),
  });
  const [manager, setManager] = useState(employee?.managerId ? { id: employee.managerId, label: managerLabel ?? "Current manager" } : null);
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setForm({ ...form, [key]: e.target.value });
  const types = (org.data?.settings.employmentTypes ?? []).filter((t) => t.active || t.code === employee?.employmentType);
  // The join date is locked once a transfer or separation depends on it.
  const joinLocked = !!employee && (employee.departmentHistory.length !== 1 || !!employee.endDate);

  function save(event: React.FormEvent) {
    event.preventDefault();
    submit.run(async () => {
      const saved = employee
        ? await api<Employee>(`/employees/${employee.id}`, {
            method: "PATCH",
            json: {
              fullName: form.fullName,
              workEmail: form.workEmail || undefined,
              phone: form.phone,
              jobTitle: form.jobTitle,
              employmentType: form.employmentType,
              managerId: manager?.id ?? "",
              ...(joinLocked ? {} : { joinDate: form.joinDate }),
              version: employee.version,
            },
          })
        : await api<Employee>("/employees", { json: { ...form, managerId: manager?.id } });
      onSaved(saved);
    });
  }

  return (
    <Modal title={employee ? "Edit employee" : "Add employee"} onClose={onClose} wide>
      <form onSubmit={save} className="flex flex-col gap-5" noValidate>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Employee number" error={submit.fields?.employeeNumber}>
            <input value={form.employeeNumber} onChange={set("employeeNumber")} disabled={!!employee} required maxLength={30} className={inputClass} />
          </Field>
          <Field label="Full name" error={submit.fields?.fullName}>
            <input value={form.fullName} onChange={set("fullName")} required maxLength={120} className={inputClass} />
          </Field>
          <Field label="Work email" error={submit.fields?.workEmail}>
            <input type="email" value={form.workEmail} onChange={set("workEmail")} className={inputClass} />
          </Field>
          <Field label="Phone" error={submit.fields?.phone}>
            <input value={form.phone} onChange={set("phone")} maxLength={30} className={inputClass} />
          </Field>
          {!employee && (
            <Field label="Department" error={submit.fields?.departmentId}>
              <select value={form.departmentId} onChange={set("departmentId")} required className={inputClass}>
                <option value="">Choose a department</option>
                {departments.data?.data.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </select>
            </Field>
          )}
          <Field label="Job title" error={submit.fields?.jobTitle}>
            <input value={form.jobTitle} onChange={set("jobTitle")} maxLength={120} className={inputClass} />
          </Field>
          <Field label="Employment type" error={submit.fields?.employmentType}>
            <select value={form.employmentType} onChange={set("employmentType")} required className={inputClass}>
              <option value="">Choose a type</option>
              {types.map((t) => (
                <option key={t.code} value={t.code}>
                  {t.name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Join date" error={submit.fields?.joinDate} hint={joinLocked ? "Locked because a transfer or separation has been recorded." : undefined}>
            <input type="date" value={form.joinDate} onChange={set("joinDate")} disabled={joinLocked} required className={inputClass} />
          </Field>
          <div className="sm:col-span-2">
            <Field label="Line manager" error={submit.fields?.managerId} hint="Optional. Used for resignation approvals and handover.">
              <EmployeePicker id="manager" value={manager} onChange={setManager} exclude={employee?.id} />
            </Field>
          </div>
        </div>
        {employee && <p className="text-xs text-muted">To move this person to another department, use Transfer so the history is kept.</p>}
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={submit.busy}>
            {submit.busy ? "Saving…" : employee ? "Save changes" : "Add employee"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// TransferDialog moves an employee to another department from a date.
export function TransferDialog({ employee, onClose, onSaved }: { employee: Employee; onClose: () => void; onSaved: (e: Employee) => void }) {
  const departments = useDepartments();
  const submit = useSubmit();
  const [departmentId, setDepartmentId] = useState("");
  const [effectiveDate, setEffectiveDate] = useState(todayISO());

  function save(event: React.FormEvent) {
    event.preventDefault();
    submit.run(async () => {
      onSaved(await api<Employee>(`/employees/${employee.id}/transfers`, { json: { departmentId, effectiveDate, version: employee.version } }));
    });
  }

  return (
    <Modal title="Transfer to another department" onClose={onClose}>
      <form onSubmit={save} className="flex flex-col gap-5" noValidate>
        <p className="text-sm text-muted">
          {employee.fullName} moves on the effective date. Earlier days stay counted in the current department, so past figures do not change.
        </p>
        <Field label="New department" error={submit.fields?.departmentId}>
          <select value={departmentId} onChange={(e) => setDepartmentId(e.target.value)} required className={inputClass}>
            <option value="">Choose a department</option>
            {departments.data?.data
              .filter((d) => d.id !== employee.departmentId)
              .map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
          </select>
        </Field>
        <Field label="Effective date" error={submit.fields?.effectiveDate} hint="The first day in the new department. It cannot be in the future.">
          <input type="date" value={effectiveDate} max={todayISO()} onChange={(e) => setEffectiveDate(e.target.value)} required className={inputClass} />
        </Field>
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={submit.busy || !departmentId}>
            {submit.busy ? "Transferring…" : "Transfer"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

const IMPORT_COLUMNS = "employeeNumber, fullName, departmentCode, employmentType, joinDate";

// ImportDialog checks a CSV file first and imports it only when every row is valid.
export function ImportDialog({ onClose, onImported }: { onClose: () => void; onImported: () => void }) {
  const [file, setFile] = useState<File | null>(null);
  const [report, setReport] = useState<ImportReport | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function send(dryRun: boolean) {
    if (!file) return;
    setBusy(true);
    setError(null);
    const form = new FormData();
    form.append("file", file);
    try {
      const result = await api<ImportReport>(`/employees/import${dryRun ? "?dryRun=true" : ""}`, { form });
      setReport(result);
      if (!dryRun) onImported();
    } catch (err) {
      // A failed real import still carries the row-by-row report.
      const body = err instanceof ApiError ? (err.body as { report?: ImportReport } | null) : null;
      if (body?.report) setReport(body.report);
      setError(errorMessage(err));
    }
    setBusy(false);
  }

  const checked = report !== null;
  const clean = checked && report.errors.length === 0 && report.totalRows > 0;
  const done = checked && report.imported > 0;

  return (
    <Modal title="Import employees from CSV" onClose={onClose} wide>
      <div className="flex flex-col gap-5">
        <p className="text-sm text-muted">
          The first row must be a header. Required columns: <span className="text-ink">{IMPORT_COLUMNS}</span>. Optional: workEmail, phone, jobTitle,
          managerEmployeeNumber. Dates use YYYY-MM-DD. Nothing is saved unless every row is valid.
        </p>
        <Field label="CSV file" hint="Up to 2 MB and 5,000 rows.">
          <input
            type="file"
            accept=".csv,text/csv"
            disabled={busy || done}
            onChange={(e) => {
              setFile(e.target.files?.[0] ?? null);
              setReport(null);
              setError(null);
            }}
            className="block w-full rounded-2xl bg-raised p-3 text-sm text-ink file:mr-3 file:rounded-full file:border-0 file:bg-line file:px-4 file:py-2 file:text-xs file:font-bold file:text-ink"
          />
        </Field>
        {error && <Notice tone="danger">{error}</Notice>}
        {done && <Notice>Imported {report.imported} employees.</Notice>}
        {checked && !done && clean && <Notice>All {report.totalRows} rows are valid and ready to import.</Notice>}
        {checked && !done && report.totalRows === 0 && report.errors.length === 0 && <Notice>The file has no data rows.</Notice>}
        {checked && report.errors.length > 0 && (
          <div className="flex flex-col gap-2">
            <p className="text-sm font-semibold">
              {report.totalRows - report.validRows} of {report.totalRows} rows need fixing
            </p>
            <ul className="flex max-h-56 flex-col gap-1.5 overflow-y-auto text-xs">
              {report.errors.map((e, i) => (
                <li key={i} className="flex gap-3 rounded-xl bg-raised px-3 py-2">
                  <span className="w-14 shrink-0 font-bold tabular-nums text-orange">Row {e.row}</span>
                  <span className="min-w-0">
                    <span className="font-semibold">{e.field}</span> {e.message}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            {done ? "Close" : "Cancel"}
          </Button>
          {!done && !clean && (
            <Button disabled={!file || busy} onClick={() => send(true)}>
              {busy ? "Checking…" : "Check file"}
            </Button>
          )}
          {!done && clean && (
            <Button disabled={busy} onClick={() => send(false)}>
              {busy ? "Importing…" : `Import ${report.totalRows} employees`}
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
}
