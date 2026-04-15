# Earworm

## What This Is

A CLI-driven audiobook library manager for Audible, built in Go. Earworm tracks an existing local audiobook library (typically on a NAS mount), downloads new books from Audible via `audible-cli`, and organizes them in a Libation-compatible file structure. It integrates with Audiobookshelf and Goodreads for a complete audiobook workflow.

## Core Value

Reliably download and organize Audible audiobooks into a local library with zero manual intervention — fault-tolerant downloads, automatic organization, and seamless integration with Audiobookshelf.

## Current State

**Shipped:** v1.1 Library Cleanup (2026-04-14)
**Previous:** v1.0 MVP (2026-04-06)
**Next milestone:** Not yet planned
**Codebase:** ~134k lines Go across 15+ packages, 83.2% test coverage
**Tech stack:** Go 1.23+, Cobra/Viper CLI, modernc.org/sqlite (pure Go, no CGo), charmbracelet/lipgloss
**Commands:** auth, sync, scan, status, download, organize, notify, goodreads, daemon, config, version, skip, plan, cleanup, split
**DB migrations:** 7 (schema_versions, books, audible metadata, download tracking, plan infrastructure, scan issues, operation metadata)

## Requirements

### Validated

- ✓ Scan and index local audiobook library by ASIN — v1.0
- ✓ View library state (books, metadata, download status) — v1.0
- ✓ SQLite persistence on local filesystem — v1.0
- ✓ Configurable library root path — v1.0
- ✓ Dry-run preview before downloads — v1.0
- ✓ JSON output from list/status commands — v1.0
- ✓ Authenticate with Audible via audible-cli subprocess — v1.0
- ✓ List and sync Audible library metadata — v1.0
- ✓ Detect new books not yet downloaded — v1.0
- ✓ Download M4A with cover art and chapter metadata — v1.0
- ✓ Rate-limited downloads with exponential backoff — v1.0
- ✓ Progress visibility (per-book and overall) — v1.0
- ✓ Interrupt recovery (resume from last incomplete book) — v1.0
- ✓ Failed download tracking and retry — v1.0
- ✓ Staging directory before library placement — v1.0
- ✓ Libation-compatible folder structure (Author/Title [ASIN]/) — v1.0
- ✓ Cross-filesystem moves (local to NAS) — v1.0
- ✓ Audiobookshelf scan trigger via REST API — v1.0
- ✓ Goodreads sync via CSV export — v1.0
- ✓ Daemon/polling mode for unattended operation — v1.0
- ✓ Comprehensive README with all commands — v1.0
- ✓ >80% test coverage across all packages — v1.0
- ✓ Deep library scanning (all folders, 8 issue types) — v1.1
- ✓ Plan infrastructure (plan→review→approve→apply DB-backed workflow) — v1.1
- ✓ Metadata application (write metadata.json sidecars, no audio modification) — v1.1
- ✓ Structural operations (flatten nested audio, SHA-256 verification) — v1.1
- ✓ Plan execution engine (dry-run default, resume-on-failure, audit trail) — v1.1
- ✓ CSV import for plan creation with flexible column aliases — v1.1
- ✓ Guarded cleanup command (trash-dir default, double confirmation) — v1.1
- ✓ Multi-book folder detection and split planning — v1.1
- ✓ Data safety hardening (fsync, SHA-256 cross-FS verification, idempotent resume) — v1.1
- ✓ Scan-to-plan bridge (scan issues → auto-plan creation) — v1.1
- ✓ Claude Code skill for conversational library management — v1.1

### Active

(None yet — define in next milestone)

### Out of Scope

- GUI/desktop application — CLI-only for v1
- AAX/AAXC decryption within Earworm — delegate to audible-cli
- Streaming or playback — this is a library manager, not a player
- Multi-format support beyond M4A — v1 focuses on M4A only
- Direct Audible API implementation — wrap audible-cli instead
- Running natively on NAS hardware — targets desktop/server writing to NAS mount
- Multi-service support (Libro.fm, etc.) — scope explosion; v1 is Audible-only
- Real-time webhook notifications — users can wrap CLI with their own scripts

