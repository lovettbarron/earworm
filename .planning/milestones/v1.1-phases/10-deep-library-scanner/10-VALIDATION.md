---
phase: 10
slug: deep-library-scanner
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-06
---

# Phase 10 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | None needed — `go test ./...` |
| **Quick run command** | `go test ./internal/scanner/ -run TestDeep -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/scanner/ ./internal/db/ -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 10-01-01 | 01 | 1 | SCAN-03 | unit | `go test ./internal/db/ -run TestMigration006 -count=1` | ❌ W0 | ⬜ pending |
| 10-01-02 | 01 | 1 | SCAN-03 | unit | `go test ./internal/db/ -run TestInsertScanIssue -count=1` | ❌ W0 | ⬜ pending |
| 10-01-03 | 01 | 1 | SCAN-03 | unit | `go test ./internal/db/ -run TestListScanIssues -count=1` | ❌ W0 | ⬜ pending |
| 10-01-04 | 01 | 1 | SCAN-03 | unit | `go test ./internal/db/ -run TestClearScanIssues -count=1` | ❌ W0 | ⬜ pending |
| 10-02-01 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectEmptyDir -count=1` | ❌ W0 | ⬜ pending |
| 10-02-02 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectNoASIN -count=1` | ❌ W0 | ⬜ pending |
| 10-02-03 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectNestedAudio -count=1` | ❌ W0 | ⬜ pending |
| 10-02-04 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectOrphanFiles -count=1` | ❌ W0 | ⬜ pending |
| 10-02-05 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectCoverMissing -count=1` | ❌ W0 | ⬜ pending |
| 10-02-06 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectMissingMetadata -count=1` | ❌ W0 | ⬜ pending |
| 10-02-07 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectWrongStructure -count=1` | ❌ W0 | ⬜ pending |
| 10-02-08 | 02 | 1 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDetectMultiBook -count=1` | ❌ W0 | ⬜ pending |
| 10-03-01 | 03 | 2 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDeepScanAllDirs -count=1` | ❌ W0 | ⬜ pending |
| 10-03-02 | 03 | 2 | SCAN-01 | unit | `go test ./internal/scanner/ -run TestDeepScanNonASIN -count=1` | ❌ W0 | ⬜ pending |
| 10-03-03 | 03 | 2 | SCAN-01 | integration | `go test ./internal/cli/ -run TestScanDeep -count=1` | ❌ W0 | ⬜ pending |
| 10-03-04 | 03 | 2 | ALL | regression | `go test ./internal/cli/ -run TestScan -count=1` | ✅ Existing | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/scanner/deep_test.go` — stubs for SCAN-01 deep traversal
- [ ] `internal/scanner/issues_test.go` — stubs for SCAN-01 issue detection heuristics
- [ ] `internal/db/scan_issues_test.go` — stubs for SCAN-03 persistence
- [ ] `internal/cli/scan_test.go` additions — stubs for --deep flag integration

*Existing infrastructure covers test framework needs (setupTestDB, createTestLibrary, testify, in-memory SQLite all working).*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Deep scan on real NAS library | SCAN-01 | Filesystem edge cases (symlinks, permissions, large dir counts) | Run `earworm scan --deep` against real library mount, verify no hangs/crashes |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
