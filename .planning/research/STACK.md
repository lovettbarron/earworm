# Stack Research

**Domain:** Listening-stats extraction, cross-source title matching, CSV export, and third-party CLI journaling — additions to an existing Go CLI (earworm v1.2)
**Researched:** 2026-09-20
**Confidence:** HIGH (verified against pkg.go.dev, GitHub releases, and existing repo code)

## Summary Verdict

None of the five new capabilities require a new *runtime* Go dependency for their core logic. The existing stack (stdlib `net/http`, `encoding/json`, `encoding/csv`, `os/exec`, `crypto/sha256`) already covers everything except the actual fuzzy-matching algorithm, and even that is small enough (~600 × ~70 candidate pairs) to hand-roll in well under 150 lines with full test coverage, matching the codebase's existing bias toward stdlib-first design (see `internal/audiobookshelf/client.go`, `internal/goodreads/export.go`).

The one *optional* addition worth naming explicitly is `hashicorp/go-retryablehttp` for the Audiobookshelf pagination client — and the recommendation is still **no**, for reasons below. The one genuinely new *external* (non-Go) dependency is the `dayone` npm-distributed CLI binary, which is not a Go library decision but does need an integration/testing plan.

## Recommended Stack

### Core Technologies (no new Go dependencies)

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `encoding/json` (stdlib) | Go 1.23+ (repo on 1.26.1) | Parse `audible api <endpoint>` and Audiobookshelf JSON responses | Already used throughout `internal/audible` (`ParseLibraryExport`). No new capability needed — `audible-cli`'s `api` subcommand prints plain JSON to stdout for arbitrary Audible API calls, same shape of problem already solved for `library export`. |
| `net/http` (stdlib) | Go 1.23+ | Paginated Audiobookshelf REST client | `internal/audiobookshelf/client.go` already wraps `net/http` directly with a `Bearer` token. Audiobookshelf's list endpoints (`/api/users/<id>/listening-sessions`, `/api/me/listening-stats`) use simple `page`/`limit` query params — a handful of lines in a `for` loop covers pagination completely. |
| `encoding/csv` (stdlib) | Go 1.23+ | Listening-stats CSV export | `internal/goodreads/export.go` is the exact template to follow: define a header slice, iterate, write records, flush. No new capability required — this is the same shape of problem, solved. |
| `os/exec` (stdlib) | Go 1.23+ | Subprocess calls to `audible api ...` and the `dayone` CLI | `internal/audible/audible.go`'s `cmdFactory` seam (`WithCmdFactory`) already generalizes command construction for testing. Reuse the identical pattern for both new subprocess integrations rather than inventing a second abstraction. |
| `crypto/sha256` (stdlib) | Go 1.23+ | Deterministic hashing (e.g. stable match/dedup keys, idempotency keys for journal entries) | Already the repo's sole hashing algorithm (`internal/fileops/hash.go`, `internal/organize/mover.go`, `internal/planengine/cleanup.go`). `crypto/md5` is also stdlib and technically sufficient for non-cryptographic dedup keys, but introducing a second hash algorithm into a codebase that has standardized on SHA-256 buys nothing and adds a second thing for readers to reason about. **Use SHA-256, not MD5, purely for consistency.** |

### Supporting Libraries — Fuzzy Title Matching

**No library is recommended.** Hand-roll normalization + a small hand-rolled similarity function in a new internal package (e.g. `internal/matching`). Full rationale and comparison below.

