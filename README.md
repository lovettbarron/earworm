# Earworm

A CLI-driven audiobook library manager for Audible, built in Go. Earworm downloads new books from Audible via [audible-cli](https://github.com/mkb79/audible-cli), organizes them into an [Audiobookshelf](https://www.audiobookshelf.org/)-compatible folder structure, and triggers library scans -- all from a single binary.

## Features

- SQLite-backed library state tracking
- Audible library sync via audible-cli
- Fault-tolerant batch downloads with rate limiting and crash recovery
- Automatic organization into `Author/Title [ASIN]/` folder structure
- Deep library scanning with structural issue detection
- Plan-based cleanup workflow (review before applying)
- CSV import for bulk operations with flexible column names and metadata
- Multi-book folder detection and splitting
- Skip management for unwanted books
- Audiobookshelf library scan integration
- Goodreads CSV export
- Daemon/polling mode for unattended operation
- Cross-filesystem file moves (local to NAS)

## Prerequisites

- **Go 1.23+** (building from source)
- **Python 3.9+** (required by audible-cli)

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
earworm organize --json
```

| Flag | Description |
|------|-------------|
| `--json` | Output results in JSON format |

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
| `daemon.polling_interval` | `6h` | Polling interval for daemon mode |
| `download.rate_limit_seconds` | `5` | Seconds between download requests |
| `download.max_retries` | `3` | Maximum retry attempts per book |
| `download.backoff_multiplier` | `2.0` | Exponential backoff multiplier for retries |
| `scan.recursive` | `false` | Scan subdirectories recursively |

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

## Goodreads Export

Export your audiobook library to a CSV file compatible with Goodreads import.

```bash
earworm goodreads -o library.csv
```

Then import the CSV at [goodreads.com/review/import](https://www.goodreads.com/review/import). Books are placed on the "read" shelf.

## Data Storage

- **Configuration:** `~/.config/earworm/config.yaml`
- **Database:** `~/.config/earworm/earworm.db` (SQLite, always local -- never on NAS)
- **Staging:** `~/.config/earworm/staging/` (temporary download directory)

The database stores library state. It is safe to delete and will be recreated on next scan or sync.

## License

MIT License. See [LICENSE](LICENSE) for details.
