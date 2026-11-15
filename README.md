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

Edits to departments, employees, separations and clearances send the record's current `version`; a stale version returns `409`.

### CSV import format

Header row required. Columns: `employeeNumber`, `fullName`, `departmentCode`, `employmentType`, `joinDate` (YYYY-MM-DD) are required; `workEmail`, `phone`, `jobTitle`, `managerEmployeeNumber` are optional. Nothing is saved unless every row is valid.

## Tests

```bash
cd backend
go test ./...
```

## Roadmap

1. Requirements analysis and UI/UX design
2. Project setup and authentication (done)
3. Employee and department management (backend done)
4. Resignation and exit management (separation workflow, exit clearance and exit interviews done; notifications next)
5. HR dashboard and analytics
6. n8n workflow automation
7. Testing, security and deployment

## Author

Nevindi Sadeera Lokuliyanage
