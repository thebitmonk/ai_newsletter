# Source polling fills a persistent Candidate pool, not on-demand at curation

Each **Source** is polled on its own continuous schedule by a background worker. New items are deduplicated by source-item-ID and written to a per-**Publication** `candidates` table with a TTL of `max(cadence_interval × 2, 7 days)`. **Curation** for an upcoming **Issue** reads from this pool rather than fetching from Sources inline.

We chose the pool over on-demand fetch primarily for resilience and debuggability. Feeds will transiently fail (rate limits, API outages, DNS); if curation fetches inline at its lead-time window, a single bad poll empties the Issue. The pool buffers across outages. It also gives us an honest answer to "why wasn't this video included?" — we can show that the candidate existed and where it ranked. The on-demand alternative leaves no audit trail of what was available at decision time. The pool additionally enables the calendar-health UX in the product plan ("Monday's slot has 12 candidates ready / Friday's slot has 0").

The TTL keeps the pool from growing unbounded while staying long enough to bridge multi-day feed outages for slow-cadence Publications. Future cross-Publication features (shared "discover" suggestions, source-quality analytics) attach naturally to this table; the on-demand alternative would force us to retrofit it later.
