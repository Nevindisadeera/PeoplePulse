"use client";

import { useEffect, type ComponentProps } from "react";

function cx(...parts: (string | false | null | undefined)[]) {
  return parts.filter(Boolean).join(" ");
}

export function Logo({ size = 40 }: { size?: number }) {
  return (
    <span
      className="inline-flex shrink-0 items-center justify-center rounded-full bg-ink text-ground"
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      <svg viewBox="0 0 24 24" width={size * 0.6} height={size * 0.6} fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
        <path d="M2 12h5l2.5-6 4 12 2.5-6h6" />
      </svg>
    </span>
  );
}

export function Card({ title, action, className, children }: { title?: string; action?: React.ReactNode; className?: string; children: React.ReactNode }) {
  return (
    <section className={cx("flex min-w-0 flex-col gap-5 rounded-card bg-surface p-6", className)}>
      {(title || action) && (
        <header className="flex items-center justify-between gap-3">
          {title && <h2 className="display text-[15px] font-semibold tracking-wide text-ink">{title}</h2>}
          {action}
        </header>
      )}
      {children}
    </section>
  );
}

// Pill is the rounded control used for navigation, filters and actions.
export function Pill({ active, className, ...props }: ComponentProps<"button"> & { active?: boolean }) {
  return (
    <button
      type="button"
      {...props}
      className={cx(
        "inline-flex h-10 items-center gap-2 rounded-full px-4 text-[13px] font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-50",
        active ? "bg-lime text-ground" : "bg-raised text-ink hover:bg-line",
        className,
      )}
    />
  );
}

export function IconButton({ label, className, ...props }: ComponentProps<"button"> & { label: string }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      {...props}
      className={cx("relative inline-flex size-10 shrink-0 items-center justify-center rounded-full bg-raised text-ink transition-colors hover:bg-line", className)}
    />
  );
}

// Delta is a number with a direction marker: lime up for growth, orange down for departures.
export function Delta({ direction, value, label }: { direction: "up" | "down"; value: string; label: string }) {
  const up = direction === "up";
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <span className={cx("text-[10px] leading-none", up ? "text-lime" : "text-orange")} aria-hidden="true">
        {up ? "▲" : "▼"}
      </span>
      <span className="display text-[2.1rem] font-semibold tabular-nums">{value}</span>
      <span className="truncate text-[11px] font-medium text-muted">{label}</span>
    </div>
  );
}

export function Legend({ items }: { items: { label: string; tone: "lime" | "orange" | "ink" }[] }) {
  const ring = { lime: "border-lime", orange: "border-orange", ink: "border-ink" };
  return (
    <ul className="flex flex-wrap items-center gap-x-5 gap-y-2 text-[11px] font-semibold text-ink">
      {items.map((item) => (
        <li key={item.label} className="flex items-center gap-2">
          <span className={cx("size-2.5 rounded-full border-[3px]", ring[item.tone])} aria-hidden="true" />
          {item.label}
        </li>
      ))}
    </ul>
  );
}

export function Empty({ children }: { children: React.ReactNode }) {
  return <p className="rounded-2xl bg-raised/60 px-4 py-6 text-center text-[13px] text-muted">{children}</p>;
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("animate-pulse rounded-2xl bg-raised", className)} aria-hidden="true" />;
}

export function Notice({ tone = "muted", children }: { tone?: "muted" | "danger"; children: React.ReactNode }) {
  return (
    <p role={tone === "danger" ? "alert" : undefined} className={cx("rounded-2xl px-4 py-3 text-[13px]", tone === "danger" ? "bg-danger/15 text-danger" : "bg-raised text-muted")}>
      {children}
    </p>
  );
}

// ---- form and page building blocks ----

export const inputClass =
  "h-11 w-full rounded-full bg-raised px-4 text-sm text-ink placeholder:text-muted disabled:cursor-not-allowed disabled:opacity-60";

export function Button({ variant = "primary", className, ...props }: ComponentProps<"button"> & { variant?: "primary" | "secondary" | "danger" }) {
  const tone = {
    primary: "bg-lime text-ground hover:opacity-90",
    secondary: "bg-raised text-ink hover:bg-line",
    danger: "bg-orange text-ground hover:opacity-90",
  };
  return (
    <button
      type="button"
      {...props}
      className={cx("inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-full px-5 text-[13px] font-bold transition disabled:cursor-not-allowed disabled:opacity-50", tone[variant], className)}
    />
  );
}

// Field pairs a label with its control and shows the validation message from the API under it.
export function Field({ label, error, hint, children }: { label: string; error?: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="flex min-w-0 flex-col gap-1.5 text-xs font-semibold text-muted">
      {label}
      {children}
      {error ? <span className="font-medium text-danger">{label} {error}.</span> : hint ? <span className="font-normal">{hint}</span> : null}
    </label>
  );
}

const BADGE_TONES = {
  lime: "bg-lime/15 text-lime",
  orange: "bg-orange/15 text-orange",
  muted: "bg-raised text-muted",
  ink: "bg-raised text-ink",
};

export function Badge({ tone = "muted", children }: { tone?: keyof typeof BADGE_TONES; children: React.ReactNode }) {
  return <span className={cx("inline-flex shrink-0 items-center rounded-full px-2.5 py-1 text-[11px] font-bold", BADGE_TONES[tone])}>{children}</span>;
}

export function PageHeader({ title, children }: { title: string; children?: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <h1 className="display text-[2.6rem] font-bold">{title}</h1>
      {children && <div className="flex flex-wrap items-center gap-2">{children}</div>}
    </div>
  );
}

// Modal is a centred dialog. It closes on Escape and on a click outside the panel.
export function Modal({ title, onClose, children, wide }: { title: string; onClose: () => void; children: React.ReactNode; wide?: boolean }) {
  // Listen on the document: focus can leave the dialog (a button that becomes
  // disabled while saving drops it), and Escape must still work then.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-30 flex items-end justify-center overflow-y-auto bg-black/70 p-0 sm:items-center sm:p-6"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className={cx("flex max-h-[92dvh] w-full flex-col gap-5 overflow-y-auto rounded-t-[1.75rem] bg-surface p-6 ring-1 ring-line sm:rounded-card", wide ? "sm:max-w-2xl" : "sm:max-w-lg")}
      >
        <header className="flex items-center justify-between gap-4">
          <h2 className="display text-xl font-semibold">{title}</h2>
          <button type="button" onClick={onClose} aria-label="Close" className="flex size-9 items-center justify-center rounded-full bg-raised text-lg leading-none text-ink hover:bg-line">
            ×
          </button>
        </header>
        {children}
      </div>
    </div>
  );
}

export function Pagination({ page, pageSize, total, onPage }: { page: number; pageSize: number; total: number; onPage: (page: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (total === 0) return null;
  const first = (page - 1) * pageSize + 1;
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted">
      <span>
        {first}–{Math.min(total, page * pageSize)} of {total}
      </span>
      <div className="flex items-center gap-2">
        <Button variant="secondary" disabled={page <= 1} onClick={() => onPage(page - 1)}>
          Previous
        </Button>
        <span className="tabular-nums">
          Page {page} of {pages}
        </span>
        <Button variant="secondary" disabled={page >= pages} onClick={() => onPage(page + 1)}>
          Next
        </Button>
      </div>
    </div>
  );
}
