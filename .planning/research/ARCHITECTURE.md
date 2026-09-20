# Architecture Research: Listening-Stats Subsystem (v1.2)

**Domain:** Adding listening-history ingestion, cross-source book-identity resolution, CSV export, and external journaling to an existing Go CLI audiobook manager
**Researched:** 2026-09-20
**Confidence:** HIGH (package/schema conventions — verified against current codebase); MEDIUM (Audiobookshelf listening-sessions API shape — verified via official docs); LOW (exact mechanism for fetching mutable per-day Audible listening totals — audible-cli has no documented public stats export; treat as a Phase 1 spike, not a settled fact)

## Summary Recommendation

Four new packages (`internal/bookidentity`, `internal/listening`, `internal/journal`, plus additions to `internal/db`), two modified existing packages (`internal/audible`, `internal/audiobookshelf`), one new CLI file, and one daemon-cycle edit. One migration file bundles all new tables (precedent: `005_plan_infrastructure.sql` bundled `plans` + `plan_operations` + `audit_log`). Identity resolution is its own package because it is pure, algorithmic, and reused by both ingestion and later reconciliation — the same reason `metadata` is separate from `organize`. Inference (the allocation algorithm) is **not** materialized as a general-purpose table; it is computed on demand at export/journal time and only persisted as a stamped, versioned snapshot inside the `journal_entries` ledger, which exists for idempotency, not as a source of truth.

## Standard Architecture

### System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│ CLI layer (internal/cli)                                             │
│  stats.go: "stats sync", "stats export", "stats journal"             │
│  daemon.go (MODIFIED): cycle += listening.SyncAll step                │
├──────────────────────────────────────────────────────────────────────┤
│ Orchestration (internal/listening)            Subprocess (internal/  │
│  ┌────────────┐  ┌────────────┐  ┌─────────┐   journal)              │
│  │ source     │  │ sync.go    │  │ rollup/ │   ┌──────────────────┐  │
│  │ adapters   │→ │ (watermark │→ │ export/ │→  │ journal.Client   │  │
│  │ (normalize)│  │ + overlap) │  │ attrib. │   │ (cmdFactory,     │  │
│  └─────┬──────┘  └─────┬──────┘  └────┬────┘   │  mirrors audible)│  │
│        │               │              │        └────────┬─────────┘  │
├────────┼───────────────┼──────────────┼─────────────────┼───────────┤
│ Identity resolution (internal/bookidentity)               │          │
│  Normalize(title, author) → Score(a,b) → Resolve/Merge     │          │
├─────────┼───────────────┼──────────────┼───────────────────┼─────────┤
│ Data layer (internal/db) — all SQL lives here, no exceptions         │
│  book_identity.go   listening.go        journal.go                   │
│  (identities +      (sessions, daily     (journal_entries ledger)    │
│   source mappings)   totals, watermark,                              │
│                       rollup)                                        │
├──────────────────────────────────────────────────────────────────────┤
│ External integrations (existing packages, extended)                  │
│  internal/audible (MODIFIED): + ListeningStats(ctx, from, to)         │
│  internal/audiobookshelf (MODIFIED): + ListeningSessions(ctx, since)  │
│  external journaling CLI (subprocess, e.g. a local journal tool)      │
└──────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | New/Modified |
|-----------|----------------|--------------|
| `internal/db` (book_identity.go, listening.go, journal.go) | All SQL for the new tables: CRUD, upserts, watermark read/write. Owns transactional identity-merge. | New files in existing package |
| `internal/bookidentity` | Pure matching algorithm: normalize title/author, score candidate pairs, decide match method + confidence. No I/O; takes structs, returns decisions. Callers persist via `internal/db`. | New package |
| `internal/listening` | Source adapters (normalize raw API/CLI output into common `Session`/`DailyTotal` structs), watermark-driven incremental sync per source, rollup recompute, allocation/attribution algorithm, CSV export. | New package |
| `internal/journal` | Subprocess wrapper for the external journaling CLI. `cmdFactory` injection, no business logic — mirrors `internal/audible`. | New package |
| `internal/audible` | Add a method to fetch per-day listening totals from Audible (mechanism TBD — see Pitfalls). | Modified (new file in existing package) |
| `internal/audiobookshelf` | Add a method to fetch listening sessions since a timestamp. | Modified (new file in existing package) |
| `internal/cli` (stats.go new; daemon.go modified) | Cobra command surface; thin — delegates to `internal/listening` and `internal/journal`, matching how `sync.go`/`daemon.go` already delegate to `internal/audible`. | New + modified files |
| `internal/config` | New viper keys: `listening.enabled`, `listening.audible.overlap_days`, `listening.audiobookshelf.overlap_minutes`, `journal.cli_path`, `journal.enabled`. | Modified |

