# Roadmap: Earworm

## Milestones

- ✅ **v1.0 MVP** — Phases 1-8 (shipped 2026-04-06) — [archive](milestones/v1.0-ROADMAP.md)
- ✅ **v1.1 Library Cleanup** — Phases 9-18.1 (shipped 2026-04-14) — [archive](milestones/v1.1-ROADMAP.md)
- 🚧 **v1.2 Listening Stats & Journaling** — Phases 19-23 (implemented, not yet audited or archived)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 1-8) — SHIPPED 2026-04-06</summary>

- [x] Phase 1: Foundation & Configuration (3/3 plans)
- [x] Phase 2: Local Library Scanning (2/2 plans)
- [x] Phase 3: Audible Integration (3/3 plans)
- [x] Phase 4: Download Pipeline (4/4 plans)
- [x] Phase 5: File Organization (2/2 plans)
- [x] Phase 6: Integrations & Polish (3/3 plans)
- [x] Phase 7: Fix Download→Organize Pipeline (2/2 plans)
- [x] Phase 8: Test Coverage & Doc Cleanup (3/3 plans)

</details>

<details>
<summary>✅ v1.1 Library Cleanup (Phases 9-18.1) — SHIPPED 2026-04-14</summary>

- [x] Phase 9: Plan Infrastructure & DB Schema (2/2 plans)
- [x] Phase 10: Deep Library Scanner (3/3 plans)
- [x] Phase 11: Structural Operations & Metadata (2/2 plans)
- [x] Phase 12: Plan Engine & CLI (2/2 plans)
- [x] Phase 13: CSV Import & Guarded Cleanup (2/2 plans)
- [x] Phase 14: Multi-Book Split & Claude Skill (2/2 plans)
- [x] Phase 15: Data Safety Hardening for NAS Ops (4/4 plans)
- [x] Phase 16: Plan Lifecycle — Draft Promotion (1/1 plan)
- [x] Phase 17: Scan-to-Plan Bridge & JSON Output (2/2 plans)
- [x] Phase 18: Metadata Wiring & Artifact Cleanup (2/2 plans)
- [x] Phase 18.1: CSV Metadata Flow & Format Flexibility (2/2 plans)

</details>

### v1.2 Listening Stats & Journaling (Phases 19-23)

- [x] Phase 19: Audible Listening Ingestion (1/1 plan)
- [x] Phase 20: Audiobookshelf Listening Ingestion (1/1 plan)
- [x] Phase 21: Identity Resolution & Dataset Export (1/1 plan)
- [x] Phase 22: Day One Journaling & Daemon Integration (1/1 plan)
- [x] Phase 23: Komga Reading Ingestion (1/1 plan)

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation & Configuration | v1.0 | 3/3 | Complete | 2026-04-03 |
| 2. Local Library Scanning | v1.0 | 2/2 | Complete | 2026-04-03 |
| 3. Audible Integration | v1.0 | 3/3 | Complete | 2026-04-04 |
| 4. Download Pipeline | v1.0 | 4/4 | Complete | 2026-04-04 |
| 5. File Organization | v1.0 | 2/2 | Complete | 2026-04-05 |
| 6. Integrations & Polish | v1.0 | 3/3 | Complete | 2026-04-05 |
| 7. Fix Download→Organize Pipeline | v1.0 | 2/2 | Complete | 2026-04-05 |
| 8. Test Coverage & Doc Cleanup | v1.0 | 3/3 | Complete | 2026-04-06 |
| 9. Plan Infrastructure & DB Schema | v1.1 | 2/2 | Complete | 2026-04-07 |
| 10. Deep Library Scanner | v1.1 | 3/3 | Complete | 2026-04-07 |
| 11. Structural Operations & Metadata | v1.1 | 2/2 | Complete | 2026-04-07 |
| 12. Plan Engine & CLI | v1.1 | 2/2 | Complete | 2026-04-10 |
| 13. CSV Import & Guarded Cleanup | v1.1 | 2/2 | Complete | 2026-04-10 |
| 14. Multi-Book Split & Claude Skill | v1.1 | 2/2 | Complete | 2026-04-11 |
| 15. Data Safety Hardening for NAS Ops | v1.1 | 4/4 | Complete | 2026-04-11 |
| 16. Plan Lifecycle — Draft Promotion | v1.1 | 1/1 | Complete | 2026-04-12 |
| 17. Scan-to-Plan Bridge & JSON Output | v1.1 | 2/2 | Complete | 2026-04-12 |
| 18. Metadata Wiring & Artifact Cleanup | v1.1 | 2/2 | Complete | 2026-04-12 |
| 18.1. CSV Metadata Flow & Format Flex | v1.1 | 2/2 | Complete | 2026-04-12 |
| 19. Audible Listening Ingestion | v1.2 | 1/1 | Complete | 2026-09-20 |
| 20. Audiobookshelf Listening Ingestion | v1.2 | 1/1 | Complete | 2026-09-20 |
| 21. Identity Resolution & Dataset Export | v1.2 | 1/1 | Complete | 2026-09-20 |
| 22. Day One Journaling & Daemon Integration | v1.2 | 1/1 | Complete | 2026-09-20 |
| 23. Komga Reading Ingestion | v1.2 | 1/1 | Complete | 2026-09-22 |

## Phase Details (v1.2)

### Phase 19: Audible Listening Ingestion

**Goal:** Pull the full Audible listening record into local SQLite and establish the shared conventions every later phase reuses.

**Requirements:** STAT-01, STAT-02, STAT-03, STAT-04, STAT-05

