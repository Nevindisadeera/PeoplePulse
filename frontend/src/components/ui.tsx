import type { ComponentProps } from "react";

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
