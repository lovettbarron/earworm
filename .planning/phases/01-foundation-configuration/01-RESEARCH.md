# Phase 1: Foundation & Configuration - Research

**Researched:** 2026-04-03
**Domain:** Go CLI project setup, Cobra/Viper, SQLite, config management
**Confidence:** HIGH

## Summary

Phase 1 is a greenfield Go project setup. The technology choices are locked (Go, Cobra, Viper, modernc.org/sqlite) and well-established in the Go ecosystem. The primary risks are version mismatches in CLAUDE.md (Cobra is v1.10.2, not v2.3.0; Viper is v1.21.0, not v1.11.0) and a missing Go installation on the development machine.

The core work is: initialize a Go module, wire up Cobra commands (`version`, `config`), configure Viper for YAML config at `~/.config/earworm/config.yaml`, set up SQLite with embedded migrations, and write comprehensive tests for the db and config layers.

**Primary recommendation:** Use stdlib `os.UserConfigDir()` for XDG-compliant config path resolution (no extra dependency needed). Use Go's `embed` package for SQL migrations. Keep the migration system simple (sequential numbered files, a `schema_versions` table) -- no need for goose/golang-migrate for this project's scale.

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Standard Go project layout -- `cmd/earworm/` for main entry point, `internal/` for private packages (`cli/`, `config/`, `db/`, `scanner/`, `audible/` stubbed for later phases)
- **D-02:** Go module path: `github.com/lovettbarron/earworm`
- **D-03:** YAML config file format, managed by Viper
- **D-04:** Config file location: `~/.config/earworm/config.yaml` (XDG-compliant)
- **D-05:** SQLite database lives alongside config: `~/.config/earworm/earworm.db`
- **D-06:** Phase 1 ships two commands: `earworm version` (build info) and `earworm config` (show/set/init subcommands)
- **D-07:** Informative output by default -- show useful context (what happened, key details). Add `--quiet` flag for silent mode.
- **D-08:** Embedded SQL migration files via Go `embed` package. A `schema_versions` table tracks applied migrations.
- **D-09:** Initial schema includes `books` table (ASIN primary key, title, author, status, local_path, timestamps) and `schema_versions` table -- ready for Phase 2 scanning.

### Claude's Discretion
- Specific config keys and defaults (library_path, staging_path, audible_cli_path, rate limit settings)
- Error message wording and formatting style
- Test file organization within packages
- README structure and content depth

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| LIB-03 | Library state persists in a local SQLite database (not on NAS mount) | modernc.org/sqlite with database/sql; DB at `~/.config/earworm/earworm.db`; embedded migrations |
| LIB-04 | User can configure the library root path (NAS mount or local directory) | Viper YAML config with `library_path` key; validated on access |
| CLI-01 | User interacts via clear CLI commands | Cobra command tree: root + `version` + `config` (with `show`/`set`/`init` subcommands) |
| CLI-02 | User can configure all settings via config file and/or CLI flags | Viper binds config file, env vars, and Cobra flags with automatic precedence |
| CLI-04 | README documents installation, setup (including audible-cli dependency), and all commands | Standard Go project README with install, config, and usage sections |
| TEST-01 | Unit tests for SQLite database layer (schema creation, CRUD operations, migrations) with table-driven tests | Go stdlib `testing` + testify/assert; in-memory SQLite for tests |
| TEST-02 | Unit tests for configuration loading and validation (config file parsing, flag binding, defaults) | Temp dir fixtures, Viper reset between tests |

