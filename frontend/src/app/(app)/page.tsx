"use client";

import { useState } from "react";
import { HeadcountSparkline, JoinersLeaversChart, SeparationDots } from "@/components/charts";
import { Card, Delta, Empty, Legend, Notice, Pill, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { formatDate, formatRate, periodStart, rateNote } from "@/lib/format";
import type { ClearanceTask, HeadcountResponse, Page, RetentionResponse, Separation, TurnoverResponse } from "@/lib/types";
import { useApi } from "@/lib/use-api";

const PERIODS = [
  { months: 3, label: "3 months" },
  { months: 6, label: "6 months" },
  { months: 12, label: "12 months" },
];

// Same colours as the separation dots: voluntary lime, involuntary orange, other white.
const CATEGORY_DOT: Record<string, string> = { voluntary: "bg-lime", involuntary: "bg-orange", other: "bg-ink" };

export default function DashboardPage() {
  const auth = useAuth();
  const [months, setMonths] = useState(12);
  const [department, setDepartment] = useState("");
  const canSeeAnalytics = auth.can("analytics.read");

  const query = `from=${periodStart(months)}${department ? `&departmentId=${department}` : ""}`;
  const headcount = useApi<HeadcountResponse>(canSeeAnalytics ? `/analytics/headcount?${query}` : null);
  const turnover = useApi<TurnoverResponse>(canSeeAnalytics ? `/analytics/turnover?${query}` : null);
  const retention = useApi<RetentionResponse>(canSeeAnalytics ? `/analytics/retention?${query}` : null);
  // The department list for the filter comes from an unfiltered request so it does not shrink when one is chosen.
  const departments = useApi<HeadcountResponse>(canSeeAnalytics ? "/analytics/headcount" : null);
  const approvals = useApi<Page<Separation>>(auth.can("separation.approve") ? "/separations?awaitingMe=true&pageSize=5" : null);
  const tasks = useApi<{ data: ClearanceTask[] }>("/me/clearance-tasks");

  const error = headcount.error ?? turnover.error ?? retention.error;
  const noStartDate = turnover.data?.warnings.includes("analytics_start_date_not_set");

  return (
    <>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <h1 className="display text-[2.6rem] font-bold">Dashboard</h1>
        {canSeeAnalytics && (
          <div className="flex flex-wrap items-center gap-2">
            {PERIODS.map((p) => (
              <Pill key={p.months} active={months === p.months} aria-pressed={months === p.months} onClick={() => setMonths(p.months)}>
                {p.label}
              </Pill>
            ))}
            <label className="sr-only" htmlFor="department">
              Department
            </label>
            <select
              id="department"
              value={department}
              onChange={(e) => setDepartment(e.target.value)}
              className="h-10 rounded-full bg-raised px-4 text-[13px] font-semibold text-ink"
            >
              <option value="">All departments</option>
              {departments.data?.byDepartment.map((d) => (
                <option key={d.departmentId} value={d.departmentId}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      {error && <Notice tone="danger">The dashboard figures could not be loaded. {error.message}</Notice>}
      {noStartDate && !error && (
        <Notice>
          No analytics start date is set for your organization, so these figures assume your employee records are complete for the whole period. An
          administrator can set the date in organization settings.
        </Notice>
      )}

      {canSeeAnalytics && (
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]">
          <div className="grid gap-6 sm:grid-cols-2">
            <HeadcountCard data={headcount.data} loading={headcount.loading} />
            <TurnoverCard data={turnover.data} loading={turnover.loading} />
            <Card title="Joiners and leavers" className="sm:col-span-2">
              {headcount.loading && <Skeleton className="h-64" />}
              {headcount.data && (
                <>
                  <JoinersLeaversChart months={headcount.data.monthly} />
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <Legend items={[{ label: "Joiners", tone: "lime" }, { label: "Leavers", tone: "orange" }]} />
                    {headcount.data.monthly.some((m) => m.partial) && <span className="text-[11px] text-muted">* month to date</span>}
                  </div>
                </>
              )}
            </Card>
          </div>
          <DepartmentsCard headcount={headcount.data} turnover={turnover.data} loading={headcount.loading || turnover.loading} />
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        {canSeeAnalytics && <RetentionCard data={retention.data} loading={retention.loading} />}
        {auth.can("separation.approve") && (
          <Card title="Awaiting your approval">
            {approvals.loading && <Skeleton className="h-24" />}
            {approvals.error && <Empty>Approvals could not be loaded.</Empty>}
            {approvals.data?.data.length === 0 && <Empty>Nothing is waiting for your decision.</Empty>}
            <ul className="flex flex-col gap-2">
              {approvals.data?.data.map((s) => (
                <li key={s.id} className="flex items-center justify-between gap-3 rounded-2xl bg-raised px-4 py-3">
                  <div className="min-w-0">
                    <p className="truncate text-[13px] font-semibold">{s.employee?.fullName ?? "Employee"}</p>
                    <p className="text-[11px] text-muted">Last working day {formatDate(s.lastWorkingDay)}</p>
                  </div>
                  <span className="shrink-0 rounded-full bg-lime px-3 py-1 text-[10px] font-bold uppercase text-ground">Resignation</span>
                </li>
              ))}
            </ul>
          </Card>
        )}
        <Card title="Your clearance tasks">
          {tasks.loading && <Skeleton className="h-24" />}
          {tasks.error && <Empty>Clearance tasks could not be loaded.</Empty>}
          {tasks.data?.data.length === 0 && <Empty>You have no open clearance tasks.</Empty>}
          <ul className="flex flex-col gap-2">
            {tasks.data?.data.slice(0, 5).map((t) => (
              <li key={t.item.itemId} className="flex items-center justify-between gap-3 rounded-2xl bg-raised px-4 py-3">
                <div className="min-w-0">
                  <p className="truncate text-[13px] font-semibold">{t.item.title}</p>
                  <p className="truncate text-[11px] text-muted">
                    {t.employee?.fullName} · due {formatDate(t.item.dueDate)}
                  </p>
                </div>
                <span className={`shrink-0 rounded-full px-3 py-1 text-[10px] font-bold uppercase ${t.overdue ? "bg-orange text-ground" : "bg-line text-ink"}`}>
                  {t.overdue ? "Overdue" : t.item.category}
                </span>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </>
  );
}

function HeadcountCard({ data, loading }: { data: HeadcountResponse | null; loading: boolean }) {
  if (loading || !data) return <Card title="Headcount">{loading && <Skeleton className="h-36" />}</Card>;
  const joiners = data.monthly.reduce((sum, m) => sum + m.joiners, 0);
  const leavers = data.monthly.reduce((sum, m) => sum + m.leavers, 0);
  return (
    <Card title="Headcount" action={<span className="display text-2xl font-bold tabular-nums">{data.currentHeadcount}</span>}>
      <div className="grid grid-cols-2 gap-4">
        <Delta direction="up" value={String(joiners)} label="Joined in period" />
        <Delta direction="down" value={String(leavers)} label="Left in period" />
      </div>
      <HeadcountSparkline months={data.monthly} />
    </Card>
  );
}

function TurnoverCard({ data, loading }: { data: TurnoverResponse | null; loading: boolean }) {
  if (loading || !data) return <Card title="Turnover">{loading && <Skeleton className="h-36" />}</Card>;
  const { total, voluntary, involuntary } = data.overall;
  const note = rateNote(total);
  return (
    <Card
      title="Turnover"
      action={
        <span className="display text-2xl font-bold tabular-nums" title={`${total.numerator} separations ÷ ${total.denominator} average headcount`}>
          {formatRate(total)}
        </span>
      }
    >
      <div className="grid grid-cols-2 gap-4">
        <Delta direction="up" value={formatRate(voluntary)} label={`Voluntary · ${voluntary.numerator}`} />
        <Delta direction="down" value={formatRate(involuntary)} label={`Involuntary · ${involuntary.numerator}`} />
      </div>
      <SeparationDots months={data.monthly} />
      <Legend items={[{ label: "Voluntary", tone: "lime" }, { label: "Involuntary", tone: "orange" }, { label: "Other", tone: "ink" }]} />
      <p className="text-[11px] text-muted">
        {total.numerator} separations ÷ {total.denominator} average headcount{note ? `. ${note}.` : ""}
      </p>
    </Card>
  );
}

function RetentionCard({ data, loading }: { data: RetentionResponse | null; loading: boolean }) {
  if (loading || !data) return <Card title="Retention">{loading && <Skeleton className="h-24" />}</Card>;
  const r = data.retention;
  const note = rateNote(r);
  return (
    <Card title="Retention">
      <div className="flex items-end gap-4">
        <span className="display text-6xl font-bold tabular-nums text-lime">{formatRate(r)}</span>
      </div>
      <p className="text-[13px] text-muted">
        {r.numerator} of the {r.denominator} people employed on {formatDate(data.from)} were still employed on {formatDate(data.to)}.
      </p>
      {note && <p className="text-[11px] text-muted">{note}.</p>}
    </Card>
  );
}

function DepartmentsCard({ headcount, turnover, loading }: { headcount: HeadcountResponse | null; turnover: TurnoverResponse | null; loading: boolean }) {
  if (loading || !headcount || !turnover) return <Card title="Departments">{loading && <Skeleton className="h-80" />}</Card>;
  const rates = new Map(turnover.byDepartment.map((d) => [d.departmentId, d.turnover.total]));
  const rows = [...headcount.byDepartment].sort((a, b) => b.headcount - a.headcount);
  const largest = Math.max(1, ...rows.map((d) => d.headcount));
  return (
    <Card title="Departments">
      {rows.length === 0 && <Empty>No departments yet.</Empty>}
      <ul className="flex flex-1 flex-col gap-4">
        {rows.map((d) => {
          const rate = rates.get(d.departmentId);
          const hasLeavers = (rate?.numerator ?? 0) > 0;
          return (
            <li key={d.departmentId} className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1.5">
              <span className="truncate text-[13px] font-semibold">{d.name}</span>
              <span className={`text-right text-[11px] font-semibold tabular-nums ${hasLeavers ? "text-orange" : "text-muted"}`} title={rate ? `${rate.numerator} separations ÷ ${rate.denominator} average headcount` : undefined}>
                {formatRate(rate)} turnover
              </span>
              <div className="col-span-2 h-7 rounded-full bg-raised">
                <div
                  className="flex h-7 min-w-9 items-center justify-end rounded-full bg-lime px-3 text-[11px] font-bold tabular-nums text-ground"
                  style={{ width: `${(d.headcount / largest) * 100}%` }}
                >
                  {d.headcount}
                </div>
              </div>
            </li>
          );
        })}
      </ul>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Legend items={[{ label: "Headcount today", tone: "lime" }]} />
        <span className="text-[11px] text-muted">
          Total: <span className="font-bold text-ink">{headcount.currentHeadcount}</span>
        </span>
      </div>
      <div className="flex flex-col gap-3 border-t border-line pt-5">
        <h3 className="display text-[13px] font-semibold tracking-wide text-muted">Reasons for leaving</h3>
        {turnover.byReason.length === 0 && <Empty>Nobody left in this period.</Empty>}
        <ul className="flex flex-col gap-2">
          {turnover.byReason.slice(0, 5).map((reason) => (
            <li key={reason.code} className="flex items-center gap-3 text-[13px]">
              <span className={`size-2.5 shrink-0 rounded-full ${CATEGORY_DOT[reason.category] ?? "bg-ink"}`} aria-hidden="true" />
              <span className="min-w-0 flex-1 truncate">{reason.name}</span>
              <span className="text-[11px] capitalize text-muted">{reason.category}</span>
              <span className="w-6 text-right font-bold tabular-nums">{reason.count}</span>
            </li>
          ))}
        </ul>
      </div>
    </Card>
  );
}
