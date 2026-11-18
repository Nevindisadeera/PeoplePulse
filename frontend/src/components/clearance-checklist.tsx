"use client";

import { useState } from "react";
import { api, errorMessage } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatDate, ITEM_STATUS, todayISO } from "@/lib/format";
import type { Clearance, ClearanceItem } from "@/lib/types";
import { useSubmit } from "@/lib/use-submit";
import { Badge, Button, Card, Empty, Field, Modal, Notice } from "./ui";

const ASSIGNEE: Record<ClearanceItem["assigneeType"], string> = { line_manager: "Line manager", role: "Assigned to a role", user: "Assigned to a person" };
const textareaClass = "min-h-20 w-full rounded-2xl bg-raised px-4 py-3 text-sm text-ink placeholder:text-muted";

// RemarksDialog collects the explanation required for "blocked" and "not applicable".
function RemarksDialog({ item, status, onClose, onSubmit }: { item: ClearanceItem; status: "blocked" | "not_applicable"; onClose: () => void; onSubmit: (remarks: string) => Promise<void> }) {
  const [remarks, setRemarks] = useState("");
  const submit = useSubmit();
  return (
    <Modal title={status === "blocked" ? "Mark as blocked" : "Mark as not applicable"} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit.run(() => onSubmit(remarks));
        }}
        className="flex flex-col gap-4"
      >
        <p className="text-sm text-muted">{item.title}</p>
        <Field label="Remarks" error={submit.fields?.remarks} hint={status === "blocked" ? "What is it waiting for?" : "Why does it not apply to this person?"}>
          <textarea value={remarks} onChange={(e) => setRemarks(e.target.value)} maxLength={1000} required className={textareaClass} />
        </Field>
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={submit.busy || !remarks.trim()}>
            {submit.busy ? "Saving…" : "Save"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function OverrideDialog({ clearance, onClose, onSaved }: { clearance: Clearance; onClose: () => void; onSaved: () => void }) {
  const [reason, setReason] = useState("");
  const submit = useSubmit();
  return (
    <Modal title="Allow completion with open items" onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit.run(async () => {
            await api(`/clearances/${clearance.id}/override`, { json: { reason, version: clearance.version } });
            onSaved();
          });
        }}
        className="flex flex-col gap-4"
      >
        <p className="text-sm text-muted">
          {clearance.openItems} item{clearance.openItems === 1 ? " is" : "s are"} still open. Recording an override lets the separation be completed anyway. The reason is kept in the audit log.
        </p>
        <Field label="Reason" error={submit.fields?.reason}>
          <textarea value={reason} onChange={(e) => setReason(e.target.value)} maxLength={500} required className={textareaClass} />
        </Field>
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="danger" disabled={submit.busy || reason.trim().length < 5}>
            {submit.busy ? "Saving…" : "Record override"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// ClearanceChecklist shows one employee's exit checklist. Assignees act on
// their own items; HR can act on any item. `locked` is true once the
// separation is completed or withdrawn.
export function ClearanceChecklist({ clearance, locked, onChanged }: { clearance: Clearance; locked: boolean; onChanged: () => void }) {
  const auth = useAuth();
  const [error, setError] = useState<string | null>(null);
  const [remarksFor, setRemarksFor] = useState<{ item: ClearanceItem; status: "blocked" | "not_applicable" } | null>(null);
  const [overriding, setOverriding] = useState(false);
  const manage = auth.can("clearance.manage");
  const readOnly = locked || clearance.status === "cancelled";
  const today = todayISO();

  const mine = (item: ClearanceItem) =>
    auth.status === "signedIn" && auth.can("clearance.update_assigned") && (item.userId ? item.userId === auth.user.id : item.roleId === auth.role.id);

  async function update(item: ClearanceItem, status: ClearanceItem["status"], remarks = "") {
    await api(`/clearances/${clearance.id}/items/${item.itemId}`, { method: "PATCH", json: { status, remarks, version: clearance.version } });
    onChanged();
  }
  const quick = (item: ClearanceItem, status: ClearanceItem["status"]) => {
    setError(null);
    update(item, status).catch((err) => setError(errorMessage(err)));
  };

  const categories = [...new Set(clearance.items.map((i) => i.category))];
  const done = clearance.totalItems - clearance.openItems;

  return (
    <Card
      title="Exit clearance"
      action={
        <span className="text-xs text-muted">
          {clearance.status === "cancelled" ? "Cancelled" : `${done} of ${clearance.totalItems} closed`}
        </span>
      }
    >
      {clearance.totalItems > 0 && (
        <div className="h-2 rounded-full bg-raised" role="progressbar" aria-valuemin={0} aria-valuemax={clearance.totalItems} aria-valuenow={done} aria-label="Clearance progress">
          <div className="h-2 rounded-full bg-lime" style={{ width: `${(done / clearance.totalItems) * 100}%` }} />
        </div>
      )}
      {clearance.override && <Notice>Completion with open items was allowed on {formatDate(clearance.override.at)}: {clearance.override.reason}</Notice>}
      {error && <Notice tone="danger">{error}</Notice>}
      {clearance.totalItems === 0 && <Empty>The clearance template had no items when this separation was approved.</Empty>}

      {categories.map((category) => (
        <div key={category} className="flex flex-col gap-2">
          <h3 className="text-[11px] font-semibold uppercase tracking-wide text-muted">{category}</h3>
          <ul className="flex flex-col gap-2">
            {clearance.items
              .filter((i) => i.category === category)
              .map((item) => {
                const status = ITEM_STATUS[item.status];
                const open = item.status === "pending" || item.status === "blocked";
                const overdue = open && item.dueDate < today;
                const canAct = !readOnly && (manage || mine(item));
                return (
                  <li key={item.itemId} className="flex flex-col gap-3 rounded-2xl bg-raised px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                    <div className="min-w-0">
                      <p className="text-[13px] font-semibold">{item.title}</p>
                      <p className="text-[11px] text-muted">
                        {item.fallback ? "Routed to HR" : ASSIGNEE[item.assigneeType]} · due {formatDate(item.dueDate)}
                        {overdue && <span className="font-bold text-orange"> · overdue</span>}
                      </p>
                      {item.remarks && <p className="mt-1 text-xs text-muted">“{item.remarks}”</p>}
                    </div>
                    <div className="flex shrink-0 flex-wrap items-center gap-2">
                      <Badge tone={status.tone}>{status.label}</Badge>
                      {canAct && open && (
                        <>
                          <Button size="sm" onClick={() => quick(item, "cleared")}>
                            Mark cleared
                          </Button>
                          {item.status !== "blocked" && (
                            <Button variant="secondary" size="sm" onClick={() => setRemarksFor({ item, status: "blocked" })}>
                              Blocked
                            </Button>
                          )}
                          <Button variant="secondary" size="sm" onClick={() => setRemarksFor({ item, status: "not_applicable" })}>
                            N/A
                          </Button>
                        </>
                      )}
                      {canAct && !open && (
                        <Button variant="secondary" size="sm" onClick={() => quick(item, "pending")}>
                          Reopen
                        </Button>
                      )}
                    </div>
                  </li>
                );
              })}
          </ul>
        </div>
      ))}

      {!readOnly && auth.can("clearance.override") && clearance.openItems > 0 && !clearance.override && (
        <div className="flex justify-end">
          <Button variant="secondary" onClick={() => setOverriding(true)}>
            Allow completion with open items
          </Button>
        </div>
      )}

      {remarksFor && (
        <RemarksDialog
          item={remarksFor.item}
          status={remarksFor.status}
          onClose={() => setRemarksFor(null)}
          onSubmit={async (remarks) => {
            await update(remarksFor.item, remarksFor.status, remarks);
            setRemarksFor(null);
          }}
        />
      )}
      {overriding && (
        <OverrideDialog
          clearance={clearance}
          onClose={() => setOverriding(false)}
          onSaved={() => {
            setOverriding(false);
            onChanged();
          }}
        />
      )}
    </Card>
  );
}

