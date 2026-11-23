# Contributing

How changes are made in this repository.

## Workflow

1. **Open an issue** that describes the need or the bug.
2. **Create a branch** from `main`, named for the kind of change:
   - `feature/short-description` for new behaviour
   - `fix/short-description` for a bug fix
   - `chore/short-description` for tooling, CI and documentation
3. **Commit** in small, complete steps. Write the subject line in the imperative ("Add clearance override", not "Added" or "Adds"), under about 70 characters, and use the body to say why.
4. **Open a pull request** against `main` and link the issue with `Closes #123`. The template asks what changed, why, and how it was tested.
5. **Wait for CI.** Every pull request runs the backend and frontend checks. Merge only when they are green.
6. **Merge** with a merge commit and delete the branch.

Nothing is committed directly to `main`.

## Checks to run before pushing

```bash
# backend
cd backend
gofmt -l .        # should print nothing
go vet ./...
go test ./...

# frontend
cd frontend
npm run lint
npx tsc --noEmit
npm run build
```

CI runs the same commands (see `.github/workflows/ci.yml`).

## Conventions

**Backend**
- Business rules live in the Go API. The frontend and n8n never decide an outcome on their own.
- Every data access method takes the organization ID, and it comes from the signed-in user's token, never from the request.
- A record in another organization answers 404, the same as one that does not exist.
- Rates are calculated only in `internal/metrics`.
- Changes to several records at once go through `Store.Atomic`.
- Actions that matter to an auditor write an audit entry. Sensitive free text (separation reasons, exit interview answers) stays out of logs, audit entries and webhook payloads.

**Frontend**
- Hide a control when the role lacks the permission, and still expect the API to refuse.
- Show the API's validation message under the field it belongs to.
- Show a withheld rate as "n/a" with its reason, never as 0%.
- Colours and type come from the tokens in `src/app/globals.css`.

## Testing

- Rules and calculations get unit tests with values worked out by hand.
- New endpoints are exercised end to end against a temporary MongoDB, including a second organization to prove isolation.
- New screens are driven in a real browser, at desktop and phone width.
- Never run tests against a database that holds real data.

## Security

Do not open a public issue for a security problem. Contact the repository owner privately.
Never commit secrets: `.env` files are ignored, and `.env.example` documents the variables.
