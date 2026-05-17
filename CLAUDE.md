# ai_newsletter

<!-- Project-level instructions for agent tools. Add project goals, conventions, and tooling notes here as the project grows. -->

## Stack

- **Backend**: Go with [Gin](https://gin-gonic.com/) for HTTP.
- **Frontend**: [Nuxt](https://nuxt.com/) (Vue 3).
- **Database**: PostgreSQL — primary store. All timestamps stored as `timestamptz` (UTC), never naive — see [ADR-0014](./docs/adr/0014-per-publication-timezone-utc-storage.md). The Issue body is `jsonb` per [ADR-0008](./docs/adr/0008-hybrid-issue-persistence.md).
- **Cache**: Redis — caching, rate limiting, idempotency keys, distributed locks. Not used as a queue.
- **Queue**: [NSQ](https://nsq.io/) — background work (Source polling, Curation pipeline, Dispatch). Topics-per-job, durable, with separate consumer pools per worker class.
- **Editor**: Tiptap (ProseMirror).

See [ADR-0015](./docs/adr/0015-stack-gin-nuxt-postgres-redis-nsq.md) for the rules these choices imply.

## Agent skills

### Issue tracker

Issues live as GitHub issues, accessed via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Canonical triage role names are used verbatim as GitHub labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context layout — one `CONTEXT.md` and `docs/adr/` at the repo root (created lazily by `/grill-with-docs`). See `docs/agents/domain.md`.
