# PeoplePulse

Employee Turnover & Retention Management Platform. A reusable, organization-independent web application for managing employee records, resignations, exit clearance and exit feedback, with turnover and retention analytics.

**Status:** in development. The backend covers authentication, users, roles, departments and employees; the remaining modules are being built in phases.

## Tech stack

| Layer | Technology |
| --- | --- |
| Frontend | Next.js, TypeScript, Tailwind CSS, Recharts |
| Backend | Go, REST API |
| Database | MongoDB (official Go driver) |
| Automation | n8n |
| Security | JWT, role-based access control, Argon2id password hashing, audit logging |

## Repository layout

```
backend/                    Go REST API
frontend/                   Next.js web application
scripts/seed-demo.mjs       Fills an empty organization with demo data
PeoplePulse-Proposal.pdf    Project proposal (requirements, architecture, roadmap)
PeoplePulse-Proposal.html   Same proposal as a web page
```

## Running the backend

Requirements: Go 1.25+ and a running MongoDB instance. A replica set is recommended: on a standalone MongoDB the API still works, but changes that touch several records are not transactional.

```bash
cd backend
cp .env.example .env        # then set JWT_SECRET to 32+ random characters
```

Create an organization and its first admin (run once):

```bash
# PowerShell: $env:BOOTSTRAP_PASSWORD = "a-long-password"
export BOOTSTRAP_PASSWORD="a-long-password"
go run ./cmd/bootstrap -org "My Company" -email admin@example.com -name "Admin User" -tz Asia/Colombo
```

Start the API:

```bash
go run ./cmd/api
```

The API listens on `http://localhost:8080`. Health check: `GET /healthz`.

## API (so far)

