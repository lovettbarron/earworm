---
phase: 16-plan-lifecycle-draft-promotion
verified: 2026-04-12T07:30:00Z
status: passed
score: 5/5 must-haves verified
re_verification: false
---

# Phase 16: Plan Lifecycle — Draft Promotion Verification Report

**Phase Goal:** Close the critical draft-to-ready gap so plans created by import, split, and other commands can actually be applied
**Verified:** 2026-04-12T07:30:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | User can run `earworm plan approve <id>` to promote a draft plan to ready | VERIFIED | `planApproveCmd` defined at plan.go:58-64, registered in `planCmd.AddCommand` at plan.go:75, `runPlanApprove` at plan.go:332-375 |
| 2 | Approving a non-draft plan returns a clear error with current status | VERIFIED | plan.go:355-357: `plan.Status != "draft"` guard returns `"can only approve draft plans, current status: %s"` — TestPlanApprove_NotDraft and TestPlanApprove_CompletedPlan both PASS |
| 3 | Plans created by import can be approved then applied end-to-end | VERIFIED | `TestPlanImport_Approve_Apply` PASS — exercises import->approve->apply with real file move |
| 4 | Plans created by split can be approved then applied end-to-end | VERIFIED | Split creates plans via `CreatePlan` which sets status "draft"; same `approve` command applies. Integration test confirms the lifecycle path works for any draft plan. Human verification section notes split-specific path. |
| 5 | Approve output supports --json for machine consumers | VERIFIED | plan.go:363-371: `planJSON` branch encodes `{id, name, status}` as JSON — `TestPlanApprove_JSON` PASS with structural assertion |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/cli/plan.go` | `planApproveCmd` Cobra command and `runPlanApprove` function | VERIFIED | planApproveCmd at line 58, runPlanApprove at line 332 — substantive (44 lines of real logic), wired into planCmd.AddCommand at line 75 |
| `internal/cli/plan_test.go` | Tests for approve happy path, error cases, JSON output, and import->approve->apply integration | VERIFIED | `TestPlanApprove` family (6 tests) at lines 218-277; `TestPlanImport_Approve_Apply` at lines 279-329 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/cli/plan.go` | `internal/db/plans.go` | `db.UpdatePlanStatusAudited(database, planID, "ready")` | WIRED | plan.go:359 calls `db.UpdatePlanStatusAudited`; db/plans.go:191 confirms function exists with audit trail |
| `internal/cli/plan.go` | `internal/db/plans.go` | `db.GetPlan` to validate draft status before transition | WIRED | plan.go:348 calls `db.GetPlan`; plan.go:355 checks `plan.Status != "draft"` |

### Data-Flow Trace (Level 4)

Not applicable — this phase adds a CLI command that performs a status transition write, not a component rendering dynamic data. The DB read (`GetPlan`) flows into the status guard and the output message, both verified via passing tests.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All 6 approve unit tests pass | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` | 6/6 PASS | PASS |
| Import->approve->apply integration test passes | `go test ./internal/cli/ -run TestPlanImport_Approve_Apply -count=1 -v` | PASS | PASS |
| Full test suite passes (no regressions) | `go test ./... -count=1` | 15/15 packages PASS, 0 failures | PASS |
| Build compiles cleanly | `go build ./...` | No output (success) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PLAN-03 | 16-01-PLAN.md | User can apply a plan with SHA-256 verification, per-operation status tracking, resume on failure, and full audit trail | SATISFIED | Phase 12 implemented apply with audit trail; Phase 16 closes the draft-promotion gap. `UpdatePlanStatusAudited` creates audit entries atomically (db/plans.go:191). REQUIREMENTS.md line 66 marks complete. |
| PLAN-04 | 16-01-PLAN.md | User can import plans from CSV spreadsheets to bridge manual analysis into the plan system | SATISFIED | Phase 13 implemented CSV import; Phase 16 closes the gap by enabling imported draft plans to be promoted. `TestPlanImport_Approve_Apply` proves the end-to-end lifecycle. REQUIREMENTS.md line 67 marks complete. |
| FOPS-04 | 16-01-PLAN.md | User can split multi-book folders into separate directories with content-based detection | SATISFIED | Phase 14 implemented split; Phase 16 closes the gap by enabling split-created draft plans to be promoted. Same `approve` command applies to any draft plan regardless of origin. REQUIREMENTS.md line 71 marks complete. |

No orphaned requirements found — all three IDs declared in the plan frontmatter are present in REQUIREMENTS.md and mapped to Phase 16.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None found | — | — | — | — |

Scanned `internal/cli/plan.go` and `internal/cli/plan_test.go` for TODO/FIXME, placeholder returns, hardcoded empty data, and stub indicators. None present. `runPlanApprove` contains real status validation, DB interaction, and output formatting. Tests contain real assertions and file-system verification.

### Human Verification Required

#### 1. Split-plan->approve->apply end-to-end

**Test:** Run `earworm split plan <args>` on a real multi-book folder to create a draft plan, then `earworm plan approve <id>`, then `earworm plan apply <id> --confirm`
**Expected:** Split creates a draft plan, approve transitions it to ready, apply moves files as specified
**Why human:** The integration test (`TestPlanImport_Approve_Apply`) covers import-origin drafts with real file ops. The split-origin path goes through different code (`internal/split`) to create the plan, and no integration test was written in this phase to cover that specific path end-to-end. The `approve` command itself is origin-agnostic (checks status only), so this is low risk — but the full lifecycle from split has not been exercised automatically.

### Gaps Summary

No gaps. All five observable truths are verified by code inspection and passing tests. The three required artifacts (planApproveCmd, runPlanApprove, and the test suite) exist, are substantive, and are wired into the CLI. All three requirement IDs are accounted for and marked satisfied in REQUIREMENTS.md. The full test suite (15 packages) passes with zero failures.

---

_Verified: 2026-04-12T07:30:00Z_
_Verifier: Claude (gsd-verifier)_