## Recommended Project Structure

```
internal/
├── db/
│   ├── migrations/
│   │   └── 008_listening_stats.sql   # NEW — all 7 new tables, one migration
│   ├── book_identity.go              # NEW — book_identities, book_identity_sources
│   ├── listening.go                  # NEW — sessions, daily_totals, sync_state, rollup
│   └── journal.go                    # NEW — journal_entries ledger
├── bookidentity/
│   ├── normalize.go                  # NEW — title/author normalization (casefold, strip
│   │                                  #        subtitle/series noise, punctuation)
│   ├── resolve.go                    # NEW — Score(), Resolve(), Merge() — pure logic
│   └── resolve_test.go               # NEW — table-driven match/no-match/ambiguous cases
├── listening/
│   ├── types.go                      # NEW — Session, DailyTotal, Rollup structs
│   ├── audible_source.go             # NEW — wraps audible.AudibleClient, normalizes
│   ├── audiobookshelf_source.go      # NEW — wraps a narrow consumer-defined interface
│   ├── sync.go                       # NEW — per-source incremental sync, watermark +
│   │                                  #        overlap-window re-fetch, calls bookidentity
│   ├── rollup.go                     # NEW — recompute book_listening_rollup (measured only)
│   ├── attribution.go                # NEW — allocation algorithm, pure fn, not persisted
│   └── export.go                     # NEW — CSV export, mirrors goodreads/export.go
├── journal/
│   ├── journal.go                    # NEW — Client interface, cmdFactory pattern
│   └── journal_test.go               # NEW — fake cmdFactory, mirrors audible_test.go
├── audible/
│   ├── audible.go                    # MODIFIED — add ListeningStats to AudibleClient iface
│   └── stats.go                      # NEW file in existing package — implementation
├── audiobookshelf/
│   ├── client.go                     # unchanged
│   └── sessions.go                   # NEW file in existing package — ListeningSessions
├── config/
│   └── config.go                     # MODIFIED — new default keys, ValidKeys entries
└── cli/
    ├── stats.go                      # NEW — `stats sync`, `stats export`, `stats journal`
    └── daemon.go                     # MODIFIED — cycle += listening sync step
```

### Structure Rationale

- **`bookidentity` is separate from `listening`:** matching is pure (no network, no subprocess), independently table-driven-testable, and conceptually reusable (both ingestion-time assignment and a later reconciliation/merge pass need it). This mirrors the existing `metadata` vs `organize` split — `metadata` parses/scores, `organize` orchestrates and calls `fileops`.
- **`journal` is separate from `listening`:** the subprocess boundary is the exact same shape as `internal/audible` (external CLI, `cmdFactory` injection for tests, exit-code-driven error classification). Keeping it separate means `listening`'s tests never spawn a process, and `journal`'s tests never touch SQLite.
- **All SQL stays in `internal/db`:** this is a hard existing convention (`books.go`, `plans.go` — no ORM, no repository interfaces scattered across packages). `bookidentity` and `listening` call exported `db.*` functions; they do not open `*sql.DB` queries themselves.
- **One migration, several tables:** matches `005_plan_infrastructure.sql` precedent exactly (plans + plan_operations + audit_log shipped together). Do this even though `journal_entries` isn't used until the last build phase — avoids schema drift and a second migration touching the same feature area.

## Schema Design

All DDL below is a direct sketch for `008_listening_stats.sql`. Column order and defaults follow the existing style (`NOT NULL DEFAULT ''` for text, explicit `CREATE INDEX IF NOT EXISTS`).

### Identity: surrogate key, not ASIN

