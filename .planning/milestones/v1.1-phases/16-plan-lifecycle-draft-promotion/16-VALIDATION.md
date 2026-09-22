---
phase: 16
slug: plan-lifecycle-draft-promotion
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-12
---

# Phase 16 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | None (stdlib) |
| **Quick run command** | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/cli/ -run TestPlanApprove -count=1 -v`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 16-01-01 | 01 | 1 | PLAN-03 | integration | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` | ❌ W0 | ⬜ pending |
| 16-01-02 | 01 | 1 | PLAN-03 | integration | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` | ❌ W0 | ⬜ pending |
| 16-01-03 | 01 | 1 | PLAN-04 | integration | `go test ./internal/cli/ -run TestPlanImport_Approve_Apply -count=1 -v` | ❌ W0 | ⬜ pending |
| 16-01-04 | 01 | 1 | FOPS-04 | integration | `go test ./internal/cli/ -run TestPlanApprove -count=1 -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Tests for `plan approve` happy path (draft -> ready)
- [ ] Tests for `plan approve` error cases (non-draft, not found, invalid ID)
- [ ] Tests for `plan approve --json`
- [ ] Integration test: import -> approve -> apply end-to-end

*Existing infrastructure covers all phase requirements — no new test framework or fixtures needed.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
