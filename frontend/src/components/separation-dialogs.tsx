"use client";

import { useState } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { addDays, SEPARATION_TYPES, todayISO } from "@/lib/format";
import type { Organization, SeparationDetail, SeparationSettings } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { useSubmit } from "@/lib/use-submit";
import { EmployeePicker } from "./employee-picker";
import { Button, Field, inputClass, Modal, Notice } from "./ui";

const textareaClass = "min-h-24 w-full rounded-2xl bg-raised px-4 py-3 text-sm text-ink placeholder:text-muted";

function Actions({ busy, label, busyLabel, onClose, variant, disabled }: { busy: boolean; label: string; busyLabel: string; onClose: () => void; variant?: "primary" | "danger"; disabled?: boolean }) {
  return (
    <div className="flex justify-end gap-2">
      <Button variant="secondary" onClick={onClose}>
        Cancel
      </Button>
      <Button type="submit" variant={variant} disabled={busy || disabled}>
        {busy ? busyLabel : label}
      </Button>
    </div>
  );
}

// NewSeparationDialog is used two ways: an employee submitting their own
// resignation, and HR recording a separation for someone.
export function NewSeparationDialog({ onClose, onSaved }: { onClose: () => void; onSaved: (s: SeparationDetail) => void }) {
  const auth = useAuth();
  const manage = auth.can("separation.manage");
  const org = useApi<Organization & { settings: SeparationSettings }>("/organization");
  const submit = useSubmit();
  const [employee, setEmployee] = useState<{ id: string; label: string } | null>(null);
  const [type, setType] = useState("resignation");
  const [reasonCode, setReasonCode] = useState("");
  const [details, setDetails] = useState("");
  const [lastDay, setLastDay] = useState("");
  const [submissionDate, setSubmissionDate] = useState("");

  const notice = org.data?.settings.noticePeriodDays ?? 30;
  const proposed = addDays(submissionDate || todayISO(), notice);
  // A resignation takes a voluntary reason; every other type takes an involuntary or other reason.
  const reasons = (org.data?.settings.separationReasons ?? []).filter((r) => r.active && (type === "resignation") === (r.category === "voluntary"));
  const mayRecordInvoluntary = auth.can("separation.record_involuntary");

  function save(event: React.FormEvent) {
    event.preventDefault();
    submit.run(async () => {
      const saved = await api<SeparationDetail>("/separations", {
        json: {
          ...(manage && employee ? { employeeId: employee.id } : {}),
          type,
          reasonCode,
          reasonDetails: details,
          ...(lastDay ? { requestedLastWorkingDay: lastDay } : {}),
          ...(manage && submissionDate ? { submissionDate } : {}),
        },
      });
      onSaved(saved);
    });
  }

  return (
    <Modal title={manage ? "Record a separation" : "Submit your resignation"} onClose={onClose} wide>
      <form onSubmit={save} className="flex flex-col gap-4" noValidate>
        {manage && (
          <Field label="Employee" error={submit.fields?.employeeId}>
            <EmployeePicker id="separation-employee" value={employee} onChange={setEmployee} />
          </Field>
        )}
        <div className="grid gap-4 sm:grid-cols-2">
          {manage && (
            <Field label="Type" error={submit.fields?.type} hint={type === "resignation" ? "Goes through the approval steps." : "Recorded as approved straight away."}>
              <select
                value={type}
                onChange={(e) => {
                  setType(e.target.value);
                  setReasonCode("");
                }}
                className={inputClass}
              >
                {Object.entries(SEPARATION_TYPES).map(([code, label]) => (
                  <option key={code} value={code}>
                    {label}
                  </option>
                ))}
              </select>
            </Field>
          )}
          <Field label="Reason" error={submit.fields?.reasonCode}>
            <select value={reasonCode} onChange={(e) => setReasonCode(e.target.value)} required className={inputClass}>
              <option value="">Choose a reason</option>
              {reasons
                .filter((r) => r.category !== "involuntary" || mayRecordInvoluntary)
                .map((r) => (
                  <option key={r.code} value={r.code}>
                    {r.name}
                  </option>
                ))}
            </select>
          </Field>
          {manage && (
            <Field label="Submission date" error={submit.fields?.submissionDate} hint="Leave empty for today.">
              <input type="date" value={submissionDate} max={todayISO()} onChange={(e) => setSubmissionDate(e.target.value)} className={inputClass} />
            </Field>
          )}
          <Field label="Last working day" error={submit.fields?.requestedLastWorkingDay} hint={`Leave empty for ${proposed}, which is the ${notice}-day notice period.`}>
            <input type="date" value={lastDay} onChange={(e) => setLastDay(e.target.value)} className={inputClass} />
          </Field>
        </div>
        <Field label="Details" error={submit.fields?.reasonDetails} hint="Optional. Visible to HR and the approvers.">
          <textarea value={details} onChange={(e) => setDetails(e.target.value)} maxLength={2000} className={textareaClass} />
        </Field>
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <Actions busy={submit.busy} label={manage ? "Record separation" : "Submit resignation"} busyLabel="Saving…" onClose={onClose} disabled={!reasonCode || (manage && !employee)} />
      </form>
    </Modal>
  );
}

