# Testing

What is tested, how to run it, and what is not covered yet.

## Summary

| Level | What | Where it runs |
| --- | --- | --- |
| Unit tests | 36 Go tests for rules and calculations | Locally and in CI on every pull request |
| Static checks | `gofmt`, `go vet`, ESLint, TypeScript | Locally and in CI |
| Builds | Go build, Next.js production build | Locally and in CI |
| End-to-end API runs | Each backend feature exercised against a temporary MongoDB | Run by hand per pull request; not yet in the repository |
| Browser runs | Each screen driven in a real browser | Run by hand per pull request; not yet in the repository |

CI (`.github/workflows/ci.yml`) runs the first three on Linux for every pull request and every push to `main`. A pull request is merged only when both jobs are green.

## Running the automated checks

```bash
cd backend
gofmt -l .          # prints nothing when formatting is correct
go vet ./...
go test ./...

cd ../frontend
npm ci
npm run lint
npx tsc --noEmit
npm run build
```

The unit tests need no database and no network.

## What the unit tests cover

| Package | Tests | What they establish |
| --- | --- | --- |
| `internal/metrics` | 9 | Headcount on boundary days, average headcount as the mean of daily headcount, turnover and its split, retention, the monthly identity closing = opening + joiners − leavers, transfers kept out of turnover, tenure, and that a zero denominator gives no rate |
| `internal/domain` | 13 | Separation status transitions and the approval chain, completion and tenure at exit, clearance status and remarks rules, exit interview answer validation and aggregation with suppression, department transfers, calendar dates, default roles |
| `internal/auth` | 4 | Argon2id hashing and verification, password rules, token expiry and tampering, the rate limiter |
| `internal/outbox` | 3 | Webhook signing, retry after failure, backoff timing, giving up at the attempt limit |
| `internal/httpx` | 3 | Unexpected errors hide their details, known errors are written as given, a disconnected client is not logged as a server error |
| `internal/api` | 4 | CSV import validation, row by row, and neutralising spreadsheet formulas in exports |

### How the metrics are checked

The metric tests use a four-person organization small enough to calculate by hand, with the working written beside the test data:

- Person-days in March 2026: 31 + 10 + 16 + 31 = 88, so the average headcount is 88 ÷ 31 = 2.84.
- One separation gives a turnover of 1 ÷ 2.84 = 35.23%.
- By department: 61 and 27 person-days, giving 50.82% and 0%.
- Retention: 2 of the 3 people employed on 1 March are still employed on 31 March, 66.67%, which is deliberately not 100 − 35.23.

One test also compares the person-days shortcut with a literal day-by-day mean. The same organization was later rebuilt through the API and the analytics endpoints returned the same figures.

## What was exercised end to end

Each feature was run against a temporary MongoDB on a separate port, never against a database holding real data, with a second organization present to prove isolation. The pull request for each feature lists the cases. In outline:

| Area | Cases |
| --- | --- |
| Sign-in and sessions | Wrong password, rate limiting, token refresh and rotation, replay of a used refresh token revoking the session, deactivation taking effect at once |
| Tenant isolation | For every resource: a user of one organization gets 404 for a record of another, and lists show only their own |
| Permissions | Each role tried against endpoints it may not use; department heads limited to their departments |
| Employees and departments | Validation, duplicate numbers, stale-version conflicts, transfers, CSV import with a dry run |
| Separations | Submit, each approval step, reject, withdraw, change of last working day, completion; with and without MongoDB transactions |
| Clearance | Generation on approval, per-assignee tasks, override, cancellation, due dates following the last working day |
| Exit interviews | Self-service, locking on submit, amend, suppression below the threshold, free text never leaving the API |
| Notifications and webhook | Recipients per event, signature verified by a stand-in receiver, no sensitive text in payloads |
| Analytics and reports | Figures matching the hand calculation, withheld rates, CSV content and formula neutralising |
| Password reset | No account discovery, single-use links, sessions revoked, link erased after delivery |
| Integration endpoints | Access by key only, reminder windows, monthly figures equal to the analytics endpoint |
| Screens | Every screen in a real browser (Edge), with separate sessions per role, at 1440 px and 400 px wide, watching for console and server errors |

Defects these runs found and fixed before merging include: database indexes created with wrong names, a role that could not submit its own resignation, dialogs that did not close on Escape, an exit interview that could not be resubmitted after a refused submit, and a lock file that would not install on Linux.

## Not covered yet

Stated plainly, so nobody assumes otherwise:

- **The end-to-end and browser runs are not committed.** They were scripts run by hand for each pull request. Until they are in the repository and in CI, a later change could break a flow without a test failing.
- **The n8n workflow files have not been run inside n8n.** Their scripts were executed against real API responses in Node.js, including the signature check. Importing them into n8n and sending a real email has not been done.
- **No load or performance test.** The response-time targets in the proposal are unmeasured. Analytics load an organization's records into memory, which is sized for up to about 10,000 employee records and has not been tried at that size.
- **No security scan or penetration test.** The controls described in the proposal are implemented and individually tested, but there has been no independent review, no dependency audit in CI, and no automated scan.
- **No accessibility audit.** Controls have labels, focus is visible and the layout works at phone width, but nothing has been checked with a screen reader or against WCAG criteria.
- **Backup and restore have not been rehearsed.** See the deployment guide.
- **Only Microsoft Edge was used** for browser runs. Firefox and Safari are untested.
- **No user acceptance testing.** No HR practitioner has used the system yet, and the requirements behind it are still assumptions from the project brief.

## Suggested next steps for testing

1. Commit the end-to-end API runs as Go integration tests that start against a MongoDB service in CI.
2. Commit the browser runs with Playwright and run the main flows in CI.
3. Import the n8n workflows into a real instance and send each email once.
4. Add `govulncheck` and `npm audit` to CI.
5. Load a 10,000-employee organization and measure the analytics endpoints.
6. Walk an HR practitioner through a resignation from submission to completion.
