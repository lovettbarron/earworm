---
phase: 08
slug: coverage-doc-cleanup
status: draft
nyquist_compliant: false
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
| 08-01-01 | 01 | 1 | TEST-12 | coverage | `go test ./... -coverprofile=coverage.out` | ✅ | ⬜ pending |
| 08-01-02 | 01 | 1 | TEST-12 | unit | `go test ./internal/cli/...` | ✅ | ⬜ pending |
| 08-02-01 | 02 | 1 | TEST-12 | docs | `grep -c '\[x\]' .planning/ROADMAP.md` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

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

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