</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go -- single binary distribution
- **CLI framework:** Cobra + Viper (locked)
- **Database:** modernc.org/sqlite (pure Go, no CGo)
- **Testing:** stdlib `testing` + testify/assert
- **Logging:** log/slog (stdlib)
- **Terminal output:** charmbracelet/lipgloss v2 (not needed in Phase 1 but noted for consistency)
- **File format:** M4A only for v1
- **Rate limiting:** Must include protections (Phase 4, but config keys should be defined now)
- **License:** MIT or Apache 2.0 -- must not copy/derive from Libation's GPL code

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.23+ | Language runtime | Project constraint. Needed for slog, embed, and modern stdlib features. |
| spf13/cobra | v1.10.2 | CLI command framework | De facto standard for Go CLIs. NOTE: CLAUDE.md lists v2.3.0 but Cobra v2 does not exist -- latest is v1.10.2 (Dec 2024). Import as `github.com/spf13/cobra`. |
| spf13/viper | v1.21.0 | Configuration management | Companion to Cobra. Handles YAML config, env vars, flag binding. NOTE: CLAUDE.md lists v1.11.0 but latest is v1.21.0 (Sep 2024). |
| modernc.org/sqlite | v1.36+ | SQLite database driver | Pure Go, no CGo. Uses database/sql interface. Enables single-binary cross-compilation. |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| stretchr/testify | v1.11.1 | Test assertions | All test files. Use `assert` for non-fatal and `require` for fatal assertions. |
| log/slog (stdlib) | Go 1.21+ | Structured logging | Internal logging. Text handler for CLI, JSON handler for debug. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled migrations | goose / golang-migrate | Overkill for this project. Earworm has few tables and simple schema. Embed + custom runner is ~50 lines of code. |
| os.UserConfigDir() | adrg/xdg | Extra dependency. stdlib covers the XDG case on Linux/macOS. On macOS returns `~/Library/Application Support` but CONTEXT.md locks `~/.config/earworm/` so we hardcode with `$HOME/.config` fallback. |

**CRITICAL version corrections for CLAUDE.md:**
- Cobra: v1.10.2, NOT v2.3.0 (v2 does not exist)
- Viper: v1.21.0, NOT v1.11.0
- Testify: v1.11.1, NOT v1.9+

**Installation:**
```bash
# Requires Go 1.23+ installed first
go mod init github.com/lovettbarron/earworm
go get github.com/spf13/cobra@v1.10.2
go get github.com/spf13/viper@v1.21.0
go get modernc.org/sqlite@latest
go get github.com/stretchr/testify@v1.11.1
```

## Architecture Patterns

### Recommended Project Structure
```
earworm/
├── cmd/
│   └── earworm/
│       └── main.go              # Entry point, minimal -- calls cli.Execute()
├── internal/
│   ├── cli/
│   │   ├── root.go              # Root command, global flags (--quiet, --config)
│   │   ├── version.go           # version subcommand
│   │   ├── config.go            # config subcommand (show, set, init)
│   │   └── cli_test.go          # CLI integration tests
│   ├── config/
│   │   ├── config.go            # Viper setup, defaults, validation
│   │   ├── paths.go             # Config/DB path resolution
│   │   └── config_test.go       # Config parsing/validation tests
│   └── db/
│       ├── db.go                # Open, Close, migration runner
│       ├── migrations/
│       │   └── 001_initial.sql  # Books table + schema_versions
│       ├── books.go             # Book CRUD operations (ready for Phase 2)
│       └── db_test.go           # Schema, CRUD, migration tests
├── .goreleaser.yaml             # Build configuration
├── go.mod
├── go.sum
├── LICENSE
├── CLAUDE.md
└── README.md
```

### Pattern 1: Cobra Command Registration
**What:** Each command is defined in its own file and registered in an `init()` or explicit registration function.
**When to use:** All CLI commands.
**Example:**
```go
// internal/cli/root.go
package cli

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var (
    cfgFile string
    quiet   bool
)

var rootCmd = &cobra.Command{
    Use:   "earworm",
    Short: "Audiobook library manager for Audible",
    Long:  `Earworm tracks your local audiobook library, downloads new books from Audible, and organizes them for Audiobookshelf.`,
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        return initConfig()
    },
}

func Execute() error {
    return rootCmd.Execute()
}

func init() {
    rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default $HOME/.config/earworm/config.yaml)")
    rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "suppress non-essential output")
}
```

