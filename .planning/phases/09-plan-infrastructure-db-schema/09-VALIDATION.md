---
phase: 9
slug: plan-infrastructure-db-schema
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-07
---

# Phase 9 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) + testify |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test ./internal/db/...` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/db/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 09-01-01 | 01 | 1 | PLAN-01 | unit | `go test ./internal/db/ -run TestMigration005` | ❌ W0 | ⬜ pending |
| 09-01-02 | 01 | 1 | PLAN-01 | unit | `go test ./internal/db/ -run TestPlanCRUD` | ❌ W0 | ⬜ pending |
| 09-01-03 | 01 | 1 | SCAN-02 | unit | `go test ./internal/db/ -run TestLibraryItem` | ❌ W0 | ⬜ pending |
| 09-01-04 | 01 | 1 | PLAN-01 | unit | `go test ./internal/db/ -run TestAuditLog` | ❌ W0 | ⬜ pending |
| 09-01-05 | 01 | 1 | INTG-01 | integration | `go test ./internal/db/ -run TestPlanWithAudit` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/db/plan_test.go` — stubs for PLAN-01 plan/operation CRUD
- [ ] `internal/db/library_item_test.go` — stubs for SCAN-02 path-keyed items
- [ ] `internal/db/audit_test.go` — stubs for audit log entries

*Existing test infrastructure (testify, in-memory SQLite) covers framework needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Migration applies cleanly on existing DB | PLAN-01 | Requires real DB with prior migrations | Run `earworm` against a copy of production DB, verify no errors |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
