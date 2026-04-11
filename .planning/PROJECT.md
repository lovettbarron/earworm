# Earworm

## What This Is

A CLI-driven audiobook library manager for Audible, built in Go. Earworm tracks an existing local audiobook library (typically on a NAS mount), downloads new books from Audible via `audible-cli`, and organizes them in a Libation-compatible file structure. It integrates with Audiobookshelf and Goodreads for a complete audiobook workflow.

## Core Value

Reliably download and organize Audible audiobooks into a local library with zero manual intervention — fault-tolerant downloads, automatic organization, and seamless integration with Audiobookshelf.

## Current State

**Shipped:** v1.0 MVP (2026-04-06)
**In progress:** v1.1 Library Cleanup — Phase 15 complete (Data Safety Hardening for NAS Operations, gap closure done)
**Codebase:** ~134k lines Go across 15 packages, 83.2% test coverage
**Tech stack:** Go 1.23+, Cobra/Viper CLI, modernc.org/sqlite (pure Go, no CGo), charmbracelet/lipgloss
**Commands:** auth, sync, scan, status, download, organize, notify, goodreads, daemon, config, version, skip, plan, cleanup, split

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

### Active

- ✓ Deep library scanning (all folders, issue detection) — Validated in Phase 10: Deep Library Scanner
- ✓ Plan infrastructure (plan→review→apply DB-backed workflow) — Validated in Phase 9: Plan Infrastructure & DB Schema
- ✓ Metadata application (write `metadata.json`, no audio modification) — Validated in Phase 11: Structural Operations & Metadata
- ✓ Structural operations (flatten nested audio, SHA-256 verification) — Validated in Phase 11: Structural Operations & Metadata
- ✓ Plan execution engine (review, apply with dry-run default, resume-on-failure) — Validated in Phase 12: Plan Engine & CLI
- ✓ CSV import for plan creation — Validated in Phase 13: CSV Import & Guarded Cleanup
- ✓ Guarded cleanup command (separated deletions, explicit confirmation) — Validated in Phase 13: CSV Import & Guarded Cleanup
- ✓ Execution logging and audit trail — Validated in Phase 9: Plan Infrastructure & DB Schema
- ✓ Multi-book folder detection and split planning — Validated in Phase 14: Multi-Book Split & Claude Skill
- ✓ Claude Code skill for conversational library cleanup — Validated in Phase 14: Multi-Book Split & Claude Skill
- ✓ Data safety hardening (fsync, SHA-256 verification, FlattenDir guard, audit logging, idempotent resume) — Validated in Phase 15: Data Safety Hardening for NAS Operations

### Out of Scope

- GUI/desktop application — CLI-only for v1
- AAX/AAXC decryption within Earworm — delegate to audible-cli
- Streaming or playback — this is a library manager, not a player
- Multi-format support beyond M4A — v1 focuses on M4A only
- Direct Audible API implementation — wrap audible-cli instead
- Running natively on NAS hardware — targets desktop/server writing to NAS mount
- Multi-service support (Libro.fm, etc.) — scope explosion; v1 is Audible-only
- Real-time webhook notifications — users can wrap CLI with their own scripts

## Current Milestone: v1.1 Library Cleanup

**Goal:** Extend Earworm with safe, plan-based library cleanup capabilities for organizing non-Audible books, fixing metadata, and restructuring folders — all with zero destructive defaults.

**Target features:**
- Deep library scanning (all folders, not just ASIN-bearing)
- Plan infrastructure (plan→review→apply→cleanup flow with DB persistence)
- Metadata application (`metadata.json` writes, no audio file modification)
- Structural operations (flatten nested audio, split multi-book folders with SHA-256 verification)
- CSV import (bridge manual analysis to plan system)
- Guarded cleanup command (only deletions, separated and explicit)
- Execution logging and audit trail
- Claude Code skill for conversational orchestration

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
*Last updated: 2026-04-11 after Phase 15 gap closure complete*
