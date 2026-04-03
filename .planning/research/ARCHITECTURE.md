# Architecture Patterns

**Domain:** CLI audiobook library management (Audible download, organization, integration)
**Researched:** 2026-04-03

## Recommended Architecture

Earworm follows a **layered CLI architecture** with clear separation between user interaction, business logic, external process management, and data persistence. The design prioritizes fault tolerance in the download pipeline and clean subprocess boundaries with audible-cli.

```
+------------------+
|   CLI Layer      |  (Cobra commands, user I/O, progress display)
+------------------+
         |
+------------------+
|  Service Layer   |  (Library, Download, Organizer, Integration services)
+------------------+
    |    |    |    |
+------+------+------+------+
| Store | Subprocess | HTTP  | Filesystem |
| (DB)  | (audible)  | (APIs)| (scanner)  |
+------+------+------+------+
```

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| **CLI (cmd/)** | Parse commands, validate input, display output/progress | Service layer only |
| **Library Service** | Scan local files, reconcile with DB, track book state | Store, Filesystem Scanner |
| **Download Service** | Queue downloads, manage retry/backoff, rate limit | Subprocess Manager, Store |
| **Organizer Service** | Move/rename files into target structure, write metadata | Filesystem, Store |
| **Integration Service** | Trigger Audiobookshelf scan, Goodreads sync | HTTP Client, Store |
| **Store (SQLite)** | Persist library state, download status, config | None (leaf dependency) |
| **Subprocess Manager** | Execute audible-cli commands, parse output, handle errors | OS process (audible-cli) |
| **Filesystem Scanner** | Walk directories, identify audiobook files, extract metadata | OS filesystem |
| **HTTP Client** | Make API calls to Audiobookshelf, handle auth | External APIs |

### Data Flow

**1. Library Scan Flow (local files to database)**
```
Filesystem (NAS mount)
  --> Filesystem Scanner (walk dirs, match patterns)
    --> Library Service (parse metadata from path/file)
      --> Store (upsert book records, mark as "local")
```

**2. Audible Sync Flow (discover new books)**
```
CLI "sync" command
  --> Download Service
    --> Subprocess Manager: `audible library export --output -`
      --> Parse JSON response
        --> Library Service: reconcile remote vs local
          --> Store (mark new books as "available", existing as "synced")
```

**3. Download Flow (fetch audiobooks)**
```
CLI "download" command (or auto after sync)
  --> Download Service (build download queue from "available" books)
    --> Rate Limiter (token bucket, respect Audible limits)
      --> Subprocess Manager: `audible download --asin <ASIN> --aaxc`
        --> Monitor stdout/stderr for progress
          --> On success: Store (mark "downloaded", record temp path)
          --> On failure: Retry with exponential backoff + jitter
            --> After max retries: Store (mark "failed", log error)
```

**4. Organization Flow (structure files)**
```
Download complete event
  --> Organizer Service
    --> Read book metadata from Store
    --> Compute target path: `<library>/<author>/<series>/<seq> - <title> [<asin>]/`
    --> Move/copy files (M4A, cover art, metadata)
    --> Store (update path, mark "organized")
```

**5. Integration Flow (notify external systems)**
```
Organization complete (batch or individual)
  --> Integration Service
    --> POST /api/libraries/<id>/scan (Audiobookshelf)
    --> Optional: Goodreads sync via subprocess
      --> Store (update integration timestamps)
```

## Project Structure

Use the standard Go CLI layout with Cobra. This is the dominant pattern used by kubectl, Terraform, and similar tools.

```
earworm/
  cmd/                          # Cobra command definitions
    root.go                     # Root command, global flags, config init
    scan.go                     # `earworm scan` - index local library
    sync.go                     # `earworm sync` - check Audible for new books
    download.go                 # `earworm download` - fetch books
    organize.go                 # `earworm organize` - structure files
    status.go                   # `earworm status` - show library state
    config.go                   # `earworm config` - manage settings
  internal/
    library/                    # Library service (scan, reconcile, query)
      library.go
      scanner.go                # Filesystem walking and file detection
    download/                   # Download service (queue, retry, rate limit)
      download.go
      queue.go
      ratelimiter.go
    organize/                   # File organization service
      organize.go
      naming.go                 # Path template computation
    integration/                # External API integrations
      audiobookshelf.go
      goodreads.go
    audible/                    # audible-cli subprocess wrapper
      client.go                 # Command execution, output parsing
      auth.go                   # Authentication management
      types.go                  # Parsed audible-cli response types
    store/                      # SQLite database layer
      store.go                  # DB connection, migrations
      books.go                  # Book CRUD operations
      downloads.go              # Download state tracking
      migrations/               # SQL migration files
        001_initial.sql
    config/                     # Application configuration
      config.go
    model/                      # Shared domain types
      book.go
      download.go
      library.go
  main.go                       # Entry point, calls cmd.Execute()
  go.mod
  go.sum
```