### Pattern 2: Viper Config with Defaults
**What:** Set sensible defaults, bind to Cobra flags, load from YAML file.
**When to use:** Configuration initialization.
**Example:**
```go
// internal/config/config.go
package config

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/spf13/viper"
)

func SetDefaults() {
    viper.SetDefault("library_path", "")
    viper.SetDefault("staging_path", "")
    viper.SetDefault("audible_cli_path", "audible")  // assume on PATH
    viper.SetDefault("audiobookshelf.url", "")
    viper.SetDefault("audiobookshelf.token", "")
    viper.SetDefault("audiobookshelf.library_id", "")
    viper.SetDefault("download.rate_limit_seconds", 5)
    viper.SetDefault("download.max_retries", 3)
    viper.SetDefault("download.backoff_multiplier", 2.0)
}

func ConfigDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("cannot determine home directory: %w", err)
    }
    return filepath.Join(home, ".config", "earworm"), nil
}

func Validate() error {
    // library_path is not required until Phase 2 (scan)
    // Just ensure paths that ARE set are valid
    if p := viper.GetString("library_path"); p != "" {
        if _, err := os.Stat(p); err != nil {
            return fmt.Errorf("library_path %q does not exist: %w", p, err)
        }
    }
    return nil
}
```

### Pattern 3: Embedded SQL Migrations
**What:** SQL files embedded at compile time, applied sequentially on startup.
**When to use:** Database initialization and schema updates.
**Example:**
```go
// internal/db/db.go
package db

import (
    "database/sql"
    "embed"
    "fmt"
    "sort"
    "strings"

    _ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Open(dbPath string) (*sql.DB, error) {
    db, err := sql.Open("sqlite", dbPath)
    if err != nil {
        return nil, fmt.Errorf("open database: %w", err)
    }

    // Enable WAL mode for better concurrent read performance
    if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
        db.Close()
        return nil, fmt.Errorf("set WAL mode: %w", err)
    }

    if err := runMigrations(db); err != nil {
        db.Close()
        return nil, fmt.Errorf("run migrations: %w", err)
    }

    return db, nil
}

func runMigrations(db *sql.DB) error {
    // Create schema_versions table if not exists
    _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_versions (
        version INTEGER PRIMARY KEY,
        applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )`)
    if err != nil {
        return fmt.Errorf("create schema_versions: %w", err)
    }

    // Read and sort migration files
    entries, err := migrationFS.ReadDir("migrations")
    if err != nil {
        return fmt.Errorf("read migrations dir: %w", err)
    }
    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Name() < entries[j].Name()
    })

    for _, entry := range entries {
        if !strings.HasSuffix(entry.Name(), ".sql") {
            continue
        }
        // Parse version from filename (e.g., "001_initial.sql" -> 1)
        var version int
        fmt.Sscanf(entry.Name(), "%d_", &version)

        // Check if already applied
        var count int
        db.QueryRow("SELECT COUNT(*) FROM schema_versions WHERE version = ?", version).Scan(&count)
        if count > 0 {
            continue
        }

        // Read and execute migration
        content, err := migrationFS.ReadFile("migrations/" + entry.Name())
        if err != nil {
            return fmt.Errorf("read migration %s: %w", entry.Name(), err)
        }

        tx, err := db.Begin()
        if err != nil {
            return fmt.Errorf("begin tx for migration %d: %w", version, err)
        }

        if _, err := tx.Exec(string(content)); err != nil {
            tx.Rollback()
            return fmt.Errorf("execute migration %d: %w", version, err)
        }

        if _, err := tx.Exec("INSERT INTO schema_versions (version) VALUES (?)", version); err != nil {
            tx.Rollback()
            return fmt.Errorf("record migration %d: %w", version, err)
        }

        if err := tx.Commit(); err != nil {
            return fmt.Errorf("commit migration %d: %w", version, err)
        }
    }

    return nil
}
```

### Pattern 4: Version Command with GoReleaser ldflags
**What:** Build-time version injection via linker flags.
**When to use:** `earworm version` command.
**Example:**
```go
// cmd/earworm/main.go
package main