The core problem: ASIN is not universal. A book heard only in one source (e.g., a library loan or a manually-added Audiobookshelf item) has no ASIN; a book in neither the `books` table nor either listening source obviously doesn't exist yet. So identity must be a **local surrogate key** (`book_identities.id`), with ASIN as an optional, best-effort join key — not the primary key.

```sql
-- One row per distinct real-world book, regardless of source coverage.
CREATE TABLE IF NOT EXISTS book_identities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    canonical_title  TEXT NOT NULL,
    canonical_author TEXT NOT NULL,
    asin TEXT,                          -- nullable; may match books.asin when known
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_book_identities_asin
    ON book_identities(asin) WHERE asin IS NOT NULL;

-- One row per (source, native key) the source uses to identify a book.
-- This is where match method + confidence live, per-mapping, not per-identity —
-- an identity assembled from two sources can have two different confidences.
CREATE TABLE IF NOT EXISTS book_identity_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES book_identities(id),
    source TEXT NOT NULL,               -- 'audible' | 'audiobookshelf'
    source_key TEXT NOT NULL,           -- ASIN, ABS library-item id, etc.
    source_title  TEXT NOT NULL DEFAULT '',
    source_author TEXT NOT NULL DEFAULT '',
    match_method TEXT NOT NULL,         -- 'asin_exact' | 'title_author_fuzzy' | 'manual' | 'unmatched'
    match_confidence REAL NOT NULL DEFAULT 0,  -- 0.0–1.0
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source, source_key)
);
CREATE INDEX IF NOT EXISTS idx_bis_identity ON book_identity_sources(identity_id);
```

**Resolution rule (implemented in `internal/bookidentity`):**
1. If the incoming record carries an ASIN and it matches an existing `books.asin` or an existing `book_identities.asin` → `match_method='asin_exact'`, `confidence=1.0`.
2. Else, normalize title+author (casefold, strip series/subtitle noise, punctuation) and score against existing `book_identity_sources` rows from the *other* source only (do not match within the same source — a source never needs to match itself). Score above a configurable threshold → `title_author_fuzzy` with the computed confidence; below threshold → create a brand-new `book_identities` row with `match_method='unmatched'`.
3. Every ingested record gets an identity assigned at ingestion time — there is no "orphan" record without an identity_id. Some identities will simply have only one `book_identity_sources` row for a long time (or forever), which is the expected, correct state for single-source books.

**Merge path (the hard case):** if source A is ingested before source B ever adds a matching record, both may have independently auto-created identities. When a later fuzzy match crosses that threshold, `internal/bookidentity.Merge` must run as a single transaction in `internal/db`: repoint every `listening_sessions.identity_id`, `listening_daily_totals.identity_id`, `book_listening_rollup.identity_id`, and `book_identity_sources.identity_id` from the losing id to the keeping id (lowest id wins, deterministic), then delete the losing `book_identities` row. This is rare at personal-library scale (dozens to low-thousands of rows) — a full table scan per merge is fine; do not over-engineer this into an event log.

### Measured facts: sessions (immutable) and daily totals (mutable)

```sql
-- Exact per-session events. Expected source: Audiobookshelf's documented
-- listening-sessions API (session-level, closable, immutable once closed).
CREATE TABLE IF NOT EXISTS listening_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES book_identities(id),
    source TEXT NOT NULL DEFAULT 'audiobookshelf',
    source_session_id TEXT NOT NULL,    -- native session id, the idempotency key
    started_at DATETIME NOT NULL,
    ended_at   DATETIME NOT NULL,
    duration_seconds INTEGER NOT NULL,
    device TEXT NOT NULL DEFAULT '',
    raw_payload TEXT NOT NULL DEFAULT '',   -- JSON, for debugging/reprocessing
    ingested_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source, source_session_id)
);
CREATE INDEX IF NOT EXISTS idx_sessions_identity ON listening_sessions(identity_id);
CREATE INDEX IF NOT EXISTS idx_sessions_started   ON listening_sessions(started_at);

-- Per-source, per-book, per-day aggregate. Expected source: Audible's own
-- listening totals — mechanism to fetch these is NOT confirmed (Audible does
-- not publish an official export API; audible-cli has no documented stats
-- command as of this research). Treat retrieval as a Phase 1 spike. Whatever
-- the mechanism, treat these rows as *mutable*: a day's total can be revised
-- for several days after it first appears (cross-device sync lag).
CREATE TABLE IF NOT EXISTS listening_daily_totals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES book_identities(id),
    source TEXT NOT NULL DEFAULT 'audible',
    listen_date TEXT NOT NULL,          -- ISO date 'YYYY-MM-DD' in source's local-day bucket
    seconds_listened INTEGER NOT NULL,
    raw_payload TEXT NOT NULL DEFAULT '',
    fetched_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source, identity_id, listen_date)
);
CREATE INDEX IF NOT EXISTS idx_daily_identity ON listening_daily_totals(identity_id);
CREATE INDEX IF NOT EXISTS idx_daily_date      ON listening_daily_totals(listen_date);
```

