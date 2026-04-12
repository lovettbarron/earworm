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
| FOPS-02 | Phase 11, Phase 18 | Pending (metadata wiring in Ph18) |
| FOPS-03 | Phase 13 | Complete |
| FOPS-04 | Phase 14, Phase 16 | Complete (draft promotion gap closure in Ph16) |
| INTG-01 | Phase 9, Phase 18 | Complete (frontmatter fix in Ph18) |
| INTG-02 | Phase 14, Phase 17 | Complete (--json gap closure in Ph17) |
| SAFE-01 | Phase 15 | Complete |
| SAFE-02 | Phase 15 | Complete |
| SAFE-03 | Phase 15 | Complete |
| SAFE-04 | Phase 15 | Complete |
| SAFE-05 | Phase 15 | Complete |

**Coverage:**
- v1.1 requirements: 13 total + 5 SAFE (phase-local)
- Mapped to phases: 18
- Unmapped: 0

---
*Requirements defined: 2026-04-07*
*Last updated: 2026-04-07 after roadmap creation*
