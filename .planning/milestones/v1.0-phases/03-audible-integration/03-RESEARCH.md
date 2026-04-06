# Phase 3: Audible Integration - Research

**Researched:** 2026-04-04
**Domain:** audible-cli subprocess wrapping, Go os/exec, SQLite schema migration, CLI command structure
**Confidence:** HIGH

## Summary

Phase 3 wraps audible-cli as a subprocess to authenticate with Audible, sync library metadata into the local SQLite database, detect new (undownloaded) books, and provide dry-run output. No actual downloading occurs -- that is Phase 4.

The core technical challenges are: (1) building a robust subprocess wrapper around audible-cli using Go's os/exec with proper stdin pass-through for interactive auth, (2) parsing audible-cli's JSON export output into Go structs, (3) extending the existing database schema with new Audible-specific columns via migration 003, and (4) implementing the UpsertBook sync logic where remote metadata overwrites local metadata but local-only fields (status, local_path) are preserved.

The Go testing ecosystem supports subprocess testing via the well-established "TestHelperProcess" re-exec pattern from the standard library. This pattern lets tests spawn the test binary itself as a fake subprocess, enabling realistic testing of command construction, output parsing, and error handling without requiring audible-cli to be installed.

**Primary recommendation:** Build `internal/audible/` package with interface-based design so the wrapper can be swapped with a fake implementation in tests. Use audible-cli's `library export --format json` for structured data parsing rather than scraping `library list` text output.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Earworm manages auth directly -- `earworm auth` wraps `audible quickstart` to guide the user through Audible login interactively. Earworm stores/references the audible-cli profile path in its own config.
- **D-02:** Structured subprocess invocation via `os/exec`. Build commands programmatically, parse stdout/stderr line-by-line, map exit codes to typed Go errors. Each audible-cli command gets a dedicated Go wrapper function.
- **D-03:** Full wrapper inventory for Phase 3: wrap `quickstart` (auth), `library list` (sync), `library export` (richer metadata), and `download` (stubbed interface for Phase 4). Build the complete wrapper interface now even though download isn't used until Phase 4.
- **D-04:** Extend the existing `books` table with new columns for remote metadata: audible_status, purchase_date, runtime_minutes, narrators, series_name, series_position. A book can exist as local-only, remote-only, or both (matched on ASIN).
- **D-05:** Remote wins on sync -- remote metadata always overwrites local metadata fields. Audible is the source of truth for title, author, narrators, etc. Local-only fields (status, local_path) are preserved and never overwritten by sync.
- **D-06:** Always full sync -- every `earworm sync` pulls the complete Audible library and upserts all books. Simple, reliable, handles deletions. audible-cli doesn't support incremental fetching anyway.
- **D-07:** A book is "new" (needs download) if its ASIN exists in the Audible library but has no local_path or status is not 'downloaded'/'organized'. Simple set difference after sync.
- **D-08:** Dry-run is triggered via `earworm download --dry-run`. Shows each book that would be downloaded: Author - Title [ASIN] (runtime). Total count at the end. Respects `--json` flag for machine-readable output.
- **D-09:** `earworm auth` uses pass-through interactive mode -- invokes `audible quickstart` with stdin/stdout connected to the terminal. User interacts directly with audible-cli's prompts. Earworm wraps the process but doesn't intercept the interactive flow.
- **D-10:** Auth failures during sync are detected by parsing audible-cli error output. Show clear guidance: "Authentication expired. Run `earworm auth` to re-authenticate." Abort sync cleanly.
- **D-11:** Pre-flight auth check before every sync -- run a lightweight audible-cli command to verify auth is valid before starting the full sync. Fail fast with guidance rather than discovering auth issues mid-sync.

### Claude's Discretion
- Specific audible-cli output parsing patterns (varies by audible-cli version)
- Which lightweight command to use for pre-flight auth check
- Schema migration numbering and column types for new metadata fields
- Error message wording and formatting
- Test fixture design for fake subprocess testing

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AUD-01 | User can authenticate with Audible via wrapped audible-cli subprocess | D-01, D-09: `earworm auth` wraps `audible quickstart` with stdin/stdout pass-through. Auth flow research in audible-cli quickstart section. |
| AUD-02 | User can list all books in their Audible account | D-03: `library list` wrapper for display, `library export --format json` for structured data |
| AUD-03 | User can sync their Audible library metadata to the local database | D-04, D-05, D-06: Full sync with upsert, remote-wins strategy, new schema columns |
| AUD-04 | User can detect new books available in Audible but not yet downloaded locally | D-07: Set difference query -- ASIN in remote but no local_path or status not downloaded/organized |
| LIB-05 | User can preview what would be downloaded without downloading (dry-run mode) | D-08: `earworm download --dry-run` with formatted and --json output |
| TEST-05 | Unit tests for audible-cli subprocess wrapper (command construction, output parsing, error handling) | TestHelperProcess re-exec pattern + interface-based mocking |
| TEST-06 | Integration tests for Audible sync flow (auth validation, library metadata sync, new book detection) | In-memory SQLite + fake audible wrapper for end-to-end sync testing |
</phase_requirements>

