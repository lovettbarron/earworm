# Project Research Summary

**Project:** Earworm - CLI Audiobook Library Manager
**Domain:** CLI audiobook library management (Audible ecosystem, NAS-targeted)
**Researched:** 2026-04-03
**Confidence:** HIGH

## Executive Summary

Earworm is a Go CLI tool that orchestrates audible-cli to download, organize, and manage Audible audiobook libraries, targeting users who run Audiobookshelf on a NAS. The expert approach for this kind of tool is clear: build a thin orchestration layer around audible-cli (which handles the hard problems of Audible auth, DRM, and download), persist all state in SQLite, and organize files into an Audiobookshelf-compatible folder structure. The Go ecosystem provides everything needed -- Cobra for CLI structure, a pure-Go SQLite driver for zero-CGo builds, and stdlib subprocess management for wrapping audible-cli. The stack is well-understood and low-risk.

The core differentiator is fault-tolerant batch downloads. Libation (the main competitor) is notorious for crashing mid-download on large libraries, corrupting its database, and requiring GUI interaction. Earworm's value proposition is simple: reliably download your entire Audible library to a NAS without babysitting. This means the download pipeline -- queue management, per-book state tracking, crash recovery, and rate limiting -- is the heart of the project and deserves the most design attention.

The critical risks are: (1) SQLite database corruption from being placed on a network filesystem (must store DB locally, never on NAS), (2) Audible rate limiting with undocumented thresholds (must use conservative defaults), (3) subprocess lifecycle issues when wrapping audible-cli from Go (zombie processes, pipe deadlocks), and (4) auth token mismanagement leading to account lockouts. All four are well-understood and preventable with the patterns documented in the architecture research. None are novel problems.

## Key Findings

### Recommended Stack

The stack is deliberately stdlib-heavy and CGo-free, producing a single static binary. External runtime dependency is limited to Python (for audible-cli), which sits behind a clean process boundary. See [STACK.md](STACK.md) for full details.

**Core technologies:**
- **Go 1.23+**: Project language -- single binary distribution, excellent CLI and subprocess ecosystem
- **Cobra + Viper**: CLI framework and config -- industry standard (kubectl, docker, gh use Cobra), config file + env var + flag binding
- **modernc.org/sqlite**: Pure Go SQLite -- eliminates CGo/cross-compilation headaches, performance irrelevant at this scale (hundreds of rows)
- **dhowden/tag + ffprobe fallback**: M4A metadata reading -- pure Go for common cases, ffprobe for edge cases
- **lipgloss + bubbles**: Terminal output styling and progress bars -- lightweight, no full TUI framework needed
- **stdlib (os/exec, net/http, log/slog)**: Subprocess management, HTTP client, logging -- sufficient for all needs, zero unnecessary dependencies

### Expected Features

See [FEATURES.md](FEATURES.md) for full feature landscape and competitor analysis.

**Must have (table stakes):**
- Audible authentication via audible-cli wrapper
- Library listing from Audible account (JSON export parsing)
- Local library scanning with ASIN matching
- SQLite state tracking for all library state
- Download with status tracking per book
- Rate limiting with configurable delays and exponential backoff
- Fault-tolerant batch downloads with queue-level resume (the differentiator)
- Libation-compatible folder organization (Author/Title [ASIN])
- Cover art download
- Progress reporting during downloads
- New book detection (diff remote vs local)

**Should have (add after core is stable):**
- Audiobookshelf scan trigger (single POST endpoint, low effort)
- Chapter file download (audible-cli flag)
- Dry-run mode
- JSON output mode for scripting

**Defer (v2+):**
- Headless/daemon mode with polling (requires all v1 features to be rock-solid)
- Goodreads sync (fragile Ruby gem dependency)
- Multi-format conversion (ffmpeg rabbit hole)
- Multi-service support beyond Audible

### Architecture Approach

Layered CLI architecture with clear separation: CLI layer (Cobra commands) calls into service layer (Library, Download, Organizer, Integration services), which depends on infrastructure layer (SQLite store, subprocess manager, HTTP client, filesystem scanner). Services never import each other -- the CLI layer coordinates. See [ARCHITECTURE.md](ARCHITECTURE.md) for full details including database schema and data flow diagrams.

**Major components:**
1. **Store (SQLite)** -- Persistent state for books, download status, integration config. Local filesystem only.
2. **Subprocess Manager (audible-cli wrapper)** -- Typed Go interface over audible-cli commands. All subprocess interaction isolated here.
3. **Download Service** -- Queue management, rate limiting (token bucket), retry with exponential backoff, state machine per book.
4. **Library Service** -- Local filesystem scanning, remote library sync, reconciliation (what is new, what is downloaded).
5. **Organizer Service** -- Staging directory to final library path. Atomic moves. Audiobookshelf-compatible naming.
6. **Integration Service** -- Audiobookshelf scan trigger, future Goodreads sync.

