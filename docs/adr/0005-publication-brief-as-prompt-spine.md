# Each Publication carries a free-text Brief threaded through every LLM call

A **Publication** has a single free-text `brief` field describing its topic, audience, voice, and exclusions. The Brief is injected verbatim into every LLM call in the content pipeline — Candidate ranking during curation, Story summarization, image-prompt generation. There is no structured topic taxonomy, no separate `audience`/`tone`/`category` fields.

We chose a Brief over an implicit topic (relying only on the Source list and recency) because the summaries and selection logic *are* the product. A generic LLM summary of a YouTube video is undifferentiated commodity output; what justifies this product is that summaries are written for *this* Publication's audience in *this* Publication's voice, and that ranking reflects *this* Publication's editorial stance. None of that is possible without an explicit per-Publication artifact for the model to condition on.

We chose free-text over a structured taxonomy because designing a useful topic ontology is a research project with low payoff, owners will leave structured fields blank, and modern LLMs handle natural-language briefs natively. The Brief is intentionally a single field rather than split into `audience`/`voice`/`exclusions` sub-fields — the LLM stitches those together anyway, and one field gives the owner control over granularity.