## Standard Stack

### Core (already in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| os/exec (stdlib) | Go 1.26 | audible-cli subprocess management | Project convention (CLAUDE.md). exec.CommandContext for timeout, pipe stdout/stderr for output parsing. |
| modernc.org/sqlite | v1.48.1 | Database with new migration | Already in go.mod. Pure Go, no CGo. |
| spf13/cobra | v1.10.2 | New CLI commands (auth, sync, download) | Already in go.mod. Project convention. |
| spf13/viper | v1.21.0 | Config for audible-cli path, profile path | Already in go.mod. Project convention. |
| encoding/json (stdlib) | Go 1.26 | Parse audible-cli JSON export output | Standard library. No external dependency needed. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| context (stdlib) | Go 1.26 | Timeout/cancellation for subprocess calls | Every audible-cli invocation should have a context timeout |
| testify/assert + require | v1.11.1 | Test assertions | Already in go.mod. All tests. |

### No New Dependencies Required
Phase 3 requires zero new Go dependencies. Everything needed is either stdlib or already in go.mod.

## Architecture Patterns

### Recommended Project Structure
```
internal/
├── audible/           # NEW: audible-cli wrapper package
│   ├── audible.go     # Client interface + concrete implementation
│   ├── auth.go        # quickstart wrapper (interactive pass-through)
│   ├── library.go     # library list/export wrappers
│   ├── download.go    # download wrapper (stubbed for Phase 4)
│   ├── errors.go      # typed error types (AuthError, RateLimitError, etc.)
│   ├── parse.go       # JSON/output parsing helpers
│   └── audible_test.go # TestHelperProcess-based tests
├── cli/
│   ├── auth.go        # NEW: earworm auth command
│   ├── sync.go        # NEW: earworm sync command
│   └── download.go    # NEW: earworm download --dry-run command
└── db/
    ├── books.go       # MODIFIED: extend Book struct, add sync functions
    └── migrations/
        └── 003_add_audible_fields.sql  # NEW: remote metadata columns
```

### Pattern 1: Interface-Based Subprocess Wrapper
**What:** Define an `AudibleClient` interface that the CLI commands depend on. Concrete implementation calls os/exec; test implementation returns canned data.
**When to use:** All interactions with audible-cli.
**Example:**
```go
// Source: project convention + os/exec best practices
type AudibleClient interface {
    Quickstart(ctx context.Context) error
    LibraryExport(ctx context.Context) ([]LibraryItem, error)
    CheckAuth(ctx context.Context) error
    // Download stubbed for Phase 4
    Download(ctx context.Context, asin string, outputDir string) error
}

type client struct {
    audiblePath string  // path to audible-cli binary
    profilePath string  // path to audible profile dir
}
```

### Pattern 2: Interactive Subprocess Pass-Through (for auth)
**What:** Connect subprocess stdin/stdout/stderr directly to the terminal for interactive commands.
**When to use:** `earworm auth` wrapping `audible quickstart`.
**Example:**
```go
// Source: os/exec documentation
func (c *client) Quickstart(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, c.audiblePath, "quickstart")
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
```

### Pattern 3: JSON Output Parsing (for library sync)
**What:** Use `audible library export --format json --output -` (stdout) to get structured JSON, parse with encoding/json.
**When to use:** `earworm sync` to get full library metadata.
**Example:**
```go
// Source: audible-cli cmd_library.py analysis
type LibraryItem struct {
    ASIN                       string   `json:"asin"`
    Title                      string   `json:"title"`
    Subtitle                   string   `json:"subtitle"`
    Authors                    string   `json:"authors"`
    Narrators                  string   `json:"narrators"`
    SeriesTitle                string   `json:"series_title"`
    SeriesSequence             string   `json:"series_sequence"`
    RuntimeLengthMin           int      `json:"runtime_length_min"`
    PurchaseDate               string   `json:"purchase_date"`
    ReleaseDate                string   `json:"release_date"`
    IsFinished                 bool     `json:"is_finished"`
    PercentComplete            float64  `json:"percent_complete"`
    Genres                     string   `json:"genres"`
    Rating                     string   `json:"rating"`
    NumRatings                 int      `json:"num_ratings"`
    CoverURL                   string   `json:"cover_url"`
    ExtendedProductDescription string   `json:"extended_product_description"`
    DateAdded                  string   `json:"date_added"`
}
```

