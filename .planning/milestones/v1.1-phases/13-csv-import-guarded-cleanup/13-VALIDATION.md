---
phase: 13
slug: csv-import-guarded-cleanup
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-10
---

# Phase 13 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | None (stdlib `go test`) |
| **Quick run command** | `go test ./internal/planengine/ ./internal/cli/ -run "CSV\|Cleanup\|Import" -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/planengine/ ./internal/cli/ -run "CSV\|Cleanup\|Import" -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 13-01-01 | 01 | 1 | PLAN-04 | unit | `go test ./internal/planengine/ -run TestImportCSV_Valid -count=1` | ❌ W0 | ⬜ pending |
| 13-01-02 | 01 | 1 | PLAN-04 | unit | `go test ./internal/planengine/ -run TestImportCSV_BOM -count=1` | ❌ W0 | ⬜ pending |
| 13-01-03 | 01 | 1 | PLAN-04 | unit | `go test ./internal/planengine/ -run TestImportCSV_Errors -count=1` | ❌ W0 | ⬜ pending |
| 13-01-04 | 01 | 1 | PLAN-04 | integration | `go test ./internal/cli/ -run TestPlanImport -count=1` | ❌ W0 | ⬜ pending |
| 13-02-01 | 02 | 1 | FOPS-03 | unit | `go test ./internal/planengine/ -run TestCleanup_TrashMove -count=1` | ❌ W0 | ⬜ pending |
| 13-02-02 | 02 | 1 | FOPS-03 | unit | `go test ./internal/planengine/ -run TestCleanup_OnlyCompleted -count=1` | ❌ W0 | ⬜ pending |
| 13-02-03 | 02 | 1 | FOPS-03 | unit | `go test ./internal/cli/ -run TestCleanup_Confirm -count=1` | ❌ W0 | ⬜ pending |
| 13-02-04 | 02 | 1 | FOPS-03 | unit | `go test ./internal/planengine/ -run TestCleanup_Audit -count=1` | ❌ W0 | ⬜ pending |
| 13-02-05 | 02 | 1 | FOPS-03 | integration | `go test ./internal/cli/ -run TestCleanupCommand -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/planengine/csvimport_test.go` — stubs for PLAN-04 (import valid, BOM, errors)
- [ ] `internal/planengine/cleanup_test.go` — stubs for FOPS-03 (trash move, completed-only, audit)
- [ ] `internal/cli/cleanup_test.go` — stubs for FOPS-03 (confirmation, CLI integration)
- [ ] Export `IsValidOpType` from db/plans.go for CSV validation

*Existing test infrastructure covers framework and config needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Cross-filesystem trash move (NAS) | FOPS-03 | Requires actual NAS mount | Mount a second filesystem, set trash_dir there, run cleanup |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
