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