### Pattern 4: Upsert with Selective Field Preservation
**What:** When syncing remote data, update remote-sourced fields but preserve local-only fields (status, local_path, metadata_source, file_count).
**When to use:** `earworm sync` upsert logic.
**Example:**
```go
// The ON CONFLICT clause must NOT overwrite local-only fields
`INSERT INTO books (asin, title, author, narrator, ..., audible_status, purchase_date, ...)
VALUES (?, ?, ?, ?, ..., ?, ?, ...)
ON CONFLICT(asin) DO UPDATE SET
    title = excluded.title,
    author = excluded.author,
    narrator = excluded.narrator,
    -- remote metadata fields updated
    audible_status = excluded.audible_status,
    purchase_date = excluded.purchase_date,
    runtime_minutes = excluded.runtime_minutes,
    narrators = excluded.narrators,
    series_name = excluded.series_name,
    series_position = excluded.series_position,
    -- status, local_path, metadata_source NOT overwritten
    updated_at = CURRENT_TIMESTAMP`
```

### Pattern 5: TestHelperProcess for Subprocess Testing
**What:** The Go standard library pattern for testing os/exec calls. The test binary re-executes itself as a fake subprocess.
**When to use:** All audible-cli wrapper unit tests (TEST-05).
**Example:**
```go
// Source: Go stdlib os/exec/exec_test.go, https://rednafi.com/go/test-subprocesses/
func TestHelperProcess(t *testing.T) {
    if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
        return
    }
    // Route based on command args
    args := os.Args
    for i, arg := range args {
        if arg == "--" {
            args = args[i+1:]
            break
        }
    }
    switch args[0] {
    case "library":
        if args[1] == "export" {
            fmt.Print(`[{"asin":"B08C6YJ1LS","title":"Project Hail Mary",...}]`)
        }
    }
    os.Exit(0)
}

func fakeCommand(command string, args ...string) *exec.Cmd {
    cs := []string{"-test.run=TestHelperProcess", "--", command}
    cs = append(cs, args...)
    cmd := exec.Command(os.Args[0], cs...)
    cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
    return cmd
}
```

### Anti-Patterns to Avoid
- **Parsing text output from `library list`:** The list command uses colon-separated text that varies by content. Use `library export --format json` for reliable structured parsing.
- **Intercepting quickstart stdin:** D-09 mandates pass-through. Do not try to programmatically answer audible-cli prompts.
- **Overwriting local_path on sync:** The upsert must never clobber local-only fields. This is the most likely bug.
- **Testing with real audible-cli:** Tests must never depend on audible-cli being installed. Use TestHelperProcess or interface mocks.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON parsing | Custom parser | encoding/json with struct tags | audible-cli exports valid JSON; struct unmarshal handles it |
| Subprocess timeout | Manual goroutine + kill | exec.CommandContext with context.WithTimeout | Stdlib handles process cleanup on timeout |
| Config file paths | Manual path construction | viper.GetString("audible_cli_path") | Already configured in Phase 1 |
| Database migrations | Manual ALTER TABLE calls | Embedded SQL migration files (existing pattern) | Migration runner already handles this |

## Common Pitfalls

### Pitfall 1: audible-cli Export to Stdout
**What goes wrong:** `audible library export --output -` may not work -- audible-cli might not support stdout output via `-`.
**Why it happens:** audible-cli's `--output` flag defaults to a file path. The `-` convention for stdout is not universal in Python CLI tools.
**How to avoid:** Export to a temp file, then read the file. Use `os.CreateTemp` for the temp file path, pass it to `--output`, read and parse, then clean up.
**Warning signs:** Empty stdout, file-not-found errors, or audible-cli writing a literal file named `-`.

### Pitfall 2: Upsert Clobbering Local Fields
**What goes wrong:** Sync overwrites status or local_path with empty strings from remote data.
**Why it happens:** Using the existing `UpsertBook` function which overwrites ALL fields. The sync needs a separate upsert that preserves local-only fields.
**How to avoid:** Create a dedicated `SyncRemoteBook` (or `UpsertRemoteBook`) function with an ON CONFLICT clause that explicitly excludes status, local_path, metadata_source, and file_count from the update set.
**Warning signs:** Books that were "scanned" or "downloaded" reset to "unknown" after sync.