### Critical Pitfalls

See [PITFALLS.md](PITFALLS.md) for full analysis of 11 pitfalls.

1. **SQLite on network filesystem** -- Silent corruption, broken locking. Store DB in `~/.config/earworm/`, never on NAS. Add startup detection for network mount paths.
2. **Audible rate limiting** -- Undocumented thresholds, account ban risk. Default to 30-60 second delays between downloads, circuit breaker after consecutive failures.
3. **Subprocess lifecycle** -- Zombie processes, pipe deadlocks, orphans surviving crashes. Use process groups, concurrent pipe reads, context timeouts, signal forwarding.
4. **Auth token mismanagement** -- 60-minute token expiry, device slot pollution. Register once, persist locally, refresh proactively, never auto-deregister.
5. **Large file NAS writes** -- Network timeout mid-write, partial files. Download to local staging first, copy-then-verify to NAS.

## Implications for Roadmap

Based on combined research, the build order follows a clear dependency chain. Each phase produces something usable and validates assumptions before building on them.

### Phase 1: Foundation and Data Model
**Rationale:** Everything depends on the data model, configuration, and database. Getting the SQLite schema right (and on local filesystem) prevents the most critical pitfall.
**Delivers:** Go module, project structure, domain types, SQLite schema with migrations, Viper-based configuration loading, CLI skeleton with Cobra (root + config commands).
**Addresses:** SQLite state tracking, configurable output directory
**Avoids:** Pitfall 1 (SQLite on NAS -- enforce local DB path from day one), Pitfall 9 (Python environment detection -- add audible-cli detection in config/setup)

### Phase 2: Local Library Scanning
**Rationale:** Immediately useful even without downloads -- users can index their existing Libation/audible-cli library. Validates the data model against real files. Low risk, well-understood filesystem patterns.
**Delivers:** Filesystem scanner, ASIN extraction from folder names, metadata reading (dhowden/tag + ffprobe fallback), book record upsert, `earworm scan` and `earworm status` commands.
**Addresses:** Local library scanning, metadata preservation, Libation-compatible folder structure recognition
**Avoids:** Pitfall 6 (Libation compatibility -- validate folder parsing against real Libation output), Pitfall 7 (metadata complexity -- read-only, defer writes)

### Phase 3: Audible-cli Integration
**Rationale:** Hard prerequisite for downloads. The subprocess wrapper is the most architecturally sensitive component -- it must handle process lifecycle, output parsing, and auth correctly before download orchestration is built on top.
**Delivers:** audible-cli subprocess wrapper with typed interface, auth management (wrap `audible quickstart`), library export parsing (JSON), `earworm sync` command (discover new books).
**Addresses:** Audible authentication, library listing from Audible, new book detection
**Avoids:** Pitfall 2 (auth token mismanagement -- register once, persist locally), Pitfall 4 (subprocess lifecycle -- process groups, context timeouts, concurrent pipe reads)

### Phase 4: Download Pipeline
**Rationale:** Core value proposition. This is the most complex phase and the primary differentiator. Depends on Phase 3 (subprocess wrapper) and Phase 1 (state tracking). Rate limiting and fault tolerance must be built in from the start, not bolted on.
**Delivers:** Download queue with state machine, rate limiter (token bucket), retry with exponential backoff + jitter, crash recovery (resume from last incomplete book), progress reporting, `earworm download` command.
**Addresses:** Fault-tolerant batch downloads, rate limiting/backoff, download status tracking, progress reporting, cover art download
**Avoids:** Pitfall 3 (rate limiting -- conservative defaults, circuit breaker), Pitfall 5 (NAS writes -- download to local staging), Pitfall 11 (crash recovery -- state machine with verified transitions)

### Phase 5: File Organization
**Rationale:** Depends on Phase 4 output (downloaded files). Conceptually simple but important for Audiobookshelf compatibility. Staging-to-library move pattern prevents partial states.
**Delivers:** Path template computation, atomic file moves (staging to library), Audiobookshelf-compatible naming (`Author/Series/Seq - Title [ASIN]/`), `earworm organize` command.
**Addresses:** Libation-compatible folder structure, configurable output directory
**Avoids:** Pitfall 5 (NAS writes -- copy-then-verify), Pitfall 6 (Libation compatibility drift -- template system with configurable defaults)

