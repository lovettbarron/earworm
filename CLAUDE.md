<!-- GSD:project-start source:PROJECT.md -->
## Project

**Earworm**

A CLI-driven audiobook library manager for Audible, built in Go. Earworm tracks an existing local audiobook library (typically on a NAS mount), downloads new books from Audible via `audible-cli`, and organizes them in a Libation-compatible file structure. It integrates with Audiobookshelf and Goodreads for a complete audiobook workflow.

**Core Value:** Reliably download and organize Audible audiobooks into a local library with zero manual intervention — fault-tolerant downloads, automatic organization, and seamless integration with Audiobookshelf.

### Constraints

- **Language**: Go — single binary distribution, good CLI ergonomics
- **audible-cli dependency**: Python must be available on the host for audible-cli subprocess calls
- **License**: MIT or Apache 2.0 — must not copy or derive from Libation's GPL code; only reference its file structure conventions
- **File format**: M4A only for v1
- **Rate limiting**: Must include protections against hammering Audible servers (backoff, request throttling)
- **Fault tolerance**: Downloads must survive interruptions, network failures, and partial downloads with clear recovery
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Recommended Stack
### Language & Runtime
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.23+ | Application language | Single binary distribution, excellent CLI ecosystem, strong subprocess management via os/exec, cross-platform. Project constraint. | HIGH |
### CLI Framework
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| spf13/cobra | v1.10.2 | Command structure & parsing | De facto standard for Go CLIs (kubectl, docker, gh all use it). Subcommand model maps directly to earworm's needs: `earworm scan`, `earworm download`, `earworm sync`. RunE pattern for proper error propagation. | HIGH |
| spf13/viper | v1.21.0 | Configuration management | Natural companion to Cobra. Handles config file (YAML/TOML), env vars, and flag binding in one place. Needed for Audiobookshelf URL/token, library paths, audible-cli path, rate limit settings. | HIGH |
### Database
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| modernc.org/sqlite | v1.36+ | Library state persistence | CGo-free pure Go SQLite. Enables true single-binary cross-compilation without requiring a C toolchain. Performance gap vs mattn/go-sqlite3 is irrelevant for a library catalog (hundreds to low thousands of rows, not millions). Eliminates the #1 cross-compilation headache in Go. | HIGH |
### Audio Metadata
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| dhowden/tag | latest (v0.0.0-20240417053459) | M4A/MP4 metadata reading | Pure Go, no CGo. Supports MP4/M4A (AAC, ALAC) metadata: title, album, artist, year, track, disc, genre, artwork extraction. 640+ GitHub stars, 346 downstream importers. Read-only which is fine -- earworm reads existing metadata, doesn't write it. | MEDIUM |
### HTTP Client
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| net/http (stdlib) | Go 1.23+ | Audiobookshelf API integration | Earworm makes very few HTTP calls (library scan trigger, possibly listing libraries). A full client library like Resty is overkill. The Audiobookshelf API is simple REST with Bearer token auth. stdlib net/http handles this in ~20 lines per endpoint. | HIGH |
### Subprocess Management
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| os/exec (stdlib) | Go 1.23+ | Wrapping audible-cli | Go's stdlib exec is purpose-built for this. exec.CommandContext for timeout control, StdoutPipe/StderrPipe for streaming output parsing, and clean process lifecycle management. No third-party library needed. | HIGH |
- `exec.CommandContext()` with context for cancellation and timeout
- Pipe stdout/stderr for real-time progress reporting
- Parse audible-cli's output for download progress, errors, and completion
- Handle exit codes for retry logic (network failures vs auth failures vs rate limits)
### Terminal UI / Output
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| charmbracelet/lipgloss | v2.x | Styled terminal output | Clean, styled CLI output without building a full TUI. Tables for library listings, colored status indicators, styled headers. Lightweight -- just output formatting, not a full framework. | HIGH |
| charmbracelet/bubbles | latest | Progress bars, spinners | Reusable components for download progress, scanning indicators. Can be used standalone without full Bubble Tea. | MEDIUM |
### Logging
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| log/slog (stdlib) | Go 1.21+ | Structured logging | Standard library, zero dependencies, good enough performance for a CLI tool. JSON output mode for machine parsing, text mode for human reading. Avoids adding zerolog/zap dependency for a tool that mostly communicates via styled terminal output, not log files. | HIGH |
### Testing
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| testing (stdlib) | Go 1.23+ | Unit and integration tests | Go's built-in testing is sufficient. Table-driven tests are idiomatic. | HIGH |
| testify/assert | v1.11.1 | Test assertions | `require` and `assert` packages reduce test boilerplate significantly. Near-universal in Go projects. | HIGH |
### Build & Distribution
| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| goreleaser | v2.x | Cross-platform builds & releases | Standard tool for Go binary distribution. Builds for darwin/linux amd64+arm64, creates GitHub releases with checksums. Config is simple YAML. | HIGH |
## External Dependencies (Not Go Libraries)
| Dependency | Required | Purpose | Notes |
|------------|----------|---------|-------|
| audible-cli | Yes | Audible authentication & downloads | Python package (pip/uv install). Earworm wraps this as subprocess. Must be on PATH or configured path. |
| Python 3.9+ | Yes | Runtime for audible-cli | Required by audible-cli. Not used by earworm directly. |
| ffprobe | Optional | Fallback metadata extraction | Part of ffmpeg. Useful fallback if dhowden/tag can't parse specific M4A variants. Likely already installed alongside audible-cli. |
## Alternatives Considered
| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| CLI framework | cobra | urfave/cli | Cobra has richer ecosystem, better auto-completion, more Go CLI projects use it. urfave/cli v2 is fine but Cobra is the standard. |
| SQLite driver | modernc.org/sqlite | mattn/go-sqlite3 | CGo requirement kills easy cross-compilation. Performance delta irrelevant for this workload. |
| SQLite driver | modernc.org/sqlite | crawshaw.io/sqlite | Less maintained, smaller community. modernc.org has wider adoption. |
| HTTP client | net/http (stdlib) | go-resty/resty v2 | Only 2-3 API endpoints. Dependency not justified. |
| HTTP client | net/http (stdlib) | go-resty/resty v3 | v3 is very new (Dec 2025). Unnecessary dependency regardless. |
| Config | viper | koanf | Viper integrates natively with Cobra. koanf is lighter but loses the Cobra synergy. |
| Logging | log/slog | zerolog | CLI tool doesn't need zero-allocation logging. slog is stdlib with no dependency. |
| Logging | log/slog | zap | Same reasoning. Zap's structured logging is great for servers, overkill here. |
| Terminal UI | lipgloss + bubbles | bubbletea | Full TUI framework is overkill for a run-and-exit CLI. Can upgrade later if needed. |
| Audio tags | dhowden/tag | sentriz/go-taglib | go-taglib uses WASM-compiled TagLib. Heavier, more complex. dhowden/tag is simpler and sufficient for read-only M4A metadata. |
| Audio tags | dhowden/tag | ffprobe shelling | Pure library preferable to subprocess for metadata. Keep ffprobe as fallback. |
## Complete Dependency List
# Initialize module
# Core dependencies
# Audio metadata
# Terminal output
# Testing
# Dev tools (install separately)
## Project Structure Convention
## Audiobookshelf API Notes
- **Auth:** Bearer token (user API token from Audiobookshelf settings)
- **Library scan:** `POST /api/libraries/{id}/scan` -- triggers a full library scan
- **Library list:** `GET /api/libraries` -- list all libraries (to find the right ID)
- **Base URL:** User-configured (e.g., `http://nas:13378`)
## Key Technical Decisions
## Sources
- [Cobra GitHub](https://github.com/spf13/cobra) -- v1.10.2 (actual latest)
- [Viper GitHub](https://github.com/spf13/viper) -- v1.21.0 (actual latest)
- [modernc.org/sqlite on pkg.go.dev](https://pkg.go.dev/modernc.org/sqlite) -- v1.36+, SQLite 3.51.3
- [dhowden/tag GitHub](https://github.com/dhowden/tag) -- 642 stars, MP4/M4A support confirmed
- [go-resty/resty releases](https://github.com/go-resty/resty/releases) -- v2.16.5 (considered, not recommended)
- [Audiobookshelf API Reference](https://api.audiobookshelf.org/) -- library scan endpoint confirmed
- [Audiobookshelf scan discussion](https://github.com/advplyr/audiobookshelf/discussions/1012) -- confirms `/api/libraries/{id}/scan`
- [audible-cli GitHub](https://github.com/mkb79/audible-cli) -- actively maintained, Python CLI
- [Go ecosystem trends 2025 (JetBrains)](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/)
- [slog vs zerolog comparison (Leapcell)](https://leapcell.io/blog/high-performance-structured-logging-in-go-with-slog-and-zerolog)
- [Lipgloss v2 on pkg.go.dev](https://pkg.go.dev/github.com/charmbracelet/lipgloss/v2)
- [SQLite CGo vs no-CGo benchmarks](https://github.com/multiprocessio/sqlite-cgo-no-cgo)
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

### Established Patterns
- **Project layout:** `cmd/earworm/` entry point, `internal/` for private packages
- **Database:** modernc.org/sqlite with driver name "sqlite" (NOT "sqlite3"), WAL mode enabled
- **Migrations:** Embedded SQL via `//go:embed migrations/*.sql`, sequential numbered files, schema_versions tracking table
- **Config:** Viper with YAML, config at ~/.config/earworm/config.yaml, DB at ~/.config/earworm/earworm.db
- **CLI:** Cobra commands in internal/cli/, one file per command, root has --quiet and --config flags
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests, viper.Reset() between config tests
- **Error handling:** Cobra RunE pattern, wrap errors with fmt.Errorf("context: %w", err)
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

### Package Structure
- `cmd/earworm/` -- Binary entry point, version ldflags
- `internal/cli/` -- Cobra command definitions (root, version, config)
- `internal/config/` -- Viper setup, defaults, validation, path resolution
- `internal/db/` -- SQLite database, migrations, Book CRUD
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
