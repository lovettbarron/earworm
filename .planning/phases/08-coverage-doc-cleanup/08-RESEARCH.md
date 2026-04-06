# Phase 8: Test Coverage & Documentation Cleanup - Research

**Researched:** 2026-04-06
**Domain:** Go test coverage tooling, documentation maintenance
**Confidence:** HIGH

## Summary

Phase 8 closes the final v1.0 milestone gap: TEST-12 (>80% line coverage measurement) and stale documentation artifacts identified in the milestone audit. The current overall coverage is **70.7%**, roughly 10 percentage points below the 80% target. Six packages are below 80%: `cli` (58.4%), `metadata` (51.6%), `venv` (41.7%), `audible` (74.8%), `config` (79.4%), and `db` (77.6%). The biggest gains come from `cli`, `metadata`, and `venv` -- these three packages account for the majority of the deficit due to untested command runners and subprocess-dependent code paths.

The documentation cleanup is straightforward: ROADMAP.md has all 6 main phase checkboxes unchecked despite completion, the progress table is stale, and the REQUIREMENTS.md traceability was already fixed (42/43 checked, only TEST-12 remains). The `06-02-PLAN.md` checkbox is unchecked but that plan was completed. The `07-01-PLAN.md` and `07-02-PLAN.md` checkboxes need updating once Phase 7 completes.

**Primary recommendation:** Focus test effort on the three worst packages (`venv` 41.7%, `metadata` 51.6%, `cli` 58.4%), bring `db`, `config`, `audible`, and `download` above 80% with targeted tests for uncovered error paths, then update all documentation artifacts in a single pass.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TEST-12 | All packages maintain >80% line coverage; no phase ships without passing `go test ./...` | Coverage baseline measured at 70.7%. Six packages below 80% identified with per-function gap analysis. Go coverage tooling documented. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go -- single binary, good CLI ergonomics
- **Testing:** testify/assert + testify/require, in-memory SQLite for DB tests, viper.Reset() between config tests
- **Error handling:** Cobra RunE pattern, wrap errors with fmt.Errorf("context: %w", err)
- **Package layout:** cmd/earworm/ entry point, internal/ for private packages
- **GSD Workflow:** Must work through GSD commands for file changes

## Standard Stack

### Core (already in use)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| testing (stdlib) | Go 1.26 | Unit and integration tests | Built-in, already used across all packages |
| testify/assert | v1.11.1 | Test assertions | Already used everywhere |
| go test -coverprofile | Go 1.26 | Coverage measurement | Built-in Go tooling, no external deps needed |
| go tool cover | Go 1.26 | Coverage report generation | Built-in, generates per-function and HTML reports |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| go tool cover -html | Go 1.26 | Visual coverage gaps | Developer use to identify uncovered branches visually |
| go tool cover -func | Go 1.26 | Per-function coverage report | CI/verification to check per-package and total coverage |

## Architecture Patterns

### Coverage Measurement Commands

```bash
# Generate coverage profile for all packages
go test ./... -coverprofile=coverage.out

# View per-function coverage
go tool cover -func=coverage.out

# View total coverage (last line)
go tool cover -func=coverage.out | grep "total:"

# Generate HTML report for visual inspection
go tool cover -html=coverage.out -o coverage.html

# Per-package coverage in test output
go test ./... -cover
```

### Test Pattern: CLI Command Testing with Cobra

For CLI commands that are hard to test (the biggest coverage gap), the project already uses a pattern of executing commands through Cobra's test helpers:

```go
// Already established pattern in cli_test.go
func executeCommand(args ...string) (string, error) {
    buf := new(bytes.Buffer)
    rootCmd.SetOut(buf)
    rootCmd.SetErr(buf)
    rootCmd.SetArgs(args)
    err := rootCmd.Execute()
    return buf.String(), err
}
```

### Test Pattern: Subprocess Mocking for venv/metadata

The `venv` and `metadata` packages have low coverage because they shell out to external tools (Python, ffprobe). Use the established cmdFactory injection pattern:

```go
// From internal/audible -- same pattern applies to venv
type CmdFactory func(ctx context.Context, name string, args ...string) *exec.Cmd

// Test with fake command
func fakeCmdFactory(stdout, stderr string, exitCode int) CmdFactory {
    return func(ctx context.Context, name string, args ...string) *exec.Cmd {
        // Use TestHelperProcess pattern
    }
}
```

### Anti-Patterns to Avoid
- **Testing main.go:** Do not try to get coverage on `cmd/earworm/main.go` -- it is a 16-line entry point calling `cli.Execute()`. Coverage of 0% there is acceptable and standard in Go projects.
- **Chasing 100% on error type methods:** The `audible/errors.go` Error()/Unwrap() methods at 0% are trivial one-liners. Test them if easy, but they are not the priority.
- **Integration tests requiring real external services:** Do not write tests that need a real Audible account, real ffprobe installation, or real Audiobookshelf server. Use mocks/fakes.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Coverage measurement | Custom coverage scripts | `go test -coverprofile` + `go tool cover` | Built into Go toolchain |
| Coverage thresholds in CI | Manual checking | Parse `go tool cover -func` total line | One grep/awk command |
| HTML coverage reports | Custom visualizations | `go tool cover -html` | Built-in, high quality |

