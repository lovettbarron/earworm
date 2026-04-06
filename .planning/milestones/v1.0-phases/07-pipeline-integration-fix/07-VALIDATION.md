---
phase: 7
slug: pipeline-integration-fix
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-05
---

# Phase 7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) + testify |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test ./internal/organize/... ./internal/download/...` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/organize/... ./internal/download/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 07-01-01 | 01 | 1 | ORG-01 | unit | `go test ./internal/download/... -run TestVerifyAndMove` | ✅ | ⬜ pending |
| 07-01-02 | 01 | 1 | ORG-02 | integration | `go test ./internal/organize/... -run TestOrganizeBook` | ✅ | ⬜ pending |
| 07-02-01 | 02 | 2 | ORG-01, ORG-02 | integration | `go test ./internal/cli/... -run TestDownloadOrganize` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Integration test for download→organize handoff (new test file)
- [ ] Test fixture: staging directory with realistic post-download file state

*Existing infrastructure covers unit test requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Daemon cycle completes end-to-end | ORG-01, ORG-02 | Requires audible-cli auth + real Audible account | Run `earworm daemon` with authenticated profile, verify book reaches 'OK' status |
| Cross-filesystem move (local→NAS) | ORG-02 | Requires NAS mount | Run `earworm organize` with library path on NAS, verify files appear correctly |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
