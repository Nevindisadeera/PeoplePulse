// Shapes of the API responses used by the frontend.

export type User = {
  id: string;
  email: string;
  fullName: string;
  roleId: string;
  employeeId?: string;
  status: string;
};

export type Role = { id: string; name: string; permissions: string[] };

export type Session = { accessToken: string; expiresAt: string; user: User; role: Role };

// A percentage with the counts behind it. value is null when the rate cannot
// or should not be stated; flags says why.
export type Rate = { value: number | null; numerator: number; denominator: number; flags: string[] };

export type Turnover = { averageHeadcount: number; total: Rate; voluntary: Rate; involuntary: Rate; other: Rate };

export type Month = {
  period: string; // YYYY-MM
  from: string;
  to: string;
  openingHeadcount: number;
  joiners: number;
  leavers: number;
  closingHeadcount: number;
  netChange: number;
  turnover: Turnover;
  partial: boolean;
};

type AnalyticsEnvelope = {
  from: string;
  to: string;
  analyticsStartDate: string | null;
  smallGroupThreshold: number;
  scoped: boolean;
  warnings: string[];
};

export type TurnoverResponse = AnalyticsEnvelope & {
  overall: Turnover;
  rolling12Months: { from: string; to: string; turnover: Turnover };
  monthly: Month[];
  byDepartment: {
    departmentId: string;
    name: string;
    code: string;
    archived: boolean;
    turnover: Turnover;
    transfersIn: number;
    transfersOut: number;
  }[];
  byReason: { code: string; name: string; category: string; count: number }[];
  byTenureBand: Record<string, number>;
  separationsCount: number;
};

export type HeadcountResponse = AnalyticsEnvelope & {
  currentHeadcount: number;
  asOf: string;
  monthly: Month[];
  byDepartment: { departmentId: string; name: string; code: string; headcount: number }[];
};

export type RetentionResponse = AnalyticsEnvelope & { retention: Rate };

export type EmployeeBrief = { id: string; employeeNumber: string; fullName: string; departmentId: string; status: string };

export type Separation = {
  id: string;
  type: string;
  status: string;
  submissionDate: string;
  lastWorkingDay: string;
  employee?: EmployeeBrief;
  canAct: boolean;
  version: number;
};

export type ClearanceTask = {
  clearanceId: string;
  employee?: EmployeeBrief;
  lastWorkingDay?: string;
  overdue: boolean;
  item: { itemId: string; category: string; title: string; status: string; dueDate: string };
};

export type Page<T> = { data: T[]; page: number; pageSize: number; total: number };

export type NotificationList = Page<{ id: string; title: string; body: string; link: string; createdAt: string; readAt?: string }> & {
  unreadCount: number;
};