## Current Coverage Baseline

### Per-Package Status (measured 2026-04-06)

| Package | Coverage | Target | Gap | Priority |
|---------|----------|--------|-----|----------|
| cmd/earworm | 0.0% | N/A (exclude) | -- | SKIP |
| internal/audible | 74.8% | 80% | 5.2pp | MEDIUM |
| internal/audiobookshelf | 93.3% | 80% | -- | DONE |
| internal/cli | 58.4% | 80% | 21.6pp | HIGH |
| internal/config | 79.4% | 80% | 0.6pp | LOW |
| internal/daemon | 100.0% | 80% | -- | DONE |
| internal/db | 77.6% | 80% | 2.4pp | LOW |
| internal/download | 79.1% | 80% | 0.9pp | LOW |
| internal/goodreads | 83.3% | 80% | -- | DONE |
| internal/metadata | 51.6% | 80% | 28.4pp | HIGH |
| internal/organize | 82.8% | 80% | -- | DONE |
| internal/scanner | 80.0% | 80% | -- | DONE |
| internal/venv | 41.7% | 80% | 38.3pp | HIGH |
| **TOTAL** | **70.7%** | **80%** | **9.3pp** | -- |

### Key Uncovered Functions (by impact)

**HIGH priority (large uncovered functions in low-coverage packages):**

| Package | Function | Coverage | Statements |
|---------|----------|----------|------------|
| cli | runDownload | 12.8% | Large -- main download command runner |
| cli | runDaemon | 0.0% | Daemon command runner |
| cli | runSkip | 0.0% | Skip command runner |
| cli | runNotify | 30.8% | Notify command runner |
| cli | statusIndicator | 42.9% | Status display helper |
| cli | runGoodreads | 54.5% | Goodreads command runner |
| cli | runOrganize | 67.4% | Organize command runner |
| cli | runScan | 69.4% | Scan command runner |
| metadata | extractWithFFprobe | 32.1% | ffprobe subprocess call |
| metadata | extractWithTag | 22.2% | dhowden/tag extraction |
| venv | EnsureAudibleCLI | 32.1% | Python venv setup |

**LOW priority (small/trivial, easy wins):**

| Package | Function | Coverage | Notes |
|---------|----------|----------|-------|
| audible | Error/Unwrap (4 types) | 0.0% | Trivial one-liners, easy to test |
| audible | WithProgressFunc | 0.0% | One-liner option function |
| audible | SetProgressFunc | 0.0% | One-liner setter |
| audible | Quickstart | 0.0% | Subprocess wrapper, needs fake cmd |
| config | WriteDefaultConfig | 71.4% | Error path not covered |
| config | paths functions | 75.0% | Error paths in os.UserConfigDir |
| db | Open | 50.0% | Error paths |
| download | DefaultCmdFactory | 0.0% | One-line factory |

## Documentation Artifacts to Fix

### ROADMAP.md Issues

1. **Phase checkboxes:** All 6 main phase checkboxes are `[ ]` (unchecked). Phases 1-6 should be `[x]` after Phase 7 completes (Phase 7 and 8 remain in progress).
2. **Plan checkboxes:** `06-02-PLAN.md` is marked `[ ]` but was completed. `07-01-PLAN.md` and `07-02-PLAN.md` need updating after Phase 7.
3. **Progress table:** Shows stale data -- e.g., Phase 3 "1/3 Executing" when all 3 plans are done. All phases 1-6 should show completed status.

### REQUIREMENTS.md Issues

1. **Traceability table:** The audit found 12 items incorrectly marked "Pending" that were actually complete. Current state shows 42/43 checked -- this appears to have been partially fixed already. Only TEST-12 remains `[ ]` which is correct until this phase completes.

### Other Documentation Debt (from audit)

1. **9 of 17 SUMMARY.md files missing `requirements_completed` frontmatter** -- low priority, cosmetic
2. **`organize.RenameM4AFile` exported but never called** -- dead code, could remove or document
3. **`internal/audible/download.go:57` stale comment** -- minor cleanup

## Common Pitfalls

### Pitfall 1: Counting cmd/earworm/main.go Against Total
**What goes wrong:** The `cmd/earworm` package reports 0.0% and drags down totals.
**Why it happens:** main() is untestable without running the full binary.
**How to avoid:** Exclude `cmd/earworm` from coverage targets. The Go convention is to keep main.go minimal and test the packages it calls. Current main.go is 16 lines.
**Warning signs:** If the planner includes "test main.go" as a task.

### Pitfall 2: Subprocess-Heavy Packages Need Fakes, Not Real Calls
**What goes wrong:** Tests for `venv`, `metadata/ffprobe`, and `audible/auth` try to call real executables.
**Why it happens:** These packages wrap external tools (Python, ffprobe, audible-cli).
**How to avoid:** Use the established `cmdFactory` injection pattern or `TestHelperProcess` Go pattern. The project already does this in `internal/audible`.
**Warning signs:** Tests that fail on CI or machines without Python/ffprobe installed.

