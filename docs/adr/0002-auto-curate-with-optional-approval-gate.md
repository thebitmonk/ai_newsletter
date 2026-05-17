# Auto-curate cadence slots; per-Publication approval gate is opt-in

A **Publication** with a **Cadence** will, by default, auto-curate its upcoming **Issue** at a configurable lead time before send (default 24h) and auto-dispatch at the scheduled time without human action. Owners who need a brand-safety review can flip a per-Publication "require approval before send" flag, after which an un-approved Issue will skip its send window rather than dispatch.

We chose this over "always require approval" because the product's core pitch is hands-off automation; a forced approval step would gut that. We chose it over "no approval mode at all" because some owners cannot send unreviewed AI content to their list, and losing them would shrink the market. The opt-in flag costs us one boolean and one state-machine branch but preserves both audiences.

This decision shapes the Issue state machine (`drafted → approved? → sending → sent` vs. `drafted → sending → sent`), the notification policy (who gets pinged and when), and the calendar semantics ("scheduled" can mean either "will send unconditionally" or "will send if approved in time").