| Library | Version | Last Release | Verdict |
|---------|---------|---------------|---------|
| `lithammer/fuzzysearch` | v1.1.8 | May 2023 | **Rejected** — wrong tool: it's a substring/typo "did you mean" matcher (single-character-insertion fuzzy find + Levenshtein ranking of short query against a word), not a general string-similarity scorer for comparing two full titles. Would need to be repurposed awkwardly. Unmaintained 2+ years. |
| `adrg/strutil` (+ `strutil/metrics`) | v0.3.1 | Sep 2023 | **Rejected but closest fit** — clean API (`strutil.Similarity(a, b, metrics.NewJaroWinkler())`), includes Levenshtein, Jaro-Winkler, Sorensen-Dice, Jaccard. Well-designed, but every algorithm it offers is ~15-30 lines to implement directly, and the actual hard part of this problem (stripping `"(#17) "`, `" (Unabridged)"`, `" - Subtitle"` noise) is normalization logic the library can't do for you regardless. Adopting it for one function call is not proportional. |
| `hbollon/go-edlib` | v1.7.0 | Aug 2025 | **Rejected** — most actively maintained of the four, broadest algorithm set (Levenshtein, Damerau-Levenshtein, Jaro-Winkler, cosine, q-gram, etc.), reasonable choice if this were a larger or evolving matching problem. Still rejected here: it's a large surface area for a single comparison need, and its distance-based algorithms don't handle word-reordering well (e.g. `"Example Saga - Third Movement"` vs an Audible title that leads with series info) — a token-set overlap approach (below) handles that better and doesn't need any library. |
| `agext/levenshtein` | v1.2.3 | Mar 2020 | **Rejected** — single-purpose, last released 2020, no Unicode-aware token handling. Superseded in every dimension by go-edlib. |

**Why hand-rolled is the right call at this scale:**

- **Volume:** ~600 books × ~70 candidates = at most 42,000 pairwise comparisons, each on strings of ~20-60 characters. A classic O(n·m) Levenshtein DP table (~20 lines of Go) or a token-set Jaccard/Dice comparison (~15 lines using `strings.Fields` + a `map[string]struct{}`) runs this in well under a second. This is not a performance problem needing an optimized library implementation.
- **The hard part is normalization, not the metric.** Given examples like `"(#17) Example Chronicle"`, `"Second Example Tale (Unabridged)"`, `"Example Saga - Third Movement"`, the dominant source of match failure is series-number prefixes, `"(Unabridged)"`/`"(Abridged)"` suffixes, subtitle separators (`" - "`, `":"`), and punctuation/case differences — none of which any similarity library solves. That logic has to be written regardless of which (if any) distance function is chosen.
- **Token-set comparison beats pure edit-distance for this data.** Titles that reorder or drop words (series prefixes, subtitle annotations) are better served by comparing normalized *word sets* (Jaccard/Dice coefficient, or a "token sort ratio" — sort words alphabetically then compare) than by raw character-level Levenshtein distance, which penalizes reordering heavily. This is straightforward to hand-roll and arguably *more correct* for this specific data shape than what off-the-shelf character-distance libraries provide out of the box.
- **Dependency-adoption bar:** the project's own conventions (stdlib logging over zerolog/zap, stdlib HTTP over resty, pure-Go SQLite over CGo) consistently favor zero-dependency solutions when the problem is small and well-understood. A single hand-rolled ~100-150 line matcher with its own table-driven test suite (following the `testify` convention already used everywhere) is more maintainable long-term than an external dependency whose most-current option (`go-edlib`) is still a larger API surface than needed, and whose second-most-current option (`fuzzysearch`) hasn't been touched since 2023.

**Recommended design (for the requirements/roadmap phase, not fully specified here):**
1. Normalize: lowercase, strip leading `(#N)` / `#N` series markers, strip trailing parenthetical annotations (`(Unabridged)`, `(Abridged)`, narrator credits), strip subtitle after `" - "` or `":"` if a first-pass exact match fails, strip punctuation, collapse whitespace.
2. Tier 1: exact match on normalized title (+ author if available). This alone will resolve the large majority of ~600 books.
3. Tier 2: fallback token-set similarity (Dice/Jaccard on word sets) with a threshold (e.g. ≥0.7) for the remainder, surfaced to the user as "probable matches" for confirmation rather than silently auto-joined.
4. Anything below threshold, or with multiple ambiguous candidates: reported unmatched, not guessed. Silent wrong joins are worse than a manual review step for a ~70-item long tail.