## Milestone History

<details>
<summary>v1.1 Library Cleanup (shipped 2026-04-14)</summary>

Extended Earworm with safe, plan-based library cleanup capabilities for organizing non-Audible books, fixing metadata, and restructuring folders — all with zero destructive defaults. 11 phases, 24 plans, 172 commits.

See `.planning/milestones/v1.1-ROADMAP.md` for full details.
</details>

<details>
<summary>v1.0 MVP (shipped 2026-04-06)</summary>

Core audiobook library manager: scan, sync, download, organize, notify. 8 phases, 22 plans.

See `.planning/milestones/v1.0-ROADMAP.md` for full details.
</details>

## Context

- **Libation** (github.com/rmcrackan/Libation) is the existing tool being replaced. It's feature-rich but unreliable. Earworm replicates its file organization structure for compatibility but is not a fork or derivative.
- **audible-cli** (mkb79/audible-cli) handles Audible authentication and book downloading. Earworm wraps it as a subprocess, maintaining a clean process boundary. Auto-managed via embedded Python venv.
- **Audiobookshelf** is the target media server. Integration is via its REST API (library scan trigger after downloads).
- **Goodreads** integration leverages CSV export with exact Goodreads import format.
- The library typically lives on a NAS mount (e.g., SMB/NFS share) accessed from the machine running Earworm.
- Licensing is MIT/Apache (permissive). No Libation code is copied — only the file structure convention is replicated by observation.

## Constraints

- **Language**: Go — single binary distribution, good CLI ergonomics
- **audible-cli dependency**: Python must be available on the host for audible-cli subprocess calls
- **License**: MIT or Apache 2.0 — must not copy or derive from Libation's GPL code; only reference its file structure conventions
- **File format**: M4A only for v1
- **Rate limiting**: Must include protections against hammering Audible servers (backoff, request throttling)
- **Fault tolerance**: Downloads must survive interruptions, network failures, and partial downloads with clear recovery

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go for language | Single binary, good CLI ecosystem, cross-platform | ✓ Good — v1.0 |
| Wrap audible-cli as subprocess | Clean license boundary (permissive wrapper around Python tool), proven Audible auth | ✓ Good — v1.0 |
| MIT/Apache license | Avoid GPL constraints; no Libation code copied, only file structure observed | ✓ Good |
| SQLite for library state | Embedded, queryable, crash-resilient, standard for CLI tools | ✓ Good — v1.0 |
| modernc.org/sqlite (pure Go) | No CGo, true single-binary cross-compilation | ✓ Good — v1.0 |
| Libation-compatible file structure | Maximum compatibility with existing libraries and workflows | ✓ Good — v1.0 |
| API notify for Audiobookshelf | Trigger scan after downloads; Audiobookshelf handles its own metadata | ✓ Good — v1.0 |
| Separate download and organize steps | Clean pipeline: download to staging, organize to library, notify ABS | ✓ Good — validated after Phase 7 fix |
| Auto-managed audible-cli venv | User doesn't need to install audible-cli manually | ✓ Good — v1.0 |
| cmdFactory injection for subprocess testing | Avoids interface-based exec abstraction, simpler test seams | ✓ Good — v1.0 |
| Path-keyed library_items table | Tracks non-ASIN content alongside books; NormalizePath prevents duplicates | ✓ Good — v1.1 |
| Plan-based file operations | All structural changes go through plan→review→approve→apply; dry-run default | ✓ Good — v1.1 |
| Inline hashFileSHA256 to avoid import cycles | Duplicated hash helper in organize and planengine packages | ⚠️ Revisit — consider shared internal package |
| Operation metadata in JSON column | Flexible metadata storage on plan operations; CSV columns flow through to executor | ✓ Good — v1.1 |
| fsync before Close() in all copy paths | Prevents silent data loss on NAS write caches | ✓ Good — v1.1 |
| 3 actionable issue types for auto-planning | Only nested_audio, empty_dir, orphan_files, missing_metadata auto-plan; others need human judgment | ✓ Good — v1.1 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-14 after v1.1 milestone*
