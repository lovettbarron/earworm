# Requirements: Earworm

**Defined:** 2026-04-07
**Core Value:** Reliably download and organize Audible audiobooks into a local library with zero manual intervention — fault-tolerant downloads, automatic organization, and seamless integration with Audiobookshelf.

## v1.1 Requirements

Requirements for Library Cleanup milestone. Each maps to roadmap phases.

### Scanning

- [x] **SCAN-01**: User can deep-scan all library folders (not just ASIN-bearing) and detect issues: no_asin, nested_audio, multi_book, missing_metadata, wrong_structure, orphan_files, empty_dir, cover_missing
- [x] **SCAN-02**: Library items are tracked in a path-keyed DB table so plans can reference non-Audible content
- [x] **SCAN-03**: Detected scan issues are persisted in DB with severity, category, and suggested action

### Plan Infrastructure

- [x] **PLAN-01**: User can create named plans with typed action records (move, flatten, split, delete, write_metadata) and per-action status tracking
- [x] **PLAN-02**: User can review a plan via CLI with human-readable diff showing what each action will do before applying
- [x] **PLAN-03**: User can apply a plan with SHA-256 verification, per-operation status tracking, resume on failure, and full audit trail
- [x] **PLAN-04**: User can import plans from CSV spreadsheets to bridge manual analysis into the plan system

### File Operations

- [x] **FOPS-01**: User can flatten nested audio directories, moving files up to the book folder level
- [x] **FOPS-02**: User can write Audiobookshelf-compatible metadata.json sidecars without modifying audio files
- [x] **FOPS-03**: User can run a guarded cleanup command with trash-dir default, double confirmation, and audit logging — separated from plan apply
- [x] **FOPS-04**: User can split multi-book folders into separate directories with content-based detection

### Integration

- [x] **INTG-01**: All plan operations produce a full audit trail with timestamps, before/after state, and success/failure
- [x] **INTG-02**: Claude Code skill enables conversational plan creation (not execution) via Claude Code

## v1.2 Requirements

Requirements for the Listening Stats & Journaling milestone. Each maps to roadmap phases 19-22.

### Audible Listening Ingestion

- [x] **STAT-01**: User can backfill complete Audible listening history (daily totals, monthly totals, per-book status events, last playback positions) into the local database with one command
- [x] **STAT-02**: Backfill is resumable — interrupting and re-running neither duplicates rows nor refetches already-stored ranges
- [x] **STAT-03**: Audible requests respect the API's batch limits automatically (30-day maximum per daily-stats window, 25 ASINs per last-positions call) and the configured rate limit
- [x] **STAT-04**: All listening days are bucketed using a single configured timezone, applied identically to every source
- [x] **STAT-05**: Bulk status-change clusters (many books sharing a sub-second timestamp) are detected and flagged so they are never treated as genuine finish events

### Audiobookshelf Listening Ingestion

- [x] **ABSL-01**: User can configure an Audiobookshelf URL, API key and user ID, and verify connectivity with a single command
- [x] **ABSL-02**: User can backfill the complete Audiobookshelf playback-session history into the local database
- [x] **ABSL-03**: Incremental sync fetches only sessions changed since the last watermark, using an overlap window, and updates mutable sessions in place rather than duplicating them
- [x] **ABSL-04**: Session records retain per-book listening seconds, start and update timestamps, and device information
- [x] **ABSL-05**: Genre and series data missing from session metadata snapshots is enriched from the library-items endpoint

### Identity Resolution

- [x] **IDNT-01**: Books from both sources resolve to a single identity keyed by a surrogate ID, with ASIN as an optional attribute rather than the primary key
- [x] **IDNT-02**: Books present in only one source are retained in full, never dropped for failing to match
- [x] **IDNT-03**: Every identity mapping records how it was matched and with what confidence
- [x] **IDNT-04**: User can review unmatched and low-confidence matches via the CLI

### Dataset Export

- [x] **EXPT-01**: User can export listening history as CSV files to a local directory
- [x] **EXPT-02**: Export produces normalized books, days and sessions files, plus an opt-in combined timeline file
- [x] **EXPT-03**: Every exported row carries its source and an explicit attribution quality
- [x] **EXPT-04**: Inferred daily attribution is computed at export time from measured facts, never stored as if it were measured
- [x] **EXPT-05**: Exports default to a local gitignored directory and are never transmitted anywhere

### Journaling & Automation

- [x] **JRNL-01**: User can generate a Markdown listening digest for any given date
- [x] **JRNL-02**: User can sync digests to Day One idempotently — re-running updates the existing entry instead of creating duplicates
- [x] **JRNL-03**: Journal sync defaults to dry-run and requires an explicit flag to write
- [x] **JRNL-04**: Only measured facts reach the journal: exact per-book sessions where available, filtered genuine finish events otherwise; inferred attribution is excluded
- [x] **JRNL-05**: Days with no listening produce no journal entry
- [x] **JRNL-06**: The daemon can run incremental stats sync on its poll cycle without overlapping runs

