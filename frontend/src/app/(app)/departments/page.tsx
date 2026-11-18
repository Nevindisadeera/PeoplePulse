"use client";

import { Plus } from "lucide-react";
import { useState } from "react";
import { EmployeePicker } from "@/components/employee-picker";
import { Badge, Button, Card, Empty, Field, inputClass, Modal, Notice, PageHeader, Pill, Skeleton } from "@/components/ui";
import { api, ApiError, errorMessage } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { Department, Employee } from "@/lib/types";
import { useApi } from "@/lib/use-api";

function HeadName({ id }: { id?: string }) {
  const head = useApi<Employee>(id ? `/employees/${id}` : null);
  if (!id) return <span className="text-muted">No head assigned</span>;
  if (head.error) return <span className="text-muted">Head not visible to you</span>;
  return <span>{head.data ? head.data.fullName : "…"}</span>;
}

function DepartmentForm({ department, all, onClose, onSaved }: { department?: Department; all: Department[]; onClose: () => void; onSaved: () => void }) {
  const currentHead = useApi<Employee>(department?.headEmployeeId ? `/employees/${department.headEmployeeId}` : null);
  const [name, setName] = useState(department?.name ?? "");
  const [code, setCode] = useState(department?.code ?? "");
  const [parentId, setParentId] = useState(department?.parentId ?? "");
  // undefined means "not changed": the current head is shown once it has loaded.
  const [head, setHead] = useState<{ id: string; label: string } | null | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fields, setFields] = useState<Record<string, string>>();

  const shownHead =
    head !== undefined ? head : currentHead.data ? { id: currentHead.data.id, label: `${currentHead.data.fullName} (${currentHead.data.employeeNumber})` } : null;

  async function save(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setFields(undefined);
    try {
      if (department) {
        await api(`/departments/${department.id}`, {
          method: "PATCH",
          json: { name, code, parentId, ...(head !== undefined ? { headEmployeeId: head?.id ?? "" } : {}), version: department.version },
        });
      } else {
        await api("/departments", { json: { name, code, parentId: parentId || undefined, headEmployeeId: head?.id } });
      }
      onSaved();
    } catch (err) {
      const apiFields = err instanceof ApiError ? err.fields : undefined;
      setFields(apiFields);
      setError(apiFields ? "Check the highlighted fields." : errorMessage(err));
      setBusy(false);
    }
  }

  return (
    <Modal title={department ? "Edit department" : "Add department"} onClose={onClose}>
      <form onSubmit={save} className="flex flex-col gap-4" noValidate>
        <Field label="Name" error={fields?.name}>
          <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={100} className={inputClass} />
        </Field>
        <Field label="Code" error={fields?.code} hint="2 to 20 letters, digits or hyphens. Used in CSV imports.">
          <input value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} required maxLength={20} className={inputClass} />
        </Field>
        <Field label="Parent department" error={fields?.parentId} hint="Optional.">
          <select value={parentId} onChange={(e) => setParentId(e.target.value)} className={inputClass}>
            <option value="">None</option>
            {all
              .filter((d) => d.status === "active" && d.id !== department?.id)
              .map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
          </select>
        </Field>
        <Field label="Department head" error={fields?.headEmployeeId} hint="Optional. The head can see this department's people and figures.">
          <EmployeePicker id="head" value={shownHead} onChange={setHead} />
        </Field>
        {error && <Notice tone="danger">{error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={busy}>
            {busy ? "Saving…" : department ? "Save changes" : "Add department"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

export default function DepartmentsPage() {
  const auth = useAuth();
  const [showArchived, setShowArchived] = useState(false);
  const list = useApi<{ data: Department[] }>("/departments");
  const [editing, setEditing] = useState<Department | "new" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const canManage = auth.can("department.manage");

  const all = list.data?.data ?? [];
  const names = new Map(all.map((d) => [d.id, d.name]));
  const archivedCount = all.filter((d) => d.status === "archived").length;
  const shown = all.filter((d) => (showArchived ? d.status === "archived" : d.status === "active"));

  async function setStatus(department: Department, action: "archive" | "restore") {
    setError(null);
    try {
      await api(`/departments/${department.id}/${action}`, { method: "POST", json: {} });
      list.reload();
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  return (
    <>
      <PageHeader title="Departments">
        <Pill active={!showArchived} aria-pressed={!showArchived} onClick={() => setShowArchived(false)}>
          Active
        </Pill>
        <Pill active={showArchived} aria-pressed={showArchived} onClick={() => setShowArchived(true)}>
          Archived{archivedCount ? ` (${archivedCount})` : ""}
        </Pill>
        {canManage && (
          <Button onClick={() => setEditing("new")}>
            <Plus size={16} strokeWidth={2.6} aria-hidden="true" />
            Add department
          </Button>
        )}
      </PageHeader>

      {error && <Notice tone="danger">{error}</Notice>}
      {list.error && <Notice tone="danger">Departments could not be loaded. {list.error.message}</Notice>}
      {list.loading && <Skeleton className="h-64 rounded-card" />}
      {list.data && shown.length === 0 && (
        <Card>
          <Empty>{showArchived ? "No departments have been archived." : canManage ? "No departments yet. Add the first one before adding employees." : "No departments yet."}</Empty>
        </Card>
      )}

      <div className="grid gap-6 sm:grid-cols-2 xl:grid-cols-3">
        {shown.map((d) => (
          <Card key={d.id}>
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h2 className="display truncate text-xl font-semibold">{d.name}</h2>
                <p className="mt-1.5 text-xs text-muted">
                  {d.code}
                  {d.parentId && names.get(d.parentId) ? ` · part of ${names.get(d.parentId)}` : ""}
                </p>
              </div>
              {d.status === "archived" ? <Badge>Archived</Badge> : <span className="display text-4xl font-bold tabular-nums text-lime">{d.headcount}</span>}
            </div>
            <div className="flex items-center justify-between gap-3 text-[13px]">
              <HeadName id={d.headEmployeeId} />
              {d.status === "active" && <span className="shrink-0 text-xs text-muted">{d.headcount === 1 ? "person" : "people"}</span>}
            </div>
            {canManage && (
              <div className="flex flex-wrap gap-2">
                {d.status === "active" ? (
                  <>
                    <Button variant="secondary" onClick={() => setEditing(d)}>
                      Edit
                    </Button>
                    <Button variant="secondary" onClick={() => setStatus(d, "archive")}>
                      Archive
                    </Button>
                  </>
                ) : (
                  <Button variant="secondary" onClick={() => setStatus(d, "restore")}>
                    Restore
                  </Button>
                )}
              </div>
            )}
          </Card>
        ))}
      </div>

      {editing && (
        <DepartmentForm
          department={editing === "new" ? undefined : editing}
          all={all}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            list.reload();
          }}
        />
      )}
    </>
  );
}
