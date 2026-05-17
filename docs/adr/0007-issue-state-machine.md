# Issue lifecycle: eight-state machine

An **Issue** moves through a fixed eight-state machine: `planned → curating → drafted → (approved →) sending → sent`, with `failed` and `skipped` as terminal off-ramps. `approved` is only entered when the parent **Publication** has the approval gate flag from [ADR-0002](./0002-auto-curate-with-optional-approval-gate.md) turned on; otherwise the path is `drafted → sending` directly. Ad-hoc Issues created by the owner skip `planned/curating` and start in `drafted`.

The choices that aren't obvious from the diagram:

- **`approved` is a distinct state, not a flag on `drafted`.** It gives the calendar a separate badge, gives the audit log a clean timestamp, and lets the state-machine guard refuse `sending` from `drafted` without a code branch on a boolean. The cost is one extra row in the states enum; the benefit is that every approval-gated transition is type-checked at the boundary.
- **`failed` and `skipped` are distinct terminals.** `failed` is "system error, please retry" — it triggers alerts and exposes a retry button. `skipped` is "intentionally not sent" (owner cancelled, approval window missed, zero Candidates available) — silent, no alert. Conflating them would either spam owners with non-failures or hide real failures.
- **There is no `editing` state.** Editing is a mutation within `drafted` or `approved`, not a transition. The state machine governs lifecycle, not user activity.
- **There is no per-Issue `paused`.** Pausing a Publication is a Publication-level flag that prevents Cadence from spawning new `planned` Issues; per-Issue pause is handled by transitioning to `skipped`.
- **Approval-window race policy:** at `(send_time − 60s)` the Issue's state is frozen — late approvals are rejected with a clear error rather than racing the dispatcher. The 60s buffer is configurable per Publication.
- **Recovery semantics:** `failed` after curation → reset to `planned`, re-run pipeline. `failed` after send → re-enter `sending` without re-curating (don't regenerate the email the recipient may have partially received).

A state machine is among the hardest things to change once code, UI badges, notification triggers, analytics, and migrations have hardcoded the states. The cost of adding a ninth state later is high but bounded; the cost of having to re-shape these eight is much higher.
