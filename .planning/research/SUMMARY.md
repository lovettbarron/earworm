# Project Research Summary

**Project:** Earworm — v1.2 milestone: Listening Stats & Journaling
**Domain:** Cross-source personal listening analytics (Audible + Audiobookshelf) with LLM-oriented CSV export and automated Day One journaling, added to an existing Go CLI audiobook manager
**Researched:** 2026-09-20
**Confidence:** MEDIUM-HIGH (elevated from the raw source files — three factual errors in FEATURES.md and ARCHITECTURE.md were caught and corrected against live-API verification before this synthesis; see "Corrections to Source Research" below)

## Corrections to Source Research (read this first)

Three claims made in FEATURES.md and ARCHITECTURE.md were verified **empirically against live APIs** during this research cycle and found to be wrong. This SUMMARY reflects the corrected facts throughout. Anyone reading FEATURES.md or ARCHITECTURE.md directly should treat the following as superseded:

1. **Audible DOES expose usable listening history.** FEATURES.md ("Audible has no public listening-history export... the only usable signal is percent_complete") and ARCHITECTURE.md ("mechanism for Audible per-day listening totals... LOW confidence... treat as a Phase 1/2 spike") are both wrong. `audible-cli`'s generic `audible api <endpoint>` reaches the raw Audible API directly, and four endpoints were confirmed live with real data (see Key Findings -> Source Capabilities). **Consequence: Audible ingestion is not a risky spike. It is a well-understood Phase 1 activity**, and any roadmap language deferring, descoping, or spiking the Audible source should be discarded.
2. **Audible's per-book timestamps are "last status change," not clean finish events.** `listening_status.finished_at_timestamp` and `stats/status/finished`'s `event_timestamp` are the same underlying value. It can be set while a book sits at 0% and unfinished, and bulk migration artifacts exist (many books sharing one identical timestamp within a sub-second window). Any "books finished" feature must filter these clusters, not just read the field.
3. **Audiobookshelf sessions are mutable; ARCHITECTURE.md's upsert assignment is inverted.** ARCHITECTURE.md recommends `ON CONFLICT DO NOTHING` for ABS sessions (calling them "immutable") and `DO UPDATE` for Audible daily totals. This is backwards for ABS. ABS sessions stay open and keep accumulating `timeListening`/`currentTime`, and offline/mobile sessions upsert by client UUID long after `startedAt`. **ABS sessions must use `DO UPDATE` keyed on session id, with an `updatedAt` watermark and at least a 36-hour overlap window** (ABS auto-closes idle sessions at 36h). The general principle — mutable and immutable facts need different upsert semantics — is correct; the specific table assignment in ARCHITECTURE.md is not.

A fourth, non-numeric conflict: STACK.md and PITFALLS.md researched the journaling CLI against the wrong tool. STACK.md cites Day One's bundled `dayone2`/URL-scheme docs; PITFALLS.md's Pitfall 13 explicitly models its risk analysis on `jrnl.sh`. The actual, verified tool for this milestone is `@dayone/cli` v0.21.1 (npm, binary name `dayone`) — a different CLI with a different risk profile (queued outbox + explicit `sync` step, idempotent writes via caller-supplied `--entry-id`, no `--tags` flag). See Key Findings -> Stack for what carries over and what doesn't.

## Executive Summary

This milestone extends an existing single-binary Go CLI (earworm) with four capabilities: pull Audible's own listening/completion data, pull Audiobookshelf's session data, reconcile the two into one per-book timeline despite incomplete identity overlap, and write a daily digest to Day One. None of this requires a new Go runtime dependency — the codebase's existing stdlib-first conventions (`net/http`, `encoding/json`, `encoding/csv`, `os/exec`, `crypto/sha256`, the `cmdFactory` subprocess-testing pattern) cover the whole surface, including hand-rolling the title-matching algorithm rather than adopting a fuzzy-matching library. The one new external dependency is the `@dayone/cli` npm binary, shelled out to exactly like `audible-cli` already is.