The two tables use **opposite conflict semantics on purpose**:
- `listening_sessions`: `INSERT ... ON CONFLICT(source, source_session_id) DO NOTHING` — a session, once recorded, never changes. Re-fetching the same session in an overlap window is a guaranteed no-op.
- `listening_daily_totals`: `INSERT ... ON CONFLICT(source, identity_id, listen_date) DO UPDATE SET seconds_listened = excluded.seconds_listened, raw_payload = excluded.raw_payload, fetched_at = CURRENT_TIMESTAMP` — a day's value can legitimately change and the row must be overwritten, not ignored.

### Measured rollup (materialized, deterministic, rebuildable)

```sql
-- Recomputed, not hand-maintained. Safe to TRUNCATE + rebuild at any time
-- from listening_sessions + listening_daily_totals — this table is a cache,
-- never the source of truth.
CREATE TABLE IF NOT EXISTS book_listening_rollup (
    identity_id INTEGER PRIMARY KEY REFERENCES book_identities(id),
    total_seconds_audible       INTEGER NOT NULL DEFAULT 0,
    total_seconds_audiobookshelf INTEGER NOT NULL DEFAULT 0,
    first_activity_at DATETIME,
    last_activity_at  DATETIME,
    session_count INTEGER NOT NULL DEFAULT 0,
    recomputed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### Sync watermark state

```sql
CREATE TABLE IF NOT EXISTS listening_sync_state (
    source TEXT PRIMARY KEY,            -- 'audible' | 'audiobookshelf'
    watermark TEXT NOT NULL DEFAULT '', -- source-specific cursor (ISO date or session id/ts)
    last_synced_at DATETIME,            -- wall-clock time of last successful run
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Overlap-window *size* (how far back to re-fetch) is a config value, not DB state — it's an operator tuning knob, not runtime progress:
`listening.audible.overlap_days` (default e.g. 7), `listening.audiobookshelf.overlap_minutes` (default e.g. 30, to cover clock skew / late session close).

### Journal ledger (idempotency + inference snapshot)

```sql
-- Records what was actually sent to the external journaling CLI. This is
-- the ONLY place an inferred number is persisted, and it's persisted as an
-- audit/idempotency record, not as reusable derived data.
CREATE TABLE IF NOT EXISTS journal_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    identity_id INTEGER NOT NULL REFERENCES book_identities(id),
    entry_date TEXT NOT NULL,
    attributed_seconds INTEGER NOT NULL,   -- output of the allocation algorithm
    algorithm_version TEXT NOT NULL,       -- e.g. "attrib-v1" — bump on logic change
    external_ref TEXT NOT NULL DEFAULT '', -- id/hash returned by the journaling CLI, if any
    status TEXT NOT NULL DEFAULT 'pending',-- pending | emitted | failed
    error_message TEXT NOT NULL DEFAULT '',
    emitted_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(identity_id, entry_date)
);
CREATE INDEX IF NOT EXISTS idx_journal_status ON journal_entries(status);
```

`UNIQUE(identity_id, entry_date)` plus `status` gives the same idempotent-retry shape already used for `plan_operations` (`pending`/`running`/`completed`/`failed`) — re-running `stats journal` for a date range is safe: rows already `emitted` are skipped unless `--force` is passed.

**Foreign keys are documentation, not enforcement**, matching the existing codebase's explicit choice (see `AddOperation`'s comment: *"Verify plan exists (enforcing FK in Go per research pitfall 1)"*). Every `identity_id` insert should be preceded by a Go-level existence check via `db.GetBookIdentity`, not a SQLite `PRAGMA foreign_keys` reliance.

