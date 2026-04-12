---
phase: 17
slug: scan-to-plan-bridge-json-output
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-12
---

# Phase 17 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) + testify |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test ./internal/...` |
| **Full suite command** | `go test -count=1 ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/...`
- **After every plan wave:** Run `go test -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 17-01-01 | 01 | 1 | SCAN-01 | unit | `go test ./internal/scanner/...` | ❌ W0 | ⬜ pending |
| 17-01-02 | 01 | 1 | SCAN-03 | unit | `go test ./internal/cli/...` | ❌ W0 | ⬜ pending |
| 17-02-01 | 02 | 2 | INTG-02 | unit | `go test ./internal/scanner/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing test infrastructure covers all phase requirements.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| End-to-end scan→plan→apply workflow | INTG-02 | Requires real library directory structure | Run `earworm scan --deep` on test library, then `earworm scan issues --create-plan`, verify plan created |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
