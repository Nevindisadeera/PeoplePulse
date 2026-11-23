# Deployment guide

How to run PeoplePulse outside a development machine.

> **Status.** PeoplePulse has not been deployed to a production environment yet. The build and start commands below are the ones used throughout development and are known to work. The parts about a reverse proxy, running as a service, and backup and restore describe the intended set-up and have not been rehearsed. Rehearse a restore before real data goes in.

## What runs

| Part | What it is | Default port |
| --- | --- | --- |
| API | One Go binary (`backend/cmd/api`) | 8080 |
| Web app | Next.js server (`frontend`) | 3000 |
| MongoDB | Version 8, run as a replica set | 27017 |
| n8n (optional) | Sends emails | 5678 |

Browsers talk only to the web app. The web app forwards `/api/v1` to the API, so both share one address and the sign-in cookie stays first-party. Put a reverse proxy that terminates HTTPS in front of the web app. The API, MongoDB and n8n should not be reachable from the internet.

## 1. MongoDB as a replica set

The API uses transactions for changes that touch several records (for example, approving a separation also updates the employee and creates the clearance checklist). Transactions need a replica set; a single-node one is enough.

On a standalone MongoDB the API still starts and works, but those changes are not atomic, and it logs a warning at start-up. Do not run real data that way.

For a self-managed server, add to `mongod.cfg` and restart MongoDB:

```yaml
replication:
  replSetName: rs0
```

Then once, in `mongosh`:

```js
rs.initiate()
```

A managed service such as MongoDB Atlas is a replica set already.

Create a database user for the application with read and write access to its own database only, and use it in `MONGO_URI`.

## 2. API

Build:

```bash
cd backend
go build -o peoplepulse-api ./cmd/api
go build -o peoplepulse-bootstrap ./cmd/bootstrap
```

Configure with environment variables, or a `.env` file beside the binary. Real environment variables win over the file.

| Variable | Required | Notes |
| --- | --- | --- |
| `JWT_SECRET` | yes | 32 or more random characters. Changing it signs everyone out |
| `MONGO_URI` | yes | For example `mongodb://user:password@host:27017/?replicaSet=rs0` |
| `MONGO_DB` | no | Default `peoplepulse` |
| `PORT` | no | Default `8080` |
| `COOKIE_SECURE` | no | Leave at the default `true` in production. `false` is only for plain-HTTP local development |
| `APP_BASE_URL` | yes | Public address of the web app, used for links in emails |
| `CORS_ORIGIN` | no | Only matters if a browser calls the API directly from another origin |
| `ACCESS_TOKEN_TTL`, `REFRESH_TOKEN_TTL`, `INVITE_TTL`, `PASSWORD_RESET_TTL` | no | Defaults `15m`, `168h`, `72h`, `1h` |
| `N8N_WEBHOOK_URL`, `N8N_WEBHOOK_SECRET` | no | Turn on email delivery. See [n8n/README.md](../n8n/README.md) |
| `INTEGRATION_API_KEY` | no | Lets n8n read reminder and report data |

Generate secrets with a password manager or, for example, `openssl rand -hex 32`. Use a different value for each.

Create the first organization and its administrator, once per organization:

```bash
BOOTSTRAP_PASSWORD='a long password' ./peoplepulse-bootstrap \
  -org "Company name" -email admin@company.example -name "Admin Name" -tz Asia/Colombo
```

Start:

```bash
./peoplepulse-api
```

Checks: `GET /healthz` answers when the process is up, and `GET /readyz` answers 200 only when the database is reachable. Point monitoring at `/readyz`.

Run the binary under a service manager (systemd on Linux, a Windows service wrapper such as NSSM on Windows) so it restarts after a crash or reboot. The API creates its database indexes at start-up.

More than one API instance can run behind the proxy. Two things are per process and would need attention then: the sign-in rate limiter, and the outbox dispatcher (safe to run on several instances, since events are leased before sending).

## 3. Web app

The API address is read when the web app is built, so set it then:

```bash
cd frontend
npm ci
API_URL=http://localhost:8080 npm run build
npm run start        # serves on port 3000
```

Run it under the same kind of service manager as the API.

## 4. Reverse proxy and HTTPS

Any proxy that terminates TLS works. Send every request to the web app on port 3000. Example for Caddy, which obtains certificates automatically:

```
peoplepulse.company.example {
    reverse_proxy localhost:3000
}
```

Only HTTPS should be offered. With `COOKIE_SECURE=true` the browser will not send the sign-in cookie over plain HTTP.

## 5. Email (optional)

Follow [n8n/README.md](../n8n/README.md). Without it, in-app notifications work, and administrators pass on invitation and password reset links by hand from the Users screen.

## 6. Backups

Back up the MongoDB database daily and keep the copies somewhere other than the database server.

```bash
mongodump --uri "$MONGO_URI" --db peoplepulse --gzip --archive=peoplepulse-$(date +%F).gz
```

Restore into an empty database:

```bash
mongorestore --uri "$MONGO_URI" --gzip --archive=peoplepulse-2026-11-23.gz
```

`mongodump` and `mongorestore` come with the MongoDB Database Tools, installed separately from the server.

Suggested targets, to be confirmed for your organization: lose at most 24 hours of data, restore within 4 hours, keep 30 days of backups. Whatever the targets, **restore a backup into a test database before go-live and at intervals afterwards**. A backup that has never been restored is not yet a backup.

Also keep, outside the server: the `.env` values (in a secrets store) and the exported n8n workflows (already in this repository).

## 7. Upgrading

1. Take a backup.
2. Build the new API and web app.
3. Stop the old API, start the new one. It adds any new indexes itself.
4. Restart the web app.
5. Check `/readyz`, sign in, and open the dashboard.

There are no migration scripts yet: so far every change has been additive, with defaults for settings that an older organization does not have. A change that needs existing data rewritten will come with its own script and instructions.

## Before going live

- [ ] MongoDB is a replica set, and the API's start-up log has no transaction warning
- [ ] `JWT_SECRET`, the webhook secret and the integration key are long, random and different from each other
- [ ] `COOKIE_SECURE` is not `false`
- [ ] Only the web app is reachable from outside; the API, MongoDB and n8n are not
- [ ] HTTPS only
- [ ] A backup has been taken **and restored** somewhere
- [ ] The organization's time zone and analytics start date are set under Settings
- [ ] The default roles have been reviewed, and the bootstrap administrator's password changed from the one typed on the command line
- [ ] The data-protection rules that apply to your organization have been checked: who may see exit interview answers, and how long records of former employees are kept
