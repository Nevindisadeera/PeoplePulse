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

Requirements: Go 1.25+ and a running MongoDB instance.

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

Edits to departments and employees send the record's current `version`; a stale version returns `409`.

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
4. Resignation and exit management
5. HR dashboard and analytics
6. n8n workflow automation
7. Testing, security and deployment

## Author

Nevindi Sadeera Lokuliyanage
