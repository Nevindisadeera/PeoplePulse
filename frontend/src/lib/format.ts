import type { Rate } from "./types";

// formatRate shows a percentage, or "n/a" when the API withheld the value.
export function formatRate(rate: Rate | undefined): string {
  if (!rate || rate.value === null) return "n/a";
  return `${rate.value.toFixed(1)}%`;
}

// rateNote explains a missing or flagged rate in plain words.
export function rateNote(rate: Rate | undefined): string | null {
  if (!rate) return null;
  if (rate.flags.includes("no_headcount")) return "Nobody was employed in this period";
  if (rate.flags.includes("before_analytics_start")) return "Period starts before your data is complete";
  if (rate.flags.includes("small_group")) return "Small group: read the counts, not just the rate";
  return null;
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// "2026-03" -> "Mar"
export function monthLabel(period: string): string {
  return MONTHS[Number(period.slice(5, 7)) - 1] ?? period;
}

// "2026-03-10" -> "10 Mar 2026"
export function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  const [y, m, d] = iso.slice(0, 10).split("-").map(Number);
  return `${d} ${MONTHS[m - 1]} ${y}`;
}

// First day of the month `months - 1` months before today, as YYYY-MM-DD.
export function periodStart(months: number, today = new Date()): string {
  const d = new Date(Date.UTC(today.getFullYear(), today.getMonth() - (months - 1), 1));
  return d.toISOString().slice(0, 10);
}

export function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]!.toUpperCase())
    .join("");
}

export const EMPLOYEE_STATUS: Record<string, { label: string; tone: "lime" | "orange" | "muted" }> = {
  active: { label: "Active", tone: "lime" },
  serving_notice: { label: "Serving notice", tone: "orange" },
  separated: { label: "Left", tone: "muted" },
};

// "serving_notice" -> "Serving notice"
export function humanize(code: string): string {
  const text = code.replace(/_/g, " ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}

// Today's date as YYYY-MM-DD in the browser's time zone, for date inputs.
export function todayISO(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

type Tone = "lime" | "orange" | "muted" | "ink";

export const SEPARATION_STATUS: Record<string, { label: string; tone: Tone }> = {
  pending_approval: { label: "Awaiting approval", tone: "orange" },
  approved: { label: "Approved · serving notice", tone: "lime" },
  completed: { label: "Completed", tone: "muted" },
  rejected: { label: "Not approved", tone: "muted" },
  withdrawn: { label: "Withdrawn", tone: "muted" },
};

export const SEPARATION_TYPES: Record<string, string> = {
  resignation: "Resignation",
  termination: "Termination",
  redundancy: "Redundancy",
  retirement: "Retirement",
  end_of_contract: "End of contract",
  other: "Other",
};

export const ITEM_STATUS: Record<string, { label: string; tone: Tone }> = {
  pending: { label: "To do", tone: "ink" },
  cleared: { label: "Cleared", tone: "lime" },
  not_applicable: { label: "Not applicable", tone: "muted" },
  blocked: { label: "Blocked", tone: "orange" },
};

// "2026-11-18T07:19:02Z" -> "18 Nov 2026, 12:49" in the browser's time zone.
export function formatDateTime(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  return `${d.getDate()} ${["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"][d.getMonth()]} ${d.getFullYear()}, ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

// addDays("2026-11-18", 30) -> "2026-12-18"
export function addDays(iso: string, days: number): string {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d + days)).toISOString().slice(0, 10);
}
