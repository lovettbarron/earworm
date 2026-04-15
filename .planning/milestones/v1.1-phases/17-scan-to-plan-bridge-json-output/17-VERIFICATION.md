---
phase: 17-scan-to-plan-bridge-json-output
verified: 2026-04-12T00:00:00Z
status: human_needed
score: 9/10 must-haves verified
re_verification: false
human_verification:
  - test: "Run earworm scan --deep on a real library directory, then earworm scan issues --create-plan. Confirm plan is created in DB with correct operation types."
    expected: "Plan created output shows plan ID, operation count matching actionable issues, skipped count for non-actionable. Plan appears in earworm plan list with draft status."
    why_human: "End-to-end workflow requires real library directory. Integration tests cover logic but not real filesystem with actual M4A files."
  - test: "Verify SKILL.md covers the full scan-to-plan conversational workflow for INTG-02."
    expected: "Either skill already covers scan issues --create-plan, OR the phase only claims --json output is sufficient for INTG-02 (in which case no update needed)."
    why_human: "SKILL.md was not updated in Phase 17. The scan issues --create-plan command is not listed as an allowed command in the skill. Whether INTG-02 requires this depends on scope interpretation."
---

# Phase 17: Scan-to-Plan Bridge & JSON Output Verification Report

**Phase Goal:** Connect deep scan results to the plan system and add machine-readable scan output so the full scan→plan→apply workflow works end-to-end
**Verified:** 2026-04-12
**Status:** human_needed (9/10 automated checks pass; 2 items need human confirmation)
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | CreatePlanFromIssues creates a draft plan with operations for actionable issue types | VERIFIED | bridge.go line 30–73; 6 tests all pass |
| 2 | Non-actionable issues (no_asin, multi_book, cover_missing) are skipped and counted | VERIFIED | actionableTypes map excludes these; TestCreatePlanFromIssues_AllSkipped passes |
| 3 | Empty actionable set returns error instead of creating empty plan | VERIFIED | bridge.go line 41–43; TestCreatePlanFromIssues_EmptyInput passes |
| 4 | nested_audio issues produce flatten operations | VERIFIED | actionableTypes[IssueNestedAudio] = "flatten"; TestCreatePlanFromIssues_OperationTypes passes |
| 5 | empty_dir and orphan_files issues produce delete operations | VERIFIED | actionableTypes map; TestCreatePlanFromIssues_OperationTypes passes |
| 6 | User can run earworm scan issues to list persisted scan issues | VERIFIED | scanIssuesCmd registered; TestScanIssues_ListAll passes |
| 7 | User can filter issues by type with earworm scan issues --type nested_audio | VERIFIED | --type flag calls db.ListScanIssuesByType; TestScanIssues_FilterByType passes |
| 8 | User can create a plan from issues with earworm scan issues --create-plan | VERIFIED | runScanIssues calls scanner.CreatePlanFromIssues; TestScanIssues_CreatePlan passes |
| 9 | User can get JSON output from scan --deep --json | VERIFIED | deepScanJSON struct, json.NewEncoder in runDeepScan; TestScanDeep_JSONOutput passes |
| 10 | User can get JSON output from scan issues --json | VERIFIED | json.NewEncoder in runScanIssues; TestScanIssues_JSONOutput passes |
| 11 | Existing earworm scan (without --deep or issues) works exactly as before | VERIFIED | runScan unchanged; TestScanWithoutDeep_Unchanged passes |

