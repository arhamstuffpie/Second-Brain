# Visual Pipeline Tasks

- [x] P1 — Cover the full video timeline with duration-based frame budgets.
- [x] P2 — Add periodic/event frame selection, reasons, quality, and source IDs.
- [x] P3 — Analyze durable batches of at most eight with independent retries.
- [x] P4 — Store structured scenes, people, objects, OCR, relations, and uncertainty.
- [x] P5 — Create five-second visual windows and exact speech turns.
- [x] P6 — Write detailed visual evidence and stable source references to Memograph.
- [x] P7 — Build deterministic observation-local visual graphs.
- [x] P8 — Preserve exact speech in utterance/≤15-second evidence turns.
- [x] P9 — Use speech_evidence, visual_evidence, and context_summary records.
- [x] P10 — Use deterministic, processing-version-aware idempotency keys.
- [x] P11 — Preserve asset, recording, session, model, time, and confidence provenance.
- [x] P12 — Scope evidence-first search and provide timestamp playback URLs.
- [x] P13 — Bound graph volume, writes, windows, and expose batch age/status.
- [x] P14 — Extend durable queues and add versioned reprocessing from originals.

Verification: all Go tests pass; migrations 1–15 and repository integration tests pass on PostgreSQL 17.

Legend: `[ ]` pending · `[~]` in progress · `[x]` completed · `[!]` blocked
