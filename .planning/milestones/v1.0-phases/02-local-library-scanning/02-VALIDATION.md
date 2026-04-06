---
phase: 2
slug: local-library-scanning
status: draft
nyquist_compliant: true
wave_0_complete: true
wave_0_note: "TDD task structure (tdd='true' on code-producing tasks) satisfies Wave 0 intent -- tests are co-created with implementation in RED-GREEN-REFACTOR cycles, ensuring every task has automated verification before code is written."
created: 2026-04-03
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | None needed — `go test ./...` |
| **Quick run command** | `go test ./internal/scanner/... ./internal/metadata/... ./internal/cli/... -count=1` |
| **Full suite command** | `go test ./... -v -count=1` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/scanner/... ./internal/metadata/... ./internal/cli/... -count=1`
- **After every plan wave:** Run `go test ./... -v -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | LIB-01 | unit | `go test ./internal/scanner/... -v -run TestASIN -count=1` | TDD co-created | pending |
| 02-01-02 | 01 | 1 | LIB-01 | unit | `go test ./internal/scanner/... -v -run TestScan -count=1` | TDD co-created | pending |
| 02-01-03 | 01 | 1 | TEST-03 | unit | `go test ./internal/metadata/... -v -count=1` | TDD co-created | pending |
| 02-02-01 | 02 | 2 | LIB-02 | integration | `go test ./internal/cli/... -v -run TestStatus -count=1` | TDD co-created | pending |
| 02-02-02 | 02 | 2 | LIB-06 | integration | `go test ./internal/cli/... -v -run TestStatusJSON -count=1` | TDD co-created | pending |
| 02-02-03 | 02 | 2 | CLI-03 | integration | `go test ./internal/cli/... -v -run TestError -count=1` | TDD co-created | pending |
| 02-02-04 | 02 | 2 | TEST-04 | integration | `go test ./internal/cli/... -v -run "TestScan\|TestStatus" -count=1` | TDD co-created | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

All code-producing tasks in both plans have `tdd="true"`, which means tests are written BEFORE implementation in the RED-GREEN-REFACTOR cycle. This satisfies Wave 0 intent -- every task creates its test file as the first step, ensuring automated verification exists before any production code.

Test files created by TDD tasks:
- `internal/scanner/scanner_test.go` — covers LIB-01, TEST-03 (ASIN extraction, directory walking, incremental sync)
- `internal/scanner/asin_test.go` — covers TEST-03 (ASIN regex table-driven tests)
- `internal/metadata/metadata_test.go` — covers TEST-03 (tag extraction, fallback chain)
- `internal/cli/scan_test.go` — covers TEST-04 (scan command integration)
- `internal/cli/status_test.go` — covers LIB-02, LIB-06, TEST-04 (status command, JSON output)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Spinner renders on NAS mount | D-10 | Real NAS latency needed | Run `earworm scan` on actual NAS-mounted library, verify spinner appears |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or TDD co-creation satisfying Wave 0
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covered by TDD task structure (tests written before implementation)
- [x] No watch-mode flags
- [x] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