import (
    "fmt"
    "os"

    "github.com/lovettbarron/earworm/internal/cli"
)

// Set by goreleaser ldflags
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

func main() {
    cli.SetVersion(version, commit, date)
    if err := cli.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

### Anti-Patterns to Avoid
- **Global Viper state leaking into tests:** Always use `viper.Reset()` in test setup/teardown, or use Viper instances (not the global) for testability.
- **Opening DB without WAL mode:** SQLite defaults to journal mode which has worse concurrent read performance. Always set `PRAGMA journal_mode=WAL`.
- **Hardcoding `~` in paths:** Use `os.UserHomeDir()` -- tilde expansion is a shell feature, not a filesystem feature.
- **Using `sql.Open("sqlite3", ...)` driver name:** modernc.org/sqlite registers as `"sqlite"`, not `"sqlite3"` (that is mattn/go-sqlite3's driver name).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CLI argument parsing | Custom flag parser | Cobra | Handles subcommands, help generation, shell completions, flag binding |
| Config file loading | Custom YAML parser | Viper | Handles multiple formats, env var overrides, flag binding, defaults |
| SQL database access | Raw file I/O | database/sql + modernc.org/sqlite | Connection pooling, prepared statements, transaction support |
| Test assertions | Custom `if` checks | testify/assert + testify/require | Readable failure messages, deep equality, type-safe |

**Key insight:** Phase 1 is pure infrastructure glue. Every component has a well-established Go library. The custom code is only: migration runner (~50 lines), config validation, and command wiring.

## Common Pitfalls

### Pitfall 1: modernc.org/sqlite Driver Name
**What goes wrong:** `sql.Open("sqlite3", path)` fails silently or panics.
**Why it happens:** Developers assume the driver name matches mattn/go-sqlite3.
**How to avoid:** Always use `sql.Open("sqlite", path)` with modernc.org/sqlite.
**Warning signs:** "unknown driver" errors at runtime.

### Pitfall 2: Viper Global State in Tests
**What goes wrong:** Tests pass individually but fail when run together.
**Why it happens:** Viper uses a global singleton. Config values from one test leak into another.
**How to avoid:** Call `viper.Reset()` in test setup. Or better: pass a `*viper.Viper` instance rather than using the global.
**Warning signs:** Flaky tests, order-dependent test failures.

### Pitfall 3: Config Directory Creation
**What goes wrong:** First run fails because `~/.config/earworm/` does not exist.
**Why it happens:** Viper does not create directories. `earworm config init` must create the directory.
**How to avoid:** Use `os.MkdirAll(configDir, 0755)` before writing config or opening the database.
**Warning signs:** "no such file or directory" on first run.

### Pitfall 4: SQLite Database on NAS Mount
**What goes wrong:** Corruption, locking issues, or poor performance.
**Why it happens:** SQLite does not work reliably over network filesystems (NFS, SMB, CIFS).
**How to avoid:** Database path is hardcoded to local config directory (D-05). Never allow user to move it to NAS.
**Warning signs:** SQLITE_BUSY errors, database locked errors.

### Pitfall 5: Missing Go Installation
**What goes wrong:** Cannot build or test the project.
**Why it happens:** Go is not installed on this development machine (verified).
**How to avoid:** First task in the plan must be installing Go via Homebrew (`brew install go`).
**Warning signs:** `command not found: go`.

### Pitfall 6: CLAUDE.md Version Drift
**What goes wrong:** `go get` pulls wrong versions, or code uses APIs that don't exist.
**Why it happens:** CLAUDE.md lists Cobra v2.3.0 (does not exist) and Viper v1.11.0 (outdated).
**How to avoid:** Use verified versions from this research: Cobra v1.10.2, Viper v1.21.0, Testify v1.11.1.
**Warning signs:** Module resolution failures.

### Pitfall 7: Config Path on macOS
**What goes wrong:** `os.UserConfigDir()` returns `~/Library/Application Support` on macOS, not `~/.config`.
**Why it happens:** macOS has its own config directory convention.
**How to avoid:** Since D-04 locks the path to `~/.config/earworm/`, use `os.UserHomeDir()` + `filepath.Join(home, ".config", "earworm")` explicitly.
**Warning signs:** Config file created in wrong location.

## Code Examples

### Initial Migration SQL
```sql
-- migrations/001_initial.sql
-- Books table: tracks all known audiobooks
CREATE TABLE IF NOT EXISTS books (
    asin TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'unknown',
    local_path TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Index for status queries (used in download pipeline)
CREATE INDEX IF NOT EXISTS idx_books_status ON books(status);
```

### Config YAML Structure
```yaml
# ~/.config/earworm/config.yaml
library_path: /mnt/nas/audiobooks
staging_path: /tmp/earworm-staging
audible_cli_path: audible

audiobookshelf:
  url: http://nas:13378
  token: ""
  library_id: ""

download:
  rate_limit_seconds: 5
  max_retries: 3
  backoff_multiplier: 2.0
```

### Table-Driven Test Example
```go
// internal/db/db_test.go
func TestBookCRUD(t *testing.T) {
    db := setupTestDB(t)  // opens :memory: SQLite, runs migrations
    defer db.Close()

    tests := []struct {
        name    string
        book    Book
        wantErr bool
    }{
        {
            name: "insert valid book",
            book: Book{ASIN: "B08C6YJ1LS", Title: "Project Hail Mary", Author: "Andy Weir", Status: "unknown"},
        },
        {
            name:    "duplicate ASIN fails",
            book:    Book{ASIN: "B08C6YJ1LS", Title: "Duplicate", Author: "Test", Status: "unknown"},
            wantErr: true,
        },
        {
            name: "minimal fields",
            book: Book{ASIN: "B00DEKC9GK"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := InsertBook(db, tt.book)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}

func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := Open(":memory:")
    require.NoError(t, err)
    return db
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| mattn/go-sqlite3 (CGo) | modernc.org/sqlite (pure Go) | 2022+ | No C toolchain needed for cross-compilation |
| Custom flag parsing | Cobra v1.10+ | Ongoing | Shell completion, help generation, subcommands for free |
| Separate migration tool | Go embed + custom runner | Go 1.16 (embed) | Migrations compiled into binary, no external files needed |
| log package | log/slog | Go 1.21 | Structured logging in stdlib, no external dependency |

**Deprecated/outdated:**
- Cobra v2.3.0 (referenced in CLAUDE.md): Does not exist. Cobra is at v1.10.2.
- Viper v1.11.0 (referenced in CLAUDE.md): Outdated. Current is v1.21.0.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Everything | **NO** | -- | Install via `brew install go` |
| Python 3.9+ | audible-cli (runtime dep) | Yes | 3.14.3 | -- |
| audible-cli | Phase 3+ (not Phase 1) | No | -- | Not needed for Phase 1 |
| ffprobe | Phase 2+ metadata fallback | No | -- | Not needed for Phase 1 |
| goreleaser | Release builds | No | -- | Not needed for Phase 1 dev; install later |

**Missing dependencies with no fallback:**
- **Go 1.23+** -- Must be installed before any development. `brew install go` on macOS.

**Missing dependencies with fallback:**
- None for Phase 1. audible-cli and ffprobe are not needed until later phases.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + stretchr/testify v1.11.1 |
| Config file | None yet -- Wave 0 creates project structure |
| Quick run command | `go test ./...` |
| Full suite command | `go test -v -race -count=1 ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TEST-01 | SQLite schema creation, CRUD, migrations | unit | `go test -v -run TestDB ./internal/db/...` | Wave 0 |
| TEST-02 | Config file parsing, flag binding, defaults, validation | unit | `go test -v -run TestConfig ./internal/config/...` | Wave 0 |
| LIB-03 | DB persists state locally | unit | Covered by TEST-01 (db opens, writes, reads back) | Wave 0 |
| LIB-04 | Library root path configurable | unit | Covered by TEST-02 (config key exists, validated) | Wave 0 |
| CLI-01 | CLI commands work | integration | `go test -v -run TestCLI ./internal/cli/...` | Wave 0 |
| CLI-02 | Config via file and flags | integration | Covered by TEST-02 | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./...`
- **Per wave merge:** `go test -v -race -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/db/db_test.go` -- covers TEST-01 (schema, CRUD, migrations)
- [ ] `internal/config/config_test.go` -- covers TEST-02 (parsing, defaults, validation)
- [ ] `internal/cli/cli_test.go` -- covers CLI-01, CLI-02 (command execution)
- [ ] Go installation: `brew install go`
- [ ] `go mod init github.com/lovettbarron/earworm`

## Open Questions

1. **`earworm config set` key validation**
   - What we know: Viper allows setting arbitrary keys. We need to restrict to known keys.
   - What's unclear: Whether to validate against a hardcoded list or use Viper's `AllKeys()`.
   - Recommendation: Maintain a list of valid config keys in the config package. Reject unknown keys with a helpful error.

2. **Book status enum values**
   - What we know: D-09 says status defaults to 'unknown'.
   - What's unclear: Full set of status values needed across all phases.
   - Recommendation: Define initial set: `unknown`, `scanned`, `downloading`, `downloaded`, `organized`, `error`. Store as TEXT in SQLite. Validate in Go code, not DB constraints, for flexibility.

3. **CLAUDE.md version corrections**
   - What we know: CLAUDE.md has incorrect versions for Cobra, Viper, and Testify.
   - What's unclear: Whether to update CLAUDE.md as part of Phase 1.
   - Recommendation: Update CLAUDE.md with correct versions in Wave 0 or the first task.

## Sources

### Primary (HIGH confidence)
- [Cobra GitHub releases](https://github.com/spf13/cobra/releases) -- confirmed v1.10.2 as latest (Dec 2024), no v2 exists
- [Viper GitHub releases](https://github.com/spf13/viper/releases) -- confirmed v1.21.0 as latest (Sep 2024)
- [modernc.org/sqlite on pkg.go.dev](https://pkg.go.dev/modernc.org/sqlite) -- v1.36+, published Mar 2026, driver name is "sqlite"
- [stretchr/testify GitHub releases](https://github.com/stretchr/testify/releases) -- confirmed v1.11.1 as latest (Aug 2025)
- [GoReleaser ldflags cookbook](https://goreleaser.com/cookbooks/using-main.version/) -- version injection pattern

### Secondary (MEDIUM confidence)
- [Go database patterns 2026](https://tutorialq.com/dev/go/go-database-patterns) -- embed migration pattern
- [Goose embedded migrations](https://pressly.github.io/goose/blog/2021/embed-sql-migrations/) -- embed.FS pattern reference
- [Cobra + Viper CLI guide](https://dasroot.net/posts/2026/03/building-cli-applications-go-cobra-viper/) -- integration patterns
- [Cobra + GoReleaser version command](https://momosuke-san.medium.com/how-to-implement-a-version-command-in-a-cli-tool-built-with-cobra-and-goreleaser-e5b6dbafc6d0) -- ldflags pattern
- [os.UserConfigDir proposal](https://github.com/golang/go/issues/29960) -- XDG behavior on different platforms
- [os.UserConfigDir macOS issue](https://github.com/golang/go/issues/76320) -- confirms macOS returns ~/Library/Application Support, not ~/.config

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries are well-established, versions verified against GitHub releases and pkg.go.dev
- Architecture: HIGH -- standard Go project layout, patterns drawn from Cobra/Viper official docs and widely-adopted projects
- Pitfalls: HIGH -- verified through official docs (driver name, WAL mode, macOS config path) and direct environment probing (Go not installed)

**Research date:** 2026-04-03
**Valid until:** 2026-05-03 (30 days -- stable ecosystem, no expected breaking changes)