## Architectural Patterns

### Pattern 1: Consumer-defined narrow interfaces for source adapters

**What:** `internal/audible.AudibleClient` is already an interface with `cmdFactory` injection — just extend it. `internal/audiobookshelf.Client` is a concrete struct with no interface. Rather than retrofitting an interface onto the whole `audiobookshelf` package, define a narrow interface where it's consumed, in `internal/listening`.
**When to use:** whenever `internal/listening` needs to fake a source in tests.
**Trade-offs:** avoids touching `audiobookshelf.Client`'s existing call sites; slight duplication if multiple packages later want the same interface (acceptable at this scale).

```go
// internal/listening/audiobookshelf_source.go
type Session struct {
    SourceSessionID string
    Title, Author   string
    StartedAt, EndedAt time.Time
    DurationSeconds int
    Device          string
}

// sessionSource is satisfied by *audiobookshelf.Client; defined here (consumer side)
// so tests can inject a fake without changing the audiobookshelf package.
type sessionSource interface {
    ListeningSessions(ctx context.Context, since time.Time) ([]audiobookshelf.Session, error)
}
```

### Pattern 2: Upsert semantics chosen per mutability, not uniformly

**What:** `listening_sessions` uses `ON CONFLICT DO NOTHING`; `listening_daily_totals` uses `ON CONFLICT DO UPDATE`. Do not default to one pattern for both tables.
**When to use:** any time a new source table is added — first ask "can this record change after I've seen it once?" before writing the upsert.
**Trade-offs:** two upsert helpers in `internal/db/listening.go` instead of one generic one; worth it because silently allowing update-in-place on session rows would let a re-fetch corrupt an already-correct immutable record if the source ever returns slightly different timestamps for the same session id.

### Pattern 3: Watermark + overlap window, not a single "since" cursor

**What:** `sync.go`'s per-source sync function does not fetch strictly `since watermark`. For the mutable source it fetches from `watermark - overlap_days` through "today, inclusive"; for the immutable source it fetches from `watermark - overlap_minutes` (small skew buffer) forward. The watermark itself only advances to "now" after a successful fetch — it never advances into the overlap zone assumption, so a crash mid-sync just means the next run re-covers the same tail, which is safe because both upsert paths are idempotent.

```go
// internal/listening/sync.go
func syncAudible(ctx context.Context, database *sql.DB, src audibleSource, overlap time.Duration) error {
    state, err := db.GetSyncState(database, "audible")
    if err != nil { return fmt.Errorf("get sync state: %w", err) }

    from := time.Now().Add(-overlap) // default when no prior watermark
    if state != nil && state.Watermark != "" {
        wm, err := time.Parse(time.RFC3339, state.Watermark)
        if err == nil {
            from = wm.Add(-overlap)
        }
    }

    totals, err := src.ListeningStats(ctx, from, time.Now())
    if err != nil { return fmt.Errorf("fetch audible listening stats: %w", err) }

    for _, t := range totals {
        identityID, method, conf, err := bookidentity.Resolve(database, t.Title, t.Author, t.ASIN)
        if err != nil { return fmt.Errorf("resolve identity for %q: %w", t.Title, err) }
        if err := db.UpsertDailyTotal(database, identityID, "audible", t.Date, t.Seconds, t.RawJSON); err != nil {
            return fmt.Errorf("upsert daily total: %w", err)
        }
        _ = method
        _ = conf
    }

    return db.SetSyncWatermark(database, "audible", time.Now().Format(time.RFC3339))
}
```

### Pattern 4: Inference computed at read time, not stored generically

**What:** `internal/listening/attribution.go` exposes a pure function, `Allocate(sessions []Session, dailyTotals []DailyTotal) []DailyAttribution`, called by `export.go` and by the `stats journal` command path. Its output is never written to a general table. It is only persisted when `stats journal` actually emits an entry — at that moment the specific `attributed_seconds` + `algorithm_version` used are frozen into `journal_entries`.
**When to use:** any time derived/estimated data must stay clearly distinguishable from measured data.
**Trade-offs:** every export re-runs the allocation over the (small) raw dataset — trivial cost at personal-library scale (hundreds to low-thousands of session/day rows), and it means improving the algorithm later requires no backfill migration. The cost is discipline: nothing downstream may read `attributed_seconds` out of `journal_entries` and treat it as "the current best estimate" for a date that hasn't been journaled yet — for un-journaled dates, callers must call `Allocate` fresh.

