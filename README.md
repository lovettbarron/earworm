# Earworm

A CLI-driven audiobook library manager for Audible, built in Go. Earworm downloads new books from Audible via [audible-cli](https://github.com/mkb79/audible-cli), organizes them into an [Audiobookshelf](https://www.audiobookshelf.org/)-compatible folder structure, and triggers library scans -- all from a single binary.

## Features

- SQLite-backed library state tracking
- Audible library sync via audible-cli
- Fault-tolerant batch downloads with rate limiting and crash recovery
- Automatic organization into `Author/Title [ASIN]/` folder structure
- Books stay safely in staging when the library mount is unavailable, and are retried on the next run
- Multi-file (MP3 etc.) audiobooks converted to single chaptered M4B files
- Deep library scanning with structural issue detection
- Plan-based cleanup workflow (review before applying)
- CSV import for bulk operations with flexible column names and metadata
- Multi-book folder detection and splitting
- Skip management for unwanted books
- Audiobookshelf library scan integration
- Goodreads CSV export
- Listening history collection from Audible and Audiobookshelf, with resumable backfill
- CSV export with explicit provenance, built for LLM analysis
- Idempotent Day One journaling of measured listening
- Reading history from Komga alongside listening
- Daemon/polling mode for unattended operation
- Cross-filesystem file moves (local to NAS)
- Contextual next-step hints after each command (suppressed with `--quiet`)

## Prerequisites

- **Go 1.23+** (building from source)
- **Python 3.9+** (required by audible-cli)
- **ffmpeg / ffprobe** (optional; required only by `earworm convert`)
- **Day One CLI** (optional; required only by `earworm stats journal`)

> **Note:** audible-cli is installed automatically into an embedded Python venv on first use. You do not need to install it manually.

## Installation

### From Source

```bash
go install github.com/lovettbarron/earworm/cmd/earworm@latest
```

### From Releases

Download the latest binary for your platform from the [GitHub Releases](https://github.com/lovettbarron/earworm/releases) page.

### Build from Repository

```bash
git clone https://github.com/lovettbarron/earworm.git
cd earworm
go build -o earworm ./cmd/earworm
```

### Verify

```bash
earworm version
```

## Quick Start

```bash
# 1. Install earworm
go install github.com/lovettbarron/earworm/cmd/earworm@latest

# 2. Initialize configuration
earworm config init

# 3. Set your library path (where audiobooks are stored)
earworm config set library_path /path/to/audiobooks

# 4. Authenticate with Audible
earworm auth

# 5. Sync your Audible library
earworm sync

# 6. Preview what would be downloaded
earworm download --dry-run

# 7. Download new books
earworm download

# 8. Organize into library structure
earworm organize
```

## Commands

### Global Flags

These flags are available on all commands:

| Flag | Description |
|------|-------------|
| `--config <path>` | Use a custom config file path |
| `--quiet` / `-q` | Suppress non-essential output |

### `earworm auth`

Authenticate with Audible via audible-cli. This is an interactive process -- you will be prompted for your Audible credentials directly by audible-cli.

```bash
earworm auth
```

### `earworm sync`

Sync Audible library metadata to the local database. Each sync is a full refresh -- all books are upserted. Local-only data (download status, file paths) is preserved.

Pre-ordered books that were previously marked unavailable (because they weren't yet released when a download was attempted) are automatically reset on re-sync, so they become downloadable once released. Books confirmed unavailable 3 or more times (e.g. expired subscriptions) stay unavailable and won't be retried.

```bash
earworm sync
earworm sync --json
```

| Flag | Description |
|------|-------------|
| `--json` | Output sync summary in JSON format |

### `earworm scan`

Scan a local library directory for existing audiobooks. The library is expected to follow the structure: `Author Name/Book Title [ASIN]/book.m4a`.

```bash
earworm scan
earworm scan --recursive
earworm scan --deep          # Also detect structural issues
earworm scan --deep --json
```

| Flag | Description |
|------|-------------|
| `--recursive` / `-r` | Recursively scan nested directories |
| `--deep` | Scan all folders including those without ASINs and detect issues |
| `--json` | Output in JSON format (only with `--deep`) |

#### `earworm scan issues`

List issues detected by the last `earworm scan --deep` run.

```bash
earworm scan issues
earworm scan issues --type nested_audio
earworm scan issues --create-plan
```

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |
| `--type <type>` | Filter issues by type (e.g., `nested_audio`, `empty_dir`, `missing_metadata`) |
| `--create-plan` | Create a remediation plan from actionable issues |

### `earworm status`

Display the current state of your audiobook library including book metadata and download status.

```bash
earworm status
earworm status --author "Sanderson"
earworm status --status downloaded --json
```

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |
| `--author <name>` | Filter by author (substring match) |
| `--status <status>` | Filter by status (exact match: `scanned`, `downloaded`, `organized`, `error`) |

### `earworm download`

Download audiobooks that are in your Audible library but not yet downloaded locally. Includes rate limiting, exponential backoff, and crash recovery.

```bash
earworm download
earworm download --dry-run
earworm download --limit 5
earworm download --asin B08G9PRS1K --asin B09FKZQ843
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview downloads without downloading |
| `--json` | Output in JSON format (dry-run mode) |
| `--limit <N>` | Maximum number of books to download (0 = no limit) |
| `--asin <ASIN>` | Download specific books by ASIN (repeatable) |

**Signal handling:** Press Ctrl+C once to finish the current book and stop. Press Ctrl+C twice to force exit immediately.

### `earworm organize`

Move downloaded audiobooks from the staging directory into the library in Audiobookshelf-compatible `Author/Title [ASIN]/` folder structure. Operates on all books with `downloaded` status.

```bash
earworm organize
earworm organize --retry
earworm organize --json
```

| Flag | Description |
|------|-------------|
| `--retry` | Reset books that previously failed to organize and try them again |
| `--json` | Output results in JSON format |

The library path is checked before anything moves. If it is missing or not
writable, organize stops with an error and every book stays in staging with its
`downloaded` status, so the next run picks them up. A move that fails part way
but leaves the files in staging is reported as `pending retry` rather than as an
error. `earworm sync` and `earworm download` warn when books are still waiting
in staging.

### `earworm convert`

Convert a multi-file audiobook (MP3, M4A and similar) into a single M4B, with
one chapter per source file. Chapter titles come from the file names, and files
are ordered naturally, so `Chapter 2` sorts before `Chapter 10`.

```bash
earworm convert B0XXXXXXXX --dry-run    # show what would happen
earworm convert B0XXXXXXXX              # convert one book
earworm convert --all                   # every multi-file book in the library
earworm convert --all --keep-originals --bitrate 128k --mono=false
```

| Flag | Description |
|------|-------------|
| `--all` | Convert every organized or scanned book that is not already a single M4B |
| `--dry-run` | Show what would be converted without doing it |
| `--keep-originals` | Keep the source audio files after conversion |
| `--bitrate <rate>` | Audio bitrate (default `64k`) |
| `--sample-rate <hz>` | Sample rate (default `44100`) |
| `--mono` | Mono output (default `true`; pass `--mono=false` for stereo) |
| `--json` | Output results in JSON format |

The M4B is built in the staging directory and moved into the book's folder only
after it succeeds. **The original audio files are then deleted** unless
`--keep-originals` is given; cover art and other non-audio files are left
alone. Books that are already a single M4B are skipped. Requires `ffmpeg` and
`ffprobe` on the PATH.

### `earworm notify`

Trigger an Audiobookshelf library scan via the API. Requires Audiobookshelf configuration (see [Audiobookshelf Integration](#audiobookshelf-integration)).

```bash
earworm notify
earworm notify --json
```

| Flag | Description |
|------|-------------|
| `--json` | Output result in JSON format |

### `earworm goodreads`

Export your library to a Goodreads-compatible CSV file for import into your Goodreads shelves.

```bash
earworm goodreads -o library.csv
earworm goodreads --output ~/exports/earworm.csv
```

| Flag | Description |
|------|-------------|
| `--output` / `-o` | Output file path (required) |

### `earworm daemon`

Run earworm in polling mode for unattended operation. Periodically runs the full sync, download, organize, and notify cycle.

```bash
earworm daemon
earworm daemon --interval 4h
earworm daemon --once --verbose
```

| Flag | Description |
|------|-------------|
| `--interval <duration>` | Polling interval (default: `6h`) |
| `--verbose` | Enable verbose logging |
| `--once` | Run one cycle and exit |

### `earworm skip`

Mark books as skipped so they are excluded from future downloads. Use for subscription books you no longer have access to, or books you don't want.

```bash
earworm skip B08G9PRS1K
earworm skip B08G9PRS1K B09FKZQ843
earworm skip B08G9PRS1K --undo    # Un-skip, make downloadable again
```

| Flag | Description |
|------|-------------|
| `--undo` | Un-skip books (mark as unknown again) |

### `earworm plan`

Manage library cleanup plans. Plans contain a set of operations (`move`, `flatten`, `delete`, `write_metadata`) that can be reviewed before applying.

```bash
earworm plan list
earworm plan list --status draft
earworm plan review 5
earworm plan apply 5              # Dry-run by default
earworm plan apply 5 --confirm    # Actually apply
earworm plan import operations.csv
earworm plan approve 5
```

#### `earworm plan list`

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |
| `--status <status>` | Filter by plan status |

#### `earworm plan review <plan-id>`

Review a plan's operations before applying.

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |

#### `earworm plan apply <plan-id>`

Apply a plan's operations. Dry-run by default -- use `--confirm` to actually apply.

| Flag | Description |
|------|-------------|
| `--confirm` | Actually apply the plan (default is dry-run preview) |
| `--json` | Output in JSON format |

#### `earworm plan import <file.csv>`

Import a plan from a CSV file. The CSV must have columns for operation type, source path, and optionally destination path. Column names are flexible -- common aliases are accepted:

| Canonical | Accepted aliases |
|-----------|-----------------|
| `op_type` | `type`, `operation`, `action` |
| `source_path` | `source`, `path`, `src`, `current_path` |
| `dest_path` | `destination`, `dest`, `target` |

Metadata columns (`title`, `author`, `narrator`, `genre`, `year`, `series`, `asin`) are extracted as JSON and attached to operations for `write_metadata` use.

| Flag | Description |
|------|-------------|
| `--name <name>` | Plan name (defaults to filename without extension) |
| `--json` | Output in JSON format |

#### `earworm plan approve <plan-id>`

Transition a draft plan to ready status so it can be applied.

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |

### `earworm cleanup`

Process delete operations from completed plans by moving files to a trash directory. Requires double confirmation before any files are moved.

```bash
earworm cleanup
earworm cleanup --plan-id 5
earworm cleanup --permanent    # DANGEROUS: permanently deletes
```

| Flag | Description |
|------|-------------|
| `--plan-id <id>` | Only process deletes from this plan |
| `--permanent` | Permanently delete instead of moving to trash (**dangerous**) |
| `--json` | Output in JSON format |

### `earworm split`

Detect and split multi-book folders (folders containing audio files from multiple audiobooks).

#### `earworm split detect <path>`

Detect book groupings in a multi-book folder.

```bash
earworm split detect /path/to/multi-book-folder
earworm split detect /path/to/multi-book-folder --json
```

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |

#### `earworm split plan <path>`

Create a split plan for a multi-book folder. Run `detect` first to preview groupings.

```bash
earworm split plan /path/to/multi-book-folder
```

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |

### `earworm stats`

Collect and inspect your listening history. Listening data stays on your machine:
it is written to the local SQLite database and, in later commands, exported only
to a local directory.

```bash
# One-time import of your full Audible listening history
earworm stats backfill

# Incremental update, safe to run repeatedly
earworm stats sync

# What is stored locally
earworm stats status
```

**Flags:**

- `--source <name>` -- which source to read: `audible` (default), `abs` (Audiobookshelf) or `komga`
- `--full` -- ignore saved progress and refetch everything
- `--json` -- machine-readable summary

Backfill is **resumable**: progress is saved after each window, so an
interrupted run continues where it stopped rather than starting over. Re-running
a completed backfill refreshes recent days without duplicating anything, because
day totals are replaced rather than accumulated.

The initial backfill makes a few hundred API calls and takes several minutes at
the default rate limit. `earworm stats sync` afterwards costs only a handful.

#### What Audible actually provides

Worth knowing, because it shapes what the data can tell you:

| Signal | Coverage |
|--------|----------|
| Daily and monthly listening totals | No book attached -- Audible reports time, not what it was spent on |
| Per-book status changes | A timestamp per book, but it records the last *change*, including un-finishing |
| Per-book last playback position | Position and timestamp, for books that have been opened |
| Library metadata | Title, authors, narrators, series, genres, runtime, purchase date |

Daily-granularity data does not exist before 2015, even where monthly totals
reach further back, so `stats.backfill_start` defaults to `2015-01-01`.

Because status timestamps are change events rather than completions, earworm
screens them for **bulk clusters**: when many books share a timestamp within a
sub-second window, that is a mass-marking or account-migration artifact, not
evidence those books were finished at that moment. Such rows are flagged and
`earworm stats status` reports how many were caught.


#### Audiobookshelf listening history

Audiobookshelf records **exact per-book playback sessions**, which is the one
thing Audible cannot provide: Audible reports how long you listened, but never
to what.

```bash
# Confirm the server is reachable and the token works
earworm stats check

# Import the full session history
earworm stats backfill --source abs

# Incremental update afterwards
earworm stats sync --source abs
```

Requires an **API key**, created under Settings → API Keys in Audiobookshelf.
Set `audiobookshelf.url` and `audiobookshelf.token`; `audiobookshelf.user_id` is
resolved from the token if you leave it blank.

`earworm stats check` reports the server version and the account the token
belongs to, and distinguishes an unreachable server from a rejected token --
worth running before a long backfill.

Two behaviours are worth knowing:

- **Sessions are mutable.** A session stays open while playback continues and
  only auto-closes after 36 hours idle, so its listening time keeps growing
  after earworm first sees it. Sync therefore re-reads a 48-hour window before
  the last watermark and updates sessions in place by their server ID. Syncing
  strictly forward would freeze open sessions at a stale value; inserting
  instead of updating would double-count them.
- **Session metadata is a snapshot** taken at play time, and its genre and
  series fields are frequently empty. Set `stats.enrich` (on by default) to fill
  these from the library items themselves.

Backfill uses the admin `/api/sessions` endpoint, which paginates server-side.
The per-user endpoint loads your entire session table into memory on every page
request, which makes a large backfill needlessly expensive for the server.

#### Exporting the dataset

```bash
earworm stats export                  # writes to stats.export_dir
earworm stats export -o ./mystats     # or anywhere you choose
earworm stats export --timeline       # also write the denormalised file
earworm stats matches                 # review uncertain book matches
```

Four files, plus a README explaining them:

| File | One row per |
|------|-------------|
| `books.csv` | book, merged across sources |
| `days.csv` | day and book |
| `sessions.csv` | playback session (Audiobookshelf only) |
| `reading.csv` | finished volume (Komga) |
| `timeline.csv` | day and book, with book metadata inlined (opt-in) |

The normalised trio is the source of truth. `timeline.csv` is derived from it in
the same pass, so the two cannot drift apart. Feed the normalised files to an
LLM when it can read several files; `timeline.csv` exists for when it is easier
to paste one.

**Every row states how much to trust it.** The `attribution` column is not a
score to be averaged — the values mean different things:

| Value | Meaning |
|-------|---------|
| `exact` | A playback session recorded this book on this day. Measured. |
| `inferred-single` | No session data; one candidate book plausibly accounts for the day. Reconstructed. |
| `inferred-split` | Several books shared the day and the time was divided. Weaker. |
| `unattributed` | The duration is real; the book is unknown. |

Audible reports how long you listened but never to what, so its days are
inferred or unattributed, never `exact`. The inference walks each book backwards
from the last date there is evidence for it — a finish event or last playback
position — consuming unassigned time until its estimated listening is accounted
for. It is a hypothesis about which book the time belonged to, not a record.

**Inference is never stored.** It is computed at export time from measured
facts, so the database holds only what was actually observed, and a change to
the algorithm cannot retroactively rewrite your history.

#### How books are matched across sources

A book can appear in both sources, in one, or under different titles. ASIN is
used where both sides have one; otherwise titles are normalised (series
numbering, `(Unabridged)`, volume numbering and leading articles removed) and
compared by word overlap, with the author required to agree when both are known.

Subtitles are deliberately kept. Dropping everything after a colon or dash
would collapse `Some Saga` and `Some Saga - Second Volume` into one book and
sum their listening time, and that shape cannot be told apart from a genuine
descriptive subtitle. The cost is that a book titled `Title: Subtitle` in one
source and `Title` in the other will not match on title alone -- two visible
identities, which is recoverable, rather than a silent merge, which is not.

`books.csv` records the method in `match_method`, and `earworm stats matches`
lists everything that was not an exact ASIN match so you can eyeball it.

Matching deliberately errs toward leaving books separate. A false merge blends
two books' histories and is nearly invisible; an unmatched book is still
exported in full and is easy to spot. Books that match nothing are kept, not
dropped -- books you own outside Audible are a normal part of a listening
history.

#### Journaling to Day One

```bash
earworm stats journals                        # list destinations
earworm config set journal.journal_id <id>    # choose one
earworm stats journal                         # dry run (default)
earworm stats journal --write                 # actually write
earworm stats journal --date today --write    # just today
```

Requires the Day One CLI (`npm install -g @dayone/cli`, then `dayone auth
login --email <you>`). Note this is the npm CLI, not the older `dayone2` that
ships inside the Day One Mac app; they share a binary name, so install only one.

**Dry run is the default.** This is the only earworm command that modifies a
record outside its own database, so writing requires `--write` explicitly.

**Only measured facts become entries.** Audiobookshelf sessions name the book
that was played, so those become daily digests. Audible reports how long you
listened but never to what, so its days are excluded rather than guessed at.
That reconstruction lives in `earworm stats export`, where the `attribution`
column marks it for what it is. A journal is a record you will trust years
later; a plausible guess does not belong in one.

`--finishes` additionally writes an entry per finished book, dated from the
source's own finish event.

`--estimated-finishes` recovers books that were completed but have no usable
finish date, using the last playback position instead. Two cases:

- the finish timestamp was overwritten by a bulk marking, which destroys the
  date but not the fact of completion
- the book was listened to at least 95% of its runtime but never marked finished

These entries are headed `Finished (estimated)` and carry a footer saying the
date came from playback rather than a recorded finish, so the journal never
presents an estimate as a record. The estimated date is when the book was last
open, which for a completed book is within a day or so of finishing it.

This matters most for older history: a single bulk marking can wipe the finish
dates of an entire shelf at once, and the playback position is then the only
surviving evidence of when those books were actually read.

**One entry per day, covering listening and reading.** A day with both gets a
single entry with a **Listening** section and a **Reading** section, rather than
two entries competing for the same date. A day with only reading still gets an
entry. Reading is counted in volumes, not time, because Komga records that a
volume was finished but not how long it took. A day's volumes are grouped by
series and consecutive runs condensed, so a dozen chapters of one series in an
evening reads as `Some Series — 5 volumes (10–14)` rather than a dozen bullets.
Komga books flagged by `komga.unreliable_before` never appear in entries.

Entries use an ID derived from the date, and the Day One CLI treats a repeat as
an update, so re-running revises the existing entry rather than adding another.
Days with neither listening nor reading produce no entry. Writes are queued to a local outbox
and pushed with an explicit sync afterwards, reported separately so "written
but not pushed" is distinguishable from "not written".

#### Komga (comics, manga, ebooks)

```bash
earworm config set komga.url http://your-server:25600
earworm config set komga.api_key <key>       # Komga account settings
earworm stats sync --source komga
```

Komga stores one progress record per book with no reading history, which is
the same shape as Audible. It matters far less here: a book in Komga is a
single volume or chapter, so reading a series produces one dated completion per
volume, which is already a dense timeline with nothing to reconstruct.

Release-group and format suffixes are stripped from titles on import:
`Some Title (2018-2021) (Digital) (GroupName)` is stored as `Some Title`. These
carry nothing about the work, and because every volume of a series repeats the
same suffix they also made different volumes score as near-identical when
matching.

Reading lands in the same tables as listening, so `books.csv` and the journal
pick it up through the paths that already exist. Finished volumes produce
finish entries on the same terms as finished audiobooks.

**Library migrations need declaring.** Re-marking a shelf as read after
restoring a Komga instance writes completion timestamps that look exactly like
reading. Set `komga.unreliable_before` to the last day of the migration; those
books stay in the export, flagged, and are kept out of journal entries. The
flag is decided from the stored completion date, so a book that later
disappears from Komga -- a re-import changes every book id -- keeps it.

**A re-import after that date is caught by its shape.** A restore marks a run
of books read at one instant. Earworm treats a run of completions inside 30
seconds as re-marking when it spans three or more distinct series, which is
what separates it from reading: an evening of five volumes is one series, while
a restore sweeps up unrelated titles. On the library this was measured against,
one re-import wrote 78 books across 63 series in 17 seconds, and no genuine
reading session was touched.

Neither test can be perfect. A batch you marked yourself, in one or two series,
is indistinguishable from reading and is left alone; a restore of a single
series is too. Check `earworm stats status` for the flagged count after a
restore, and extend `komga.unreliable_before` if something slipped through.

### `earworm config init`

Create the default configuration file at `~/.config/earworm/config.yaml`.

```bash
earworm config init
```

### `earworm config show`

Display the current configuration with all values.

```bash
earworm config show
```

### `earworm config set`

Update a configuration setting.

```bash
earworm config set library_path /mnt/nas/audiobooks
earworm config set download.rate_limit_seconds 10
earworm config set audiobookshelf.url http://nas:13378
```

### `earworm version`

Display build version, commit hash, and build date.

```bash
earworm version
```

## Configuration

Config file location: `~/.config/earworm/config.yaml`

| Key | Default | Description |
|-----|---------|-------------|
| `library_path` | *(none)* | Path to audiobook library -- can be a NAS mount (required) |
| `staging_path` | `~/.config/earworm/staging` | Temporary download directory |
| `audible_cli_path` | `audible` | Path to audible-cli binary (default uses managed venv) |
| `audible.profile_path` | *(none)* | Path to audible-cli profile directory |
| `audiobookshelf.url` | *(none)* | Audiobookshelf server URL |
| `audiobookshelf.token` | *(none)* | Audiobookshelf API token |
| `audiobookshelf.library_id` | *(none)* | Audiobookshelf library ID |
| `audiobookshelf.user_id` | *(resolved)* | Audiobookshelf user ID; looked up from the token when blank |
| `daemon.polling_interval` | `6h` | Polling interval for daemon mode |
| `download.rate_limit_seconds` | `5` | Seconds between download requests |
| `download.max_retries` | `3` | Maximum retry attempts per book |
| `download.backoff_multiplier` | `2.0` | Exponential backoff multiplier for retries |
| `scan.recursive` | `false` | Scan subdirectories recursively |
| `library.probe_timeout_seconds` | `10` | How long to wait for the library path to respond before declaring it unavailable |
| `library.remount_command` | *(none)* | Shell command run once to restore the mount when it is unreachable; unmount verbs are refused |
| `library.allow_unmounted` | `false` | Skip the check that a library under a mount root is actually mounted |
| `stats.timezone` | *(UTC)* | IANA timezone used to bucket listening days, e.g. `Europe/Berlin` |
| `stats.backfill_start` | `2015-01-01` | Earliest date a backfill reaches |
| `stats.rate_limit_seconds` | `2` | Seconds between listening-history API calls |
| `stats.export_dir` | `~/.config/earworm/stats` | Where CSV exports are written |
| `stats.enrich` | `true` | Fetch library items to fill in genres and series missing from session data |
| `komga.url` | *(none)* | Komga server URL |
| `komga.api_key` | *(none)* | Komga API key, from account settings |
| `komga.unreliable_before` | *(none)* | Completions on or before this date are flagged as migration artifacts |
| `journal.cli_path` | `dayone` | Path to the Day One CLI binary |
| `journal.journal_id` | *(none)* | Destination journal; see `earworm stats journals` |
| `journal.include_finishes` | `false` | Also write an entry per finished book |
| `journal.estimated_finishes` | `false` | Recover finishes whose date must be estimated from playback |
| `journal.daemon_write` | `false` | Allow the daemon to write journal entries unattended |
| `daemon.stats_sync` | `true` | Sync listening history on each daemon cycle |

## Audiobookshelf Integration

Earworm can trigger an [Audiobookshelf](https://www.audiobookshelf.org/) library scan after organizing downloads, so new books appear automatically in your media server.

### Setup

1. **Get your API token:** In Audiobookshelf, go to **Settings > Users > (your user) > API Token**. Copy the token.

2. **Get your library ID:** Check the URL when viewing your library in Audiobookshelf -- the ID is in the path (e.g., `http://nas:13378/library/abc123` -- the ID is `abc123`).

3. **Configure earworm:**

```yaml
# ~/.config/earworm/config.yaml
audiobookshelf:
  url: http://your-server:13378
  token: your-api-token
  library_id: your-library-id
```

Or via CLI:

```bash
earworm config set audiobookshelf.url http://your-server:13378
earworm config set audiobookshelf.token your-api-token
earworm config set audiobookshelf.library_id your-library-id
```

### Usage

After downloads are organized, trigger a scan manually:

```bash
earworm notify
```

In daemon mode, the scan is triggered automatically after each organize cycle.

## Daemon Mode

Run earworm in the background for fully unattended audiobook management. The daemon runs a full cycle (sync, download, organize, notify) at a configurable interval.

### Basic Usage

```bash
earworm daemon                    # Poll every 6 hours (default)
earworm daemon --interval 4h     # Poll every 4 hours
earworm daemon --once             # Run one cycle and exit
```

### systemd Service

Create `/etc/systemd/system/earworm.service`:

```ini
[Unit]
Description=Earworm audiobook library manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=your-user
ExecStart=/usr/local/bin/earworm daemon
Restart=on-failure
RestartSec=60

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable earworm
sudo systemctl start earworm
sudo systemctl status earworm
```

### launchd (macOS)

Create a plist at `~/Library/LaunchAgents/com.earworm.daemon.plist` with the `earworm daemon` command. Load with `launchctl load`.

### Listening stats in the daemon

With `daemon.stats_sync` on (the default), each cycle also syncs listening
history from Audible and Audiobookshelf, and reading from Komga when
`komga.url` and `komga.api_key` are set. Komga is synced before the journal
step, so an entry written in the evening includes that day's reading. Journal writes stay off unless
`journal.daemon_write` is explicitly enabled, so the daemon will not modify a
personal record on its own.

A stats sync that outlasts the polling interval causes the next cycle to skip
the step rather than run two concurrently, which would interleave writes to the
same rows and journal entries.

## Goodreads Export

Export your audiobook library to a CSV file compatible with Goodreads import.

```bash
earworm goodreads -o library.csv
```

Then import the CSV at [goodreads.com/review/import](https://www.goodreads.com/review/import). Books are placed on the "read" shelf.

## When the Library Is Unavailable

The library usually lives on a network mount, and a mount that goes away does
not fail — it hangs. A filesystem call on a dead SMB share blocks
uninterruptibly, so a command that simply starts reading never returns and
never says why. A daemon that hits this stops running cycles entirely, silently.

Every command that touches the library now probes it first, in a goroutine that
is abandoned if it does not answer within `library.probe_timeout_seconds`. An
unreachable library becomes an immediate, explanatory error:

```
Error: library path /Volumes/media/Audible did not respond within 10s;
the mount is present but not answering
```

The probe opens the directory and reads an entry rather than calling stat: a
stale mount can answer a stat from cached metadata while real access hangs.

**In the daemon**, an unreachable library skips only the steps that need it.
Sync, listening and reading stats, and journaling all reach networked services
and the local database, never the mount, so they keep running. An offline NAS
costs you downloads and file organisation, not your whole pipeline.

**Mounting is verified, not assumed.** An unmounted share fails in the most
expensive way available: the mount point stays behind as an ordinary empty
directory, every readability check passes, and earworm creates parent
directories on demand. Without a check, a library sync would write gigabytes to
the boot disk into a path that looks exactly right, and nothing downstream
would notice.

So a library path under `/Volumes`, `/mnt`, `/media` or `/run/media` is
verified to be on a different filesystem from the boot volume before anything
is written. A library that genuinely lives on local disk is unaffected and
needs no configuration; set `library.allow_unmounted=true` only for a local
library that happens to sit beneath one of those directories.

**earworm will not unmount anything.** A remount command containing `umount`,
`unmount`, `eject`, `mkfs`, `diskutil erase` and similar is refused. Mounting
is recoverable; a forced unmount can cut short another process's write to the
same share and truncate a file, and nothing about restoring a mount requires
tearing one down first. Unmount by hand if you need to.

**Remounting.** Set `library.remount_command` to a shell command that restores
the mount and earworm runs it once when the path is unreachable, then re-probes
to confirm. Success is judged by the path answering, not by the command's exit
code, because a mount command can exit zero without the share becoming usable.
It is empty by default: the command is host-specific and needs credentials, and
guessing at one risks mounting the wrong thing over the right place. On macOS
it is usually something like:

```bash
earworm config set library.remount_command "open 'smb://user@host/share'"
```

## Listening Data and Privacy

Listening and reading history is personal data. Earworm keeps it local:

- It is stored in the SQLite database under `~/.config/earworm/`, alongside your library state.
- Nothing is transmitted anywhere else. The only outbound requests are to the sources you configure, and to Day One when you run `earworm stats journal --write` or enable `journal.daemon_write`.
- CSV exports are written to `stats.export_dir` on your own machine, which the repository's `.gitignore` excludes so exports cannot be committed by accident.

**Set `stats.timezone`** to your own zone. Day boundaries decide which day a
late-evening listening session belongs to, and the default of UTC will split
evening sessions across two days if you are far from it. Earworm never uses the
host's local zone implicitly, so that stored data does not change meaning when
run from a machine in a different zone.

## Data Storage

- **Configuration:** `~/.config/earworm/config.yaml`
- **Database:** `~/.config/earworm/earworm.db` (SQLite, always local -- never on NAS)
- **Staging:** `~/.config/earworm/staging/` (temporary download directory)

The database stores library state, plus listening and reading history. It is safe to delete: library state is recreated on the next scan or sync, and history is rebuilt by running `earworm stats backfill` for each source.

## License

MIT License. See [LICENSE](LICENSE) for details.
