---
phase: 11
slug: structural-operations-metadata
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-07
---

# Phase 11 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) + testify/assert + testify/require |
| **Config file** | none — standard Go test infrastructure |
| **Quick run command** | `go test ./internal/fileops/ -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/fileops/ -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 11-01-01 | 01 | 1 | FOPS-01 | unit | `go test ./internal/fileops/ -run TestSHA256` | ❌ W0 | ⬜ pending |
| 11-01-02 | 01 | 1 | FOPS-01 | unit | `go test ./internal/fileops/ -run TestFlatten` | ❌ W0 | ⬜ pending |
| 11-02-01 | 02 | 1 | FOPS-02 | unit | `go test ./internal/fileops/ -run TestMetadata` | ❌ W0 | ⬜ pending |
| 11-02-02 | 02 | 1 | FOPS-01,02 | integration | `go test ./internal/fileops/ -run TestIntegration` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/fileops/` package — new package for file operation primitives
- [ ] Test files created alongside implementation (TDD or co-created)

*Existing Go test infrastructure covers framework requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Cross-filesystem flatten (NAS mount) | FOPS-01 | Requires actual NAS mount | 1. Create nested audio dir on NAS 2. Run flatten 3. Verify files moved up |
| metadata.json ABS compatibility | FOPS-02 | Requires running Audiobookshelf | 1. Write metadata.json 2. Trigger ABS scan 3. Verify metadata appears |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