## Data Flow

### Incremental sync (daemon or manual `stats sync`)

```
[daemon cycle / `earworm stats sync`]
    ↓
internal/listening.SyncAll(ctx, db)
    ↓                                   ↓
syncAudiobookshelf(...)          syncAudible(...)
    ↓ fetch since watermark-overlap      ↓ fetch since watermark-overlap_days
audiobookshelf.ListeningSessions   audible.ListeningStats
    ↓ normalize → Session              ↓ normalize → DailyTotal
bookidentity.Resolve (per record: asin-exact → fuzzy → new identity)
    ↓
db.UpsertSession (ON CONFLICT DO NOTHING)   db.UpsertDailyTotal (ON CONFLICT DO UPDATE)
    ↓
db.SetSyncWatermark(source, now)
    ↓ (both sources done)
internal/listening.RecomputeRollup(db)  -- rebuilds book_listening_rollup from raw tables
```

### Export / journal (manual, on demand)

```
[`earworm stats export` | `earworm stats journal`]
    ↓
listening.LoadRawData(db, dateRange) -- sessions + daily_totals for the range
    ↓
listening.Allocate(...) -- pure allocation algorithm, computed fresh every call
    ↓                                        ↓
export.go: write CSV                  journal path: for each un-emitted
  (measured + attributed columns,     (identity, date) -> journal.Client.AddEntry(...)
   clearly labeled which is which)    -> db.RecordJournalEntry(status, algorithm_version)
```

## Idempotency & Partial-Failure Recovery

- **Sync is safe to interrupt at any point:** each source's loop upserts row-by-row and only advances that source's watermark after the full fetch+upsert loop for that source completes. A crash mid-loop leaves the watermark unchanged, so the next run re-covers the same window — both upsert paths tolerate that (no-op for sessions, overwrite-with-same-or-corrected-value for daily totals).
- **Do not batch both sources' watermark updates in one transaction** — they are independent external systems with independent failure modes; syncing Audiobookshelf successfully while Audible fails (or vice versa) should still commit the successful side's watermark. Two separate `Set SyncWatermark` calls, not one.
- **Journal emission is idempotent via the ledger's unique constraint:** `stats journal` should query `journal_entries` for `(identity_id, entry_date)` pairs already `status='emitted'` and skip them; a `--force` flag re-runs allocation and re-invokes the external CLI, overwriting the ledger row. Never invoke the external journaling CLI for a date already marked `emitted` without an explicit force flag — this is the same "guarded, non-destructive by default" posture the project already uses for `cleanup`.
- **Rollup rebuild is always safe to re-run in full:** `RecomputeRollup` should `DELETE FROM book_listening_rollup` then re-`INSERT ... SELECT ... GROUP BY identity_id` inside one transaction. Because it's fully derived from `listening_sessions` + `listening_daily_totals`, there's no partial-failure state to reason about beyond "did the transaction commit."

## Anti-Patterns

### Anti-Pattern 1: Storing inferred attribution as if it were a fact table

**What people do:** create a `book_listening_daily_attributed` table populated by the allocation algorithm and treat it like `listening_daily_totals`.
**Why it's wrong:** the moment the allocation algorithm improves, every downstream consumer of that table is silently reading stale inference with no way to tell old numbers from new ones, and a "fix the algorithm" change becomes a data migration.
**Do this instead:** compute allocation at read time (`attribution.go`); only freeze a snapshot into `journal_entries` at the moment something external and irreversible happens (the journal CLI is actually invoked).

### Anti-Pattern 2: Treating ASIN as the join key for listening data

**What people do:** key `listening_sessions`/`listening_daily_totals` directly on `books.asin` since that's the existing pattern in `books.go`.
**Why it's wrong:** breaks immediately for any book present in only one listening source with no ASIN (a library loan logged in Audiobookshelf, for instance) — exactly the case called out in the requirements.
**Do this instead:** surrogate `book_identities.id`, with ASIN as an optional attribute used as the strongest (but not only) matching signal.

