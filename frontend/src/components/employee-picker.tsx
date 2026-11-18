"use client";

import { useState } from "react";
import type { Employee, Page } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { inputClass } from "./ui";

// EmployeePicker chooses one current employee by typing part of a name or
// number. The search runs on the server, so it works for any organization size.
export function EmployeePicker({
  id,
  value,
  onChange,
  exclude,
}: {
  id: string;
  value: { id: string; label: string } | null;
  onChange: (value: { id: string; label: string } | null) => void;
  exclude?: string; // an employee who cannot be chosen, such as the person being edited
}) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const term = query.trim();
  const results = useApi<Page<Employee>>(open && term.length >= 2 ? `/employees?pageSize=8&q=${encodeURIComponent(term)}` : null);
  const options = (results.data?.data ?? []).filter((e) => e.id !== exclude && e.status !== "separated");

  if (value) {
    return (
      <div className="flex h-11 items-center justify-between gap-2 rounded-full bg-raised pl-4 pr-1.5 text-sm text-ink">
        <span className="truncate">{value.label}</span>
        <button type="button" onClick={() => onChange(null)} className="rounded-full px-3 py-1.5 text-[11px] font-bold text-muted hover:bg-line hover:text-ink">
          Remove
        </button>
      </div>
    );
  }

  return (
    <div className="relative">
      <input
        id={id}
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        placeholder="Type a name or employee number"
        autoComplete="off"
        role="combobox"
        aria-expanded={open && term.length >= 2}
        aria-controls={`${id}-options`}
        className={inputClass}
      />
      {open && term.length >= 2 && (
        <ul id={`${id}-options`} role="listbox" className="absolute left-0 right-0 top-12 z-10 flex max-h-56 flex-col overflow-y-auto rounded-2xl bg-raised p-1.5 shadow-2xl ring-1 ring-line">
          {results.loading && <li className="px-3 py-2 text-xs font-normal text-muted">Searching…</li>}
          {!results.loading && options.length === 0 && <li className="px-3 py-2 text-xs font-normal text-muted">No current employee matches “{term}”.</li>}
          {options.map((e) => (
            <li key={e.id} role="option" aria-selected="false">
              <button
                type="button"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => {
                  onChange({ id: e.id, label: `${e.fullName} (${e.employeeNumber})` });
                  setQuery("");
                  setOpen(false);
                }}
                className="flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-sm font-medium text-ink hover:bg-line"
              >
                <span className="truncate">{e.fullName}</span>
                <span className="shrink-0 text-xs text-muted">{e.employeeNumber}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
