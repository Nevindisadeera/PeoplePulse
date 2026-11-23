"use client";

import { Copy, Plus } from "lucide-react";
import { useState } from "react";
import { EmployeePicker } from "@/components/employee-picker";
import { Check } from "@/components/settings-parts";
import { Badge, Button, Card, Empty, Field, inputClass, Modal, Notice, PageHeader, Pagination, Pill, Skeleton } from "@/components/ui";
import { api, errorMessage } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { formatDateTime } from "@/lib/format";
import type { AccountRole, AccountUser, Employee, Page, Permission } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { useSubmit } from "@/lib/use-submit";

const USER_STATUS = { active: { label: "Active", tone: "lime" }, invited: { label: "Invited", tone: "orange" }, inactive: { label: "Deactivated", tone: "muted" } } as const;
const PAGE_SIZE = 20;

// ---- users ----

// LinkDialog shows a single-use link once, with a copy button.
function LinkDialog({ title, label, link, onClose, children }: { title: string; label: string; link: string; onClose: () => void; children: React.ReactNode }) {
  const [copied, setCopied] = useState(false);
  return (
    <Modal title={title} onClose={onClose}>
      <p className="text-sm text-muted">{children}</p>
      <div className="flex items-center gap-2">
        <input readOnly value={link} aria-label={label} onFocus={(e) => e.target.select()} className={`${inputClass} font-mono text-xs`} />
        <Button
          variant="secondary"
          onClick={() =>
            navigator.clipboard
              .writeText(link)
              .then(() => setCopied(true))
              .catch(() => setCopied(false))
          }
        >
          <Copy size={14} aria-hidden="true" />
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <div className="flex justify-end">
        <Button onClick={onClose}>Done</Button>
      </div>
    </Modal>
  );
}