### Anti-Pattern 3: One "upsert" helper for both mutable and immutable source tables

**What people do:** write a single generic `UpsertListeningRecord` used for both sessions and daily totals "for consistency."
**Why it's wrong:** silently allows either accidental overwrite of an immutable session record, or accidental no-op on a daily total that genuinely needs updating after a retroactive correction.
**Do this instead:** two explicit functions in `internal/db/listening.go` — `UpsertSession` (`DO NOTHING`) and `UpsertDailyTotal` (`DO UPDATE`) — with doc comments stating the mutability assumption.

## Testability: Data Layer vs. External-Process Invocation

- `internal/journal` must follow the exact `internal/audible` shape: an interface (`Client` with `AddEntry(ctx, Entry) error`), a `cmdFactory func(ctx, name string, args ...string) *exec.Cmd` field settable via functional option (`WithCmdFactory`), and a real `exec.CommandContext` default. This lets `internal/cli/stats.go`'s tests and any future `internal/listening` integration tests inject a fake process without touching the filesystem or an actual CLI binary — mirrors how `sync_test.go`/`download_test.go` already fake `audible.AudibleClient`.
- `internal/listening` must never call `os/exec` directly — it depends on the `journal.Client` interface, not the concrete subprocess wrapper, so its own tests (watermark logic, allocation algorithm, rollup math) run with zero subprocess and zero real network, only a `:memory:` SQLite handle (existing test convention).
- `internal/bookidentity`'s core scoring functions (`Normalize`, `Score`) should take plain strings in and return plain values out — no `*sql.DB` parameter at all. Only the thin `Resolve`/`Merge` wrapper functions that need to look up/write `book_identity_sources` take a `*sql.DB`. This keeps the fuzzy-matching logic testable with pure table-driven tests (no DB setup) while keeping persistence conventions consistent with the rest of the codebase.

## Build Order (4 Phases)

