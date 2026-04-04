---
phase: 5
slug: file-organization
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-04
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing + testify v1.11.1 |
| **Config file** | None needed (Go convention) |
| **Quick run command** | `go test ./internal/organize/... -v` |
| **Full suite command** | `go test ./... -v` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/organize/... -v`
- **After every plan wave:** Run `go test ./... -v`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | ORG-01 | unit | `go test ./internal/organize/... -run TestBuildBookPath -v` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | ORG-01 | unit | `go test ./internal/organize/... -run TestSanitize -v` | ❌ W0 | ⬜ pending |
| 05-01-03 | 01 | 1 | ORG-01 | unit | `go test ./internal/organize/... -run TestFirstAuthor -v` | ❌ W0 | ⬜ pending |
| 05-01-04 | 01 | 1 | ORG-03 | unit | `go test ./internal/organize/... -run TestMoveFile -v` | ❌ W0 | ⬜ pending |
| 05-01-05 | 01 | 1 | ORG-03 | unit | `go test ./internal/organize/... -run TestCopyVerify -v` | ❌ W0 | ⬜ pending |
| 05-02-01 | 02 | 2 | ORG-02 | integration | `go test ./internal/organize/... -run TestOrganizeBook -v` | ❌ W0 | ⬜ pending |
| 05-02-02 | 02 | 2 | TEST-10 | integration | `go test ./internal/organize/... -run TestEndToEnd -v` | ❌ W0 | ⬜ pending |
| 05-03-01 | 03 | 2 | TEST-09 | unit | `go test ./internal/organize/... -v` | ❌ W0 | ⬜ pending |
| 05-03-02 | 03 | 2 | TEST-10 | integration | `go test ./internal/cli/... -run TestOrganize -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/organize/path_test.go` — stubs for ORG-01, TEST-09 (path construction, sanitization)
- [ ] `internal/organize/mover_test.go` — stubs for ORG-03, TEST-09 (cross-fs move, size verify)
- [ ] `internal/organize/organizer_test.go` — stubs for ORG-02, TEST-10 (end-to-end organization)
- [ ] `internal/cli/organize_test.go` — stubs for TEST-10 (CLI integration)

*Existing infrastructure covers framework needs (testify already in go.mod).*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| NAS/SMB mount moves | ORG-03 | Requires real NAS hardware | Mount SMB share, run `earworm organize`, verify files appear |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
