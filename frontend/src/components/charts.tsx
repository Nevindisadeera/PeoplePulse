"use client";

import { Bar, BarChart, CartesianGrid, LabelList, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { monthLabel } from "@/lib/format";
import type { Month } from "@/lib/types";

const LIME = "#b9f15c";
const ORANGE = "#f7941d";
const INK = "#f4f4f2";
const MUTED = "#9b9b96";
const LINE = "#333333";

const tooltipStyle = {
  background: "#272727",
  border: "none",
  borderRadius: 14,
  color: INK,
  fontSize: 12,
  padding: "8px 12px",
};

// Sparkline shows month-end headcount as a single unlabelled line.
export function HeadcountSparkline({ months }: { months: Month[] }) {
  const data = months.map((m) => ({ month: monthLabel(m.period), headcount: m.closingHeadcount }));
  return (
    <div className="h-20" role="img" aria-label="Month-end headcount trend">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 6, right: 4, bottom: 4, left: 4 }}>
          <YAxis hide domain={["dataMin - 1", "dataMax + 1"]} />
          <Tooltip contentStyle={tooltipStyle} cursor={{ stroke: LINE }} labelFormatter={(_, p) => p[0]?.payload.month ?? ""} />
          <Line type="monotone" dataKey="headcount" name="Headcount" stroke={LIME} strokeWidth={2} dot={false} activeDot={{ r: 4, fill: LIME }} isAnimationActive={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

// DotMatrix draws one column per month and one dot per separation, coloured
// by category, so small counts stay readable as counts.
export function SeparationDots({ months }: { months: Month[] }) {
  const tallest = Math.max(4, ...months.map((m) => m.turnover.total.numerator));
  const step = 11;
  const width = months.length * step;
  const height = tallest * step;
  return (
    <svg viewBox={`0 0 ${width} ${height}`} className="h-20 w-full" preserveAspectRatio="xMidYMax meet" role="img" aria-label="Separations per month, one dot each">
      {months.map((m, col) => {
        const dots = [
          ...Array(m.turnover.voluntary.numerator).fill(LIME),
          ...Array(m.turnover.involuntary.numerator).fill(ORANGE),
          ...Array(m.turnover.other.numerator).fill(INK),
        ] as string[];
        const cx = col * step + step / 2;
        if (dots.length === 0) {
          return <circle key={m.period} cx={cx} cy={height - step / 2} r={1.6} fill={LINE} />;
        }
        return dots.map((fill, row) => <circle key={`${m.period}-${row}`} cx={cx} cy={height - row * step - step / 2} r={3.4} fill={fill} />);
      })}
    </svg>
  );
}

// JoinersLeavers shows monthly joiners and leavers as rounded bars with their counts.
export function JoinersLeaversChart({ months }: { months: Month[] }) {
  const data = months.map((m) => ({ month: monthLabel(m.period) + (m.partial ? "*" : ""), Joiners: m.joiners, Leavers: m.leavers }));
  const label = { fill: "#0c0c0c", fontSize: 10, fontWeight: 700 };
  return (
    <div className="h-64" role="img" aria-label="Joiners and leavers per month">
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data} margin={{ top: 8, right: 4, bottom: 0, left: 4 }} barGap={4} barCategoryGap="22%">
          <CartesianGrid vertical={false} stroke={LINE} strokeDasharray="2 6" />
          <XAxis dataKey="month" tickLine={false} axisLine={false} tick={{ fill: MUTED, fontSize: 11, fontWeight: 600 }} />
          <YAxis hide allowDecimals={false} domain={[0, "dataMax"]} />
          <Tooltip contentStyle={tooltipStyle} cursor={{ fill: "rgba(255,255,255,0.04)" }} />
          <Bar dataKey="Joiners" fill={LIME} radius={12} maxBarSize={26} minPointSize={3} isAnimationActive={false}>
            <LabelList dataKey="Joiners" position="insideTop" offset={8} formatter={(v) => (Number(v) > 0 ? String(v) : "")} {...label} />
          </Bar>
          <Bar dataKey="Leavers" fill={ORANGE} radius={12} maxBarSize={26} minPointSize={3} isAnimationActive={false}>
            <LabelList dataKey="Leavers" position="insideTop" offset={8} formatter={(v) => (Number(v) > 0 ? String(v) : "")} {...label} />
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

// TurnoverTrend plots the monthly rates. A month whose rate was withheld is a
// gap in the line, never a zero.
export function TurnoverTrend({ months }: { months: Month[] }) {
  const data = months.map((m) => ({
    month: monthLabel(m.period) + (m.partial ? "*" : ""),
    Total: m.turnover.total.value,
    Voluntary: m.turnover.voluntary.value,
    Involuntary: m.turnover.involuntary.value,
  }));
  return (
    <div className="h-72" role="img" aria-label="Monthly turnover rate">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid vertical={false} stroke={LINE} strokeDasharray="2 6" />
          <XAxis dataKey="month" tickLine={false} axisLine={false} tick={{ fill: MUTED, fontSize: 11, fontWeight: 600 }} />
          <YAxis width={44} tickLine={false} axisLine={false} tick={{ fill: MUTED, fontSize: 11 }} tickFormatter={(v) => `${v}%`} />
          <Tooltip contentStyle={tooltipStyle} cursor={{ stroke: LINE }} formatter={(v) => (v === null || v === undefined ? "n/a" : `${Number(v).toFixed(1)}%`)} />
          <Line type="monotone" dataKey="Total" stroke={INK} strokeWidth={2.5} dot={{ r: 3, fill: INK }} connectNulls={false} isAnimationActive={false} />
          <Line type="monotone" dataKey="Voluntary" stroke={LIME} strokeWidth={2} dot={false} connectNulls={false} isAnimationActive={false} />
          <Line type="monotone" dataKey="Involuntary" stroke={ORANGE} strokeWidth={2} dot={false} connectNulls={false} isAnimationActive={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
