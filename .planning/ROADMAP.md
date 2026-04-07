# Roadmap: Earworm

## Milestones

- ✅ **v1.0 MVP** — Phases 1-8 (shipped 2026-04-06) — [Archive](milestones/v1.0-ROADMAP.md)
- 🚧 **v1.1 Library Cleanup** — Phases 9-14 (in progress)

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

### 🚧 v1.1 Library Cleanup (In Progress)

**Milestone Goal:** Extend Earworm with safe, plan-based library cleanup capabilities for organizing non-Audible books, fixing metadata, and restructuring folders — all with zero destructive defaults.

- [ ] **Phase 9: Plan Infrastructure & DB Schema** - DB tables, plan CRUD, audit logger, and library_items tracking for non-ASIN content
- [ ] **Phase 10: Deep Library Scanner** - Full folder traversal with issue detection and DB persistence
- [ ] **Phase 11: Structural Operations & Metadata** - Flatten nested audio, write metadata.json sidecars, SHA-256 verification
- [ ] **Phase 12: Plan Engine & CLI** - Wire scan results into reviewable, executable plans with per-operation tracking
- [ ] **Phase 13: CSV Import & Guarded Cleanup** - CSV-to-plan bridge and separated deletion command with safety guards
- [ ] **Phase 14: Multi-Book Split & Claude Skill** - Content-based folder splitting and conversational plan creation

## Phase Details

### Phase 9: Plan Infrastructure & DB Schema
**Goal**: The database and core abstractions exist for plan-based library operations — plans, operations, audit logs, and path-keyed library items can be created, queried, and persisted
**Depends on**: Phase 8 (v1.0 complete)
**Requirements**: PLAN-01, SCAN-02, INTG-01
**Success Criteria** (what must be TRUE):
  1. User can create a named plan with typed action records (move, flatten, split, delete, write_metadata) via Go API
  2. Library items are tracked in a path-keyed DB table that represents non-ASIN content alongside existing books
  3. Every plan mutation (create, status change) produces an audit log entry with timestamp, before/after state, and success/failure
  4. Plan and operation records survive CLI restarts (DB-persisted with migration)
**Plans**: TBD

### Phase 10: Deep Library Scanner
**Goal**: Users can see everything in their library — not just ASIN-bearing folders — and understand what needs fixing
**Depends on**: Phase 9
**Requirements**: SCAN-01, SCAN-03
**Success Criteria** (what must be TRUE):
  1. User can run `earworm scan --deep` and see all folders in the library, including those without ASINs
  2. Detected issues (no_asin, nested_audio, multi_book, missing_metadata, wrong_structure, orphan_files, empty_dir, cover_missing) are displayed with severity and suggested action
  3. Scan issues are persisted in DB and survive CLI restarts
  4. Deep scan does not break or modify existing ASIN-based scan behavior
**Plans**: TBD

### Phase 11: Structural Operations & Metadata
**Goal**: The file operation primitives exist for plan execution — flatten nested directories, write metadata sidecars, and verify file integrity via SHA-256
**Depends on**: Phase 9
**Requirements**: FOPS-01, FOPS-02
**Success Criteria** (what must be TRUE):
  1. User can flatten a nested audio directory, moving all audio files up to the book folder level with SHA-256 verification
  2. User can write an Audiobookshelf-compatible metadata.json sidecar for a book folder without modifying any audio files
  3. All file moves are verified via SHA-256 hash comparison (source before, destination after) before source deletion
  4. Failed operations leave source files intact (no data loss on verification failure)
**Plans**: TBD

### Phase 12: Plan Engine & CLI
**Goal**: Users can go from scan results to reviewed, executed plans through CLI commands — the full scan-to-plan-to-apply workflow works end to end
**Depends on**: Phase 10, Phase 11
**Requirements**: PLAN-02, PLAN-03
**Success Criteria** (what must be TRUE):
  1. User can review a plan via CLI and see a human-readable diff showing source, destination, and operation type for every action
  2. User can apply a plan with `--confirm` and see per-operation progress with pass/fail status
  3. Plan application resumes from the last successful operation after interruption or failure
  4. Applied plans record SHA-256 hashes and per-operation status in the audit trail
  5. Plans default to dry-run (no mutation without explicit confirmation)
**Plans**: TBD

### Phase 13: CSV Import & Guarded Cleanup
**Goal**: Users can bridge manual spreadsheet analysis into the plan system and safely delete unwanted files through a separated, guarded command
**Depends on**: Phase 12
**Requirements**: PLAN-04, FOPS-03
**Success Criteria** (what must be TRUE):
  1. User can run `earworm plan import FILE.csv` and get a named plan created from CSV rows with validation feedback
  2. CSV import handles BOM, CRLF normalization, and reports row-level validation errors with line numbers
  3. User can run `earworm cleanup` with trash-dir default (not permanent deletion), double confirmation prompt, and full audit logging
  4. Cleanup command only processes delete operations from completed plans — it cannot delete arbitrary files
**Plans**: TBD

### Phase 14: Multi-Book Split & Claude Skill
**Goal**: Users can handle the hardest structural issue (multi-book folders) and optionally use Claude Code for conversational plan creation
**Depends on**: Phase 12
**Requirements**: FOPS-04, INTG-02
**Success Criteria** (what must be TRUE):
  1. User can split a multi-book folder into separate directories based on content detection (audio metadata comparison)
  2. Split operations use SHA-256 verification and produce audit trail entries like all other file operations
  3. Claude Code skill can orchestrate scan and plan creation through conversation, producing plans the user reviews before applying
  4. Claude Code skill never executes plans — only creates them for human review and application
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 9 → 10 → 11 → 12 → 13 → 14

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Foundation & Configuration | v1.0 | 3/3 | Complete | 2026-04-03 |
| 2. Local Library Scanning | v1.0 | 2/2 | Complete | 2026-04-03 |
| 3. Audible Integration | v1.0 | 3/3 | Complete | 2026-04-04 |
| 4. Download Pipeline | v1.0 | 4/4 | Complete | 2026-04-04 |
| 5. File Organization | v1.0 | 2/2 | Complete | 2026-04-05 |
| 6. Integrations & Polish | v1.0 | 3/3 | Complete | 2026-04-05 |
| 7. Fix Download→Organize Pipeline | v1.0 | 2/2 | Complete | 2026-04-06 |
| 8. Test Coverage & Doc Cleanup | v1.0 | 3/3 | Complete | 2026-04-06 |
| 9. Plan Infrastructure & DB Schema | v1.1 | 0/0 | Not started | - |
| 10. Deep Library Scanner | v1.1 | 0/0 | Not started | - |
| 11. Structural Operations & Metadata | v1.1 | 0/0 | Not started | - |
| 12. Plan Engine & CLI | v1.1 | 0/0 | Not started | - |
| 13. CSV Import & Guarded Cleanup | v1.1 | 0/0 | Not started | - |
| 14. Multi-Book Split & Claude Skill | v1.1 | 0/0 | Not started | - |