### Pitfall 3: Viper State Leaking Between CLI Tests
**What goes wrong:** CLI tests that set config values pollute other tests.
**Why it happens:** Viper uses global state.
**How to avoid:** Call `viper.Reset()` in test cleanup, as already established in the project conventions.
**Warning signs:** Tests that pass individually but fail in batch.

### Pitfall 4: Over-Mocking CLI Commands Gives False Coverage
**What goes wrong:** Tests mock so much that the actual command logic is not exercised.
**Why it happens:** CLI commands (runDownload, runOrganize, etc.) have real side effects.
**How to avoid:** Use dependency injection seams already in the code (db, audible client, etc.) to test the command logic with controlled inputs. The existing cli_test.go patterns show how.
**Warning signs:** Coverage increases but the tests do not assert meaningful behavior.

### Pitfall 5: Documentation Updates Getting Out of Sync
**What goes wrong:** Updating ROADMAP.md checkboxes without verifying actual plan completion.
**Why it happens:** Mechanical task without careful cross-referencing.
**How to avoid:** Cross-reference each plan's SUMMARY.md existence and content before marking complete. Check git log for evidence of execution.
**Warning signs:** Marking Phase 7 plans complete before Phase 7 actually finishes.

## Code Examples

### Running Coverage and Checking Threshold

```bash
# Run all tests with coverage
go test ./... -coverprofile=coverage.out

# Extract total percentage
TOTAL=$(go tool cover -func=coverage.out | grep "total:" | awk '{print $NF}' | tr -d '%')
echo "Total coverage: ${TOTAL}%"

# Check threshold (in a script or CI)
if (( $(echo "$TOTAL < 80.0" | bc -l) )); then
    echo "FAIL: Coverage ${TOTAL}% is below 80% threshold"
    exit 1
fi
```

### Testing Error Type Methods (easy wins for audible package)

```go
func TestAuthError(t *testing.T) {
    inner := errors.New("connection refused")
    err := &AuthError{Err: inner}
    assert.Contains(t, err.Error(), "connection refused")
    assert.Equal(t, inner, err.Unwrap())
}
```

### Testing CLI Commands with Dependency Injection

```go
func TestRunNotify(t *testing.T) {
    // Set up test HTTP server for ABS
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    defer ts.Close()

    viper.Reset()
    viper.Set("audiobookshelf.url", ts.URL)
    viper.Set("audiobookshelf.token", "test-token")
    viper.Set("audiobookshelf.library_id", "lib1")

    out, err := executeCommand("notify")
    require.NoError(t, err)
    assert.Contains(t, out, "scan")
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing stdlib + testify v1.11.1 |
| Config file | None needed (Go convention) |
| Quick run command | `go test ./... -count=1` |
| Full suite command | `go test ./... -coverprofile=coverage.out -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TEST-12 | All packages >80% line coverage | coverage measurement | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out \| grep total:` | N/A -- measured, not a test file |

### Sampling Rate
- **Per task commit:** `go test ./... -count=1`
- **Per wave merge:** `go test ./... -coverprofile=coverage.out -count=1`
- **Phase gate:** Total coverage >= 80%, all tests green

### Wave 0 Gaps
- None -- existing test infrastructure covers all needs. No new framework or config required.

## Open Questions

1. **Should cmd/earworm be excluded from the 80% target?**
   - What we know: It is a 16-line main.go with 0% coverage. Standard Go practice excludes it.
   - What's unclear: Whether the 80% target means "per-package" or "total across all packages."
   - Recommendation: Interpret as "overall total >= 80%" which naturally excludes the negligible impact of main.go. The current 70.7% total already excludes meaningful cmd/earworm weight.

2. **Phase 7 completion timing**
   - What we know: Phase 8 depends on Phase 7. Phase 7 plans are created but marked incomplete in ROADMAP.
   - What's unclear: Whether Phase 7 will introduce new code that changes coverage baselines.
   - Recommendation: Plan Phase 8 assuming Phase 7 is complete. Re-measure baseline at start of execution.

## Sources

### Primary (HIGH confidence)
- Direct measurement: `go test ./... -coverprofile` run on the codebase (2026-04-06)
- Direct measurement: `go tool cover -func` per-function analysis
- `.planning/v1.0-MILESTONE-AUDIT.md` -- gap identification
- `.planning/REQUIREMENTS.md` -- current traceability state
- `.planning/ROADMAP.md` -- current checkbox/progress state

### Secondary (MEDIUM confidence)
- Go documentation on coverage tooling (stable, well-known toolchain features)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- Go's built-in coverage tools, no third-party dependencies needed
- Architecture: HIGH -- patterns already established in the codebase, just need more tests
- Pitfalls: HIGH -- based on direct analysis of the codebase and established project patterns

**Research date:** 2026-04-06
**Valid until:** 2026-05-06 (stable -- coverage tooling does not change rapidly)
