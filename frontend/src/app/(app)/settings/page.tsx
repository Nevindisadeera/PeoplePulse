"use client";

import { useState } from "react";
import { AssigneeSelect, cell, Check, RemoveButton, SaveBar, slugify, useAssigneeOptions, useSave } from "@/components/settings-parts";
import { Card, Field, inputClass, Notice, PageHeader, Pill, Skeleton } from "@/components/ui";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { ChainStep, ClearanceTemplateItem, EmploymentType, FullOrganization, InterviewQuestion, InterviewTemplate, SeparationReason } from "@/lib/types";
import { useApi } from "@/lib/use-api";

type SectionProps = { org: FullOrganization; onSaved: () => void };

// Rows carry a client-side key so React keeps inputs stable while rows are added and removed.
let nextKey = 1;
const keyed = <T,>(items: T[]) => items.map((item) => ({ ...item, key: nextKey++ }));
const Intro = ({ children }: { children: React.ReactNode }) => <p className="max-w-3xl text-sm text-muted">{children}</p>;

function OrganizationSection({ org, onSaved }: SectionProps) {
  const state = useSave();
  const [form, setForm] = useState({
    name: org.name,
    timeZone: org.timeZone,
    noticePeriodDays: String(org.settings.noticePeriodDays),
    analyticsStartDate: org.settings.analyticsStartDate ?? "",
    smallGroupThreshold: String(org.settings.smallGroupThreshold),
  });
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => {
    state.touch();
    setForm({ ...form, [key]: e.target.value });
  };
  const save = () =>
    state.save(async () => {
      await api("/organization", {
        method: "PATCH",
        json: { ...form, noticePeriodDays: Number(form.noticePeriodDays), smallGroupThreshold: Number(form.smallGroupThreshold) },
      });
      onSaved();
    });
  return (
    <Card title="Organization">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Name" error={state.fields?.name}>
          <input value={form.name} onChange={set("name")} maxLength={120} className={inputClass} />
        </Field>
        <Field label="Time zone" error={state.fields?.timeZone} hint="An IANA name such as Asia/Colombo. Dates and “today” follow it.">
          <input value={form.timeZone} onChange={set("timeZone")} className={inputClass} />
        </Field>
        <Field label="Notice period (days)" error={state.fields?.noticePeriodDays} hint="Used to propose a last working day. HR can change it per separation.">
          <input type="number" min={0} max={365} value={form.noticePeriodDays} onChange={set("noticePeriodDays")} className={inputClass} />
        </Field>
        <Field label="Small group threshold" error={state.fields?.smallGroupThreshold} hint="Rates for groups with a lower average headcount are flagged.">
          <input type="number" min={1} max={50} value={form.smallGroupThreshold} onChange={set("smallGroupThreshold")} className={inputClass} />
        </Field>
        <Field
          label="Analytics start date"
          error={state.fields?.analyticsStartDate}
          hint="The earliest date from which your records are complete, including people who have since left. No rate is shown for a period that starts before it. Leave empty if you have not checked."
        >
          <input type="date" value={form.analyticsStartDate} onChange={set("analyticsStartDate")} className={inputClass} />
        </Field>
      </div>
      <SaveBar state={state} onSave={save} />
    </Card>
  );
}

function TypesSection({ org, onSaved }: SectionProps) {
  const state = useSave();
  const [rows, setRows] = useState(() => keyed(org.settings.employmentTypes.map((t) => ({ ...t, existing: true }))));
  const edit = (key: number, patch: Partial<EmploymentType>) => {
    state.touch();
    setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  };
  const save = () =>
    state.save(async () => {
      await api("/settings/employment-types", {
        method: "PUT",
        json: { employmentTypes: rows.map((r) => ({ code: r.code || slugify(r.name), name: r.name, countsInHeadcount: r.countsInHeadcount, active: r.active })) },
      });
      onSaved();
    });
  return (
    <Card title="Employment types">
      <Intro>Types that do not count in headcount, such as interns, are left out of turnover and retention entirely. A type in use can be deactivated but not removed.</Intro>
      <ul className="flex flex-col gap-2">
        {rows.map((r) => (
          <li key={r.key} className="flex flex-wrap items-center gap-2">
            <input aria-label="Type name" value={r.name} onChange={(e) => edit(r.key, { name: e.target.value })} placeholder="Name" className={`${cell} flex-1 sm:w-auto`} />
            <Check label="Counts in headcount" checked={r.countsInHeadcount} onChange={(v) => edit(r.key, { countsInHeadcount: v })} />
            <Check label="Active" checked={r.active} onChange={(v) => edit(r.key, { active: v })} />
            <RemoveButton label={`Remove ${r.name || "type"}`} onClick={() => { state.touch(); setRows(rows.filter((x) => x.key !== r.key)); }} />
          </li>
        ))}
      </ul>
      <SaveBar state={state} onSave={save} addLabel="Add type" onAdd={() => setRows([...rows, { key: nextKey++, code: "", name: "", countsInHeadcount: true, active: true, existing: false }])} />
    </Card>
  );
}

