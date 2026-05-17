# ai_newsletter

A platform for running automated AI-curated newsletter publications. An owner defines a Publication with a Brief (editorial voice) and a Cadence; the platform polls configured Sources continuously, ranks Candidate items against the Brief, generates Stories with summaries + a cover image, and assembles a draft Issue ready to review in a Tiptap editor.

The domain glossary lives in [`CONTEXT.md`](./CONTEXT.md). The 16 architecture decisions are in [`docs/adr/`](./docs/adr/). Read both before making non-trivial changes.

## Stack

- **Backend**: Go (Gin) — `cmd/server`
- **Frontend**: Nuxt 3 (in progress, see PRD #12) — `web/`
- **Database**: PostgreSQL
- **Queue**: NSQ
- **Cache / coordination**: Redis
- **Auth**: Firebase Auth (Admin SDK on the backend, JS SDK on the frontend)
- **Blob storage**: Cloudflare R2 (S3-compatible)
- **LLM**: OpenAI (default `gpt-5.4-mini`)
- **Image generation**: OpenAI (default `gpt-image-1.5`)

Stack rationale in [ADR-0015](./docs/adr/0015-stack-gin-nuxt-postgres-redis-nsq.md) and [ADR-0016](./docs/adr/0016-firebase-auth-replaces-local-auth.md).

## Local dev

### 1. Infrastructure

```bash
make up           # postgres, redis, nsqd+lookupd+admin, firebase emulator
make migrate-up   # apply schema migrations
```

Ports are offset to avoid colliding with anything you might already run locally:

| Service | Host port |
|---|---|
| Postgres | 5433 |
| Redis | 6380 |
| nsqd (TCP / HTTP) | 5150 / 5151 |
| nsqlookupd (TCP / HTTP) | 5160 / 5161 |
| NSQ admin UI | http://localhost:5171 |
| Firebase Auth emulator | 9099 |

### 2. Environment

Copy `.env.example` to `.env` and fill in the secret values:

```bash
cp .env.example .env
$EDITOR .env
```

- **OpenAI** (`OPENAI_API_KEY`) — required for the curation pipeline.
- **Cloudflare R2** (`R2_*` block) — required for cover image storage.
- **Firebase** — required for auth. See "Firebase setup" below.

### 3. Run

```bash
make run-env   # loads .env and runs the Go backend on :8080
```

Frontend (when it lands per PRD #12):

```bash
cd web && npm install && npm run dev   # :3000
```

## Firebase setup

### For local dev (emulator)

`make up` starts the Firebase Auth emulator on `:9099` via `firebase-tools`. To route the backend at it instead of real Firebase, uncomment the emulator line in `.env`:

```
FIREBASE_AUTH_EMULATOR_HOST=localhost:9099
```

When the emulator is in use, `GOOGLE_APPLICATION_CREDENTIALS` may be left empty — the Admin SDK doesn't need credentials to talk to the emulator.

### For production (real Firebase project)

1. Create a Firebase project at https://console.firebase.google.com.
2. Authentication → Sign-in method → enable **Email/Password** and **Google**.
3. Project Settings → General → **Project ID** → paste into `FIREBASE_PROJECT_ID`.
4. Project Settings → Service accounts → **Generate new private key** → downloads a JSON file. Save it OUTSIDE the repo (e.g. `~/secrets/firebase-<project-id>.json`) and set `GOOGLE_APPLICATION_CREDENTIALS` to that absolute path.
5. Project Settings → **Your apps** → register a Web app → copy the SDK config object → fill in the three `NUXT_PUBLIC_FIREBASE_*` variables in `.env`.

**Never commit a service-account JSON.** `.gitignore` blocks `firebase-*.json` and `*-firebase-adminsdk-*.json`, but the safer habit is to keep service-account files outside the repo entirely.

## Tests

```bash
make test       # full Go suite — requires docker compose up first
```

The HTTP-level integration tests use the live Postgres from `make up` and substitute a `firebaseauth.FakeVerifier` for the real Firebase token validator. No external services are touched from `go test`.

Frontend tests (when present): `cd web && npm test`.

## Layout

```
cmd/
  server/             Go binary entry point
internal/
  auth/               Bearer middleware (Firebase ID token verifier)
  blobstore/          Cloudflare R2 wrapper
  cadence/            RRULE expander + cadence scheduler worker
  candidates/         Per-Publication candidate pool store
  curation/           End-to-end curation pipeline orchestrator
  db/                 Postgres connection pool
  firebaseauth/       Firebase token verification
  httpx/              HTTP error-envelope helpers
  imagegen/           OpenAI cover-image generator
  issues/             Issue store + state machine (ADR-0007)
  issuedoc/           ProseMirror doc assembler (ADR-0008)
  issuesapi/          Issue HTTP handlers
  llmclient/          OpenAI Chat Completions wrapper (ranker + summarizer)
  nsqx/               NSQ producer/consumer + idempotency helpers
  publications/       Publication CRUD store + handlers
  server/             Gin router wiring
  sourceadapter/      Source poller + per-type adapter implementations
  sources/            Source CRUD store + handlers
  users/              User upsert by firebase_uid
db/
  migrations/         golang-migrate SQL files
docs/
  adr/                Architecture Decision Records (read these)
firebase-emulator/    Local Firebase Auth emulator config
```