**Key structural decisions:**
- **`internal/`** prevents external imports. This is a standalone tool, not a library.
- **`model/`** contains shared types used across services, avoiding circular imports.
- **Services never import each other directly.** The CLI layer coordinates between them. This keeps boundaries clean and makes testing straightforward.
- **`cmd/`** is thin -- it parses flags, constructs service dependencies, and calls service methods. No business logic lives here.

## Patterns to Follow

### Pattern 1: Subprocess Wrapper with Structured Output

Wrap audible-cli behind a Go interface that returns typed data. Never let raw subprocess output leak into business logic.

**What:** A `Client` struct that executes audible-cli commands and parses responses into Go types.
**When:** Every interaction with audible-cli.
**Why:** Isolates the fragile subprocess boundary. If audible-cli changes its output format, only the wrapper needs updating.

```go
// internal/audible/client.go
type Client struct {
    BinaryPath string
    ProfileDir string
    Password   string // optional, for encrypted auth files
}

type Book struct {
    ASIN        string    `json:"asin"`
    Title       string    `json:"title"`
    Authors     []string  `json:"authors"`
    SeriesTitle string    `json:"series_title"`
    SeriesSeq   string    `json:"series_sequence"`
    PurchaseDate time.Time `json:"purchase_date"`
}

func (c *Client) ListLibrary(ctx context.Context) ([]Book, error) {
    // exec.CommandContext with timeout
    // Parse JSON output from `audible library export --output -`
    // Return typed slice
}

func (c *Client) Download(ctx context.Context, asin string, outputDir string) error {
    // exec.CommandContext with longer timeout
    // `audible download --asin <asin> --aaxc --output-dir <dir>`
    // Stream stderr for progress indicators
    // Return error with structured context on failure
}
```

**Critical:** Always use `exec.CommandContext` with a `context.Context` for timeout control. Downloads can take minutes for large books -- the context must propagate cancellation from user interrupt (Ctrl+C).

### Pattern 2: State Machine for Download Lifecycle

Track each book through explicit states in SQLite. This is essential for fault tolerance.

**What:** An enum of download states persisted per book.
**When:** Every download-related operation.

```go
type DownloadState string

const (
    StateAvailable   DownloadState = "available"    // Known to exist on Audible
    StateQueued      DownloadState = "queued"        // In download queue
    StateDownloading DownloadState = "downloading"   // Active download
    StateDownloaded  DownloadState = "downloaded"    // Raw file exists
    StateOrganized   DownloadState = "organized"     // Moved to final location
    StateFailed      DownloadState = "failed"        // Download failed (retryable)
    StateSkipped     DownloadState = "skipped"       // User chose to skip
)
```

On crash/restart, the service queries for books in `downloading` state, checks if partial files exist, and either resumes or resets to `queued`. Books in `downloaded` but not `organized` get re-queued for organization.

### Pattern 3: Rate-Limited Download Queue

Process downloads sequentially with rate limiting between requests. Audible is a commercial service -- hammering it risks account action.

**What:** A download queue that processes one book at a time with configurable delays and exponential backoff on failure.
**When:** All downloads.

```go
type DownloadQueue struct {
    limiter  *rate.Limiter    // golang.org/x/time/rate
    client   *audible.Client
    store    *store.Store
    maxRetry int
    baseWait time.Duration    // e.g., 30 seconds between downloads
}
```

Use `golang.org/x/time/rate` (standard extended library) for token-bucket rate limiting. Add jitter to backoff to avoid thundering herd if multiple instances run.

### Pattern 4: File Organization with Atomic Moves

Move files to their final location atomically to prevent partial states.

**What:** Download to a staging directory, then move to the organized location.
**When:** After every successful download.
**Why:** If the process crashes mid-organization, files are either fully in staging or fully organized -- never partially in both.

```
staging/                        # Temporary download target
  <asin>/
    book.aaxc
    cover.jpg
    chapters.json

library/                        # Final organized location (NAS mount)
  <Author>/
    <Series>/
      <Seq> - <Title> [<asin>]/
        <Title>.m4a
        cover.jpg
```

