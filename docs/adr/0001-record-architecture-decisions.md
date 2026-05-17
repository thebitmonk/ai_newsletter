# 1. Record architecture decisions

Date: 2026-05-17

## Status

Accepted

## Context

We need a lightweight, version-controlled way to capture significant architectural decisions in this repo so that future contributors — human and AI — can see not just what was chosen, but why, and what was rejected. Without this, reasoning gets lost in chat history or commit messages and the project drifts as old assumptions become invisible.

## Decision

We will use **Architecture Decision Records (ADRs)** in the format introduced by Michael Nygard ([Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)). One ADR per significant decision, numbered sequentially (`0001-`, `0002-`, …), stored as Markdown under `docs/adr/`.

Each ADR captures:

- **Status** — Proposed, Accepted, Superseded by ADR-NNNN, Deprecated
- **Context** — the forces at play, the problem, the constraints
- **Decision** — what we chose
- **Consequences** — what becomes easier and harder as a result

Agent skills (`/diagnose`, `/improve-codebase-architecture`, `/tdd`) read this directory to ground proposals against past decisions and to flag conflicts ("contradicts ADR-NNNN — but worth reopening because…") rather than silently overriding them.

## Consequences

- Future architectural choices are easier to revisit because the reasoning is preserved.
- ADRs become load-bearing: when a decision is reversed, the old ADR gets marked **Superseded by ADR-NNNN**, never deleted.
- New ADRs cost upfront thinking-and-writing time. Worth it for genuinely significant or hard-to-reverse choices (data model shape, third-party dependency boundaries, deployment topology); not worth it for routine code patterns.
- An empty `docs/adr/` after this ADR is a normal state — write the next one only when a real decision needs recording.