function InviteDialog({ roles, onClose, onInvited }: { roles: AccountRole[]; onClose: () => void; onInvited: () => void }) {
  const submit = useSubmit();
  const [email, setEmail] = useState("");
  const [fullName, setFullName] = useState("");
  const [roleId, setRoleId] = useState("");
  const [link, setLink] = useState<string | null>(null);

  function save(event: React.FormEvent) {
    event.preventDefault();
    submit.run(async () => {
      const result = await api<{ inviteToken: string }>("/users", { json: { email, fullName, roleId } });
      setLink(`${window.location.origin}/accept-invite?token=${result.inviteToken}`);
      onInvited();
    });
  }

  if (link) {
    return (
      <LinkDialog title="Invitation created" label="Invitation link" link={link} onClose={onClose}>
        Send this link to {fullName}. It works once and expires in 72 hours. It is shown only now, so copy it before closing. If email delivery is set up, the same link has also been
        emailed.
      </LinkDialog>
    );
  }

  return (
    <Modal title="Invite a user" onClose={onClose}>
      <form onSubmit={save} className="flex flex-col gap-4" noValidate>
        <Field label="Full name" error={submit.fields?.fullName}>
          <input value={fullName} onChange={(e) => setFullName(e.target.value)} maxLength={120} className={inputClass} />
        </Field>
        <Field label="Email" error={submit.fields?.email}>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} className={inputClass} />
        </Field>
        <Field label="Role" error={submit.fields?.roleId} hint="The role decides what the person can see and do.">
          <select value={roleId} onChange={(e) => setRoleId(e.target.value)} className={inputClass}>
            <option value="">Choose a role</option>
            {roles.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </Field>
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={submit.busy || !email || !fullName || !roleId}>
            {submit.busy ? "Inviting…" : "Create invitation"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function EditUserDialog({ user, roles, isSelf, onClose, onSaved }: { user: AccountUser; roles: AccountRole[]; isSelf: boolean; onClose: () => void; onSaved: () => void }) {
  const submit = useSubmit();
  const linked = useApi<Employee>(user.employeeId ? `/employees/${user.employeeId}` : null);
  const [fullName, setFullName] = useState(user.fullName);
  const [roleId, setRoleId] = useState(user.roleId);
  // undefined means "not changed".
  const [employee, setEmployee] = useState<{ id: string; label: string } | null | undefined>(undefined);
  const shownEmployee = employee !== undefined ? employee : linked.data ? { id: linked.data.id, label: `${linked.data.fullName} (${linked.data.employeeNumber})` } : null;

  const [resetLink, setResetLink] = useState<string | null>(null);

  const patch = (json: Record<string, unknown>) =>
    submit.run(async () => {
      await api(`/users/${user.id}`, { method: "PATCH", json });
      onSaved();
    });
  const createResetLink = () =>
    submit.run(async () => {
      const result = await api<{ resetToken: string }>(`/users/${user.id}/reset-link`, { json: {} });
      setResetLink(`${window.location.origin}/reset-password?token=${result.resetToken}`);
    });

  if (resetLink) {
    return (
      <LinkDialog title="Password reset link" label="Reset link" link={resetLink} onClose={onClose}>
        Give this link to {user.fullName}. It works once and expires in one hour. It is shown only now. Their current password keeps working until they use it.
      </LinkDialog>
    );
  }

  return (
    <Modal title={`Edit ${user.fullName}`} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          patch({ fullName, ...(roleId !== user.roleId ? { roleId } : {}), ...(employee !== undefined ? { employeeId: employee?.id ?? "" } : {}) });
        }}
        className="flex flex-col gap-4"
        noValidate
      >
        <Field label="Full name" error={submit.fields?.fullName}>
          <input value={fullName} onChange={(e) => setFullName(e.target.value)} maxLength={120} className={inputClass} />
        </Field>
        <Field label="Role" error={submit.fields?.roleId} hint={isSelf ? "You cannot change your own role." : undefined}>
          <select value={roleId} onChange={(e) => setRoleId(e.target.value)} disabled={isSelf} className={inputClass}>
            {roles.map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Employee record" error={submit.fields?.employeeId} hint="Links the account to a person, so they can submit their own resignation and approve as a line manager.">
          <EmployeePicker id="user-employee" value={shownEmployee} onChange={setEmployee} />
        </Field>
        {submit.error && <Notice tone="danger">{submit.error}</Notice>}
        <div className="flex flex-wrap justify-between gap-2">
          {user.status === "active" ? (
            <div className="flex flex-wrap gap-2">
              {!isSelf && (
                <Button variant="danger" disabled={submit.busy} onClick={() => patch({ status: "inactive" })}>
                  Deactivate
                </Button>
              )}
              <Button variant="secondary" disabled={submit.busy} onClick={createResetLink}>
                Create reset link
              </Button>
            </div>
          ) : user.status === "inactive" ? (
            <Button variant="secondary" disabled={submit.busy} onClick={() => patch({ status: "active" })}>
              Reactivate
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button variant="secondary" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={submit.busy}>
              {submit.busy ? "Saving…" : "Save changes"}
            </Button>
          </div>
        </div>
      </form>
    </Modal>
  );
}

function UsersTab({ roles }: { roles: AccountRole[] }) {
  const auth = useAuth();
  const [page, setPage] = useState(1);
  const list = useApi<Page<AccountUser>>(`/users?page=${page}&pageSize=${PAGE_SIZE}`);
  const [dialog, setDialog] = useState<AccountUser | "invite" | null>(null);
  const canManage = auth.can("user.manage");
  const roleName = new Map(roles.map((r) => [r.id, r.name]));
  const me = auth.status === "signedIn" ? auth.user.id : "";

  return (
    <>
      {canManage && (
        <div className="flex justify-end">
          <Button onClick={() => setDialog("invite")}>
            <Plus size={16} strokeWidth={2.6} aria-hidden="true" />
            Invite user
          </Button>
        </div>
      )}
      <Card>
        {list.error && <Notice tone="danger">Users could not be loaded. {list.error.message}</Notice>}
        {list.loading && <Skeleton className="h-56" />}
        {list.data && (
          <div className="-mx-2 overflow-x-auto">
            <table className="w-full min-w-[640px] border-separate border-spacing-y-1.5 text-left text-[13px]">
              <thead>
                <tr className="text-[11px] uppercase tracking-wide text-muted">
                  <th className="px-4 py-1 font-semibold">User</th>
                  <th className="px-4 py-1 font-semibold">Role</th>
                  <th className="px-4 py-1 font-semibold">Last sign-in</th>
                  <th className="px-4 py-1 font-semibold">Status</th>
                  <th className="px-4 py-1" />
                </tr>
              </thead>
              <tbody>
                {list.data.data.map((u) => (
                  <tr key={u.id}>
                    <td className="rounded-l-2xl bg-raised px-4 py-3">
                      <div className="font-semibold">
                        {u.fullName}
                        {u.id === me && <span className="ml-2 text-[11px] font-normal text-muted">(you)</span>}
                      </div>
                      <div className="text-[11px] text-muted">{u.email}</div>
                    </td>
                    <td className="bg-raised px-4 py-3">{roleName.get(u.roleId) ?? "—"}</td>
                    <td className="whitespace-nowrap bg-raised px-4 py-3 tabular-nums text-muted">{u.lastLoginAt ? formatDateTime(u.lastLoginAt) : "Never"}</td>
                    <td className="bg-raised px-4 py-3">
                      <Badge tone={USER_STATUS[u.status].tone}>{USER_STATUS[u.status].label}</Badge>
                    </td>
                    <td className="rounded-r-2xl bg-raised px-4 py-3 text-right">
                      {canManage && (
                        <Button variant="secondary" size="sm" onClick={() => setDialog(u)}>
                          Edit
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {list.data && <Pagination page={page} pageSize={PAGE_SIZE} total={list.data.total} onPage={setPage} />}
      </Card>
      {dialog === "invite" && <InviteDialog roles={roles} onClose={() => setDialog(null)} onInvited={list.reload} />}
      {dialog && dialog !== "invite" && (
        <EditUserDialog
          user={dialog}
          roles={roles}
          isSelf={dialog.id === me}
          onClose={() => setDialog(null)}
          onSaved={() => {
            setDialog(null);
            list.reload();
          }}
        />
      )}
    </>
  );
}

// ---- roles ----

function RoleDialog({ role, permissions, onClose, onSaved }: { role?: AccountRole; permissions: Permission[]; onClose: () => void; onSaved: () => void }) {
  const submit = useSubmit();
  const [name, setName] = useState(role?.name ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [granted, setGranted] = useState(() => new Set(role?.permissions ?? []));
  const groups = [...new Set(permissions.map((p) => p.group))];
  const readOnly = !!role?.locked;

  function save(event: React.FormEvent) {
    event.preventDefault();
    submit.run(async () => {
      const json = { name, description, permissions: [...granted] };
      if (role) await api(`/roles/${role.id}`, { method: "PATCH", json });
      else await api("/roles", { json });
      onSaved();
    });
  }

  return (
    <Modal title={role ? (readOnly ? role.name : `Edit ${role.name}`) : "Add a role"} onClose={onClose} wide>
      <form onSubmit={save} className="flex flex-col gap-5" noValidate>
        {readOnly && <Notice>This role always has every permission and cannot be changed, so an organization can never lock itself out.</Notice>}
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Name" error={submit.fields?.name}>
            <input value={name} onChange={(e) => setName(e.target.value)} disabled={readOnly} maxLength={60} className={inputClass} />
          </Field>
          <Field label="Description" error={submit.fields?.description}>
            <input value={description} onChange={(e) => setDescription(e.target.value)} disabled={readOnly} maxLength={300} className={inputClass} />
          </Field>
        </div>
        {groups.map((group) => (
          <fieldset key={group} className="flex flex-col gap-2">
            <legend className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-muted">{group}</legend>
            {permissions
              .filter((p) => p.group === group)
              .map((p) => (
                <Check
                  key={p.key}
                  label={p.description}
                  disabled={readOnly}
                  checked={granted.has(p.key)}
                  onChange={(on) => {
                    const next = new Set(granted);
                    if (on) next.add(p.key);
                    else next.delete(p.key);
                    setGranted(next);
                  }}
                />
              ))}
          </fieldset>
        ))}
        {submit.error && <Notice tone="danger">{submit.fields?.permissions ? `Not saved: ${submit.fields.permissions}.` : submit.error}</Notice>}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>
            {readOnly ? "Close" : "Cancel"}
          </Button>
          {!readOnly && (
            <Button type="submit" disabled={submit.busy || !name.trim()}>
              {submit.busy ? "Saving…" : role ? "Save changes" : "Add role"}
            </Button>
          )}
        </div>
      </form>
    </Modal>
  );
}

function RolesTab({ roles, reload }: { roles: AccountRole[]; reload: () => void }) {
  const auth = useAuth();
  const permissions = useApi<{ data: Permission[] }>("/permissions");
  const [dialog, setDialog] = useState<AccountRole | "new" | null>(null);
  const canManage = auth.can("role.manage");
  const total = permissions.data?.data.length ?? 0;

  return (
    <>
      {canManage && (
        <div className="flex justify-end">
          <Button onClick={() => setDialog("new")}>
            <Plus size={16} strokeWidth={2.6} aria-hidden="true" />
            Add role
          </Button>
        </div>
      )}
      {roles.length === 0 && (
        <Card>
          <Empty>No roles.</Empty>
        </Card>
      )}
      <div className="grid gap-6 sm:grid-cols-2 xl:grid-cols-3">
        {roles.map((r) => (
          <Card key={r.id}>
            <div className="flex items-start justify-between gap-3">
              <h2 className="display text-xl font-semibold">{r.name}</h2>
              {r.locked ? <Badge tone="lime">Full access</Badge> : r.isSystemDefault ? <Badge>Default</Badge> : <Badge tone="ink">Custom</Badge>}
            </div>
            <p className="flex-1 text-[13px] text-muted">{r.description || "No description."}</p>
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs text-muted">
                {r.permissions.length} of {total} permissions
              </span>
              <Button variant="secondary" size="sm" onClick={() => setDialog(r)}>
                {canManage && !r.locked ? "Edit" : "View"}
              </Button>
            </div>
          </Card>
        ))}
      </div>
      {dialog && permissions.data && (
        <RoleDialog
          role={dialog === "new" ? undefined : canManage ? dialog : { ...dialog, locked: true }}
          permissions={permissions.data.data}
          onClose={() => setDialog(null)}
          onSaved={() => {
            setDialog(null);
            reload();
          }}
        />
      )}
    </>
  );
}

export default function UsersPage() {
  const auth = useAuth();
  const canUsers = auth.can("user.read");
  const canRoles = auth.can("role.read");
  const [tab, setTab] = useState<"users" | "roles">(canUsers ? "users" : "roles");
  const roles = useApi<{ data: AccountRole[] }>(canRoles ? "/roles" : null);
  const errorText = roles.error ? errorMessage(roles.error) : null;

  if (!canUsers && !canRoles) return <Notice tone="danger">You do not have permission to see users or roles.</Notice>;

  return (
    <>
      <PageHeader title="Users and roles">
        {canUsers && (
          <Pill active={tab === "users"} aria-pressed={tab === "users"} onClick={() => setTab("users")}>
            Users
          </Pill>
        )}
        {canRoles && (
          <Pill active={tab === "roles"} aria-pressed={tab === "roles"} onClick={() => setTab("roles")}>
            Roles
          </Pill>
        )}
      </PageHeader>
      {errorText && <Notice tone="danger">Roles could not be loaded. {errorText}</Notice>}
      {tab === "users" && <UsersTab roles={roles.data?.data ?? []} />}
      {tab === "roles" && (roles.data ? <RolesTab roles={roles.data.data} reload={roles.reload} /> : <Skeleton className="h-56 rounded-card" />)}
    </>
  );
}
