---
phase: 15
slug: data-safety-hardening-for-nas-operations
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-11
---

# Phase 15 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | None (Go convention) |
| **Quick run command** | `go test ./internal/organize/ ./internal/fileops/ ./internal/planengine/ ./internal/cli/ -v -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/organize/ ./internal/fileops/ ./internal/planengine/ ./internal/cli/ -v -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 15-01-01 | 01 | 1 | SAFE-01 | unit | `go test ./internal/organize/ -run TestCopyFile_Fsync -v` | ❌ W0 | ⬜ pending |
| 15-01-02 | 01 | 1 | SAFE-01 | unit | `go test ./internal/fileops/ -run TestVerifiedCopy_Fsync -v` | ❌ W0 | ⬜ pending |
| 15-01-03 | 01 | 1 | SAFE-02 | unit | `go test ./internal/organize/ -run TestCopyVerifyDelete_SHA256 -v` | ❌ W0 | ⬜ pending |
| 15-01-04 | 01 | 1 | SAFE-03 | unit | `go test ./internal/fileops/ -run TestFlattenDir_SkipsCleanupOnError -v` | ❌ W0 | ⬜ pending |
| 15-02-01 | 02 | 1 | SAFE-04 | unit | `go test ./internal/cli/ -run TestCleanup_PermanentDeleteAudit -v` | ❌ W0 | ⬜ pending |
| 15-02-02 | 02 | 1 | SAFE-05 | unit | `go test ./internal/planengine/ -run TestApplyPlan_ResumeAlreadyMoved -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/organize/mover_test.go` — TestCopyFile_Fsync, TestCopyVerifyDelete_SHA256
- [ ] `internal/fileops/copy_test.go` — TestVerifiedCopy_Fsync
- [ ] `internal/fileops/flatten_test.go` — TestFlattenDir_SkipsCleanupOnError
- [ ] `internal/cli/cleanup_test.go` — TestCleanup_PermanentDeleteAudit
- [ ] `internal/planengine/engine_test.go` — TestApplyPlan_ResumeAlreadyMoved

*All test stubs created as part of plan tasks — no separate Wave 0 needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| NFS mount fsync behavior | SAFE-01 | Requires real NFS mount | 1. Mount NFS share 2. Run `earworm plan apply --confirm` on test plan 3. Verify files synced via `md5sum` on server |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