type Action = "approve" | "reject" | "withdraw" | "last-working-day" | "complete";

const COPY: Record<Action, { title: string; label: string; busy: string; intro: (s: SeparationDetail) => string }> = {
  approve: { title: "Approve this step", label: "Approve", busy: "Approving…", intro: (s) => `You are approving the resignation of ${s.employee?.fullName ?? "this employee"}.` },
  reject: { title: "Do not approve", label: "Do not approve", busy: "Saving…", intro: () => "The request stops here. The employee and the person who submitted it are told, with your comment." },
  withdraw: {
    title: "Withdraw this separation",
    label: "Withdraw",
    busy: "Withdrawing…",
    intro: (s) => (s.status === "approved" ? "This separation is already approved. Withdrawing it cancels the clearance checklist and returns the employee to active." : "The request is cancelled and the approvers are told."),
  },
  "last-working-day": { title: "Change the last working day", label: "Save", busy: "Saving…", intro: () => "Open clearance items that were due on the old date move to the new one." },
  complete: {
    title: "Complete this separation",
    label: "Complete",
    busy: "Completing…",
    intro: (s) => `${s.employee?.fullName ?? "The employee"} will be marked as having left on ${s.lastWorkingDay}, and will count in turnover from that date. This cannot be undone here.`,
  },
};

// ActionDialog covers the workflow actions that take a comment, a reason or a date.
export function ActionDialog({ action, separation, onClose, onSaved }: { action: Action; separation: SeparationDetail; onClose: () => void; onSaved: () => void }) {
  const submit = useSubmit();
  const [text, setText] = useState("");
  const [date, setDate] = useState(separation.lastWorkingDay);
  const copy = COPY[action];
  const textRequired = action === "reject" || action === "last-working-day" || (action === "withdraw" && separation.status === "approved");
  const textLabel = action === "last-working-day" ? "Reason" : action === "withdraw" ? "Note" : "Comment";
  const textField = action === "last-working-day" ? "reason" : action === "withdraw" ? "note" : "comment";

  function save(event: React.FormEvent) {
    event.preventDefault();
    submit.run(async () => {
      const base = `/separations/${separation.id}`;
      const version = separation.version;
      if (action === "last-working-day") await api(`${base}/last-working-day`, { method: "PATCH", json: { lastWorkingDay: date, reason: text, version } });
      else if (action === "complete") await api(`${base}/complete`, { json: { version } });
      else await api(`${base}/${action}`, { json: { [textField]: text, version } });
      onSaved();
    });
  }

  return (
    <Modal title={copy.title} onClose={onClose}>
      <form onSubmit={save} className="flex flex-col gap-4" noValidate>
        <p className="text-sm text-muted">{copy.intro(separation)}</p>
        {action === "last-working-day" && (
          <Field label="Last working day" error={submit.fields?.lastWorkingDay}>
            <input type="date" value={date} min={separation.submissionDate} onChange={(e) => setDate(e.target.value)} required className={inputClass} />
          </Field>
        )}
        {action !== "complete" && (
          <Field label={textLabel} error={submit.fields?.[textField]} hint={textRequired ? undefined : "Optional."}>
            <textarea value={text} onChange={(e) => setText(e.target.value)} maxLength={500} required={textRequired} className={textareaClass} />
          </Field>
        )}
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <Actions busy={submit.busy} label={copy.label} busyLabel={copy.busy} onClose={onClose} variant={action === "approve" || action === "last-working-day" ? "primary" : "danger"} disabled={textRequired && !text.trim()} />
      </form>
    </Modal>
  );
}
