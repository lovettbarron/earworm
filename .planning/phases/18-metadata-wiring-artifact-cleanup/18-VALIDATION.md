---
phase: 18
slug: metadata-wiring-artifact-cleanup
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-12
---

# Phase 18 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing infrastructure |
| **Quick run command** | `go test ./internal/planengine/... ./internal/fileops/... ./internal/db/...` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/planengine/... ./internal/fileops/... ./internal/db/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 18-01-01 | 01 | 1 | FOPS-02 | unit | `go test ./internal/db/... -run TestGetBookByLocalPath` | ❌ W0 | ⬜ pending |
| 18-01-02 | 01 | 1 | FOPS-02 | unit | `go test ./internal/planengine/... -run TestWriteMetadata` | ❌ W0 | ⬜ pending |
| 18-02-01 | 02 | 2 | SCAN-02, FOPS-01, PLAN-01, INTG-01 | manual | `grep -c '\[x\]' .planning/REQUIREMENTS.md` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

*Existing infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| REQUIREMENTS.md checkboxes updated | SCAN-02, FOPS-01 | Documentation artifact | Verify checkboxes are [x] for SCAN-02 and FOPS-01 |
| ROADMAP.md progress table accurate | PLAN-01 | Documentation artifact | Compare phase checkboxes and progress table against actual state |
| SUMMARY frontmatter fixed | INTG-01 | Documentation artifact | Check 09-02, 11-01, 11-02 SUMMARY files for requirements_completed field |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
