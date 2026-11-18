# PeoplePulse frontend

Next.js (App Router), TypeScript, Tailwind CSS and Recharts.

## Running

The Go API must be running first (see the repository README).

```bash
npm install
npm run dev
```

Open http://localhost:3000 and sign in with an account created by the backend's bootstrap command.

Requests to `/api/v1` are forwarded to the API by a rewrite in `next.config.ts`, so the browser sees one origin. The API address defaults to `http://localhost:8080`; set `API_URL` to change it.

## Structure

```
src/app/login          Sign-in page
src/app/(app)          Signed-in area: shell layout and pages
src/components         Shared UI pieces and charts
src/lib                API client, session handling, types, formatting
```

## How sign-in works

The access token is kept in memory only. On a page load the app asks the API for a new one using the HttpOnly refresh cookie, so a reload keeps you signed in without the token ever being stored where scripts can read it later. When a request returns 401, the client refreshes once and repeats it.

Controls are hidden when the signed-in role lacks a permission, as a convenience. The API enforces every permission itself.

## Design

A dark interface: near-black ground, charcoal cards with large radii, pill-shaped controls and condensed uppercase headings. Lime marks growth and positive movement; orange marks departures and things that need attention. The colour and type tokens are defined once in `src/app/globals.css`.

## Checks

```bash
npm run lint
npm run build
```