All paths are under `/api/v1`.

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/auth/login` | Sign in; returns an access token and sets a refresh cookie |
| POST | `/auth/refresh` | Rotate the refresh token and get a new access token |
| POST | `/auth/logout` | End the session |
| POST | `/auth/accept-invite` | Set a password from an invitation token |
| GET | `/me` | Current user and role |
| GET, POST | `/users` | List users, invite a user |
| PATCH | `/users/{id}` | Change name, role or status |
| GET, POST | `/roles` | List roles, create a role |
| PATCH | `/roles/{id}` | Edit a role |
| GET | `/permissions` | Permission catalogue |
| GET | `/notifications` | My notifications with unread count (`?unread=true` for unread only) |
| POST | `/notifications/{id}/read`, `/notifications/read-all` | Mark notifications as read |
| GET, PATCH | `/organization` | Organization profile and settings |
| PUT | `/settings/employment-types` | Replace the employment type list |
| GET, POST | `/departments` | List departments with headcount, create a department |
| GET, PATCH | `/departments/{id}` | View or edit a department |
| POST | `/departments/{id}/archive`, `/restore` | Archive or restore a department |
| GET, POST | `/employees` | List and search employees, create an employee |
| GET, PATCH | `/employees/{id}` | View or edit an employee |
| POST | `/employees/{id}/transfers` | Move an employee to another department from a date |
| GET | `/employees/{id}/timeline` | Joining, transfer and separation events |
| POST | `/employees/import` | Import employees from CSV (`?dryRun=true` to validate only) |
| PUT | `/settings/separation-reasons` | Replace the separation reason list (each reason is voluntary, involuntary or other) |
| PUT | `/settings/approval-chain` | Set the resignation approval chain (0 to 3 steps: line manager, role or user) |
| GET, POST | `/separations` | List separations (`?awaitingMe=true` for my approvals), submit a resignation or record another separation |
| GET | `/separations/{id}` | View a separation with its approval steps and history |
| POST | `/separations/{id}/approve`, `/reject` | Decide the pending approval step |
| POST | `/separations/{id}/withdraw` | Withdraw a pending request, or cancel an approved one (HR) |
| PATCH | `/separations/{id}/last-working-day` | Change the last working day, with a reason |
| POST | `/separations/{id}/complete` | Close the employment period on or after the last working day |
| PUT | `/settings/clearance-template` | Replace the exit clearance checklist template |
| GET | `/clearances` | List clearance checklists |
| GET | `/clearances/{id}` | View a checklist (HR, the departing employee, or an assignee) |
| PATCH | `/clearances/{id}/items/{itemId}` | Set an item to pending, cleared, not applicable or blocked |
| POST | `/clearances/{id}/items` | Add an extra item to one checklist |
| POST | `/clearances/{id}/override` | Allow the separation to be completed with open items, with a reason |
| GET | `/me/clearance-tasks` | Open clearance items assigned to me |
| GET, PUT | `/settings/exit-interview-template` | Exit interview questions, self-service switch and aggregation threshold |
| POST, GET | `/separations/{id}/exit-interview` | Start the interview for a separation, or fetch it |
| GET | `/exit-interviews` | List exit interviews (HR only) |
| GET, PATCH | `/exit-interviews/{id}` | View an interview, save answers and notes |
| POST | `/exit-interviews/{id}/submit` | Submit and lock an interview |
| GET | `/analytics/exit-feedback` | Aggregated answers for a period, shown only above the response threshold |
| GET | `/analytics/turnover` | Turnover for a period: overall, rolling 12 months, by month, department, reason and tenure band |
| GET | `/analytics/headcount` | Current headcount and monthly opening, joiners, leavers and closing |
| GET | `/analytics/retention` | Retention rate for a period, overall and by department |
| GET | `/analytics/tenure` | Length of service of current staff and of leavers |
| GET | `/analytics/data-quality` | Record problems that would distort the metrics |

Analytics endpoints take `from`, `to` (YYYY-MM-DD, default: the last 12 calendar months) and an optional `departmentId`.

Edits to departments, employees, separations and clearances send the record's current `version`; a stale version returns `409`.

### How the metrics are calculated

Every figure is derived from employment dates and completed separations, by one package (`backend/internal/metrics`) used by all endpoints.

| Metric | Definition |
| --- | --- |
| Headcount on a date | People whose join date is on or before it and whose end date is empty or on or after it. The last working day counts as employed |
| Average headcount | Mean of the daily headcount over every calendar day of the period |
| Turnover rate | Completed separations with a last working day in the period ÷ average headcount × 100 |
| Voluntary, involuntary, other | The same, split by the category of the separation reason. The three add up to the total |
| Department turnover | Separations from the department ÷ the department's average headcount, using dated department history. Transfers are reported beside it and are not turnover |
| Retention rate | People employed on the first day who are still employed on the last day ÷ people employed on the first day × 100. It is not 100 minus turnover |
| Tenure | Mean and median length of service, for current staff as of a date and for leavers in a period |

Every rate comes with its numerator, denominator and flags. A rate has no value (`null`), instead of a misleading number, when:

- average headcount is zero (`no_headcount`);
- the period starts before the organization's analytics start date, the date from which its data is complete (`before_analytics_start`).

It keeps its value but is flagged when the average headcount is below the small-group threshold (`small_group`, default 5) or the month has not finished (`partial_period`). Employment types the organization excludes from headcount are left out of both numerator and denominator.

### Notifications and the n8n webhook

The API decides who is notified and stores an in-app notification for each person. For email, it also stores an event in an outbox, in the same transaction as the change, and a background dispatcher posts it to the webhook in `N8N_WEBHOOK_URL`. Failed deliveries are retried with backoff (30 seconds doubling up to 1 hour, 10 attempts). Without a webhook URL, in-app notifications still work and events are not delivered.

Each webhook request is JSON with `eventId`, `type`, `organizationId`, `occurredAt` and `data` (title, body, link, recipients and a few record fields; never the separation reason). It carries three headers:

| Header | Content |
| --- | --- |
| `X-PeoplePulse-Event-Id` | Unique event ID. Delivery is at-least-once, so receivers should ignore an ID they have already processed |
| `X-PeoplePulse-Timestamp` | Unix time in seconds when the request was sent |
| `X-PeoplePulse-Signature` | Hex HMAC-SHA256 of `<timestamp>.<raw body>` using `N8N_WEBHOOK_SECRET` |

The receiver should recompute the signature and reject mismatches and timestamps older than five minutes.

Event types: `separation.approval_requested`, `separation.submitted`, `separation.approved`, `separation.rejected`, `separation.withdrawn`, `separation.last_working_day_changed`, `clearance.assigned`.

### CSV import format

Header row required. Columns: `employeeNumber`, `fullName`, `departmentCode`, `employmentType`, `joinDate` (YYYY-MM-DD) are required; `workEmail`, `phone`, `jobTitle`, `managerEmployeeNumber` are optional. Nothing is saved unless every row is valid.

## Running the frontend

Requirements: Node.js 20+ and the API running on port 8080.

```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:3000 and sign in with the admin account created by the bootstrap command. See [frontend/README.md](frontend/README.md) for details.

### Demo data

To see the dashboard with data, fill an empty organization through the API (the script refuses to run if employees already exist):

```bash
# PowerShell: $env:SEED_EMAIL = "admin@example.com"; $env:SEED_PASSWORD = "..."
SEED_EMAIL=admin@example.com SEED_PASSWORD=your-password node scripts/seed-demo.mjs
```

It creates five departments, about 55 employees, twelve completed separations over the last year and one pending resignation.

## Tests

```bash
cd backend
go test ./...

cd ../frontend
npm run lint
npm run build
```

## Roadmap

1. Requirements analysis and UI/UX design
2. Project setup and authentication (done)
3. Employee and department management (backend done)
4. Resignation and exit management (backend done)
5. HR dashboard and analytics (metric engine, analytics endpoints and the dashboard screen done; reports next)
6. n8n workflow automation
7. Testing, security and deployment

## Author

Nevindi Sadeera Lokuliyanage
