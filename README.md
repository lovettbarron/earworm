# Earworm

A CLI-driven audiobook library manager for Audible.

Earworm tracks an existing local audiobook library (typically on a NAS mount), downloads new books from Audible via [audible-cli](https://github.com/mkb79/audible-cli), and organizes them in a Libation-compatible file structure. It integrates with Audiobookshelf for a complete audiobook workflow.

## Features

- SQLite-backed library state tracking
- YAML configuration with sensible defaults
- `earworm version` -- display build information
- `earworm config init` -- create default configuration
- `earworm config show` -- display current settings
- `earworm config set <key> <value>` -- update a setting

More commands coming soon: scan, sync, download.

## Prerequisites

- Go 1.23 or later (for building from source)
- Python 3.9+ (required by audible-cli)
- [audible-cli](https://github.com/mkb79/audible-cli) -- install via `pip install audible-cli` or `uv pip install audible-cli`

## Installation

### From Source

```bash
go install github.com/lovettbarron/earworm/cmd/earworm@latest
```

### Build from Repository

```bash
git clone https://github.com/lovettbarron/earworm.git
cd earworm
go build -o earworm ./cmd/earworm
```

## Quick Start

```bash
# Initialize configuration
earworm config init

# Set your library path
earworm config set library_path /path/to/your/audiobooks

# Verify settings
earworm config show

# Check version
earworm version
```

## Configuration

Config file location: `~/.config/earworm/config.yaml`

| Key | Description | Default |
|-----|-------------|---------|
| `library_path` | Path to audiobook library (can be NAS mount) | (empty) |
| `staging_path` | Temporary download directory | (empty, uses system temp) |
| `audible_cli_path` | Path to audible-cli binary | `audible` |
| `audiobookshelf.url` | Audiobookshelf server URL | (empty) |
| `audiobookshelf.token` | Audiobookshelf API token | (empty) |
| `audiobookshelf.library_id` | Audiobookshelf library ID | (empty) |
| `download.rate_limit_seconds` | Seconds between downloads | `5` |
| `download.max_retries` | Max retry attempts per book | `3` |
| `download.backoff_multiplier` | Exponential backoff multiplier | `2.0` |

## Global Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Use a custom config file path |
| `--quiet` / `-q` | Suppress non-essential output |

## Setting Up audible-cli

1. Install: `pip install audible-cli`
2. Authenticate: `audible quickstart` (follow prompts for Audible credentials)
3. Verify: `audible library list` should show your library

Earworm wraps audible-cli as a subprocess. Authentication is handled by audible-cli directly.

## Data Storage

- Configuration: `~/.config/earworm/config.yaml`
- Database: `~/.config/earworm/earworm.db` (SQLite, always local -- never on NAS)
- The database stores library state. It is safe to delete and will be recreated on next run.

## License

MIT License. See [LICENSE](LICENSE) for details.