### Pitfall 3: Auth File Location Assumptions
**What goes wrong:** Earworm assumes audible-cli config is at `~/.audible/` but user may have a custom path.
**Why it happens:** audible-cli allows custom config directories via environment variables or flags.
**How to avoid:** Store the audible-cli config/profile directory in earworm's own config (viper key `audible.profile_path` or similar). Fall back to `~/.audible/` as default.
**Warning signs:** Auth check passes but library commands fail, or vice versa.

### Pitfall 4: Pre-flight Auth Check Command Selection
**What goes wrong:** Choosing a command that is too heavy or produces side effects for the auth check.
**Why it happens:** Need a lightweight command that proves auth is valid without fetching the full library.
**How to avoid:** Use `audible library list --bunch-size 1` or `audible api` with a minimal endpoint. The library list with bunch-size 1 fetches just 1 book, proving auth works. Alternatively, `audible manage profile list` is purely local and verifies profile exists but does NOT verify the token is still valid against Audible's servers. Best approach: `audible library list --bunch-size 1` -- it makes a real API call with minimal data.
**Warning signs:** Pre-flight check passes but sync fails with auth error (means check was local-only).

### Pitfall 5: Migration Numbering Collision
**What goes wrong:** Using 002 for the new migration when 002_add_metadata_fields.sql already exists.
**Why it happens:** Not checking existing migrations directory.
**How to avoid:** The next migration MUST be 003. Current migrations: 001_initial.sql, 002_add_metadata_fields.sql.
**Warning signs:** Migration runner errors on duplicate version numbers.

### Pitfall 6: audible-cli JSON Field Types
**What goes wrong:** JSON fields have unexpected types (string instead of int, null instead of empty string).
**Why it happens:** audible-cli serializes the raw Audible API response. Some fields may be null or have inconsistent types across books.
**How to avoid:** Use pointer types or custom JSON unmarshaling for fields that might be null. runtime_length_min is likely an int but could be null for podcasts. Use `json.Number` or `*int` where needed.
**Warning signs:** JSON unmarshal errors on specific books, nil pointer dereferences.

## Code Examples

### Existing UpsertBook Pattern (for reference)
```go
// Source: internal/db/books.go -- existing pattern to extend
_, err := db.Exec(
    `INSERT INTO books (asin, ...) VALUES (?, ...)
    ON CONFLICT(asin) DO UPDATE SET
        title = excluded.title,
        ...
        updated_at = CURRENT_TIMESTAMP`,
    book.ASIN, ...,
)
```

### New Migration 003 Schema
```sql
-- Add Audible remote metadata columns
ALTER TABLE books ADD COLUMN audible_status TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN purchase_date TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN runtime_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN narrators TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_name TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_position TEXT NOT NULL DEFAULT '';
```

### New Book Detection Query
```sql
-- Books that exist in remote (have audible_status set) but not downloaded locally
SELECT asin, title, author, narrators, runtime_minutes, series_name, series_position
FROM books
WHERE audible_status != ''
  AND (local_path = '' OR status NOT IN ('downloaded', 'organized'))
ORDER BY purchase_date DESC;
```

