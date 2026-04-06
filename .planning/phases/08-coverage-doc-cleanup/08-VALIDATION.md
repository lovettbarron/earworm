---
phase: 08
slug: coverage-doc-cleanup
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-04-06
---

# Phase 08 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — Go built-in |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `go test ./... -coverprofile=coverage.out` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `go test ./... -coverprofile=coverage.out`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 08-01-01 | 01 | 1 | TEST-12 | coverage | `go test ./internal/metadata/... -cover -count=1 && go test ./internal/venv/... -cover -count=1` | Yes | pending |
| 08-01-02 | 01 | 1 | TEST-12 | coverage | `go test ./internal/audible/... -cover -count=1 && go test ./internal/config/... -cover -count=1 && go test ./internal/db/... -cover -count=1 && go test ./internal/download/... -cover -count=1` | Yes | pending |
| 08-02-01 | 02 | 1 | TEST-12 | coverage | `go test ./internal/cli/... -cover -count=1` | Yes | pending |
| 08-02-02 | 02 | 1 | TEST-12 | coverage | `go test ./internal/cli/... -cover -count=1` | Yes | pending |
| 08-03-01 | 03 | 2 | TEST-12 | docs | `COUNT=$(grep -c '\[x\] \*\*Phase' .planning/ROADMAP.md) && test "$COUNT" -ge 6` | Yes | pending |
| 08-03-02 | 03 | 2 | TEST-12 | coverage+docs | `go test ./... -coverprofile=/tmp/cov.out -count=1 && TOTAL=$(go tool cover -func=/tmp/cov.out \| grep "total:" \| awk '{print $NF}' \| tr -d '%') && test $(echo "$TOTAL >= 80.0" \| bc) -eq 1` | Yes | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| REQUIREMENTS.md accuracy | TEST-12 | Traceability requires human review of 43 req mappings | Review each requirement's completion status against git history |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 15s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
