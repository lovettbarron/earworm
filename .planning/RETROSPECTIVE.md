# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — MVP

**Shipped:** 2026-04-06
**Phases:** 8 | **Plans:** 22 | **Tasks:** 31

### What Was Built
- Complete CLI audiobook library manager with 12+ commands (auth, sync, scan, status, download, organize, notify, goodreads, daemon, config, version, skip)
- Fault-tolerant download pipeline with rate limiting, exponential backoff, two-stage signal handling, and crash recovery
- Libation-compatible file organization with cross-filesystem move support (local to NAS)
- Audiobookshelf REST API integration and Goodreads CSV export
- Daemon/polling mode for unattended operation
- Auto-managed audible-cli Python dependency via embedded venv

### What Worked
- **Data-layer-up build order** — strict dependency chain (foundation → scanning → integration → downloads → organization → polish) meant each phase had a solid foundation
- **TDD for pipeline components** — rate limiter, backoff, staging module all built test-first; caught edge cases early
- **Milestone audit cycle** — audit after Phase 6 caught the download→organize pipeline break (ORG-01/ORG-02) before shipping, leading to targeted Phase 7 fix
- **cmdFactory injection pattern** — simple test seam for subprocess testing without complex interface abstractions
- **Pure Go SQLite (modernc.org)** — eliminated CGo and cross-compilation complexity entirely

### What Was Inefficient
- **Download→organize pipeline break** — download pipeline originally moved files to library directly, bypassing the organize step. Caught by audit but required a full Phase 7 to fix. Could have been caught earlier with E2E flow testing from Phase 5
- **SUMMARY.md frontmatter gaps** — ~50% of plan summaries missing `requirements_completed` field; documentation discipline fell off in later phases
- **Double ABS scan in daemon** — both daemon.go and organize.go trigger ABS scan; redundant HTTP call per cycle (benign but wasteful)
- **Dead API surface accumulated** — InsertBook and RenameM4AFile exported but unused in production; should have been caught during code review

### Patterns Established
- Separate download (staging) and organize (library) as distinct pipeline steps
- ABS scan triggers from organize command, not download command
- Per-book error isolation: continue processing remaining books when one fails
- Metadata fallback chain: dhowden/tag → ffprobe → folder name parsing
- Test seams via function fields (verifyFunc, sleepFunc) rather than interfaces

### Key Lessons
1. **E2E flow tests from the earliest possible phase** — the pipeline break would have been caught in Phase 5 if there had been an integration test covering the full download→organize→notify flow
2. **Milestone audits are load-bearing** — the audit process caught both the pipeline break and the orphaned TEST-12 requirement, preventing a broken v1.0 ship
3. **Pure Go dependencies pay dividends** — modernc.org/sqlite and dhowden/tag eliminated all CGo, making builds and CI trivial
4. **4 days from zero to shipped CLI** — the GSD workflow's phase-based structure with clear success criteria kept scope tight and momentum high

### Cost Observations
- Sessions: ~17 plan executions tracked
- Average plan duration: ~4 minutes
- Notable: Most plans completed in 2-8 minutes; Phase 8 coverage work took longest (8min, 7min) due to test infrastructure setup

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Phases | Plans | Key Change |
|-----------|--------|-------|------------|
| v1.0 | 8 | 22 | Baseline — established phase-based workflow, audit-before-ship pattern |

### Cumulative Quality

| Milestone | Tests | Coverage | Go LOC |
|-----------|-------|----------|--------|
| v1.0 | 200+ | 83.2% | ~134k |

### Top Lessons (Verified Across Milestones)

1. Milestone audits catch integration gaps that per-phase verification misses
2. Separate staging and library paths for file operations — never move files directly in download step
