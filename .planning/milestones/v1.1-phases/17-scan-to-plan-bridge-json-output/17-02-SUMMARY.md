---
phase: 17-scan-to-plan-bridge-json-output
plan: 02
subsystem: cli
tags: [scan-issues, json-output, cli-wiring]
dependency_graph:
  requires: [internal/scanner/bridge.go, internal/db/scan_issues.go]
  provides: [scanIssuesCmd, deepScanJSON]
  affects: [scan workflow, CLI output]
tech_stack:
  added: []
  patterns: [cobra-subcommand, json-encoding]
key_files:
  created: [internal/cli/scan_test.go]
  modified: [internal/cli/scan.go, internal/cli/cli_test.go]
decisions:
  - "JSON output uses cmd.OutOrStdout() for testability (same pattern as split command)"
metrics:
  duration: 5min
  completed: 2026-04-12
---

# Phase 17 Plan 02: CLI Wiring & JSON Output Summary

Wired scan-to-plan bridge into CLI via `earworm scan issues` subcommand and added JSON output to `earworm scan --deep --json`.

## What Was Built

- `earworm scan issues` subcommand listing persisted scan issues with `--type`, `--json`, `--create-plan` flags
- `earworm scan --deep --json` structured JSON output with full issue details
- 7 integration tests covering all new CLI paths
- Flag reset additions in `cli_test.go` for test isolation

## Key Implementation Details

- **scanIssuesCmd**: Cobra subcommand registered under scanCmd with RunE handler
- **--type filter**: Uses `db.ListScanIssuesByType` when set, else `db.ListScanIssues`
- **--create-plan**: Calls `scanner.CreatePlanFromIssues`, reports plan ID and operation counts
- **--json**: JSON output via `json.NewEncoder(cmd.OutOrStdout()).SetIndent("", "  ")`
- **deepScanJSON struct**: Combines DeepScanResult fields with full issue list for structured output
- **Flag resets**: Added `scanJSON`, `scanIssuesJSON`, `scanCreatePlan`, `scanFilterType` resets and scanCmd subcommand flag reset loop

## Commits

| Hash | Message |
|------|---------|
| 158f8b0 | feat(17-02): add scan issues subcommand and deep scan JSON output |
| f18ce23 | test(17-02): add integration tests for scan issues and deep scan JSON |

## Verification

```
go test ./internal/cli/ -run "TestScanIssues|TestScanDeep_JSON" -count=1 -v
--- PASS: TestScanIssues_ListAll
--- PASS: TestScanIssues_FilterByType
--- PASS: TestScanIssues_JSONOutput
--- PASS: TestScanIssues_CreatePlan
--- PASS: TestScanIssues_CreatePlan_NoActionable
--- PASS: TestScanIssues_EmptyList
--- PASS: TestScanDeep_JSONOutput
PASS

go test ./... -count=1
All 15 packages pass.
```

## Deviations from Plan

None - plan executed as written.

## Known Stubs

None.

## Self-Check: PASSED