### CLI Command Pattern (from existing scan.go)
```go
// Source: internal/cli/scan.go -- pattern for new commands
var syncCmd = &cobra.Command{
    Use:   "sync",
    Short: "Sync Audible library metadata to local database",
    RunE:  runSync,
}

func init() {
    rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
    // 1. Open database
    // 2. Create audible client
    // 3. Pre-flight auth check
    // 4. Export library (JSON)
    // 5. Upsert all books
    // 6. Print summary
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Parse audible-cli text output | Use `library export --format json` | Available since audible-cli 0.2+ | Reliable structured parsing, no fragile text scraping |
| CGo SQLite for migrations | modernc.org/sqlite pure Go | Project decision Phase 1 | No change needed, continues working |

**audible-cli version note:** The `--format json` flag for library export has been available since at least 2022. It is stable and safe to rely on.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed (go test discovers tests) |
| Quick run command | `go test ./internal/audible/ ./internal/db/ ./internal/cli/ -count=1` |
| Full suite command | `go test ./... -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TEST-05a | Command construction (builds correct audible-cli args) | unit | `go test ./internal/audible/ -run TestBuild -count=1` | Wave 0 |
| TEST-05b | JSON output parsing (library export) | unit | `go test ./internal/audible/ -run TestParse -count=1` | Wave 0 |
| TEST-05c | Error mapping (exit codes to typed errors) | unit | `go test ./internal/audible/ -run TestError -count=1` | Wave 0 |
| TEST-05d | Subprocess execution (TestHelperProcess) | unit | `go test ./internal/audible/ -run TestHelper -count=1` | Wave 0 |
| TEST-06a | Auth validation (pre-flight check) | integration | `go test ./internal/audible/ -run TestAuth -count=1` | Wave 0 |
| TEST-06b | Library metadata sync (upsert flow) | integration | `go test ./internal/db/ -run TestSync -count=1` | Wave 0 |
| TEST-06c | New book detection | integration | `go test ./internal/db/ -run TestNewBook -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/audible/ ./internal/db/ -count=1`
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/audible/audible_test.go` -- TestHelperProcess setup, command build tests, parse tests
- [ ] `internal/db/sync_test.go` -- sync-specific upsert tests, new book detection tests
- [ ] `internal/cli/sync_test.go` -- CLI integration tests for sync command
- [ ] `internal/cli/auth_test.go` -- CLI integration tests for auth command
- [ ] Migration 003 test in `internal/db/db_test.go` -- verify new columns exist

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Build/test | Yes | 1.26.1 | -- |
| Python 3.9+ | audible-cli runtime | Yes | 3.14.3 | -- |
| audible-cli | Subprocess calls | No (not installed) | -- | Tests use TestHelperProcess fakes; manual install needed for real usage |
| modernc.org/sqlite | Database | Yes | v1.48.1 (in go.mod) | -- |

**Missing dependencies with no fallback:**
- audible-cli is not installed on this machine. This does not block development or testing (tests use fakes), but the tool must be installed for manual end-to-end validation. Installation: `pip install audible-cli` or `uv pip install audible-cli`.

**Missing dependencies with fallback:**
- None.

## Open Questions

1. **audible-cli stdout export support**
   - What we know: `--output` takes a file path. The `-` convention for stdout is common but not guaranteed.
   - What's unclear: Whether `--output /dev/stdout` or `--output -` works with audible-cli.
   - Recommendation: Default to writing to a temp file and reading it back. This is more reliable and only adds trivial overhead for a library-size JSON file. Can optimize later if stdout works.

2. **audible-cli error output format**
   - What we know: Uses Python logging (logger.error). Exit codes include sys.exit(1) for some failures.
   - What's unclear: Exact stderr patterns for auth expiry, rate limiting, network errors.
   - Recommendation: Parse stderr for known keywords ("error", "unauthorized", "expired", "rate limit"). Map to typed Go errors. Log the full stderr for debugging. Be defensive -- unknown errors get a generic wrapper.

3. **audible_status field values**
   - What we know: audible-cli exports `is_finished` and `percent_complete` fields.
   - What's unclear: Exact enumeration of possible remote status values.
   - Recommendation: Store `is_finished` as boolean and `percent_complete` as float. The `audible_status` column from D-04 can map these: "finished" if is_finished=true, "in_progress" if percent_complete > 0, "new" otherwise.

## Sources

### Primary (HIGH confidence)
- audible-cli source: `cmd_library.py` -- confirmed 18 export fields, JSON format, --format flag
- audible-cli source: `cmd_quickstart.py` -- confirmed interactive auth flow, profile creation
- Go stdlib os/exec documentation -- subprocess management patterns
- Existing codebase: `internal/db/books.go`, `internal/db/db.go`, `internal/cli/scan.go` -- established patterns

### Secondary (MEDIUM confidence)
- [Re-exec testing Go subprocesses](https://rednafi.com/go/test-subprocesses/) -- TestHelperProcess pattern documentation
- [Go stdlib exec_test.go](https://go.dev/src/os/exec/exec_test.go) -- canonical subprocess testing pattern
- [audible-cli GitHub repository](https://github.com/mkb79/audible-cli) -- command reference

### Tertiary (LOW confidence)
- audible-cli JSON output field types -- inferred from Python source but not verified with actual output

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries already in go.mod, no new dependencies
- Architecture: HIGH -- follows established project patterns, well-understood os/exec usage
- Pitfalls: HIGH -- identified from source code analysis and real project patterns
- audible-cli output parsing: MEDIUM -- JSON format confirmed but exact field types not validated against real output

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (30 days -- stable domain, audible-cli changes infrequently)
