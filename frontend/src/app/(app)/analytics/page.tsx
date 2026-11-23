"use client";

import { useState } from "react";
import { TurnoverTrend } from "@/components/charts";
import { Card, Empty, Legend, Notice, PageHeader, Pill, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { formatDate, formatRate, humanize, periodStart } from "@/lib/format";
import type { DataQuality, HeadcountResponse, Rate, RetentionDetail, TenureResponse, TenureStats, TurnoverResponse } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const PERIODS = [
  { months: 6, label: "6 months" },
  { months: 12, label: "12 months" },
  { months: 24, label: "24 months" },
];

const FLAG_TEXT: Record<string, string> = {
  no_headcount: "nobody employed",
  before_analytics_start: "before data is complete",
  small_group: "small group",
  partial_period: "month to date",
};
const BAND_LABEL: Record<string, string> = {
  under_6_months: "Under 6 months",
  "6_to_12_months": "6 to 12 months",
  "1_to_2_years": "1 to 2 years",
  "2_to_5_years": "2 to 5 years",
  over_5_years: "Over 5 years",
};
const WARNING_TEXT: Record<string, string> = {
  analytics_start_date_not_set: "No analytics start date is set, so these figures assume your records are complete for the whole period.",
  period_starts_before_analytics_start_date: "This period starts before your analytics start date, so its rates are withheld. Counts are still shown.",
  period_ends_in_future_clipped_to_today: "The period was cut at today.",
};

// RateCell shows a rate with the counts behind it and any caveat.
function RateCell({ rate, of }: { rate: Rate; of: string }) {
  const flags = rate.flags.map((f) => FLAG_TEXT[f] ?? humanize(f));
  return (
    <div>
      <span className="font-bold tabular-nums">{formatRate(rate)}</span>
      <span className="ml-2 text-[11px] tabular-nums text-muted">
        {rate.numerator} of {rate.denominator} {of}
      </span>
      {flags.length > 0 && <div className="text-[11px] text-orange">{flags.join(" · ")}</div>}
    </div>
  );
}

function Bars({ rows, tone = "lime" }: { rows: [string, number][]; tone?: "lime" | "orange" }) {
  const largest = Math.max(1, ...rows.map(([, n]) => n));
  return (
    <ul className="flex flex-col gap-2">
      {rows.map(([label, n]) => (
        <li key={label} className="grid grid-cols-[9rem_minmax(0,1fr)_2rem] items-center gap-3 text-xs">
          <span className="truncate font-semibold">{label}</span>
          <div className="h-5 rounded-full bg-raised">{n > 0 && <div className={`h-5 rounded-full ${tone === "lime" ? "bg-lime" : "bg-orange"}`} style={{ width: `${(n / largest) * 100}%` }} />}</div>
          <span className="text-right font-bold tabular-nums">{n}</span>
        </li>
      ))}
    </ul>
  );
}

function TenureCard({ title, stats, bands, tone, empty }: { title: string; stats: TenureStats; bands: string[]; tone: "lime" | "orange"; empty: string }) {
  return (
    <Card title={title} action={<span className="text-xs text-muted">{stats.count} people</span>}>
      {stats.count === 0 ? (
        <Empty>{empty}</Empty>
      ) : (
        <>
          <div className="flex flex-wrap gap-x-10 gap-y-3">
            <div>
              <p className="display text-4xl font-bold tabular-nums">{stats.meanYears?.toFixed(1)}</p>
              <p className="text-[11px] text-muted">years on average</p>
            </div>
            <div>
              <p className="display text-4xl font-bold tabular-nums">{stats.medianYears?.toFixed(1)}</p>
              <p className="text-[11px] text-muted">years, the middle person</p>
            </div>
          </div>
          <Bars rows={bands.map((b) => [BAND_LABEL[b] ?? humanize(b), stats.bands[b] ?? 0])} tone={tone} />
        </>
      )}
    </Card>
  );
}

export default function AnalyticsPage() {
  const auth = useAuth();
  const [months, setMonths] = useState(12);
  const [department, setDepartment] = useState("");
  const query = `from=${periodStart(months)}${department ? `&departmentId=${department}` : ""}`;
  const turnover = useApi<TurnoverResponse>(`/analytics/turnover?${query}`);
  const retention = useApi<RetentionDetail>(`/analytics/retention?${query}`);
  const tenure = useApi<TenureResponse>(`/analytics/tenure?${query}`);
  const departments = useApi<HeadcountResponse>("/analytics/headcount");
  const quality = useApi<DataQuality>(auth.can("employee.manage") ? "/analytics/data-quality" : null);

  const t = turnover.data;
  const error = turnover.error ?? retention.error ?? tenure.error;

  return (
    <>
      <PageHeader title="Analytics">
        {PERIODS.map((p) => (
          <Pill key={p.months} active={months === p.months} aria-pressed={months === p.months} onClick={() => setMonths(p.months)}>
            {p.label}
          </Pill>
        ))}
        <select value={department} onChange={(e) => setDepartment(e.target.value)} aria-label="Department" className="h-10 rounded-full bg-raised px-4 text-[13px] font-semibold text-ink">
          <option value="">All departments</option>
          {departments.data?.byDepartment.map((d) => (
            <option key={d.departmentId} value={d.departmentId}>
              {d.name}
            </option>
          ))}
        </select>
      </PageHeader>

      {error && <Notice tone="danger">The figures could not be loaded. {error.message}</Notice>}
      {t?.warnings.map((w) => <Notice key={w}>{WARNING_TEXT[w] ?? humanize(w)}</Notice>)}
      {t?.scoped && !department && <Notice>You are seeing the departments you head, not the whole organization.</Notice>}

      <div className="grid gap-6 lg:grid-cols-3">
        {(
          [
            ["Turnover in period", t?.overall.total],
            ["Voluntary", t?.overall.voluntary],
            ["Involuntary", t?.overall.involuntary],
          ] as [string, Rate | undefined][]
        ).map(([title, rate]) => (
          <Card key={title} title={title}>
            {!rate ? (
              <Skeleton className="h-16" />
            ) : (
              <>
                <p className={`display text-5xl font-bold tabular-nums ${title === "Involuntary" ? "text-orange" : title === "Voluntary" ? "text-lime" : ""}`}>{formatRate(rate)}</p>
                <p className="text-xs text-muted">
                  {rate.numerator} separations ÷ {rate.denominator} average headcount
                  {rate.flags.length > 0 && <span className="text-orange"> · {rate.flags.map((f) => FLAG_TEXT[f] ?? f).join(" · ")}</span>}
                </p>
              </>
            )}
          </Card>
        ))}
      </div>

      <Card title="Monthly turnover rate">
        {turnover.loading && <Skeleton className="h-72" />}
        {t && (
          <>
            <TurnoverTrend months={t.monthly} />
            <div className="flex flex-wrap items-center justify-between gap-3">
              <Legend items={[{ label: "Total", tone: "ink" }, { label: "Voluntary", tone: "lime" }, { label: "Involuntary", tone: "orange" }]} />
              <span className="text-[11px] text-muted">A gap means the rate is not stated for that month. * month to date.</span>
            </div>
          </>
        )}
      </Card>

      <Card title="Departments" action={t && <span className="text-xs text-muted">{formatDate(t.from)} to {formatDate(t.to)}</span>}>
        {(turnover.loading || retention.loading) && <Skeleton className="h-48" />}
        {t && retention.data && t.byDepartment.length === 0 && <Empty>No departments had staff in this period.</Empty>}
        {t && retention.data && t.byDepartment.length > 0 && (
          <div className="-mx-2 overflow-x-auto">
            <table className="w-full min-w-[680px] border-separate border-spacing-y-1.5 text-left text-[13px]">
              <thead>
                <tr className="text-[11px] uppercase tracking-wide text-muted">
                  <th className="px-4 py-1 font-semibold">Department</th>
                  <th className="px-4 py-1 font-semibold">Turnover</th>
                  <th className="px-4 py-1 font-semibold">Retention</th>
                  <th className="px-4 py-1 font-semibold">Transfers</th>
                </tr>
              </thead>
              <tbody>
                {t.byDepartment.map((d) => {
                  const kept = retention.data!.byDepartment.find((r) => r.departmentId === d.departmentId)?.retention;
                  return (
                    <tr key={d.departmentId}>
                      <td className="rounded-l-2xl bg-raised px-4 py-3 font-semibold">
                        {d.name}
                        {d.archived && <span className="ml-2 text-[11px] font-normal text-muted">archived</span>}
                      </td>
                      <td className="bg-raised px-4 py-3">
                        <RateCell rate={d.turnover.total} of="avg" />
                      </td>
                      <td className="bg-raised px-4 py-3">{kept ? <RateCell rate={kept} of="at start" /> : <span className="text-muted">nobody at the start</span>}</td>
                      <td className="rounded-r-2xl bg-raised px-4 py-3 tabular-nums text-muted">
                        {d.transfersIn} in · {d.transfersOut} out
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        <p className="text-[11px] text-muted">Transfers between departments are shown beside turnover and are never counted as turnover. Retention is not 100 minus turnover: it follows the people who were there on the first day.</p>
      </Card>

      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="Reasons for leaving" action={t && <span className="text-xs text-muted">{t.separationsCount} separations</span>}>
          {t && t.byReason.length === 0 && <Empty>Nobody left in this period.</Empty>}
          {t && t.byReason.length > 0 && <Bars rows={t.byReason.map((r) => [r.name, r.count])} tone="orange" />}
        </Card>
        <Card title="Overall retention">
          {retention.data && (
            <>
              <p className="display text-5xl font-bold tabular-nums text-lime">{formatRate(retention.data.retention)}</p>
              <p className="text-sm text-muted">
                {retention.data.retention.numerator} of the {retention.data.retention.denominator} people employed on {formatDate(retention.data.from)} were still employed on{" "}
                {formatDate(retention.data.to)}. People hired during the period are not part of this figure.
              </p>
            </>
          )}
        </Card>
      </div>

      {tenure.data && (
        <div className="grid gap-6 lg:grid-cols-2">
          <TenureCard title="Length of service: current staff" stats={tenure.data.currentStaff} bands={tenure.data.bands} tone="lime" empty="Nobody is employed on this date." />
          <TenureCard title="Length of service: leavers" stats={tenure.data.leavers} bands={tenure.data.bands} tone="orange" empty="Nobody left in this period." />
        </div>
      )}

      {quality.data && (
        <Card title="Data quality" action={<span className="text-xs text-muted">{quality.data.employeeRecords} employee records checked</span>}>
          {!quality.data.analyticsStartDateSet && <Notice>Set the analytics start date in organization settings once you have checked that former employees are recorded too.</Notice>}
          {quality.data.issues.length === 0 ? (
            <Empty>No problems found in the employee records.</Empty>
          ) : (
            <ul className="flex flex-col gap-2">
              {quality.data.issues.map((issue) => (
                <li key={issue.code} className="rounded-2xl bg-raised px-4 py-3 text-[13px]">
                  <p className="font-semibold">
                    {issue.count} record{issue.count === 1 ? "" : "s"}: {issue.description}
                  </p>
                  <p className="text-xs text-muted">For example {issue.examples.join(", ")}</p>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}
    </>
  );
}