**Score:** 11/11 truths verified by automated checks

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/scanner/bridge.go` | CreatePlanFromIssues function | VERIFIED | 73 lines, exports CreatePlanFromIssues, actionableTypes map, BridgeResult struct |
| `internal/scanner/bridge_test.go` | Unit tests (min 80 lines) | VERIFIED | 144 lines, 6 test functions all passing |
| `internal/cli/scan.go` | scanIssuesCmd subcommand, --json flag on scan --deep | VERIFIED | Contains scanIssuesCmd, scanJSON, scanIssuesJSON, scanCreatePlan, scanFilterType, deepScanJSON struct |
| `internal/cli/scan_test.go` | Integration tests (min 50 lines) | VERIFIED | 329 lines, 7 new test functions covering all new functionality |
| `internal/cli/cli_test.go` | Flag reset for new scan subcommand flags | VERIFIED | scanIssuesJSON = false at line 30; nested subcommand loop at lines 63–72 resets scanCmd subcommands |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/scanner/bridge.go` | `internal/db/plans.go` | db.CreatePlan and db.AddOperation | WIRED | bridge.go lines 45, 55 call both functions; TestCreatePlanFromIssues_DraftStatus confirms plan status |
| `internal/scanner/bridge.go` | `internal/scanner/issues.go` | IssueNestedAudio, IssueEmptyDir, IssueOrphanFiles | WIRED | actionableTypes map uses all three constants; bridge.go lines 18–22 |
| `internal/cli/scan.go` | `internal/scanner/bridge.go` | scanner.CreatePlanFromIssues call in --create-plan handler | WIRED | scan.go line 302: `result, err := scanner.CreatePlanFromIssues(database, issues)` |
| `internal/cli/scan.go` | `internal/db/scan_issues.go` | db.ListScanIssues and db.ListScanIssuesByType | WIRED | scan.go lines 292–295 call both functions conditionally |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| `internal/cli/scan.go` runScanIssues | issues []db.ScanIssue | db.ListScanIssues / db.ListScanIssuesByType from SQLite | Yes — queries DB table scan_issues | FLOWING |
| `internal/cli/scan.go` runDeepScan JSON path | issues []db.ScanIssue | db.ListScanIssues after scanner.DeepScanLibrary persists | Yes — persisted then queried | FLOWING |
| `internal/scanner/bridge.go` CreatePlanFromIssues | planID int64 | db.CreatePlan writes to DB, db.AddOperation writes operations | Yes — DB mutations verified by GetPlan in tests | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Bridge unit tests | `go test ./internal/scanner/ -run TestCreatePlanFromIssues -count=1` | 6/6 PASS | PASS |
| CLI integration tests | `go test ./internal/cli/ -run "TestScanIssues|TestScanDeep_JSON" -count=1` | 7/7 PASS | PASS |
| Full test suite | `go test ./... -count=1` | 15/15 packages OK | PASS |
| go vet | `go vet ./internal/cli/ ./internal/scanner/` | No issues | PASS |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| SCAN-03 | 17-01 | Detected scan issues are persisted in DB with severity, category, and suggested action | SATISFIED | CreatePlanFromIssues reads from db.ScanIssue (includes severity, IssueType, SuggestedAction). Bridge converts persisted issues to plan operations. |
| SCAN-01 | 17-02 | User can deep-scan all library folders and detect issues | SATISFIED | scan --deep path unchanged; --json flag adds structured output. TestScanDeep_JSONOutput verifies JSON includes issue counts and issues array. |
| INTG-02 | 17-02 | Claude Code skill enables conversational plan creation (not execution) via Claude Code | PARTIAL — see Human Verification | scan --deep --json IS in SKILL.md. scan issues --create-plan is NOT in SKILL.md. Whether INTG-02 requires skill update is a scope question. |

**Orphaned requirements check:** No additional requirements in REQUIREMENTS.md map to Phase 17 beyond SCAN-01, SCAN-03, INTG-02.

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None found | — | — | — | — |

Scanned files: `internal/scanner/bridge.go`, `internal/scanner/bridge_test.go`, `internal/cli/scan.go`, `internal/cli/scan_test.go`, `internal/cli/cli_test.go`. No TODOs, placeholder returns, or hardcoded empty data paths that reach rendering.

---

### Human Verification Required

#### 1. End-to-End Scan→Plan Workflow

**Test:** On a machine with a real audiobook library directory, run:
1. `earworm scan --deep` (or `earworm scan --deep --json`)
2. `earworm scan issues` — verify output lists detected issues
3. `earworm scan issues --create-plan` — verify plan is created
4. `earworm plan review <id>` — verify operations match issue types

**Expected:** Plan created with correct operation types (flatten for nested_audio, delete for empty_dir/orphan_files), non-actionable issues skipped with count, plan appears as draft in plan list.

**Why human:** Integration tests use in-memory DB and minimal directory structures. Real library may surface edge cases (permissions, symlinks, large directory trees) not covered by tests.

#### 2. INTG-02 Scope Confirmation — SKILL.md Update

**Test:** Open `.claude/skills/earworm/SKILL.md`. Check whether `earworm scan issues` and `earworm scan issues --create-plan` should be listed as allowed commands.

**Expected:** Either (a) the skill should be updated to include `earworm scan issues --create-plan` as an allowed command in the Library Scan workflow (enabling full conversational scan→plan via Claude Code), or (b) the phase scope for INTG-02 was intentionally limited to adding `--json` output for existing skill commands, not adding new skill commands.

**Why human:** This is a scope/intent question. The SKILL.md covers `earworm scan --deep --json` which was the stated gap for INTG-02 in Phase 17. Whether `scan issues --create-plan` belongs in the skill depends on whether the full conversational workflow should include it. The automated tests confirm the CLI command works; the skill integration requires a design decision.

---

### Gaps Summary

No blocking gaps found. All automated checks pass. The phase goal — "connect deep scan results to the plan system and add machine-readable scan output" — is achieved:

- `CreatePlanFromIssues` bridges scan issues to draft plan operations (Plan 01 complete)
- `earworm scan issues` with `--json`, `--type`, `--create-plan` flags connects the CLI to the bridge (Plan 02 complete)
- `earworm scan --deep --json` provides machine-readable output (consumed by SKILL.md)
- All 13 new tests pass; full suite green

The two human verification items are confirmatory (real-world test of already-verified logic) and a scope question about SKILL.md update for INTG-02. Neither blocks the phase goal.

---

_Verified: 2026-04-12_
_Verifier: Claude (gsd-verifier)_
