"use client";

import { Trash2 } from "lucide-react";
import { useState } from "react";
import { useAuth } from "@/lib/auth";
import type { Page, Role, User } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { useSubmit } from "@/lib/use-submit";
import { Button, Notice } from "./ui";

export const cell = "h-10 w-full min-w-0 rounded-full bg-raised px-4 text-[13px] text-ink placeholder:text-muted disabled:opacity-60";

// "Fixed-term contract" -> "fixed_term_contract", for codes the API stores.
export function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 30);
}

// useSave wraps a settings save and remembers that it succeeded, so the
// section can say "Saved" until the next edit.
export function useSave() {
  const submit = useSubmit();
  const [saved, setSaved] = useState(false);
  return {
    ...submit,
    saved,
    touch: () => setSaved(false),
    save: (action: () => Promise<void>) =>
      submit.run(async () => {
        setSaved(false);
        await action();
        setSaved(true);
      }),
  };
}

export function SaveBar({ state, onSave, onAdd, addLabel }: { state: ReturnType<typeof useSave>; onSave: () => void; onAdd?: () => void; addLabel?: string }) {
  // List sections report every problem against one key, so show the first field message when there is one.
  const detail = state.fields ? Object.values(state.fields)[0] : null;
  return (
    <div className="flex flex-col gap-3">
      {state.error && <Notice tone="danger">{detail ? `Not saved: ${detail}.` : state.error}</Notice>}
      <div className="flex flex-wrap items-center justify-between gap-3">
        {onAdd ? (
          <Button variant="secondary" onClick={onAdd}>
            {addLabel}
          </Button>
        ) : (
          <span />
        )}
        <div className="flex items-center gap-3">
          {state.saved && (
            <span className="text-xs font-semibold text-lime" role="status">
              Saved
            </span>
          )}
          <Button onClick={onSave} disabled={state.busy}>
            {state.busy ? "Saving…" : "Save changes"}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function Check({ label, checked, onChange, disabled }: { label: string; checked: boolean; onChange: (checked: boolean) => void; disabled?: boolean }) {
  return (
    <label className="inline-flex shrink-0 cursor-pointer items-center gap-2 text-xs font-semibold text-ink">
      <input type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} className="size-4 accent-[#b9f15c]" />
      {label}
    </label>
  );
}

export function RemoveButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button type="button" aria-label={label} title={label} onClick={onClick} className="flex size-10 shrink-0 items-center justify-center rounded-full bg-raised text-muted hover:bg-line hover:text-ink">
      <Trash2 size={15} aria-hidden="true" />
    </button>
  );
}

type AssigneeValue = { type: "line_manager" | "role" | "user"; roleId?: string; userId?: string };

// useAssigneeOptions loads the roles and users offered when assigning a step
// or a checklist item. Either list is empty if the role cannot read it.
export function useAssigneeOptions() {
  const auth = useAuth();
  const roles = useApi<{ data: Role[] }>(auth.can("role.read") ? "/roles" : null);
  const users = useApi<Page<User>>(auth.can("user.read") ? "/users?pageSize=100" : null);
  return { roles: roles.data?.data ?? [], users: (users.data?.data ?? []).filter((u) => u.status === "active") };
}

// AssigneeSelect chooses who does something: the employee's line manager,
// everyone holding a role, or one named user.
export function AssigneeSelect({ value, onChange, options, label }: { value: AssigneeValue; onChange: (value: AssigneeValue) => void; options: ReturnType<typeof useAssigneeOptions>; label: string }) {
  return (
    <div className="flex min-w-0 flex-1 flex-wrap gap-2">
      <select aria-label={`${label}: who`} value={value.type} onChange={(e) => onChange({ type: e.target.value as AssigneeValue["type"] })} className={`${cell} sm:w-44 sm:flex-none`}>
        <option value="line_manager">Line manager</option>
        <option value="role">Everyone in a role</option>
        <option value="user">One person</option>
      </select>
      {value.type === "role" && (
        <select aria-label={`${label}: role`} value={value.roleId ?? ""} onChange={(e) => onChange({ type: "role", roleId: e.target.value })} className={`${cell} flex-1 sm:w-auto`}>
          <option value="">Choose a role</option>
          {options.roles.map((r) => (
            <option key={r.id} value={r.id}>
              {r.name}
            </option>
          ))}
        </select>
      )}
      {value.type === "user" && (
        <select aria-label={`${label}: person`} value={value.userId ?? ""} onChange={(e) => onChange({ type: "user", userId: e.target.value })} className={`${cell} flex-1 sm:w-auto`}>
          <option value="">Choose a person</option>
          {options.users.map((u) => (
            <option key={u.id} value={u.id}>
              {u.fullName}
            </option>
          ))}
        </select>
      )}
    </div>
  );
}
