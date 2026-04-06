---
phase: 1
slug: foundation-configuration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-03
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + stretchr/testify v1.11.1 |
| **Config file** | None yet — Wave 0 creates project structure |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `go test -v -race -count=1 ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `go test -v -race -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | LIB-03 | unit | `go test -v -run TestDB ./internal/db/...` | ❌ W0 | ⬜ pending |
| 01-01-02 | 01 | 1 | LIB-04 | unit | `go test -v -run TestConfig ./internal/config/...` | ❌ W0 | ⬜ pending |
| 01-02-01 | 02 | 1 | CLI-01 | integration | `go test -v -run TestCLI ./internal/cli/...` | ❌ W0 | ⬜ pending |
| 01-02-02 | 02 | 1 | CLI-02 | integration | `go test -v -run TestConfig ./internal/config/...` | ❌ W0 | ⬜ pending |
| 01-02-03 | 02 | 1 | CLI-04 | unit | `go test -v -run TestQuiet ./internal/cli/...` | ❌ W0 | ⬜ pending |
| 01-03-01 | 03 | 2 | TEST-01 | unit | `go test -v -run TestDB ./internal/db/...` | ❌ W0 | ⬜ pending |
| 01-03-02 | 03 | 2 | TEST-02 | unit | `go test -v -run TestConfig ./internal/config/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/db/db_test.go` — stubs for TEST-01 (schema, CRUD, migrations)
- [ ] `internal/config/config_test.go` — stubs for TEST-02 (parsing, defaults, validation)
- [ ] `internal/cli/cli_test.go` — stubs for CLI-01, CLI-02 (command execution)
- [ ] Go installation: `brew install go`
- [ ] `go mod init github.com/lovettbarron/earworm`

*Wave 0 is handled by the first plan's initial tasks.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| README documents installation steps | SC-4 | Documentation review | Verify README.md contains installation steps and audible-cli dependency setup |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