The recommended approach, corrected from the raw research, is to ingest Audible first (Phase 1), because it is now a verified, well-understood set of four API calls rather than the "risky spike" the uncorrected research assumed — and because it also establishes the shared conventions (canonical timezone bucketing, clock injection, mutable-vs-immutable upsert helpers, cmdFactory reuse) that Audiobookshelf ingestion (Phase 2) will conform to. Identity resolution is empirically hard, not theoretically hard: of a 66-book verification sample, only 20 books join by ASIN, 11 more join by normalized title, and 30 books (representing roughly 38% of listening hours) exist only in Audiobookshelf with no Audible counterpart at all. This confirms ARCHITECTURE.md's surrogate-key design (`book_identities.id`, ASIN as an optional attribute, not a primary key) is the right call, and it means the export/journal layer must carry explicit source and confidence provenance on every derived fact, never silently blending measured session data with Audible's coarser signals or with inferred day-level attribution.

The key risks are: (1) treating Audible's status-change timestamps as clean finish events without filtering bulk-migration clusters; (2) getting the mutable/immutable upsert assignment backwards, which the ARCHITECTURE.md draft already did once; (3) writing inferred attribution (the day-level allocation algorithm, which only explains ~59% of Audible-era hours with confidence) into the user's real journal as if it were fact; and (4) building the Day One integration against the wrong CLI's behavior model. All four are addressed in the phase plan below.

## Key Findings

### Recommended Stack