**Phase 1 — Schema + identity resolution + single-source ingestion (Audiobookshelf)**
Ship `008_listening_stats.sql` (all 7 tables, even though some aren't used yet). Build `internal/db/book_identity.go` and `internal/db/listening.go` CRUD. Build `internal/bookidentity` (normalize, score, resolve — no merge yet, since only one source exists). Add `audiobookshelf/sessions.go` (`ListeningSessions`). Build `internal/listening` skeleton with only the Audiobookshelf adapter + `syncAudiobookshelf` + watermark read/write. Add `earworm stats sync --source audiobookshelf`. *Rationale:* proves schema, identity assignment, and idempotent session upsert on the simpler (immutable, no-overlap-window) source before introducing the harder mutable-source and cross-source-merge complexity.

**Phase 2 — Second source with mutable-record handling + identity merge**
Spike and confirm the actual mechanism for Audible per-day listening totals (this is the one open unknown — audible-cli has no documented stats export as of this research; may require an undocumented endpoint via the underlying `audible` Python library, or may need to be descoped/re-specified). Add `audible/stats.go` + extend `AudibleClient`. Build `syncAudible` with overlap-window re-fetch and `UpsertDailyTotal`'s `DO UPDATE` semantics. Extend `internal/bookidentity` with `Merge` (transactional repoint across `listening_sessions`, `listening_daily_totals`, `book_identity_sources`) and add tests for the "two sources independently created two identities for the same book" case. *Rationale:* this is the riskiest phase — sequence it right after single-source proof, before anything (rollup, export) depends on both sources being reconciled correctly.

**Phase 3 — Rollup + allocation + CSV export**
Build `internal/listening/rollup.go` (`RecomputeRollup`, measured-only, rebuild-in-transaction). Build `internal/listening/attribution.go` (pure `Allocate` function; unit-test edge cases: day with only one source's data, day with conflicting totals from both sources, day with zero data). Build `internal/listening/export.go` (CSV, mirrors `goodreads/export.go` pattern, columns clearly split into measured vs. attributed). Add `earworm stats export`. *Rationale:* both rollup and export are read-only over data that Phases 1–2 already made reliable; no new mutation risk.

**Phase 4 — Journaling + daemon integration**
Build `internal/journal` (subprocess wrapper, `cmdFactory` pattern, tests with a fake factory — no real CLI invoked in tests). Add `journal_entries` CRUD to `internal/db/journal.go` (table already exists from Phase 1's migration). Add `earworm stats journal [--date] [--force]` wiring `Allocate` output into `journal.Client.AddEntry`, recording ledger rows. Modify `internal/cli/daemon.go`'s cycle closure to add a "Step 5: listening sync" call to `listening.SyncAll`, gated by `viper.GetBool("listening.enabled")` — **do not** auto-run journal emission from the daemon by default; keep it a manual command, consistent with the project's existing "no destructive/irreversible action runs unattended by default" posture (`cleanup` requires explicit invocation; `plan apply` requires promotion). *Rationale:* journaling is the only step that talks to an external, user-facing system (their journal) and is the last thing that should be automated, if ever.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Audiobookshelf | REST, Bearer token, extend existing `internal/audiobookshelf.Client` with `ListeningSessions(ctx, since)` hitting the documented listening-sessions endpoint | MEDIUM confidence — endpoint exists per official docs (`audiobookshelf.org/docs/.../listening-sessions`), exact response schema not verified here; confirm fields (session id, book id, start/end, duration) before writing the migration's `raw_payload` assumptions into typed columns |
| Audible | Subprocess via `internal/audible`, extend `AudibleClient` with a stats-fetching method | LOW confidence on mechanism — Audible does not publish an official listening-history export API; audible-cli has no documented stats command as of this research. Treat as a Phase 2 spike; the schema/sync design does not depend on the exact mechanism, only on "produces per-day, per-book seconds, mutable for ~N days" |
| External journaling CLI | Subprocess via new `internal/journal`, `cmdFactory` pattern identical to `internal/audible` | Path configurable via `journal.cli_path`; treat exit codes as the retry/failure signal, same as `audible.Download` |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| `cli/stats.go` ↔ `internal/listening` | Direct function calls (`listening.SyncAll`, `listening.Export`) | Thin CLI layer, same shape as `cli/sync.go` calling into `audible` + `db` directly |
| `cli/daemon.go` ↔ `internal/listening` | Direct call added inside the existing `cycle` closure | No change to `internal/daemon.Run` itself — it stays domain-agnostic |
| `internal/listening` ↔ `internal/bookidentity` | Direct call, passing `*sql.DB` through to `Resolve`/`Merge` | `bookidentity` owns no lifecycle state; it's a library, not a service |
| `internal/listening` ↔ `internal/journal` | `internal/listening` builds `journal.Entry` values from `Allocate` output; `cli/stats.go` wires the `journal.Client` and passes it to whatever emits, OR `listening` takes the client as a constructor param | Keep `journal.Client` as an injected dependency (interface), never constructed inside `listening`, so tests can substitute a fake |
| `internal/listening` / `internal/bookidentity` ↔ `internal/db` | Direct exported function calls (`db.UpsertSession`, `db.ResolveOrCreateIdentity`, etc.) | No SQL outside `internal/db`, no exceptions — existing hard convention |

## Sources

- Direct reads of current codebase: `internal/db/db.go`, `internal/db/books.go`, `internal/db/plans.go`, `internal/db/migrations/005_plan_infrastructure.sql`, `internal/audible/audible.go`, `internal/audiobookshelf/client.go`, `internal/daemon/daemon.go`, `internal/cli/daemon.go`, `internal/cli/sync.go`, `internal/goodreads/export.go`, `internal/config/config.go` — HIGH confidence, these establish every convention cited above.
- [Audiobookshelf: Listening Sessions](https://audiobookshelf.org/docs/documentation/server-management/listening-sessions/) — confirms session-level listening data is a first-class, documented server feature. MEDIUM confidence (confirms existence/shape at a high level; exact API field names not verified).
- [Audiobookshelf API Reference](https://api.audiobookshelf.org/) — general API reference; listening-sessions endpoint details should be re-verified against this before Phase 1 implementation.
- Audible help center ("Listening History data cannot be downloaded or exported") and general search results — confirms there is **no official** Audible listening-history export mechanism, which is why the Phase 2 build order treats the Audible side as a spike, not a settled integration. LOW confidence on any specific unofficial mechanism; do not commit to one in the roadmap without a dedicated research pass.

---
*Architecture research for: Earworm v1.2 listening-stats subsystem*
*Researched: 2026-09-20*
