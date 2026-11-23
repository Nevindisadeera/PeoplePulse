# n8n workflows

Four workflows that send PeoplePulse's emails. The Go API decides what happened, who is told and what is due. n8n only receives or fetches that, formats it and sends it. No business rule lives here.

| File | Trigger | What it does |
| --- | --- | --- |
| `events-to-email.json` | Webhook from the API | Verifies the signature, then sends one email per recipient. Covers resignations submitted and decided, clearance tasks assigned, last working day changes, invitations and password resets |
| `clearance-reminders.json` | Daily at 08:00 | Asks the API for clearance items that are overdue or due soon, and sends each assignee one digest |
| `last-working-day-reminders.json` | Daily at 08:00 | Asks the API for upcoming last working days and sends HR and line managers one digest each |
| `monthly-report.json` | 1st of the month at 07:00 | Asks the API for last month's figures and emails them to everyone who may export reports |

> **Status.** These files have not yet been imported into a running n8n instance. What has been checked: each file's connections are consistent, and every Code node script was run in Node.js against real responses from the API, including signature verification with a valid, a tampered, an expired and a missing signature. Node versions, the SMTP credential and the schedule nodes should be confirmed on the first import. If n8n reports a node version it does not know, open the node and save it; n8n upgrades it.

## Set up

### 1. Configure the API

In `backend/.env`:

```
APP_BASE_URL=https://your-app-address
N8N_WEBHOOK_URL=http://localhost:5678/webhook/peoplepulse-events
N8N_WEBHOOK_SECRET=<32 or more random characters>
INTEGRATION_API_KEY=<a different 32 or more random characters>
```

Restart the API. It logs a line if either secret is too short.

### 2. Configure n8n

Set these environment variables for n8n:

| Variable | Value |
| --- | --- |
| `PEOPLEPULSE_API_URL` | Address of the Go API as n8n reaches it, for example `http://localhost:8080` |
| `PEOPLEPULSE_INTEGRATION_KEY` | The same value as `INTEGRATION_API_KEY` |
| `PEOPLEPULSE_WEBHOOK_SECRET` | The same value as `N8N_WEBHOOK_SECRET` |
| `PEOPLEPULSE_MAIL_FROM` | The sender address, for example `PeoplePulse <no-reply@your-domain>` |
| `NODE_FUNCTION_ALLOW_BUILTIN` | `crypto` (the signature check uses Node's `crypto` module) |
| `N8N_BLOCK_ENV_ACCESS_IN_NODE` | `false` (the workflows read the variables above) |

### 3. Import

1. In n8n, create an **SMTP** credential for your mail service.
2. Import each JSON file (**Workflows → Import from file**).
3. In every "Send…" node, choose your SMTP credential.
4. Activate the workflows. The webhook URL of `events-to-email` must match `N8N_WEBHOOK_URL`.

### 4. Try it

- Submit a resignation in PeoplePulse. The approver should receive an email within a few seconds.
- Run `clearance-reminders` by hand with **Execute workflow** while a separation with open clearance items is serving notice.

## How the API and n8n trust each other

**API to n8n.** Each event is a POST with three headers:

| Header | Content |
| --- | --- |
| `X-PeoplePulse-Event-Id` | Unique ID of the event |
| `X-PeoplePulse-Timestamp` | Unix time in seconds when it was sent |
| `X-PeoplePulse-Signature` | Hex HMAC-SHA256 of `<timestamp>.<raw body>` with the shared secret |

The first node after the webhook recomputes the signature over the raw body and refuses a mismatch or a timestamp older than five minutes. A refused request gets a 401 and the API retries it later.

**n8n to API.** The scheduled workflows send `X-Integration-Key`. That key opens three read-only endpoints under `/api/v1/integrations` and nothing else: it cannot approve, edit or read any other record.

## Things to know

- **Delivery is at-least-once.** If the API does not see a 2xx answer it sends the event again, with backoff, up to ten times. In rare cases a person can therefore get the same email twice.
- **If n8n is down**, nothing is lost: events wait in the API's outbox, in-app notifications still appear, and every screen keeps working.
- **Reminders are computed fresh on each run.** The daily schedule means one digest per person per day. Running a reminder workflow by hand sends the digest again.
- **An item assigned to a role nobody holds reminds nobody.** Check the clearance template's roles if reminders seem to be missing.
- **Keep execution data short-lived.** Events contain names and email addresses. The workflows are saved with "do not save successful executions"; keep pruning enabled for failed ones.
- **Invitation and reset links** pass through n8n once. The API erases them from its own database after delivery.
- Reminder windows are organization settings: clearance reminders (default 3 days ahead) and last working day reminders (default 7 days ahead).

## Endpoints the workflows call

| Method | Path | Returns |
| --- | --- | --- |
| GET | `/api/v1/integrations/reminders/clearance` | Per organization, per assignee: open items that are overdue or due inside the window |
| GET | `/api/v1/integrations/reminders/last-working-day` | Per organization, per recipient: approved separations ending inside the window, or past it and not completed |
| GET | `/api/v1/integrations/reports/monthly?month=YYYY-MM` | Per organization: headcount movement, turnover with counts, open work and recipients. Default month: the previous one |