### Supporting Libraries — HTTP Pagination / Retry

| Library | Version | Verdict |
|---------|---------|---------|
| `hashicorp/go-retryablehttp` | v0.7.8 (Jun 2025) | **Rejected.** Actively maintained and a reasonable choice in general, but disproportionate here: the Audiobookshelf integration is a handful of authenticated GET calls against a REST API the user's own server, on a local/trusted network, for a one-shot CLI invocation — not a long-running service hammering a flaky third-party API. `net/http.Client` with a `context.Context` timeout (the existing pattern in `internal/audiobookshelf/client.go`) plus a small hand-rolled retry-on-5xx loop (5-10 lines, mirroring the exponential-backoff logic the project already implements for Audible downloads per the `PROJECT.md` rate-limiting constraint) is sufficient and keeps the dependency graph flat. |

**Pagination approach:** Audiobookshelf's list endpoints use `page` (zero-indexed) and `limit` query params, with `limit=0` returning all results unpaginated. For ~70 items, a single `limit=0` call may be sufficient; if page-by-page is preferred for safety against large libraries, a simple `for page := 0; ; page++` loop checking `len(results) < limit` to detect the last page is all that's needed — no helper library changes this code meaningfully.

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `httptest.Server` (stdlib, `net/http/httptest`) | Test the Audiobookshelf pagination/stats client | Already the established pattern in `internal/audiobookshelf/client_test.go` and `internal/cli/notify_test.go`. Extend it: stand up a fake multi-page handler that returns different JSON per `page` query param to test pagination termination logic. |
| `cmdFactory` injection seam (existing pattern, `internal/audible`) | Test subprocess wrapping for `audible api ...` and `dayone` | **Use this exact pattern, do not invent a second one.** `internal/audible/audible.go`'s `WithCmdFactory(f func(ctx, name, args...) *exec.Cmd)` + the `TestHelperProcess`/`GO_WANT_HELPER_PROCESS` Go subprocess-test idiom (visible in `internal/audible/audible_test.go`) is already proven in this codebase for exactly this shape of problem (external CLI, JSON/text stdout, stderr-based error classification, context cancellation). Apply it identically to: (a) a new `audible api` wrapper for whatever endpoint listening-stats needs, and (b) a new `dayone` client that pipes journal text via stdin (`cmd.Stdin = strings.NewReader(entryText)`) instead of args/flags. `internal/venv/venv.go` independently reinforces this as the repo's standard subprocess-testing convention — two existing packages already agree on it. |
| `testify/assert` + `testify/require` | Assertions for new matcher, CSV writer, and stats client tests | Already a repo-wide dependency (v1.11.1); no version change needed. |

## Installation

No new Go module dependencies are required for the core feature set described in this milestone.

```bash
# No `go get` needed for: JSON parsing, HTTP pagination, CSV export,
# subprocess wrapping, or hashing — all stdlib, already in go.mod.

# If title-matching needs later prove the hand-rolled approach insufficient
# (unlikely at ~600x70 scale), the fallback choice would be:
go get github.com/hbollon/go-edlib@v1.7.0   # NOT currently recommended — see rationale above
```

External (non-Go) tooling the milestone depends on, managed outside `go.mod`:

```bash
# dayone CLI — npm-distributed binary, shelled out to via os/exec.
# This is an external runtime dependency analogous to the existing
# Python/audible-cli requirement, NOT a Go library.
npm install -g dayone   # or npx dayone, per whatever the requirements phase settles on
```

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|--------------------------|
| Hand-rolled normalization + token-set similarity | `hbollon/go-edlib` v1.7.0 | If the matching problem grows significantly (e.g. matching against multiple additional sources, or needing several distance algorithms interchangeably/configurably), go-edlib's broader, actively-maintained algorithm set becomes proportional. Not the case at ~600×70 scale with two known sources. |
| Hand-rolled normalization + token-set similarity | `adrg/strutil` v0.3.1 | If the team wants a well-tested, documented Jaro-Winkler/Sorensen-Dice implementation rather than hand-rolling the metric (while still hand-rolling normalization), this is the cleanest single-purpose option of the four. Reasonable minority choice, not the default recommendation. |
| Hand-rolled retry loop | `hashicorp/go-retryablehttp` v0.7.8 | If Audiobookshelf calls expand significantly (many more endpoints, long-running daemon polling against Audiobookshelf rather than one-shot CLI calls), a real retry-client library starts paying for itself. Not justified for the current scope of a few stats/session endpoints called once per `earworm stats` invocation. |
| `crypto/sha256` (existing) | `crypto/md5` (stdlib) | Never, in this codebase — MD5 is stdlib-equivalent effort but breaks the established single-hash-algorithm convention for no benefit. Only relevant if some external system this milestone integrates with *requires* MD5 specifically (not indicated by anything in the milestone context). |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|--------------|
| `lithammer/fuzzysearch` | Solves a different problem (substring/"did you mean" fuzzy find against a word list), not two-full-string similarity scoring. Unmaintained since May 2023. | Hand-rolled token-set similarity |
| `agext/levenshtein` | Single-metric, unmaintained since March 2020, no advantage over hand-rolling the same ~20-line DP algorithm. | Hand-rolled Levenshtein/Dice, or `go-edlib` if a library is truly wanted |
| Any general-purpose HTTP client library (`resty`, etc.) for Audiobookshelf pagination | Same reasoning the existing STACK.md already applied to the scan-trigger client: too few endpoints to justify the dependency. Adding pagination doesn't change that math — it's still 2-3 endpoints, called rarely, on a trusted local server. | `net/http` stdlib, same client shape as `internal/audiobookshelf/client.go` |
| `hashicorp/go-retryablehttp` for this milestone | Correctly built and maintained, but disproportionate for a handful of one-shot GETs against the user's own local Audiobookshelf instance. | Hand-rolled retry loop, same spirit as the Audible download backoff already required by `PROJECT.md` |
| A second subprocess-testing abstraction (e.g. an interface-based `exec.Cmd` mock, or a third-party process-mocking library) | The repo has explicitly chosen `cmdFactory` injection over interface abstraction (`PROJECT.md` Key Decisions: "cmdFactory injection for subprocess testing — Avoids interface-based exec abstraction, simpler test seams"). Introducing a second pattern for the `dayone`/`audible api` integrations would fragment testing conventions across the codebase. | Reuse `WithCmdFactory` + `TestHelperProcess` pattern from `internal/audible` |
| Embedding/vendoring a Node.js runtime for `dayone` the way Python is embedded for `audible-cli` | Not indicated as necessary by the milestone context, and is a much heavier lift (Node runtime management vs. Python venv management) for what may be a single CLI invocation per journal entry. This is an architecture decision for the requirements/roadmap phase, not a stack dependency — flagging it here only so it isn't silently assumed. | Require `dayone` pre-installed on PATH with a clear, actionable error message if missing (mirrors how `audible-cli`'s presence is checked before the venv auto-management kicks in) |

## Stack Patterns by Variant

**If the ~600×70 matching problem later needs to scale to multiple additional sources (e.g. a third catalog) or needs configurable/pluggable similarity strategies:**
- Reconsider `hbollon/go-edlib` v1.7.0 at that point — its actively maintained, multi-algorithm surface becomes proportional to a genuinely multi-source problem.
- Because it stays out of scope now, revisit only if a future milestone's requirements explicitly call for it.

