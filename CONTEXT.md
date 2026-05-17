# ai_newsletter — Context

A platform for running automated AI-curated newsletter publications. An Account Owner operates one or more Publications; the system curates content from external Sources, generates Issues, and (eventually) sends them to Contacts.

## Language

**Account**:
The workspace that owns all resources — Publications, Contacts, Subscriptions, Sources, Issues, Dispatches. Never a person. Always referenced by `account_id`, never collapsed into the User that created it. See [ADR-0013](./docs/adr/0013-account-user-separated-schema-one-to-one-enforced.md).
_Avoid_: workspace (synonym, prefer Account), tenant, org, customer

**User**:
A human identity that can log in. A separate entity from **Account** — Users are people, Accounts are workspaces. At v1 each Account has exactly one User, enforced in application code; the schema permits many-to-many through `account_members` so v2 can add teammates without migration.
_Avoid_: customer (Account is the customer-of-record), member (the join row, not the person)

**Account Owner**:
The current sole role a **User** holds on an **Account** (`account_members.role = 'owner'`). The role enum exists for future Editor/Viewer values but only `owner` is used at v1.
_Avoid_: admin, root, primary user

**Publication**:
A long-lived branded channel on a specific topic — the thing an Account Owner "runs." Has its own Sources, design defaults, and Contact list. Analogous to a Substack or Beehiiv publication.
_Avoid_: newsletter, channel, blog

**Issue**:
A single dated send belonging to one **Publication**. The unit that is curated, edited in Tiptap, scheduled, shown on the calendar, and dispatched over SMTP.
_Avoid_: newsletter, post, edition, email

**Story**:
One curated item inside an **Issue**, derived from exactly one external **Source** item (a YouTube video, a tweet, a Substack post, etc.). Has a generated summary and a generated image. Born and dies with its parent Issue — never moves or is reused across Issues.
_Avoid_: item, block, card, snippet

**Cadence**:
A **Publication**-level schedule rule (e.g. "every Monday 09:00 UTC") that auto-spawns empty future **Issues** as calendar slots. Slots are filled by curation closer to send time; the owner can also manually insert ad-hoc Issues between slots.
_Avoid_: schedule, frequency, recurrence

**Brief**:
A single free-text field on a **Publication** describing its topic, audience, voice, and exclusions in the owner's words. The Brief is injected into every LLM call in the content pipeline (ranking, summarization, image prompt) — it's the editorial spine. See [ADR-0005](./docs/adr/0005-publication-brief-as-prompt-spine.md).
_Avoid_: prompt, description, topic, persona

**Source**:
A specific external feed the system polls for content — a YouTube channel, an X/Twitter handle, a Substack publication, an RSS URL, a domain. Belongs to exactly one **Publication**. Not a topic query; see [ADR-0003](./docs/adr/0003-source-model-specific-feeds-not-topic-search.md).
_Avoid_: feed, channel (ambiguous with Publication), input

**Candidate**:
A single item fetched from a **Source** (a specific video, tweet, post, article), deduplicated by source-item-ID, sitting in a per-**Publication** pool with a TTL waiting to possibly become a **Story**. Most Candidates never become Stories — they expire unused. See [ADR-0004](./docs/adr/0004-candidate-pool-not-on-demand-fetch.md).
_Avoid_: item (too generic), entry, post (overloaded with Issue)

**Contact**:
A real person an account can email, identified by email address, scoped to the **Account**, never to a single **Publication**. Carries cleanable/enrichable attributes (name, company, enrichment data) and a global suppression status that blocks all sends across all Publications. See [ADR-0009](./docs/adr/0009-account-contacts-with-publication-subscriptions.md).
_Avoid_: subscriber, lead, recipient, user

**Subscription**:
The explicit link between a **Contact** and a **Publication**, carrying its own status (`active`, `unsubscribed`, `bounced`, `complained`, `pending_double_optin`). An **Issue** sends to all `active` Subscriptions of its parent Publication. A Contact with one unsubscribed Subscription can still be active on others.
_Avoid_: membership, signup, opt-in, list-entry

**Suppression**:
An account-global block on sending to a **Contact**, set by `hard_bounced`, `complained`, `manually_suppressed`, or `gdpr_deleted` reasons. Suppressed Contacts are excluded from every send across every Publication regardless of Subscription state — Suppression always wins. See [ADR-0009](./docs/adr/0009-account-contacts-with-publication-subscriptions.md).
_Avoid_: blocklist, banned, opt-out (subscription-level, not contact-level)

**Cleaning**:
A defined set of data-integrity and deliverability operations on **Contacts** (format normalisation, validation, dedup, MX check, bounce/complaint Suppression, optional engagement-cleanup and role-account filtering). Always operates *on* Contact data — never *adds* to it. Distinct from **Enrichment**. See [ADR-0011](./docs/adr/0011-contact-cleaning-scope-and-defaults.md).
_Avoid_: hygiene, scrubbing, validation (one slice of cleaning, not the whole)

**Enrichment**:
The process of *adding* attributes to a **Contact** from external sources (Apollo, Clearbit) — name, company, job title, enrichment metadata. Categorically distinct from **Cleaning**: enrichment grows data, cleaning maintains it. Different cadence, different cost, different failure modes.
_Avoid_: cleaning (different concept), augmentation, lookup

**Dispatch**:
A single attempt to deliver a specific **Issue** to a specific **Contact** through a specific **Subscription**. One row per Issue × recipient. Holds the provider-side message ID and is the row that webhook events (bounce, complaint, open, click) update by reference. Without it, "did Contact X receive Issue Y?" is unanswerable. See [ADR-0012](./docs/adr/0012-byo-sending-per-publication-credentials-api-only.md).
_Avoid_: send, message, delivery (overloaded), email-row

## Relationships

- A **User** belongs to exactly one **Account** at v1 (schema supports many-to-many)
- An **Account** (acted on by its **Account Owner**) owns one or more **Publications**
- A **Publication** has at most one **Cadence** (optional — a Publication can also run purely ad-hoc)
- A **Publication** owns its own list of **Sources** (not shared across Publications in an account)
- A **Publication** produces zero or more **Issues** over time
- A **Source** produces zero or more **Candidates** over time, into its Publication's pool
- **Curation** for an **Issue** reads from the Publication's Candidate pool and promotes selected Candidates to **Stories**
- An **Issue** contains one or more **Stories**
- A **Story** is derived from exactly one **Candidate** (and transitively one **Source**)
- A **Story** belongs to exactly one **Issue** and is never reused
- An **Account** has many **Contacts**; a **Contact** has many **Subscriptions**; each **Subscription** ties one Contact to one Publication
- An **Issue** is sent to all `active` **Subscriptions** of its parent **Publication**

## Example dialogue

> **Dev:** "If a user edits a **Story** in the March 17 **Issue**, does that edit affect future **Issues** that pull from the same source video?"
> **Domain expert:** "No. A **Story** lives and dies with its **Issue**. The source video might get re-summarised into a new Story in next week's Issue, but that's a fresh Story."

## Flagged ambiguities

- "newsletter" was used ambiguously for both the brand and a single send — resolved: **Publication** is the brand, **Issue** is the send. The word "newsletter" is reserved for marketing copy only.

## See also

- ADRs: [`docs/adr/`](./docs/adr/)
- Agent skill configuration: [`docs/agents/`](./docs/agents/)