function ReasonsSection({ org, onSaved }: SectionProps) {
  const state = useSave();
  const [rows, setRows] = useState(() => keyed(org.settings.separationReasons));
  const edit = (key: number, patch: Partial<SeparationReason>) => {
    state.touch();
    setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  };
  const save = () =>
    state.save(async () => {
      await api("/settings/separation-reasons", {
        method: "PUT",
        json: { separationReasons: rows.map((r) => ({ code: r.code || slugify(r.name), name: r.name, category: r.category, active: r.active })) },
      });
      onSaved();
    });
  return (
    <Card title="Separation reasons">
      <Intro>The category decides how a separation is counted: voluntary, involuntary or other turnover. Resignations use voluntary reasons. A reason already used keeps its category and cannot be removed, only renamed or deactivated.</Intro>
      <ul className="flex flex-col gap-2">
        {rows.map((r) => (
          <li key={r.key} className="flex flex-wrap items-center gap-2">
            <input aria-label="Reason name" value={r.name} onChange={(e) => edit(r.key, { name: e.target.value })} placeholder="Reason" className={`${cell} flex-1 sm:w-auto`} />
            <select aria-label={`Category of ${r.name}`} value={r.category} onChange={(e) => edit(r.key, { category: e.target.value as SeparationReason["category"] })} className={`${cell} sm:w-40 sm:flex-none`}>
              <option value="voluntary">Voluntary</option>
              <option value="involuntary">Involuntary</option>
              <option value="other">Other</option>
            </select>
            <Check label="Active" checked={r.active} onChange={(v) => edit(r.key, { active: v })} />
            <RemoveButton label={`Remove ${r.name || "reason"}`} onClick={() => { state.touch(); setRows(rows.filter((x) => x.key !== r.key)); }} />
          </li>
        ))}
      </ul>
      <SaveBar state={state} onSave={save} addLabel="Add reason" onAdd={() => setRows([...rows, { key: nextKey++, code: "", name: "", category: "voluntary", active: true }])} />
    </Card>
  );
}

function ChainSection({ org, onSaved }: SectionProps) {
  const state = useSave();
  const options = useAssigneeOptions();
  const [rows, setRows] = useState(() => keyed(org.settings.approvalChain));
  const save = () =>
    state.save(async () => {
      await api("/settings/approval-chain", { method: "PUT", json: { approvalChain: rows.map(({ approverType, roleId, userId }) => ({ approverType, roleId, userId })) } });
      onSaved();
    });
  return (
    <Card title="Resignation approval">
      <Intro>
        A resignation goes through these steps in order. With no steps it is approved as soon as it is submitted. If a step&apos;s approver cannot be found, for example the employee has no line
        manager, the step goes to the HR Manager role. Requests already in progress keep the steps they started with.
      </Intro>
      {rows.length === 0 && <Notice>No approval steps. Resignations are approved on submission.</Notice>}
      <ol className="flex flex-col gap-2">
        {rows.map((r, i) => (
          <li key={r.key} className="flex flex-wrap items-center gap-2">
            <span className="display flex size-10 shrink-0 items-center justify-center rounded-full bg-lime text-sm font-bold text-ground">{i + 1}</span>
            <AssigneeSelect
              label={`Step ${i + 1}`}
              options={options}
              value={{ type: r.approverType, roleId: r.roleId, userId: r.userId }}
              onChange={(v) => { state.touch(); setRows(rows.map((x) => (x.key === r.key ? { key: x.key, approverType: v.type, roleId: v.roleId, userId: v.userId } : x))); }}
            />
            <RemoveButton label={`Remove step ${i + 1}`} onClick={() => { state.touch(); setRows(rows.filter((x) => x.key !== r.key)); }} />
          </li>
        ))}
      </ol>
      <SaveBar state={state} onSave={save} addLabel="Add step" onAdd={rows.length < 3 ? () => setRows([...rows, { key: nextKey++, approverType: "line_manager" } as ChainStep & { key: number }]) : undefined} />
    </Card>
  );
}