**If Audiobookshelf listening-stats calls become a recurring daemon/polling workload rather than an on-demand `earworm stats` command:**
- Reconsider `hashicorp/go-retryablehttp` — daemon-mode long-running polling against a network service is exactly the situation retry libraries are built for, unlike a one-shot CLI invocation.
- Not warranted for the milestone as scoped (on-demand extraction + CSV/journal export).

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|------------------|-------|
| Go 1.26.1 (repo's current `go.mod`) | All stdlib packages named above (`net/http`, `encoding/json`, `encoding/csv`, `os/exec`, `crypto/sha256`) | No version constraints — all stable stdlib since long before Go 1.23. |
| `github.com/stretchr/testify` v1.11.1 (existing) | New matcher/CSV/stats tests | No change needed; already pinned in `go.mod`. |
| `hbollon/go-edlib` v1.7.0 (if adopted later) | Go 1.18+ (generics-era module) | Compatible with repo's Go 1.26.1 if this path is revisited. |
| `dayone` npm CLI | Node.js runtime (version not verified — confirm during requirements phase against the specific npm package's `engines` field) | External runtime dependency, not a Go module; no `go.mod` interaction. Needs its own "is this on PATH" preflight check, mirroring the existing `audible-cli` presence check. |

## Sources

- Repo inspection: `internal/audible/audible.go`, `internal/audible/library.go`, `internal/audible/audible_test.go`, `internal/audiobookshelf/client.go`, `internal/audiobookshelf/client_test.go`, `internal/db/db.go`, `internal/goodreads/export.go`, `internal/venv/venv.go`, `go.mod`, `.planning/PROJECT.md` — confirmed existing stack, conventions, and test seams directly from source.
- [pkg.go.dev: lithammer/fuzzysearch/fuzzy](https://pkg.go.dev/github.com/lithammer/fuzzysearch/fuzzy) — v1.1.8, published May 9, 2023, flagged as not the module's latest indexed version.
- [pkg.go.dev: adrg/strutil](https://pkg.go.dev/github.com/adrg/strutil) — v0.3.1, published Sep 27, 2023; metrics list (Hamming, Levenshtein, Jaro, Jaro-Winkler, Smith-Waterman-Gotoh, Sorensen-Dice, Jaccard, Overlap).
- [GitHub: hbollon/go-edlib releases](https://github.com/hbollon/go-edlib/releases) / [pkg.go.dev](https://pkg.go.dev/github.com/hbollon/go-edlib) — v1.7.0, published Aug 19, 2025.
- [pkg.go.dev: agext/levenshtein](https://pkg.go.dev/github.com/agext/levenshtein) / [GitHub v1.2.3 tag](https://github.com/agext/levenshtein/tree/v1.2.3) — v1.2.3, published Mar 12, 2020, marked stable/frozen API.
- [pkg.go.dev: hashicorp/go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp) / [GitHub](https://github.com/hashicorp/go-retryablehttp) — v0.7.8, published Jun 18, 2025.
- [Audiobookshelf API Reference](https://api.audiobookshelf.org/) — confirmed `GET /api/me/listening-sessions`, `GET /api/me/listening-stats`, `GET /api/users/<id>/listening-stats` endpoints; confirmed `page`/`limit` pagination model with `limit=0` meaning "no limit"; confirmed Bearer-token auth (query-param token also supported for GET).
- [Day One CLI documentation](https://dayoneapp.com/guides/day-one-for-mac/command-line-interface-cli/) and [npm: dayone](https://www.npmjs.com/package/dayone) — confirmed `new` subcommand reads entry text from stdin by default (`--no-stdin` to override), confirming the stdin-piping integration shape assumed in the milestone context.
- [mkb79/audible-cli GitHub](https://github.com/mkb79/audible-cli) — confirmed `audible api <endpoint>` exists for arbitrary Audible API calls returning plain JSON, same integration shape as the already-implemented `library export --format json`.

---
*Stack research for: listening-stats extraction, CSV export, and Day One journaling additions to earworm v1.2*
*Researched: 2026-09-20*
