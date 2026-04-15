---
phase: 12
slug: plan-engine-cli
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-10
---

# Phase 12 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | none — stdlib |
| **Quick run command** | `go test ./internal/planengine/ ./internal/cli/ -run TestPlan -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/planengine/ ./internal/cli/ -run TestPlan -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 12-01-01 | 01 | 1 | PLAN-03 | unit | `go test ./internal/planengine/ -run TestApply -count=1` | ❌ W0 | ⬜ pending |
| 12-01-02 | 01 | 1 | PLAN-03 | unit | `go test ./internal/planengine/ -run TestResume -count=1` | ❌ W0 | ⬜ pending |
| 12-01-03 | 01 | 1 | PLAN-03 | unit | `go test ./internal/planengine/ -run TestAuditHash -count=1` | ❌ W0 | ⬜ pending |
| 12-02-01 | 02 | 2 | PLAN-02 | integration | `go test ./internal/cli/ -run TestPlanReview -count=1` | ❌ W0 | ⬜ pending |
| 12-02-02 | 02 | 2 | PLAN-03 | integration | `go test ./internal/cli/ -run TestPlanApplyDryRun -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/planengine/engine_test.go` — stubs for PLAN-03 (apply, resume, audit hash)
- [ ] `internal/cli/plan_test.go` — stubs for PLAN-02, PLAN-03d (review, dry-run)

*Existing infrastructure covers test framework — no new framework install needed.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