### Phase 6: External Integrations and Polish
**Rationale:** Additive features that enhance the tool but are not required for core functionality. The tool is fully usable after Phase 5.
**Delivers:** Audiobookshelf scan trigger, chapter file download, dry-run mode, JSON output mode, CLI polish (help text, shell completions).
**Addresses:** Audiobookshelf scan trigger, chapter file download, dry-run mode, JSON output
**Avoids:** Pitfall 10 (Audiobookshelf API assumptions -- fire-and-forget with optional polling, graceful failure)

### Phase Ordering Rationale

- **Dependency chain is strict:** Store must exist before scanning, subprocess wrapper before downloads, downloads before organization.
- **Phase 2 (scanning) before Phase 3 (audible-cli):** Scanning is simpler, validates the data model, and is immediately useful for existing libraries. It also defers the subprocess complexity until the foundation is proven.
- **Phase 4 (downloads) is isolated as its own phase:** It is the most complex and highest-risk component. It deserves focused attention without being mixed with other concerns.
- **Phase 6 is "nice to have":** The tool delivers full value after Phase 5. Phase 6 is polish and integrations that can ship incrementally.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 3 (Audible-cli Integration):** audible-cli's exact output formats, error codes, and auth flow edge cases need hands-on exploration. Documentation is sparse for automation use cases.
- **Phase 4 (Download Pipeline):** Rate limiting thresholds are undocumented. Will need empirical testing with a small library to calibrate safe defaults. Crash recovery logic needs careful state machine design.

Phases with standard patterns (skip deep research):
- **Phase 1 (Foundation):** Standard Go project setup, Cobra/Viper, SQLite schema. Well-documented everywhere.
- **Phase 2 (Local Scanning):** Filesystem walking and metadata reading are straightforward Go patterns.
- **Phase 5 (File Organization):** File move/copy operations with path templating. Standard patterns.
- **Phase 6 (Integrations):** Audiobookshelf API is simple REST. One endpoint.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All libraries are mature, well-documented, and widely used. Pure-Go constraint is achievable. |
| Features | HIGH | Competitor analysis is thorough. Libation's weaknesses are well-documented in GitHub issues. MVP scope is clear. |
| Architecture | HIGH | Layered CLI architecture is a standard Go pattern (kubectl, Terraform). Data flow is straightforward. |
| Pitfalls | MEDIUM-HIGH | Critical pitfalls (SQLite on NAS, subprocess lifecycle) are well-documented. Rate limiting thresholds are unknown (LOW confidence on specifics). |

**Overall confidence:** HIGH

### Gaps to Address

- **Audible rate limit thresholds:** No public documentation. Must use conservative defaults and tune empirically. Start with 30-60 second delays, measure against a small library.
- **dhowden/tag M4A edge cases:** Library may not handle all Audible-specific M4A atoms. Plan ffprobe fallback from the start; do not depend solely on dhowden/tag.
- **Libation folder structure defaults:** Libation's naming template defaults are not version-pinned in documentation. Validate against actual Libation output on disk, not assumed conventions.
- **audible-cli output stability:** audible-cli's JSON export format and error output are not formally versioned. The subprocess wrapper should be defensive (handle missing/extra fields).
- **M4B vs M4A file extensions:** Libation uses .m4b; audible-cli produces .m4a/.aaxc. Earworm must handle both when scanning existing libraries.

## Sources

### Primary (HIGH confidence)
- [Cobra CLI framework](https://github.com/spf13/cobra) -- CLI structure, v2.3.0
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) -- Pure Go SQLite, v1.36+
- [SQLite Over a Network](https://sqlite.org/useovernet.html) -- NAS pitfall documentation
- [Audiobookshelf API](https://api.audiobookshelf.org/) -- Library scan endpoint
- [audible-cli GitHub](https://github.com/mkb79/audible-cli) -- Subprocess target, CLI interface
- [Audible Authentication Docs](https://audible.readthedocs.io/en/latest/auth/authentication.html) -- Token lifecycle

### Secondary (MEDIUM confidence)
- [Libation GitHub issues](https://github.com/rmcrackan/Libation/issues) -- Crash reports validating fault-tolerance differentiator
- [Libation Naming Templates](https://getlibation.com/docs/features/naming-templates) -- Folder structure conventions
- [dhowden/tag](https://github.com/dhowden/tag) -- M4A metadata reading capabilities
- [Go exec.Command lifecycle](https://segmentfault.com/a/1190000041466423/en) -- Subprocess pitfall documentation

### Tertiary (LOW confidence)
- Audible rate limit behavior -- inferred from community experience, no official documentation
- Libation default naming template specifics -- user-configurable, version-dependent

---
*Research completed: 2026-04-03*
*Ready for roadmap: yes*