### Komga Reading

- [x] **KOMG-01**: User can configure a Komga URL and API key, and verify connectivity
- [x] **KOMG-02**: User can backfill read and in-progress books from Komga, with series and volume number
- [x] **KOMG-03**: Completions predating a configured cutoff are flagged as unreliable, retained in the export and excluded from journal entries
- [x] **KOMG-04**: A day with both listening and reading produces one combined journal entry
- [x] **KOMG-05**: Finished volumes produce finish entries on the same terms as finished audiobooks

## Future Requirements

Deferred to v1.2+. Tracked but not in current roadmap.

### Advanced Operations

- **ADV-01**: Duplicate detection and merging across library
- **ADV-02**: Format conversion support beyond M4A

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Audio tag writing | Risk of corrupting audio files; metadata.json sidecars are safer |
| Duplicate detection/merging | High complexity, ambiguous merge semantics |
| Format conversion | Scope explosion; v1 is M4A only |
| Plan execution via Claude Code skill | Safety — humans must explicitly apply plans |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SCAN-01 | Phase 10, Phase 17 | Complete (bridge gap closure in Ph17) |
| SCAN-02 | Phase 9 | Complete |
| SCAN-03 | Phase 10, Phase 17 | Complete (bridge gap closure in Ph17) |
| PLAN-01 | Phase 9, Phase 18 | Complete (frontmatter fix in Ph18) |
| PLAN-02 | Phase 12 | Complete |
| PLAN-03 | Phase 12, Phase 16 | Complete (draft promotion gap closure in Ph16) |
| PLAN-04 | Phase 13, Phase 16 | Complete (draft promotion gap closure in Ph16) |
| FOPS-01 | Phase 11, Phase 18 | Complete (checkbox fix in Ph18) |
| FOPS-02 | Phase 11, Phase 18 | Complete (metadata wiring in Ph18) |
| FOPS-03 | Phase 13 | Complete |
| FOPS-04 | Phase 14, Phase 16 | Complete (draft promotion gap closure in Ph16) |
| INTG-01 | Phase 9, Phase 18 | Complete (frontmatter fix in Ph18) |
| INTG-02 | Phase 14, Phase 17 | Complete (--json gap closure in Ph17) |
| SAFE-01 | Phase 15 | Complete |
| SAFE-02 | Phase 15 | Complete |
| SAFE-03 | Phase 15 | Complete |
| SAFE-04 | Phase 15 | Complete |
| SAFE-05 | Phase 15 | Complete |

| STAT-01 | Phase 19 | Complete |
| STAT-02 | Phase 19 | Complete |
| STAT-03 | Phase 19 | Complete |
| STAT-04 | Phase 19 | Complete |
| STAT-05 | Phase 19 | Complete |
| ABSL-01 | Phase 20 | Complete |
| ABSL-02 | Phase 20 | Complete |
| ABSL-03 | Phase 20 | Complete |
| ABSL-04 | Phase 20 | Complete |
| ABSL-05 | Phase 20 | Complete |
| IDNT-01 | Phase 21 | Complete |
| IDNT-02 | Phase 21 | Complete |
| IDNT-03 | Phase 21 | Complete |
| IDNT-04 | Phase 21 | Complete |
| EXPT-01 | Phase 21 | Complete |
| EXPT-02 | Phase 21 | Complete |
| EXPT-03 | Phase 21 | Complete |
| EXPT-04 | Phase 21 | Complete |
| EXPT-05 | Phase 21 | Complete |
| JRNL-01 | Phase 22 | Complete |
| JRNL-02 | Phase 22 | Complete |
| JRNL-03 | Phase 22 | Complete |
| JRNL-04 | Phase 22 | Complete |
| JRNL-05 | Phase 22 | Complete |
| JRNL-06 | Phase 22 | Complete |

| KOMG-01 | Phase 23 | Complete |
| KOMG-02 | Phase 23 | Complete |
| KOMG-03 | Phase 23 | Complete |
| KOMG-04 | Phase 23 | Complete |
| KOMG-05 | Phase 23 | Complete |

**v1.2 Coverage:**
- v1.2 requirements: 25 total
- Mapped to phases: 25
- Unmapped: 0

**Coverage:**
- v1.1 requirements: 13 total + 5 SAFE (phase-local)
- Mapped to phases: 18
- Unmapped: 0

---
*Requirements defined: 2026-04-07*
*Last updated: 2026-09-22 — Phase 23 (KOMG-01..05) complete*