function ClearanceSection({ org, onSaved }: SectionProps) {
  const state = useSave();
  const options = useAssigneeOptions();
  const [rows, setRows] = useState(() => keyed(org.settings.clearanceTemplate));
  const edit = (key: number, patch: Partial<ClearanceTemplateItem>) => {
    state.touch();
    setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  };
  const save = () =>
    state.save(async () => {
      await api("/settings/clearance-template", {
        method: "PUT",
        json: { clearanceTemplate: rows.map(({ category, title, assigneeType, roleId, userId, active }) => ({ category, title, assigneeType, roleId, userId, active })) },
      });
      onSaved();
    });
  return (
    <Card title="Exit clearance checklist">
      <Intro>Each active item is copied onto an employee&apos;s checklist when their separation is approved, due on the last working day. Checklists already created are not changed.</Intro>
      <ul className="flex flex-col gap-3">
        {rows.map((r) => (
          <li key={r.key} className="flex flex-col gap-2 rounded-2xl bg-raised/40 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <input aria-label="Category" value={r.category} onChange={(e) => edit(r.key, { category: e.target.value })} placeholder="Category" className={`${cell} sm:w-40 sm:flex-none`} />
              <input aria-label="Item" value={r.title} onChange={(e) => edit(r.key, { title: e.target.value })} placeholder="What must be done" className={`${cell} flex-1 sm:w-auto`} />
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <AssigneeSelect
                label={r.title || "Item"}
                options={options}
                value={{ type: r.assigneeType, roleId: r.roleId, userId: r.userId }}
                onChange={(v) => edit(r.key, { assigneeType: v.type, roleId: v.roleId, userId: v.userId })}
              />
              <Check label="Active" checked={r.active} onChange={(v) => edit(r.key, { active: v })} />
              <RemoveButton label={`Remove ${r.title || "item"}`} onClick={() => { state.touch(); setRows(rows.filter((x) => x.key !== r.key)); }} />
            </div>
          </li>
        ))}
      </ul>
      <SaveBar state={state} onSave={save} addLabel="Add item" onAdd={() => setRows([...rows, { key: nextKey++, category: "", title: "", assigneeType: "line_manager", active: true }])} />
    </Card>
  );
}

function InterviewSection() {
  const template = useApi<InterviewTemplate>("/settings/exit-interview-template");
  if (template.error) return <Notice tone="danger">The exit interview settings could not be loaded. {template.error.message}</Notice>;
  if (!template.data) return <Skeleton className="h-72 rounded-card" />;
  return <InterviewForm template={template.data} />;
}

