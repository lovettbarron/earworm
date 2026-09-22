---
phase: 17-scan-to-plan-bridge-json-output
plan: 01
subsystem: scanner
tags: [bridge, plan-creation, scan-issues]
dependency_graph:
  requires: [internal/db/plans.go, internal/db/scan_issues.go, internal/scanner/issues.go]
  provides: [CreatePlanFromIssues]
  affects: [scan-to-plan workflow]
tech_stack:
  added: []
  patterns: [actionable-issue-filtering, bridge-pattern]
key_files:
  created: [internal/scanner/bridge.go, internal/scanner/bridge_test.go]
  modified: []
decisions:
  - "Only 3 issue types are actionable (nested_audio->flatten, empty_dir->delete, orphan_files->delete); others require human judgment"
metrics:
  duration: 2min
  completed: 2026-04-12
---

# Phase 17 Plan 01: Scan-to-Plan Bridge Summary

CreatePlanFromIssues bridges deep scanner issues to draft plan operations, filtering actionable types (nested_audio, empty_dir, orphan_files) and skipping types requiring human review.

## What Was Built

- `internal/scanner/bridge.go`: `CreatePlanFromIssues()` function that accepts scan issues and creates a draft plan with operations for actionable issue types
- `internal/scanner/bridge_test.go`: 6 test cases covering all acceptance criteria

## Key Implementation Details

- **Actionable types map**: `IssueNestedAudio` -> "flatten", `IssueEmptyDir` -> "delete", `IssueOrphanFiles` -> "delete"
- **Non-actionable types**: `no_asin`, `multi_book`, `cover_missing`, `missing_metadata`, `wrong_structure` are skipped with count
- **Error on empty**: Returns descriptive error when no actionable issues found (including count of manual-review issues)
- **BridgeResult**: Returns plan ID, created count, and skipped count
- **Draft status**: Plans created via `db.CreatePlan` default to "draft" status

## Commits

| Hash | Message |
|------|---------|
| 0986f83 | test(17-01): add failing tests for scan-to-plan bridge |
| 3fc79fe | feat(17-01): implement CreatePlanFromIssues scan-to-plan bridge |

## Verification

```
go test ./internal/scanner/ -run TestCreatePlanFromIssues -count=1 -v
--- PASS: TestCreatePlanFromIssues_ActionableOnly
--- PASS: TestCreatePlanFromIssues_AllSkipped
--- PASS: TestCreatePlanFromIssues_EmptyInput
--- PASS: TestCreatePlanFromIssues_OperationTypes
--- PASS: TestCreatePlanFromIssues_DraftStatus
--- PASS: TestCreatePlanFromIssues_PlanNaming
PASS
```

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None.

## Self-Check: PASSED
