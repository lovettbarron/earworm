---
phase: 3
slug: audible-integration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-04
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) + testify/assert + testify/require |
| **Config file** | none — existing infrastructure from Phase 1 |
| **Quick run command** | `go test ./internal/audible/... ./internal/db/... ./internal/cli/...` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/audible/... ./internal/db/... ./internal/cli/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | AUD-01 | integration | `go test ./internal/cli/ -run TestAuth` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | AUD-02 | unit | `go test ./internal/audible/ -run TestLibraryList` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | AUD-03 | unit+integration | `go test ./internal/audible/ -run TestSync` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | AUD-04 | unit | `go test ./internal/db/ -run TestNewBooks` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | LIB-05 | integration | `go test ./internal/cli/ -run TestDryRun` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | TEST-05 | unit | `go test ./internal/audible/ -run TestHelper` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | TEST-06 | integration | `go test ./internal/cli/ -run TestSyncFlow` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/audible/audible_test.go` — stubs for subprocess wrapper tests (TestHelperProcess pattern)
- [ ] `internal/db/books_test.go` — extend with upsert/new-book detection test stubs
- [ ] `internal/cli/auth_test.go` — auth command integration test stubs
- [ ] `internal/cli/sync_test.go` — sync command integration test stubs

*Existing test infrastructure (go test, testify) covers all framework needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Interactive auth flow | AUD-01 | Requires real Audible account and browser | Run `earworm auth`, complete Audible login, verify profile created |
| Real Audible library sync | AUD-03 | Requires real Audible account with books | Run `earworm sync` after auth, verify books appear in `earworm status` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