**Scope:**
- `internal/audible`: raw `audible api <endpoint>` caller with `-p` params, reusing the existing cmdFactory seam
- Four endpoints: `stats/aggregates` (daily, 30-day windows; monthly, 12-month windows), `stats/status/finished` (continuation-token pagination), `annotations/lastpositions` (25 ASINs per call), `library` with `listening_status`
- New package `internal/listening`: canonical day bucketing against a configured `time.Location`, `Clock` interface for testable time, bulk-cluster detection
- Migration 008: `listening_days`, `book_listening`, `stats_sync_state`
- CLI: `earworm stats backfill --source audible`, `earworm stats sync`

**Success criteria:**
1. A backfill run populates daily totals, per-book status events and last positions from a mocked audible-cli
2. Re-running backfill produces no duplicate rows and skips already-covered ranges
3. Batch limits are enforced in code, not left to the caller
4. Day bucketing is timezone-explicit and covered by tests that would fail under `time.Local`
5. A synthetic sub-second cluster of finish events is flagged rather than counted

---

### Phase 20: Audiobookshelf Listening Ingestion

**Goal:** Ingest exact per-book playback sessions from Audiobookshelf with correct mutable-record handling.

**Requirements:** ABSL-01, ABSL-02, ABSL-03, ABSL-04, ABSL-05

**Scope:**
- Expand `internal/audiobookshelf` beyond `ScanLibrary`: API-key auth, `GET /api/me`, `GET /api/sessions?user=<uuid>` paginated backfill, `POST /api/items/batch/get` enrichment
- Lenient JSON decoding for numeric fields that arrive as int, float or string
- Watermark sync on `updatedAt` with a 36-hour overlap; upsert sessions with `ON CONFLICT DO UPDATE`
- Migration 009: `listening_sessions`
- CLI: `earworm stats backfill --source abs`, `earworm config abs-check`

**Success criteria:**
1. Connectivity check reports server version and resolves the configured user
2. Backfill pages through a mocked multi-page session list and stores every session
3. A session whose `timeListening` grows between syncs updates in place rather than duplicating
4. A float-valued `timeListening` decodes without error
5. Sessions missing genres are enriched from a batch item fetch

---

### Phase 21: Identity Resolution & Dataset Export

**Goal:** Reconcile books across sources and produce the LLM-ready CSV dataset.

**Requirements:** IDNT-01, IDNT-02, IDNT-03, IDNT-04, EXPT-01, EXPT-02, EXPT-03, EXPT-04, EXPT-05

**Scope:**
- New package `internal/bookidentity`: title normalization (series prefixes, `(Unabridged)`, subtitles, punctuation) and token-set similarity; no new dependency
- Migration 010: `book_identities`, `book_identity_sources` with `match_method` and `match_confidence`
- New package `internal/statsexport`: greedy backward allocation computed at export time, three normalized CSVs plus opt-in `timeline.csv`
- CLI: `earworm stats export`, `earworm stats matches`

**Success criteria:**
1. Books matching by ASIN, by normalized title, and matching nothing all resolve to distinct identities with the correct recorded method
2. A single-source book survives export intact
3. Every exported row carries source and attribution columns
4. Allocation output is reproducible for the same input and is absent from the database
5. Export writes to a gitignored local directory by default

---

### Phase 22: Day One Journaling & Daemon Integration

**Goal:** Turn measured listening into journal entries safely, and run the whole pipeline unattended.

**Requirements:** JRNL-01, JRNL-02, JRNL-03, JRNL-04, JRNL-05, JRNL-06

**Scope:**
- New package `internal/journal`: Markdown digest formatter, deterministic entry IDs (SHA-256 truncated to 32 hex chars), `dayone` subprocess client using the cmdFactory seam
- Two entry kinds: exact per-book daily digests where session data exists, filtered genuine finish events otherwise
- Migration 011: `journal_entries` ledger for idempotency and audit
- Explicit `dayone sync` as its own failable step after writes
- Daemon hook with a single-flight guard
- CLI: `earworm stats journal [--date] [--write]`

**Success criteria:**
1. A digest renders correctly from stored sessions without touching the network
2. Re-running a journal sync for the same date updates the existing entry rather than adding one
3. Dry-run is the default; writing requires an explicit flag
4. No inferred attribution appears in any generated entry
5. A day with no listening produces no entry, and the daemon cannot start overlapping syncs

---

### Phase 23: Komga Reading Ingestion

**Goal:** Bring comics, manga and ebooks read via Komga into the same picture as
listening, so the journal and dataset cover reading as well.

**Requirements:** KOMG-01 .. KOMG-05

**Scope:**
- New `internal/komga`: API-key client, paginated `/api/v1/books` filtered by
  read status, `readProgress` decoding
- Ingest into the existing `book_listening` table with `source='komga'`; the
  schema already carries series, volume, completion timestamp and an
  unreliable-provenance flag, so no new table is needed
- A configured `komga.unreliable_before` date flags books whose completion
  timestamp came from a library migration rather than real reading. This cannot
  be inferred: measured against real data, migration re-marks and genuine reads
  are indistinguishable (69% vs 97% instant writes, 3.5 vs 2.5 minute spans)
- Journal day entries gain a Reading section alongside Listening, so a day with
  both produces one entry rather than two
- Finished books produce finish entries through the existing path, with flagged
  books excluded automatically

**Success criteria:**
1. Backfill stores read and in-progress books with series and volume number
2. Books completed before the configured cutoff are flagged, appear in the
   export, and are excluded from journal entries
3. A day with both listening and reading produces a single combined entry
4. Finished volumes produce finish entries without new journal code
5. Re-running backfill neither duplicates rows nor loses flags

