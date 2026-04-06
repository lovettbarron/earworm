---
phase: 6
slug: integrations-polish
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-04
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing stdlib + testify v1.11.1 |
| **Config file** | None needed (go test built-in) |
| **Quick run command** | `go test ./internal/audiobookshelf/... ./internal/goodreads/... ./internal/daemon/... ./internal/cli/... -run "Test(Notify\|Goodreads\|Daemon)" -v` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/audiobookshelf/... ./internal/goodreads/... ./internal/daemon/... ./internal/cli/... -v`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | INT-01 | integration | `go test ./internal/audiobookshelf/... -v` | ❌ W0 | ⬜ pending |
| 06-01-02 | 01 | 1 | INT-02 | unit | `go test ./internal/config/... -v` | ✅ partial | ⬜ pending |
| 06-02-01 | 02 | 1 | INT-03 | unit | `go test ./internal/goodreads/... -v` | ❌ W0 | ⬜ pending |
| 06-03-01 | 03 | 2 | INT-04 | integration | `go test ./internal/daemon/... -v` | ❌ W0 | ⬜ pending |
| 06-04-01 | 04 | 2 | TEST-11 | integration | `go test ./internal/audiobookshelf/... ./internal/goodreads/... ./internal/daemon/... -v` | ❌ W0 | ⬜ pending |
| 06-05-01 | 05 | 3 | CLI-05 | manual | N/A (review README content) | ❌ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/audiobookshelf/client_test.go` — stubs for INT-01, INT-02 (httptest mock)
- [ ] `internal/goodreads/export_test.go` — stubs for INT-03 (CSV format verification)
- [ ] `internal/daemon/daemon_test.go` — stubs for INT-04 (lifecycle, context cancellation)
- [ ] CLI test flag resets in `internal/cli/cli_test.go` for new commands (notify, goodreads, daemon)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| README covers all v1 commands | CLI-05 | Documentation review requires human judgment | Review README.md sections: install, quickstart, command reference, config reference |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