function InterviewForm({ template }: { template: InterviewTemplate }) {
  const state = useSave();
  const [enabled, setEnabled] = useState(template.enabled);
  const [selfService, setSelfService] = useState(template.employeeSelfService);
  const [minimum, setMinimum] = useState(String(template.minAggregateResponses));
  const [rows, setRows] = useState(() => keyed(template.questions.map((q) => ({ ...q, optionsText: (q.options ?? []).join(", "), existing: true }))));
  const edit = (key: number, patch: Partial<InterviewQuestion & { optionsText: string }>) => {
    state.touch();
    setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  };
  const save = () =>
    state.save(async () => {
      const saved = await api<InterviewTemplate>("/settings/exit-interview-template", {
        method: "PUT",
        json: {
          enabled,
          employeeSelfService: selfService,
          minAggregateResponses: Number(minimum),
          questions: rows.map((r) => ({
            id: r.existing ? r.id : "",
            text: r.text,
            type: r.type,
            options: r.type === "choice" ? r.optionsText.split(",").map((o) => o.trim()).filter(Boolean) : undefined,
            required: r.required,
            active: r.active,
          })),
        },
      });
      // New questions now have ids; treat them as existing from here on.
      setRows(keyed(saved.questions.map((q) => ({ ...q, optionsText: (q.options ?? []).join(", "), existing: true }))));
    });
  return (
    <Card title="Exit interview">
      <Intro>
        Questions are copied onto each interview when it is created, so later changes do not alter interviews already started. The type of a saved question cannot be changed, because earlier
        answers would lose their meaning; deactivate it and add a new one.
      </Intro>
      <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
        <Check label="Exit interviews are used" checked={enabled} onChange={(v) => { state.touch(); setEnabled(v); }} />
        <Check label="Employees can fill in their own form" checked={selfService} onChange={(v) => { state.touch(); setSelfService(v); }} />
        <label className="flex items-center gap-2 text-xs font-semibold text-ink">
          Show combined results from
          <input type="number" min={1} max={50} value={minimum} onChange={(e) => { state.touch(); setMinimum(e.target.value); }} aria-label="Minimum responses" className={`${cell} w-20`} />
          responses
        </label>
      </div>
      <ul className="flex flex-col gap-3">
        {rows.map((r) => (
          <li key={r.key} className="flex flex-col gap-2 rounded-2xl bg-raised/40 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <input aria-label="Question" value={r.text} onChange={(e) => edit(r.key, { text: e.target.value })} placeholder="Question" className={`${cell} flex-1 sm:w-auto`} />
              <select aria-label="Answer type" value={r.type} disabled={r.existing} onChange={(e) => edit(r.key, { type: e.target.value as InterviewQuestion["type"] })} className={`${cell} sm:w-44 sm:flex-none`}>
                <option value="rating">Rating 1 to 5</option>
                <option value="choice">One of several options</option>
                <option value="text">Free text</option>
              </select>
            </div>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
              {r.type === "choice" && (
                <input aria-label="Options" value={r.optionsText} onChange={(e) => edit(r.key, { optionsText: e.target.value })} placeholder="Options, separated by commas" className={`${cell} flex-1 sm:w-auto`} />
              )}
              <Check label="Required" checked={r.required} onChange={(v) => edit(r.key, { required: v })} />
              <Check label="Active" checked={r.active} onChange={(v) => edit(r.key, { active: v })} />
              <RemoveButton label="Remove question" onClick={() => { state.touch(); setRows(rows.filter((x) => x.key !== r.key)); }} />
            </div>
          </li>
        ))}
      </ul>
      <SaveBar state={state} onSave={save} addLabel="Add question" onAdd={() => setRows([...rows, { key: nextKey++, id: "", text: "", type: "rating", required: false, active: true, optionsText: "", existing: false }])} />
    </Card>
  );
}

export default function SettingsPage() {
  const auth = useAuth();
  const canOrg = auth.can("org.settings.manage");
  const canInterview = auth.can("exit_interview.manage");
  const tabs = [
    ...(canOrg
      ? [
          { id: "organization", label: "Organization" },
          { id: "types", label: "Employment types" },
          { id: "reasons", label: "Separation reasons" },
          { id: "chain", label: "Approval" },
          { id: "clearance", label: "Clearance" },
        ]
      : []),
    ...(canInterview ? [{ id: "interview", label: "Exit interview" }] : []),
  ];
  const [tab, setTab] = useState(tabs[0]?.id ?? "");
  const org = useApi<FullOrganization>("/organization");

  if (tabs.length === 0) return <Notice tone="danger">You do not have permission to change settings.</Notice>;

  return (
    <>
      <PageHeader title="Settings" />
      <div className="flex flex-wrap gap-2">
        {tabs.map((t) => (
          <Pill key={t.id} active={tab === t.id} aria-pressed={tab === t.id} onClick={() => setTab(t.id)}>
            {t.label}
          </Pill>
        ))}
      </div>
      {org.error && <Notice tone="danger">Settings could not be loaded. {org.error.message}</Notice>}
      {!org.data && !org.error && <Skeleton className="h-72 rounded-card" />}
      {/* Sections are keyed by tab so each starts from the saved values when opened. */}
      {org.data && tab === "organization" && <OrganizationSection key="organization" org={org.data} onSaved={org.reload} />}
      {org.data && tab === "types" && <TypesSection key="types" org={org.data} onSaved={org.reload} />}
      {org.data && tab === "reasons" && <ReasonsSection key="reasons" org={org.data} onSaved={org.reload} />}
      {org.data && tab === "chain" && <ChainSection key="chain" org={org.data} onSaved={org.reload} />}
      {org.data && tab === "clearance" && <ClearanceSection key="clearance" org={org.data} onSaved={org.reload} />}
      {tab === "interview" && <InterviewSection />}
    </>
  );
}