For NAS mounts (cross-filesystem), use copy-then-delete rather than `os.Rename`, since rename fails across mount points. Verify integrity (file size at minimum) before deleting the staging copy.

### Pattern 5: Audiobookshelf-Compatible Folder Naming

Follow the Audiobookshelf directory convention so the media server can auto-detect books.

**What:** Structure files as `Author/Series/Seq - Title [ASIN]/` for series books, or `Author/Title [ASIN]/` for standalone books.
**When:** During file organization.

Audiobookshelf parses folder names for metadata using these conventions:
- Author folder: supports "Last, First" and multiple authors separated by commas
- Series folder: optional, only for series books
- Title folder: can include `<seq> - <title>`, publish year, narrator in `{curly braces}`
- ASIN in brackets helps Audiobookshelf match metadata

```go
func ComputeTargetPath(book model.Book, libraryRoot string) string {
    author := sanitizePath(book.PrimaryAuthor())
    title := sanitizePath(book.Title)
    asin := book.ASIN

    if book.SeriesTitle != "" {
        series := sanitizePath(book.SeriesTitle)
        seq := book.SeriesSequence
        return filepath.Join(libraryRoot, author, series,
            fmt.Sprintf("%s - %s [%s]", seq, title, asin))
    }
    return filepath.Join(libraryRoot, author,
        fmt.Sprintf("%s [%s]", title, asin))
}
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Direct audible-cli Calls from Business Logic
**What:** Calling `exec.Command("audible", ...)` scattered throughout the codebase.
**Why bad:** No centralized error handling, no consistent timeout management, impossible to test, audible-cli output format changes break everything.
**Instead:** All audible-cli interaction goes through the `internal/audible.Client` wrapper. Business logic never knows it is talking to a subprocess.

### Anti-Pattern 2: In-Memory Download State
**What:** Tracking download progress only in memory (e.g., a map or channel).
**Why bad:** Process crash loses all state. User restarts and has no idea which books downloaded, which failed, which are partially complete.
**Instead:** Every state transition persists to SQLite before the action begins. The database is the source of truth, not runtime state.

### Anti-Pattern 3: Parsing audible-cli Text Output
**What:** Using regex to parse human-readable output from audible-cli.
**Why bad:** Fragile. Output format changes break parsing silently. Localization differences cause subtle bugs.
**Instead:** Use `audible library export` (JSON output) for library data. For downloads, track subprocess exit codes and verify output files exist rather than parsing progress text.

### Anti-Pattern 4: Organizing Files In-Place During Download
**What:** Downloading directly to the final library location.
**Why bad:** Partial downloads appear as valid books. Audiobookshelf scans may pick up incomplete files. NAS write operations are slower and less reliable than local disk.
**Instead:** Download to a local staging directory first, then move to the library location as a separate step. This also enables verification before the move.

### Anti-Pattern 5: Hardcoded Folder Templates
**What:** Building paths with string concatenation and no configuration.
**Why bad:** Users have different conventions. Audiobookshelf settings vary. Path separators differ across OS.
**Instead:** Use `filepath.Join` and make the naming template configurable (with a sensible Audiobookshelf-compatible default).

## Scalability Considerations

| Concern | At 100 books | At 1,000 books | At 10,000+ books |
|---------|--------------|----------------|-------------------|
| SQLite performance | Trivial | Trivial | Still fine; add indexes on ASIN, state, author |
| Library scan time | < 1 second | 2-5 seconds | 10-30 seconds on NAS (I/O bound) |
| Download time | Rate-limited by Audible, not by Earworm | Same; sequential with delays | Same; consider priority/filter options |
| Memory usage | Negligible | Negligible | Negligible (streaming, not loading all into memory) |
| File organization | Instant | Few seconds | Few minutes on NAS; batch operations help |

SQLite handles this scale easily. The bottleneck is always Audible's rate limiting and NAS I/O, never the local application.

## Database Schema (Core Tables)

```sql
CREATE TABLE books (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    asin        TEXT UNIQUE NOT NULL,
    title       TEXT NOT NULL,
    authors     TEXT NOT NULL,           -- JSON array
    narrators   TEXT,                    -- JSON array
    series_title TEXT,
    series_seq  TEXT,
    purchase_date TEXT,
    runtime_minutes INTEGER,
    local_path  TEXT,                    -- NULL if not yet organized
    state       TEXT NOT NULL DEFAULT 'available',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE downloads (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id     INTEGER NOT NULL REFERENCES books(id),
    state       TEXT NOT NULL DEFAULT 'queued',
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT,
    staging_path TEXT,
    started_at  TEXT,
    completed_at TEXT,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE integrations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    service     TEXT NOT NULL,           -- 'audiobookshelf', 'goodreads'
    last_sync   TEXT,
    config      TEXT,                    -- JSON blob for service-specific config
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_books_asin ON books(asin);
CREATE INDEX idx_books_state ON books(state);
CREATE INDEX idx_downloads_state ON downloads(state);
CREATE INDEX idx_downloads_book_id ON downloads(book_id);
```

## Suggested Build Order

Dependencies flow bottom-up. Build foundational components first, then compose them.

```
Phase 1: Foundation
  model/ --> store/ --> config/
  (Domain types, database, configuration -- everything else depends on these)

Phase 2: Local Library
  internal/library/scanner.go --> internal/library/library.go
  cmd/scan.go, cmd/status.go
  (Scan existing files, populate DB, display state -- useful immediately)

Phase 3: Audible Integration
  internal/audible/client.go --> internal/audible/auth.go
  (Subprocess wrapper, authentication -- prerequisite for downloads)

Phase 4: Download Pipeline
  internal/download/ (queue, rate limiter, retry logic)
  cmd/sync.go, cmd/download.go
  (Sync library list from Audible, download new books -- core value)

Phase 5: File Organization
  internal/organize/
  cmd/organize.go
  (Move downloaded files into Audiobookshelf-compatible structure)

Phase 6: External Integrations
  internal/integration/audiobookshelf.go
  internal/integration/goodreads.go
  (Notify Audiobookshelf after downloads, sync Goodreads)
```

**Why this order:**
- Phases 1-2 are immediately useful (index an existing library) and validate the data model.
- Phase 3 is a hard prerequisite for Phase 4 (cannot download without audible-cli wrapper).
- Phase 4 is the core value proposition and the most complex component. It deserves its own phase.
- Phase 5 depends on Phase 4 output (downloaded files) but is conceptually simple.
- Phase 6 is additive -- the tool is fully functional without integrations.

## Key Technology Decisions for Architecture

| Decision | Choice | Rationale |
|----------|--------|-----------|
| CLI framework | Cobra v2 | Industry standard for Go CLIs. Used by kubectl, Terraform, Hugo. |
| Configuration | Viper | Pairs with Cobra. Handles config files, env vars, flags. |
| SQLite driver | `modernc.org/sqlite` | Pure Go, no CGO required. Enables easy cross-compilation for single-binary distribution. Performance delta is irrelevant at this scale. |
| HTTP client | `net/http` (stdlib) | Audiobookshelf API is simple REST. No need for a heavy HTTP framework. |
| Rate limiting | `golang.org/x/time/rate` | Standard extended library. Token bucket algorithm. |
| Retry/backoff | `cenkalti/backoff/v4` | Well-maintained, supports exponential backoff with jitter. |
| Subprocess | `os/exec` (stdlib) | Standard library is sufficient. Always use `CommandContext`. |
| Logging | `log/slog` (stdlib) | Structured logging added in Go 1.21. No external dependency needed. |

## Sources

- [Cobra CLI framework](https://github.com/spf13/cobra) - Go CLI standard
- [Go CLI architecture example](https://github.com/skport/golang-cli-architecture) - Structural patterns
- [audible-cli](https://github.com/mkb79/audible-cli) - Subprocess target, command interface
- [Audiobookshelf API](https://api.audiobookshelf.org/) - Library scan endpoint
- [Audiobookshelf docs](https://www.audiobookshelf.org/docs) - Book structure conventions
- [Libation naming templates](https://getlibation.com/docs/features/naming-templates) - File organization reference
- [Libation Audiobookshelf settings](https://github.com/rmcrackan/Libation/issues/1292) - Compatibility recommendations
- [good-audible-story-sync](https://github.com/cheshire137/good-audible-story-sync) - Goodreads sync tool
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) - Pure Go SQLite driver
- [cenkalti/backoff](https://github.com/cenkalti/backoff) - Exponential backoff library
- [Go subprocess best practices](https://calmops.com/programming/golang/go-process-management-subprocess/) - exec.CommandContext patterns
- [Go SQLite benchmarks](https://github.com/cvilsmeier/go-sqlite-bench) - Driver performance comparison
