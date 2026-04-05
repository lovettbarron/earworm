# Earworm

## What This Is

A CLI-driven audiobook library manager for Audible, built in Go. Earworm tracks an existing local audiobook library (typically on a NAS mount), downloads new books from Audible via `audible-cli`, and organizes them in a Libation-compatible file structure. It integrates with Audiobookshelf and Goodreads for a complete audiobook workflow.

## Core Value

Reliably download and organize Audible audiobooks into a local library with zero manual intervention — fault-tolerant downloads, automatic organization, and seamless integration with Audiobookshelf.

## Requirements

### Validated

- [x] Scan and index an existing local audiobook library (M4A files in Libation-compatible folder structure) — Validated in Phase 02: local-library-scanning
- [x] Track library state in SQLite (books, metadata, download status) — Validated in Phase 02: local-library-scanning
- [x] Authenticate with Audible via wrapped `audible-cli` subprocess — Validated in Phase 03: audible-integration
- [x] Check for newly available audiobooks in the Audible account — Validated in Phase 03: audible-integration
- [x] Download new audiobooks with fault-tolerant retry, rate limiting, and graceful recovery — Validated in Phase 04: download-pipeline
- [x] Organize downloaded files in Libation-compatible structure (cover art, metadata, M4A) — Validated in Phase 05: file-organization

### Active

(No active requirements — all v1 requirements validated)

### Recently Validated (Phase 06)
- [x] Polling capability for new book detection — Validated in Phase 06: integrations-polish (daemon mode)
- [x] Trigger Audiobookshelf library scan via API after downloads — Validated in Phase 06: integrations-polish
- [x] Goodreads integration via CSV export — Validated in Phase 06: integrations-polish
- [x] Clear CLI interface with good user communication during downloads — Validated in Phase 06: integrations-polish (README)
- [x] Documentation updated with each phase — Validated in Phase 06: integrations-polish (full README rewrite)

### Out of Scope

- GUI/desktop application — CLI-only for v1
- AAX/AAXC decryption within Earworm — delegate to audible-cli
- Streaming or playback — this is a library manager, not a player
- Multi-format support beyond M4A — v1 focuses on M4A only
- Direct Audible API implementation — wrap audible-cli instead
- Running natively on NAS hardware — targets desktop/server writing to NAS mount

## Context

- **Libation** (github.com/rmcrackan/Libation) is the existing tool being replaced. It's feature-rich but unreliable. Earworm replicates its file organization structure for compatibility but is not a fork or derivative.
- **audible-cli** (mkb79/audible-cli) handles Audible authentication and book downloading. Earworm wraps it as a subprocess, maintaining a clean process boundary.
- **Audiobookshelf** is the target media server. Integration is via its REST API (library scan trigger after downloads).
- **Goodreads** integration leverages existing open-source CLI tools that sync Audible libraries to Goodreads shelves.
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
| Go for language | Single binary, good CLI ecosystem, cross-platform | ✓ Validated Phase 01 |
| Wrap audible-cli as subprocess | Clean license boundary (permissive wrapper around Python tool), proven Audible auth | ✓ Validated Phase 03 |
| MIT/Apache license | Avoid GPL constraints; no Libation code copied, only file structure observed | — Pending |
| SQLite for library state | Embedded, queryable, crash-resilient, standard for CLI tools | ✓ Validated Phase 01-02 |
| Libation-compatible file structure | Maximum compatibility with existing libraries and workflows | ✓ Validated Phase 02 |
| API notify for Audiobookshelf | Trigger scan after downloads; Audiobookshelf handles its own metadata | — Pending |

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
*Last updated: 2026-04-04 after Phase 3 (Audible Integration) complete — audible-cli subprocess wrapper, `earworm auth`/`sync`/`download --dry-run` commands, SyncRemoteBook with local field preservation, new book detection, all tests passing*
