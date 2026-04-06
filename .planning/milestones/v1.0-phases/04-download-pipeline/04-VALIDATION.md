---
phase: 4
slug: download-pipeline
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-04
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | None needed (go test discovers tests) |
| **Quick run command** | `go test ./internal/download/ ./internal/audible/ ./internal/db/ -count=1` |
| **Full suite command** | `go test ./... -count=1` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/download/ ./internal/audible/ ./internal/db/ -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 1 | TEST-07a | unit | `go test ./internal/download/ -run TestRateLimiter -count=1` | ❌ W0 | ⬜ pending |
| 04-01-02 | 01 | 1 | TEST-07b | unit | `go test ./internal/download/ -run TestBackoff -count=1` | ❌ W0 | ⬜ pending |
| 04-01-03 | 01 | 1 | TEST-07c | unit | `go test ./internal/download/ -run TestRetry -count=1` | ❌ W0 | ⬜ pending |
| 04-01-04 | 01 | 1 | TEST-07d | unit | `go test ./internal/download/ -run TestProgress -count=1` | ❌ W0 | ⬜ pending |
| 04-01-05 | 01 | 1 | TEST-07e | unit | `go test ./internal/download/ -run TestVerify -count=1` | ❌ W0 | ⬜ pending |
| 04-02-01 | 02 | 2 | TEST-08a | integration | `go test ./internal/download/ -run TestResume -count=1` | ❌ W0 | ⬜ pending |
| 04-02-02 | 02 | 2 | TEST-08b | integration | `go test ./internal/download/ -run TestAuthAbort -count=1` | ❌ W0 | ⬜ pending |
| 04-02-03 | 02 | 2 | TEST-08c | integration | `go test ./internal/download/ -run TestFailureTracking -count=1` | ❌ W0 | ⬜ pending |
| 04-02-04 | 02 | 2 | TEST-08d | integration | `go test ./internal/download/ -run TestOrphanCleanup -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/download/` — entire package is new (pipeline, rate limiter, backoff, progress, staging, all tests)
- [ ] `internal/db/migrations/004_add_download_tracking.sql` — new migration
- [ ] DB functions: UpdateDownloadStart, UpdateDownloadComplete, UpdateDownloadError, ListDownloadable

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Real audible-cli download | DL-01, DL-02 | Requires Audible credentials and audible-cli installed | Install audible-cli, authenticate, run `earworm download --limit 1` |
| Rate limit visible delays | DL-05 | Requires observing terminal output timing | Run download with --verbose, verify delays between books |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
