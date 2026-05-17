# Sources are specific feeds, not topic searches

A **Source** describes a particular channel the owner has picked — a YouTube channel ID, an X/Twitter handle, a Substack publication, an RSS URL, a domain. The system polls each Source for new items. Sources are **not** queries against platform search APIs ("LLM videos from the last 7 days").

We chose feeds over topic search because (a) RSS/channel polling is near-free per item while platform search APIs (YouTube Data API quota, X API tiers, Google News) eat margin per Publication; (b) feed output is predictable and trusted by the owner, where search results vary wildly and surface low-quality content that damages the Publication's reputation; (c) a feed-poller is a tractable v1 backend project, while topic-relevance ranking across heterogeneous platforms is a research project. The "build the backend infra layer first" sequencing in the product plan rules out the research project for now.

A "discover sources from a topic" feature can be layered on later as owner-facing tooling that suggests feeds — the runtime ingestion model stays feed-based.
