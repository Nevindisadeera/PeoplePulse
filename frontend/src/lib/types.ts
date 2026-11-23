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

export type DepartmentAssignment = { departmentId: string; from: string; to?: string };

export type Employee = {
  id: string;
  employeeNumber: string;
  fullName: string;
  workEmail?: string;
  phone?: string;
  departmentId: string;
  departmentHistory: DepartmentAssignment[];
  jobTitle?: string;
  employmentType: string;
  managerId?: string;
  joinDate: string;
  endDate?: string;
  status: "active" | "serving_notice" | "separated";
  version: number;
};

export type Department = {
  id: string;
  name: string;
  code: string;
  parentId?: string;
  headEmployeeId?: string;
  status: "active" | "archived";
  version: number;
  headcount: number;
};

export type EmploymentType = { code: string; name: string; countsInHeadcount: boolean; active: boolean };

export type Organization = {
  id: string;
  name: string;
  timeZone: string;
  settings: { employmentTypes: EmploymentType[]; noticePeriodDays: number; analyticsStartDate: string | null; smallGroupThreshold: number };
};

export type TimelineEvent = { date: string; type: "joined" | "transferred" | "separated"; departmentName?: string; fromDepartmentName?: string };

export type ImportReport = {
  dryRun: boolean;
  totalRows: number;
  validRows: number;
  imported: number;
  errors: { row: number; field: string; message: string }[];
};

export type SeparationReason = { code: string; name: string; category: "voluntary" | "involuntary" | "other"; active: boolean };

export type ApprovalStep = {
  order: number;
  approverType: "line_manager" | "role" | "user";
  roleId?: string;
  userId?: string;
  fallback?: boolean;
  decision: "pending" | "approved" | "rejected";
  decidedAt?: string;
  comment?: string;
};

export type ClearanceSummary = { id: string; status: string; totalItems: number; openItems: number; overridden: boolean };

export type SeparationDetail = Separation & {
  employeeId: string;
  reasonCode: string;
  category: string;
  reasonDetails?: string;
  requestedLastWorkingDay: string;
  approvalSteps: ApprovalStep[];
  statusHistory: { status: string; at: string; note?: string }[];
  clearance?: ClearanceSummary;
  tenureDaysAtExit?: number;
};

export type ClearanceItem = {
  itemId: string;
  category: string;
  title: string;
  assigneeType: "line_manager" | "role" | "user";
  roleId?: string;
  userId?: string;
  fallback?: boolean;
  status: "pending" | "cleared" | "not_applicable" | "blocked";
  remarks?: string;
  dueDate: string;
};

export type Clearance = {
  id: string;
  separationId: string;
  employeeId: string;
  status: "in_progress" | "completed" | "cancelled";
  items: ClearanceItem[];
  override?: { at: string; reason: string };
  version: number;
  employee?: EmployeeBrief;
  lastWorkingDay?: string;
  totalItems: number;
  openItems: number;
};

export type SeparationSettings = { separationReasons: SeparationReason[]; noticePeriodDays: number };

export type Assignee = { approverType?: string; assigneeType?: string; roleId?: string; userId?: string };

export type ChainStep = { approverType: "line_manager" | "role" | "user"; roleId?: string; userId?: string };

export type ClearanceTemplateItem = { category: string; title: string; assigneeType: "line_manager" | "role" | "user"; roleId?: string; userId?: string; active: boolean };

export type InterviewQuestion = { id: string; text: string; type: "rating" | "choice" | "text"; options?: string[]; required: boolean; active: boolean };

export type InterviewTemplate = { enabled: boolean; employeeSelfService: boolean; minAggregateResponses: number; questions: InterviewQuestion[] };

// The organization with every settings section, as returned by GET /organization.
export type FullOrganization = {
  id: string;
  name: string;
  timeZone: string;
  settings: {
    employmentTypes: EmploymentType[];
    noticePeriodDays: number;
    separationReasons: SeparationReason[];
    approvalChain: ChainStep[];
    clearanceTemplate: ClearanceTemplateItem[];
    analyticsStartDate: string | null;
    smallGroupThreshold: number;
  };
};

// A user account as listed for administrators.
export type AccountUser = {
  id: string;
  email: string;
  fullName: string;
  roleId: string;
  employeeId?: string;
  status: "invited" | "active" | "inactive";
  lastLoginAt?: string;
};

export type AccountRole = { id: string; name: string; description: string; permissions: string[]; isSystemDefault: boolean; locked: boolean };

export type Permission = { key: string; group: string; description: string };

export type Report = {
  type: string;
  from: string;
  to: string;
  generatedAt: string;
  report: { title: string; columns: string[]; rows: string[][]; notes: string[] };
};

export type AuditEntry = {
  id: string;
  actorName?: string;
  actorType: string;
  action: string;
  entityType: string;
  entityId: string;
  changes?: Record<string, unknown>;
  ip: string;
  at: string;
};
