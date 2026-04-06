---
phase: 01-foundation-configuration
plan: 02
subsystem: cli-config
tags: [cobra, viper, cli, config, yaml]

requires: [01-01]
provides:
  - Cobra CLI framework with root, version, and config commands
  - Viper config system with YAML file at ~/.config/earworm/
  - Global --quiet and --config flags
  - Config show/set/init subcommands
  - 20 passing unit tests for config and CLI
affects: [02-local-library-scanning, 03-audible-integration, 04-download-engine]

tech-stack:
  added: [spf13/cobra v1.10.2, spf13/viper v1.21.0, gopkg.in/yaml.v3]
  patterns: [PersistentPreRunE for config init, cmd.OutOrStdout for testable output, viper.Reset in tests]

key-files:
  created:
    - internal/cli/root.go
    - internal/cli/version.go
    - internal/cli/config.go
    - internal/config/config.go
    - internal/config/paths.go
    - internal/config/config_test.go
    - internal/cli/cli_test.go
  modified:
    - cmd/earworm/main.go
    - go.mod
    - go.sum

key-decisions:
  - "Config path hardcoded to ~/.config/earworm/ (XDG-style, not macOS Application Support)"
  - "Config set validates keys against allowlist, rejects unknown keys"
  - "Config init refuses to overwrite existing config files"
  - "PersistentPreRunE on root command initializes config for all subcommands"
  - "viper.Reset() in every test to avoid global state leakage"

patterns-established:
  - "CLI test helper: executeCommand(t, args...) with viper.Reset and buffer capture"
  - "Config test helper: resetViper(t) with t.Cleanup"
  - "Config defaults set via viper.SetDefault in SetDefaults()"

requirements-completed: [LIB-04, CLI-01, CLI-02, TEST-02]

duration: 10min
completed: 2026-04-03
---

# Phase 1 Plan 2: Cobra CLI + Viper Config Summary

**CLI framework with Cobra commands and Viper-managed YAML configuration, plus 20 passing tests**

## Performance

- **Duration:** 10 min
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- Built Cobra CLI framework with root command, version, and config subcommands
- Implemented Viper config system with YAML file, sensible defaults, and validation
- Added global --quiet and --config flags
- Config commands: init (create default), show (display), set (modify with key validation)
- All 20 tests passing (14 config + 6 CLI) with race detection

## Task Commits

1. **Task 1: Create config package and CLI command framework** - `f5ec240` (feat)
2. **Task 2: Write and pass config and CLI unit tests** - `5a99337` (test)

## Files Created/Modified
- `internal/cli/root.go` - Root Cobra command with --quiet/-q and --config flags
- `internal/cli/version.go` - Version subcommand showing build info
- `internal/cli/config.go` - Config subcommand with init/show/set
- `internal/config/config.go` - Viper setup, defaults, validation, WriteDefaultConfig
- `internal/config/paths.go` - ConfigDir, ConfigFilePath, DBPath resolution
- `internal/config/config_test.go` - 14 config unit tests
- `internal/cli/cli_test.go` - 6 CLI integration tests
- `cmd/earworm/main.go` - Updated to use cli.Execute() with version injection
- `go.mod` / `go.sum` - Added Cobra, Viper, and yaml.v3 dependencies

## Deviations from Plan
None — all planned functionality implemented as specified.

## Issues Encountered
None.

## Known Stubs
None — all planned functionality is fully implemented and tested.

## Next Phase Readiness
- CLI framework and config system complete, ready for Plan 03 (README + GoReleaser)
- All subsequent phases can use the config system for settings

---
*Phase: 01-foundation-configuration*
*Completed: 2026-04-03*
