# Phase 1: Foundation & Configuration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-03
**Phase:** 01-foundation-configuration
**Areas discussed:** Project layout, Config design, CLI command structure, Database schema

---

## Project Layout

| Option | Description | Selected |
|--------|-------------|----------|
| Standard Go layout | cmd/earworm/ for main, internal/ for private packages. Idiomatic for CLI tools. | ✓ |
| Flat layout | Everything in root or few top-level packages. Simpler but can get messy. | |
| Domain-grouped | Group by domain (library/, audible/) rather than by layer. | |

**User's choice:** Standard Go layout
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| github.com/albair/earworm | Standard GitHub-based module path | |
| github.com/lovett-barron/earworm | If GitHub username differs from local user | |
| You decide | Claude picks | |

**User's choice:** github.com/lovettbarron/earworm (custom input)
**Notes:** User specified exact module path

---

## Config Design

| Option | Description | Selected |
|--------|-------------|----------|
| YAML | Most common for Go CLI tools. Viper handles natively. | ✓ |
| TOML | Explicit, no indentation ambiguity. Popular in Rust/Go. | |
| You decide | Claude picks | |

**User's choice:** YAML
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| ~/.config/earworm/ | XDG-compliant. Config and DB together. | ✓ |
| ~/.earworm/ | Simpler dotfile convention. | |
| Alongside the binary | Portable but unusual for CLI tools. | |

**User's choice:** ~/.config/earworm/
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, same dir | ~/.config/earworm/earworm.db — all state together | ✓ |
| XDG data dir | ~/.local/share/earworm/ — separates config from data | |
| You decide | Claude picks | |

**User's choice:** Same directory as config
**Notes:** None

---

## CLI Command Structure

| Option | Description | Selected |
|--------|-------------|----------|
| version + config | Minimal but verifiable — proves CLI and config work | ✓ |
| version + config + status | Also status stub proving DB integration | |
| You decide | Claude determines | |

**User's choice:** version + config
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| Quiet with --verbose | Minimal default, --verbose for details | |
| Informative default | Show useful info, --quiet for silent mode | ✓ |
| You decide | Claude picks | |

**User's choice:** Informative default
**Notes:** None

---

## Database Schema

| Option | Description | Selected |
|--------|-------------|----------|
| Embedded SQL + version table | SQL files via Go embed, schema_version tracking | ✓ |
| Code-based migrations | Migration functions in Go code | |
| You decide | Claude picks | |

**User's choice:** Embedded SQL + version table
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| Books table + config metadata | Books table ready for Phase 2, plus schema_version | ✓ |
| Schema version only | Just migration tracking, add books in Phase 2 | |
| You decide | Claude determines | |

**User's choice:** Books table + config metadata
**Notes:** None

---

## Claude's Discretion

- Specific config keys and defaults
- Error message wording and formatting style
- Test file organization within packages
- README structure and content depth

## Deferred Ideas

None — discussion stayed within phase scope