No new Go module dependency is required. `net/http`, `encoding/json`, `encoding/csv`, `os/exec`, and `crypto/sha256` (for consistency with the codebase's existing single-hash-algorithm convention) cover HTTP pagination, JSON parsing of both `audible api` and Audiobookshelf responses, CSV export, and subprocess wrapping. Fuzzy title matching is hand-rolled (~100-150 lines: normalize, then tier-1 exact match, then tier-2 token-set similarity) rather than adopting a library — at the verified scale (dozens to low-thousands of books) every candidate library (`lithammer/fuzzysearch`, `adrg/strutil`, `hbollon/go-edlib`, `agext/levenshtein`) is either the wrong tool for full-title comparison, unmaintained, or disproportionate to the actual hard part of the problem (stripping series/subtitle/edition noise, which no library does for you regardless).

**Core technologies:**
- `net/http` (stdlib) — Audiobookshelf pagination client, extends the existing `internal/audiobookshelf/client.go` Bearer-token pattern. Use the admin, genuinely paginated `GET /api/sessions?user=<uuid>` for backfill — not `GET /api/me/listening-sessions`, which loads the user's entire session table into memory per page.
- `os/exec` (stdlib) — subprocess wrapper for both `audible api <endpoint>` and the `dayone` CLI, reusing the exact `cmdFactory`/`WithCmdFactory` injection seam already proven in `internal/audible`. Do not invent a second subprocess-testing abstraction.
- `encoding/csv` (stdlib) — LLM-shaped export, following `internal/goodreads/export.go`'s header-slice/iterate/flush pattern exactly.
- Hand-rolled `internal/matching` (or `internal/bookidentity`) — normalize + tiered exact/fuzzy match, no third-party dependency.

**Explicit do-not-add list:** `lithammer/fuzzysearch` (wrong problem shape, unmaintained since 2023), `agext/levenshtein` (unmaintained since 2020, no advantage over hand-rolling), `hashicorp/go-retryablehttp` (disproportionate for a handful of one-shot GETs against the user's own local server), any general-purpose HTTP client library for Audiobookshelf, a second subprocess-mocking abstraction alongside `cmdFactory`, and embedding/vendoring a Node.js runtime for the `dayone` CLI (require it pre-installed on PATH with a clear error, mirroring the existing `audible-cli` presence check).

**Journaling CLI — corrected:** `@dayone/cli` v0.21.1 (npm, binary `dayone`), not `dayone2` and not `jrnl`. Verified command shape: `dayone entry write --journal-id <id> --body-stdin --date <date> --entry-id <id>`. Supplying a caller-generated `--entry-id` makes writes **idempotent** — rewriting the same id updates in place rather than duplicating, which is a materially better idempotency primitive than the "track already-written state in earworm's own DB" workaround the uncorrected pitfalls research proposed as the only option (still track it in earworm's DB for cross-referencing and status reporting, but the CLI itself now does the heavy lifting). Writes are **queued to a local outbox** and require an explicit `dayone sync` afterward — this is a new failure mode not covered by the original pitfalls research (see Gaps). There is **no `--tags` flag** in 0.21.1, which conflicts with FEATURES.md's "tagging automated entries is a cheap, valuable extra" recommendation; defer tagging or investigate inline `#hashtag` support in the entry body as a substitute.

### Source Capabilities (corrected — read before implementing Phase 1 or 2)

**Audible**, via `audible api <endpoint>` subprocess calls, exposes four usable endpoints:

| Endpoint | Granularity | Pagination/limits | Returns |
|---|---|---|---|
| `/1.0/stats/aggregates` | Per-day or per-month total listening | `daily_listening_interval_duration` max 30 days/call; `monthly_listening_interval_duration` max 12 months/call; requires a start date and `store=Audible` | Total listening time in **milliseconds**, bucketed by day or month. Verified: ~2,627 days of daily-granularity history, ~6,499 total hours, spanning 2015-01-04 to present. Daily-granularity data does not exist before 2015; monthly totals reach further back. |
| `/1.0/stats/status/finished` | One record per ASIN (current state, **not** an event log) | Paginated via `continuation_token` | `asin`, `event_timestamp`, `is_marked_as_finished`, `update_date` |
| `/1.0/annotations/lastpositions` | Per-ASIN current playback position | `asins=` comma-separated, max **25 ASINs/call**, and the parameter has a **500-character limit** | Per-ASIN `last_position_heard` = `{last_updated, position_ms, status}` where status is `Exists` or `DoesNotExist` |
| `/1.0/library` with `response_groups=listening_status` | Per-book snapshot | Standard library pagination | `finished_at_timestamp`, `is_finished`, `percent_complete`, `time_remaining_seconds`, plus `purchase_date`, `library_status.date_added`, `runtime_length_min`, `category_ladders`, `series`, `thesaurus_subject_keywords` |

**Critical caveat:** `listening_status.finished_at_timestamp` and `stats/status/finished`'s `event_timestamp` are the **same underlying value** — verified identical across every book where both appear. It records the *last status change*, which may be an un-finish (a book can carry a timestamp while sitting at 0% and unfinished). Bulk migration artifacts exist: dozens of books have been observed sharing one identical timestamp within a sub-second window — this is a backend migration artifact, not that many real finishes on one day. **Any "books finished on date X" feature must detect and filter these sub-second clusters before treating a timestamp as a real event.**

**Audiobookshelf**, via REST (Bearer token, existing `internal/audiobookshelf/client.go`):

- Use `GET /api/sessions?user=<uuid>` (admin-scoped, genuinely server-side paginated) for backfill. Do **not** use `GET /api/me/listening-stats` variants that load the user's entire session history into memory per page request.
- Sessions are **mutable**: they remain open and keep accumulating `timeListening`/`currentTime` while playback continues, and offline/mobile client sessions upsert by a client-supplied UUID long after `startedAt`. Upsert with `ON CONFLICT DO UPDATE` keyed on session id, using an `updatedAt` watermark with at least a **36-hour overlap window** (ABS auto-closes idle sessions at 36h).
- `timeListening` can arrive as a **FLOAT** despite conceptually being an integer-seconds field (a real observed value: `2723.0009765625`) — do not assume a clean integer type when parsing.
- Session `mediaMetadata` is a point-in-time snapshot that can have **empty genre/series arrays**; genre/series enrichment requires a separate `POST /api/items/batch/get` call, not the session payload alone.
- Verified server version: 2.36.0.

**Identity resolution — measured, not theoretical:** of a 66-book verification sample from the Audiobookshelf source, only 20 books join to the Audible library by ASIN, 11 more join by normalized title, and 30 books (representing roughly 38% of total listening hours) are not in the Audible library at all. **Book identity cannot key on ASIN.** Unmatched books are a valid, expected end state, not an error condition, and must be retained (not dropped) in the schema.

**Time overlap between sources is nearly nonexistent:** across twelve years of combined history, exactly **one day** has data from both sources. Concatenation of the two timelines is safe with a single, explicitly documented tie-break rule for that one collision case — this is not a complex statistical reconciliation problem.

**Inferred attribution is a minority-confidence estimate, not a measurement:** a greedy backward allocation from each book's end date attributes roughly 59% of Audible-era listening hours to specific books; 46% of hours land on days with exactly one candidate book (the unambiguous, higher-confidence case). This is inference and must carry an explicit attribution/confidence column through export and journaling — it must never be written into the user's Day One journal as a stated fact.

### Expected Features

**Must have (table stakes) — corrected from FEATURES.md's understated Audible characterization:**
- Per-session listening facts from Audiobookshelf (start time, duration, book) — timestamped event log via the paginated admin sessions endpoint.
- Rich per-book facts from Audible: daily/monthly totals, finish-state, last position, percent-complete — four endpoints, not one coarse field. Treat the finish/last-status timestamp with the migration-artifact filter described above.
- Historical backfill for both sources, resumable via a persisted cursor/watermark (Audible's daily-granularity history alone is ~2,627 days, requiring roughly 88 paginated calls at the 30-day/call limit).
- Daily incremental sync for both sources, idempotent against the backfill.
- Merge into one canonical per-book timeline keyed on a surrogate `book_identities.id`, with explicit `source`/`confidence` tagging — never silently average or overwrite a high-confidence session-derived value with a lower-confidence Audible-derived one.
- Basic aggregate stats (totals, pace, top authors/narrators/genres).
- Time-of-day/day-of-week distribution — this genuinely does still require Audiobookshelf specifically; Audible's aggregates are day/month totals with no intra-day timestamp, so this FEATURES.md constraint holds even after the correction.
- CSV export in the LLM-shaped, tidy, multi-file, ISO-8601-dated form FEATURES.md specifies (sessions/books/daily_summary, `snake_case` + units in column names, explicit `source`/`confidence` columns, empty cells for "unknown" rather than `0`).

**Should have (differentiators):**
- Export purpose-built for LLM consumption rather than a human dashboard — no comparable tool (Audiobookshelf's own stats page, Trakt, ListenBrainz, StoryGraph) targets this.
- Explicit source/confidence/estimation-method provenance on every derived fact — now more important than the original research assumed, given that ~41% of Audible-era hours have no single unambiguous book attribution.
- Automated Day One daily-digest journal entries, skip-by-default on zero-activity days.
- Re-listen detection, series completion tracking, streak calculation — all P2, gated on P1 data being trustworthy first.

**Explicit anti-features (do not build):** charts/visual dashboards inside earworm; subjective mood/sentiment/taste-cluster tagging; real-time per-event webhook journal entries; treating any single-source signal as equal-confidence to a directly measured session; gamified streak notifications; per-event (rather than daily-digest) journal entries; a "nothing happened" filler entry on every zero-activity day by default; two-way sync with Day One.

### Architecture Approach

Four new packages (`internal/bookidentity`, `internal/listening`, `internal/journal`, plus new files in `internal/db`), two extended existing packages (`internal/audible`, `internal/audiobookshelf`), one new CLI surface (`stats.go`), and one daemon-cycle edit. One migration bundles all new tables, following the existing `005_plan_infrastructure.sql` precedent of shipping a feature area's full schema together even before every table is used.

**Major components:**
1. `internal/db` (book_identity.go, listening.go, journal.go) — all SQL for the new tables lives here, no exceptions, matching the codebase's existing hard convention.
2. `internal/bookidentity` — pure, I/O-free matching algorithm (normalize -> score -> resolve/merge); reusable by both ingestion-time assignment and later reconciliation, mirroring the existing `metadata`/`organize` split.
3. `internal/listening` — source adapters, watermark-driven incremental sync, rollup recompute, the day-level allocation/attribution algorithm (computed at read time, never materialized as a general fact table), and CSV export.
4. `internal/journal` — subprocess wrapper for `dayone`, `cmdFactory` injection identical in shape to `internal/audible`, no business logic.

**Schema, corrected:** identity uses a surrogate key (`book_identities.id`) with ASIN as an optional, best-effort attribute — not a primary key — which the empirical 20/11/30 identity split above confirms is necessary, not merely cautious. Upsert semantics are chosen per-table by mutability, **and the assignment must be corrected from ARCHITECTURE.md's draft**: Audiobookshelf `listening_sessions` are mutable and require `ON CONFLICT DO UPDATE` keyed on `source_session_id` with an `updated_at` watermark and >=36-hour overlap window; Audible's per-book facts (daily/monthly aggregates, finished-status, last-position, listening_status) are all current-state snapshots rather than event logs and should likewise use `DO UPDATE` semantics keyed on `(source, asin, bucket)`. Inference (the allocation algorithm) is computed on demand at export/journal time and is only persisted as a stamped, versioned snapshot inside a `journal_entries` ledger — for idempotency and audit, not as a reusable source of truth.

### Critical Pitfalls

1. **Two clocks, one calendar-day bucket.** Audiobookshelf's server-local `date`/`dayOfWeek` fields and Audible's bare date strings are not directly comparable. Derive every day bucket from the raw epoch/instant through one explicit, config-declared IANA timezone; never trust a source's pre-computed date field or `time.Local`. Established in Phase 1 (Audible, first ingestion pipeline), conformed to in Phase 2 (Audiobookshelf).
2. **Mutable-vs-immutable upsert assignment must be correct per source, not assumed.** This is the exact mistake ARCHITECTURE.md's draft made (Correction 3 above). Two explicit upsert helpers, documented with the mutability assumption in a comment, are non-negotiable — never one generic "upsert" for both table shapes.
3. **Bulk/migration-artifact timestamp clusters must be filtered before treating an Audible finish timestamp as a real event.** A naive "books finished per day" query will otherwise report dozens of false completions on a single migration date. Address in Phase 1 (ingestion/normalization layer), verify in Phase 3 (aggregation).
4. **Inferred attribution must never be materialized as if it were measured.** Given that only ~59% of Audible-era hours are attributable at all, and only 46% unambiguously, any table or export column carrying an allocated/estimated value needs an explicit `is_estimated`/`confidence` column propagated all the way to CSV and journal output. Compute at read time in Phase 3; freeze only into the Phase 4 journal ledger at the moment an entry is actually emitted.
5. **Automated writes to a real personal journal are a different risk class than a wrong CSV row.** Use `@dayone/cli`'s idempotent `--entry-id` (deterministic, e.g. a hash of `identity_id + entry_date`) rather than relying solely on read-back detection; default to a dry-run/preview mode until the user explicitly enables live writes; and treat the `dayone sync` step as a distinct, separately-failable operation (see Gaps) rather than assuming a successful `entry write` means the entry has reached the account.

## Implications for Roadmap

### Phase 1: Audible ingestion + shared conventions
**Rationale:** Audible is now verified, low-risk, and self-contained (native ASIN identity, no fuzzy matching required yet) — the corrected finding that it is not a spike makes it the natural first phase rather than the last. It is also the right place to establish conventions every later phase depends on: canonical timezone/day-bucket rule, `Clock` interface for testable time-dependent logic, the mutable-vs-immutable upsert pattern (correctly assigned this time), `cmdFactory` reuse for a new `audible api` wrapper, rate-limit/backoff reuse from `internal/download` for the endpoint's pagination limits (30-day, 12-month, 25-ASIN, `continuation_token` caps), and secrets-handling conventions (no tokens in argv/logs/fixtures).
**Delivers:** `008_listening_stats.sql` (all new tables), `internal/db/book_identity.go` + `listening.go` CRUD, `internal/audible` extended with methods for all four endpoints, normalized into shared `DailyTotal`/`BookStatus` structs, `book_identities` populated via trivial ASIN-native assignment (single source, no merge logic needed yet), sync watermark read/write, `earworm stats sync --source audible`.
**Addresses:** Table-stakes "rich per-book facts from Audible," historical backfill, daily incremental sync (Audible side).
**Avoids:** Pitfall 3 (timestamp cluster filtering), Pitfall 1 (canonical bucketing established here, not retrofitted), Pitfall 8 (resumable, rate-limited backfill).

### Phase 2: Audiobookshelf ingestion
**Rationale:** Introduces the harder mutable-record and paginated-admin-endpoint handling once the shared conventions from Phase 1 exist to conform to, and is the source that actually supplies time-of-day/day-of-week granularity.
**Delivers:** `internal/audiobookshelf/sessions.go` using `GET /api/sessions?user=<uuid>`, `DO UPDATE` upsert keyed on session id with a 36h+ overlap window, handling for `timeListening` arriving as float, and `POST /api/items/batch/get` enrichment for sessions with empty genre/series snapshots. `internal/listening` skeleton with the Audiobookshelf adapter and its own watermark.
**Uses:** `net/http` stdlib pagination pattern, `httptest.Server`-based tests extending `internal/audiobookshelf/client_test.go`.
**Implements:** the corrected mutable-session upsert pattern from Architecture (Pattern 2, corrected).

### Phase 3: Identity resolution + rollup + CSV export
**Rationale:** Both sources are now ingested reliably; this phase is read/reconciliation-only and carries no new ingestion-mutation risk. The empirical 20-exact/11-fuzzy/30-unmatched identity split from the verification sample should directly inform and validate the fuzzy-match threshold tuning here.
**Delivers:** `internal/bookidentity` (normalize/score/resolve/merge, with the merge path exercising the "two sources independently created two identities for the same book" case), `internal/listening/rollup.go` (measured-only, rebuildable), `internal/listening/attribution.go` (pure allocation function; given the near-zero, one-day-in-twelve-years time overlap between sources this is mostly straightforward concatenation plus one documented tie-break rule, with the day-level greedy-backward allocation only needed for Audible-only hours), `internal/listening/export.go` (multi-file CSV: sessions/books/daily_summary, tidy, ISO-8601, explicit `source`/`confidence`/`is_estimated` columns).
**Delivers:** `earworm stats export`.

### Phase 4: Day One journaling + daemon hook
**Rationale:** This is the only phase that writes to an external, user-facing, largely unstructured personal store — correctly sequenced last, after the data it summarizes is trustworthy.
**Delivers:** `internal/journal` (subprocess wrapper for `dayone`, `cmdFactory` pattern, tests via a fake factory), `journal_entries` ledger CRUD, `earworm stats journal [--date] [--force]` using deterministic `--entry-id` values for idempotency, an explicit `dayone sync` invocation as its own tracked/failable step, daily-digest cadence with skip-by-default on zero-activity days, and daemon integration gated behind a config flag with journal emission kept manual/dry-run by default (consistent with the project's existing "no destructive action runs unattended by default" posture for `cleanup`/`plan apply`).

### Phase Ordering Rationale

- Phase order follows the corrected risk profile, not the original architecture draft's assumption that Audible was the risky, deferrable source — it is the opposite once verified.
- Identity resolution is deferred to Phase 3 (after both sources are ingested) rather than attempted incrementally during Phase 1/2, because Phase 1's Audible-only data needs no fuzzy matching (native ASIN), and building the merge/fuzzy path against only one populated source would be untestable against the real cross-source discrepancy this milestone exists to solve.
- Journaling is last because it is the only phase whose failure mode (a corrupted or duplicated entry in a real personal journal) is categorically worse than a wrong CSV row or a stale export, and because it depends on the aggregation logic from Phase 3 being trustworthy.
- This ordering matches PITFALLS.md's own phase-to-pitfall mapping (which already assumed Audible-first), so no pitfall remediation needs re-sequencing — only ARCHITECTURE.md's "Build Order" section (which assumed Audiobookshelf-first, Audible-as-spike) is superseded and should not be used as written.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 4:** the `@dayone/cli` queued-outbox + explicit `sync` step is a two-phase-commit problem (write succeeds locally/queued, but `sync` can fail independently for auth/network reasons) that no source file has fully designed error handling for. Needs a dedicated research or design pass before implementation. Also needs a concrete decision on tagging given the confirmed absence of `--tags` in 0.21.1.
- **Phase 3:** the day-level allocation algorithm's tie-break rule for the single cross-source overlap day, and the fuzzy-match threshold, both benefit from validation against a real (redacted) sample before finalizing — treat the 20/11/30 split as a starting calibration point, not a guarantee it generalizes to every user's library shape.

Phases with standard patterns (skip research-phase):
- **Phase 1:** all four Audible endpoints, their pagination limits, and response shapes are now verified directly; this is implementation, not research.
- **Phase 2:** Audiobookshelf's admin sessions endpoint, mutability behavior, and float/empty-array quirks are verified directly; implementation can proceed against the corrected facts in this document without a further research pass.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Verified against pkg.go.dev/GitHub releases and direct repo inspection; the one soft spot is the `dayone` npm package's Node.js `engines` requirement, not independently verified. |
| Features | MEDIUM-HIGH | Table-stakes/differentiator/anti-feature analysis is sound; the underlying Audible-capability characterization it was built on has been corrected in this document — features depending on the corrected facts (richer Audible data, timestamp-cluster filtering) are new obligations, not gaps in the feature list itself. |
| Architecture | MEDIUM | Package decomposition, schema shape, and identity-resolution design are HIGH confidence and now empirically validated by the 20/11/30 identity split. The specific upsert-semantics assignment and the "Build Order" phase sequencing in the source file are both corrected in this document — do not implement ARCHITECTURE.md's Build Order or upsert table as originally written. |
| Pitfalls | MEDIUM-HIGH | Go-language and architecture guidance is HIGH confidence (verified against existing repo conventions). Journaling-specific pitfalls (13) were researched against `jrnl.sh`'s behavior model rather than the actual `@dayone/cli`; the underlying principles (idempotency, dry-run default, no destructive auto-writes) still hold, but the specific mechanism (`--entry-id` idempotency, queued outbox + separate `sync` step) is better and different from what was assumed, and the outbox/sync failure mode is a genuine gap (see below). Phase-to-pitfall mapping already matches the corrected phase order, so no remapping was needed there. |

**Overall confidence:** MEDIUM-HIGH

### Gaps to Address

- **Day One outbox/sync failure handling:** no source file designed for the case where `dayone entry write` succeeds (queued) but the subsequent `dayone sync` fails. Needs explicit status tracking in `journal_entries` (e.g. `queued` vs `synced` vs `sync_failed`) distinct from the existing `pending`/`emitted`/`failed` states, and a retry/reconciliation command. Address during Phase 4 planning.
- **Day One tagging:** FEATURES.md recommends tagging automated entries; the verified CLI (`@dayone/cli` v0.21.1) has no `--tags` flag. Needs a decision (inline `#hashtag` in body text, or defer tagging entirely) before Phase 4 implementation.
- **Node.js runtime requirement for `dayone`:** not independently verified against the npm package's `engines` field. Confirm before Phase 4 and add a PATH/version preflight check mirroring the existing `audible-cli` presence check.
- **Fuzzy-match threshold tuning:** the recommended tier-2 threshold (e.g. >=0.7 token-set similarity) is a starting point from STACK.md's design sketch, not tuned against the actual 11-book fuzzy-match cohort. Validate during Phase 3.
- **Audible aggregate mutability window:** ABS's 36-hour session auto-close is confirmed; the equivalent "how long can an Audible daily/monthly aggregate be revised after first appearing" window is not independently confirmed. Default to a conservative overlap window (e.g. 7 days, matching STACK/ARCHITECTURE's original suggestion) and validate empirically during Phase 1.

## Sources

### Primary (HIGH confidence — verified live during this research cycle, superseding conflicting claims in the four research files)
- Live `audible api` calls against `/1.0/stats/aggregates`, `/1.0/stats/status/finished`, `/1.0/annotations/lastpositions`, and `/1.0/library` with `response_groups=listening_status` — confirmed endpoint shapes, pagination/batch limits, and the finished-timestamp/migration-cluster behavior described above.
- Live Audiobookshelf server calls (verified server version 2.36.0) — confirmed `GET /api/sessions?user=<uuid>` pagination behavior vs. the in-memory `/api/me/listening-sessions` endpoint, session mutability, float `timeListening`, and empty-genre-snapshot behavior.
- Live `@dayone/cli` v0.21.1 invocation — confirmed `entry write` flags (`--journal-id`, `--body-stdin`, `--date`, `--entry-id`), idempotent-by-entry-id behavior, queued-outbox + explicit `sync` requirement, and absence of a `--tags` flag.
- Direct repo inspection: `internal/audible/`, `internal/audiobookshelf/`, `internal/db/`, `internal/goodreads/export.go`, `internal/download/` (rate limiting/backoff), `.planning/PROJECT.md`.

### Secondary (MEDIUM confidence)
- `.planning/research/STACK.md` — stack recommendations and do-not-add list are sound; its Day One CLI characterization is superseded by the primary verification above.
- `.planning/research/FEATURES.md` — table-stakes/differentiator/anti-feature/MVP analysis is sound; its Audible-capability characterization ("no history export, only percent_complete") is superseded by the primary verification above.
- `.planning/research/ARCHITECTURE.md` — package decomposition and schema shape are sound and now empirically validated; its upsert-semantics table and "Build Order (4 Phases)" section are superseded by the primary verification above and should not be implemented as written.
- `.planning/research/PITFALLS.md` — Go/architecture pitfalls and phase-to-pitfall mapping are sound and already assume the corrected (Audible-first) phase order; Pitfall 13's journaling risk model is based on `jrnl.sh` rather than the actual `@dayone/cli` and should be read with the corrections above in mind.

### Tertiary (LOW confidence, needs validation)
- `@dayone/cli`'s Node.js runtime `engines` requirement — not independently verified.
- Audible daily/monthly aggregate revision window — not independently verified; treat as unconfirmed and use a conservative default.

---
*Research completed: 2026-09-20*
*Ready for roadmap: yes*
