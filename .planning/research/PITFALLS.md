# Pitfalls Research

**Domain:** Cross-source listening analytics + journaling sync for an existing Go CLI (earworm v1.2)
**Researched:** 2026-09-20
**Confidence:** MEDIUM-HIGH (Go-language and architecture guidance HIGH, confirmed against earworm's existing `internal/download` and `internal/audible` conventions; third-party API field shapes MEDIUM, confirmed via Audiobookshelf's own API reference but not exhaustively schema-verified; audible-side history API shape LOW — the official Audible API is unofficial/reverse-engineered and `audible-cli`'s command surface is pre-1.0, so exact field names must be verified against the installed CLI version at implementation time)

## Critical Pitfalls

### Pitfall 1: Two sources, two clocks — server-local day buckets vs UTC epoch collide

**What goes wrong:**
Audiobookshelf's listening-session/stats endpoints expose both a millisecond epoch (`startTime`) and a pre-bucketed `date`/`dayOfWeek` field that the *server* computed using **its own local timezone**, not the client's, and not UTC. The other source (Audible-side history) reports plain date strings (e.g. `"2026-09-19"`) with no timezone attached at all. If earworm re-derives "day" from the ABS epoch using the sync host's local timezone (which may differ from the ABS server's timezone — common when ABS runs on a NAS in one TZ and earworm's daemon runs elsewhere, or after a DST shift changes the UTC offset mid-comparison), sessions land in the wrong bucket relative to the date-string source, producing off-by-one-day drift that only shows up near midnight or at DST boundaries.

**Why it happens:**
Two different "day" semantics are being merged as if they were the same value: one is a timestamp interpreted through a timezone, the other is a pre-flattened string with an implicit (and undocumented) timezone baked in at the source. Nobody notices until a session that started at 11:45pm shows up on different calendar days across the two sources.

**How to avoid:**
- Pick **one canonical bucketing rule** for the whole system and document it in code and in the schema comment: "all listening activity is bucketed by calendar day in `[configured timezone]`, derived from the raw epoch timestamp where available." Store this timezone name (IANA, e.g. `"America/Los_Angeles"`) explicitly in config — never infer it from the host.
- Always derive the day bucket from the **raw epoch millisecond value**, never trust a source's pre-computed `date`/`dayOfWeek` field as authoritative. Recompute it yourself so both sources go through the same code path:
  ```go
  loc, err := time.LoadLocation(cfg.Analytics.BucketTimezone) // explicit, from config
  if err != nil {
      return fmt.Errorf("load bucket timezone %q: %w", cfg.Analytics.BucketTimezone, err)
  }
  day := time.UnixMilli(startTimeMs).In(loc).Format("2006-01-02")
  ```
- For the date-string source, do not assume it aligns with the epoch source's timezone. Parse it with `time.ParseInLocation` into the *same* canonical location rather than `time.Parse` (which silently assumes UTC):
  ```go
  d, err := time.ParseInLocation("2006-01-02", dateStr, loc)
  ```
  If the date-string source's own timezone is genuinely unknown, store that ambiguity explicitly (e.g. a `bucket_confidence` column) rather than pretending it's precise — a date string with no offset is fundamentally lower-fidelity than an epoch timestamp, and reconciliation logic in Phase 3 needs to know which of two conflicting day-buckets to trust.
- Never call `time.Now()`, `time.Local`, or `.Local()` anywhere in ingestion or bucketing code. Grep for `time.Local` in CI as a guardrail.

**Warning signs:**
- Session counts for "today" change depending on what time of day you run the sync.
- A book's last-session date differs by exactly one day between the two sources' raw exports.
- Bug reports cluster around DST transition weekends (first weekend of November/March in most US timezones).

**Phase to address:**
Phase 1 (Audible ingestion) establishes the canonical bucketing rule and timezone config field since it's the first ingestion pipeline; Phase 2 (Audiobookshelf ingestion) must conform to the identical rule and is where the epoch-vs-server-local mismatch is most concrete. Add a cross-source day-alignment test at the start of Phase 3 (identity resolution) since that's where mismatches actually cause visible bugs (double-bucketed or missing days in exports).

---

### Pitfall 2: DST transitions silently drop or double-count an hour of listening

**What goes wrong:**
Converting epoch milliseconds to local calendar dates across a DST boundary can put two sessions that are 61 minutes apart (in wall-clock terms) into the same bucket during "fall back," or split a single continuous listening session across two buckets that appear to have a 1-hour gap during "spring forward." If any downstream code computes "hours listened per day" as `sum(durations)` per bucket, DST weekends produce a day that looks like it has 23 or 25 hours of calendar time, which is usually fine for duration sums (durations are absolute, not clock-based) but breaks any logic that tries to derive duration by subtracting two local wall-clock times instead of using the epoch delta directly.

**Why it happens:**
Mixing "duration as reported by the API" (a plain number of seconds, DST-safe) with "duration derived by subtracting two localized timestamps" (DST-unsafe, because local clocks jump).

**How to avoid:**
- Never compute a session's duration as `endLocal.Sub(startLocal)` when both are `time.Time` values produced via `.In(loc)`. Always use the source-reported duration field, or if you must derive it, subtract the underlying epoch/UTC instants (`time.Time` subtraction in Go is DST-safe *if both values carry correct absolute instants* — the danger is treating the *bucket label* as if it were an instant).
- Keep the bucket **label** (`"2026-11-01"`) and the **instant** (`time.Time` in UTC or with full offset) as separate stored values; never reconstruct an instant by re-parsing a bucket label.
- Add a unit test that seeds sessions spanning a known DST transition (e.g., a session starting `2026-11-01T01:30:00-07:00` and ending `2026-11-01T01:15:00-08:00` PT, when the clock falls back) and asserts total duration matches the source-reported seconds field, not a wall-clock subtraction.

**Warning signs:**
- Daily aggregate reports show impossible totals (>24h) or negative durations on the first weekend of November/March.
- Session duration computed in code disagrees with the source API's own duration field for sessions on DST-transition days.

**Phase to address:**
Phase 2 (Audiobookshelf ingestion), since ABS is the source with epoch-millisecond precision where this manifests concretely. Phase 3 should include a DST-boundary fixture in the identity-resolution/aggregation test suite.

---

### Pitfall 3: Mixed units — seconds, milliseconds, and minutes coexist across and within sources

**What goes wrong:**
Audiobookshelf uses milliseconds for `startTime` but seconds for `currentTime`/`timeListening` in the same payload. Audible-side data may report duration in minutes (common for "time listened" summaries) rather than seconds. If a shared `Duration` type or column is populated inconsistently — one pipeline writing seconds, another writing minutes, into the same `duration_seconds` column — every downstream aggregate is wrong by a constant factor of 60, which is easy to miss because the numbers still look plausible (e.g., "3 minutes" stored as "3" in a seconds column looks like a very short but valid session).

**Why it happens:**
Each source's ingestion code is written and tested in isolation against that source's own docs; nobody writes a cross-source consistency test that would catch a 60x scaling error.

**How to avoid:**
- Convert to a single canonical unit (`time.Duration`, stored as whole seconds in SQLite) at the boundary of each source's parser, never later. Name intermediate variables with their unit explicitly (`startTimeMs`, `durationMinutes`) so a reviewer can see the conversion:
  ```go
  duration := time.Duration(rawSeconds) * time.Second // ABS: already seconds
  duration := time.Duration(rawMinutes) * time.Minute // Audible-side: minutes
  ```
- Write one round-trip test per source that feeds a known raw value (e.g., `timeListening: 125`) and asserts the resulting stored `duration_seconds` matches an expected constant, rather than only testing that "some positive number" was stored.
- Add a sanity-check constraint at write time: reject/flag any single session with `duration_seconds > 24*3600` (no one listens to one session for more than a day) — this catches unit-scaling bugs (seconds mistaken for minutes inflate by 60x, tripping the bound almost immediately) as well as bad data from the source itself.

**Warning signs:**
- Aggregate "hours listened this week" is off by roughly 60x or 1000x from what the user knows to be true.
- Two sources report wildly different totals for the same overlapping period, but the ratio between them is suspiciously close to 60 or 1000.

**Phase to address:**
Phase 1 and Phase 2 each own unit-conversion correctness for their own source at ingestion time. Phase 3 (identity resolution) should add the cross-source magnitude sanity check as a reconciliation-time assertion, since that's where numbers from both sources are compared directly.

---

### Pitfall 4: `encoding/json` silently reshapes a documented integer into a float, string, or `nil`

**What goes wrong:**
Fields documented as integers (session IDs, duration in seconds, progress counts) can arrive as JSON numbers with a decimal point (`125.0`), as strings (`"125"`), or occasionally as `null` when a field is legitimately absent (e.g., an in-progress session with no `endTime` yet). If the Go struct field is typed `int` and the JSON has `125.0`, `encoding/json` will actually unmarshal that fine into `int` — but if the API is decoded into `map[string]interface{}` or `interface{}` anywhere in the pipeline (common when probing an undocumented/unofficial API), every JSON number becomes `float64`, and large integer IDs risk losing precision past 2^53, while `strconv`-style manual conversions from that `interface{}` will panic on a type assertion (`v.(int)`) if the source sneaks in a string.

**Why it happens:**
Pre-1.0 and reverse-engineered APIs are inconsistent between endpoints, between library editions, and sometimes between two calls to the *same* endpoint (e.g., an ID field that's numeric for old records and a UUID string for new ones after a backend migration).

**How to avoid:**
- Never decode into bare `interface{}`/`map[string]interface{}` for values you're going to do arithmetic on. Define explicit structs per source, and for any field with known type ambiguity, use a custom type that accepts multiple JSON representations:
  ```go
  // FlexInt unmarshals a JSON number OR a JSON string containing a number.
  type FlexInt int64

  func (f *FlexInt) UnmarshalJSON(b []byte) error {
      var n int64
      if err := json.Unmarshal(b, &n); err == nil {
          *f = FlexInt(n)
          return nil
      }
      var s string
      if err := json.Unmarshal(b, &s); err != nil {
          return fmt.Errorf("FlexInt: not a number or string: %s", b)
      }
      n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
      if err != nil {
          return fmt.Errorf("FlexInt: parse %q: %w", s, err)
      }
      *f = FlexInt(n)
      return nil
  }
  ```
- If you must decode dynamically (e.g., logging raw payloads for debugging), use `json.Decoder.UseNumber()` so numbers come through as `json.Number` (a string under the hood) instead of lossy `float64`.
- Treat `null`/missing fields as a distinct state (`sql.NullInt64` or a pointer) rather than defaulting to zero — a `nil` `endTime` on an in-progress session is not the same as a session that ended at epoch 0.

**Warning signs:**
- Intermittent unmarshal errors that only reproduce on specific books/sessions, not consistently.
- IDs that "mostly work" but occasionally collide or truncate on older library records.

**Phase to address:**
Phase 1 and Phase 2, in each source's own parsing layer — this must be solved before any data reaches the shared schema, not patched later in Phase 3.

---

### Pitfall 5: Division by a zero (or missing) duration

**What goes wrong:**
Any derived metric that divides by session duration or elapsed time (pace, percent-complete-per-day, average session length) will panic-adjacent (Go doesn't panic on float division by zero — it produces `+Inf`/`NaN` — but `NaN` then silently poisons every downstream sum/average, and integer division by zero *does* panic) whenever a source returns a zero-length session (a session that was opened and immediately closed, common with app backgrounding) or a book with no recorded playback duration yet.

**Why it happens:**
Zero-duration records are edge cases that don't show up in a developer's own small test library but are common at scale (every accidental tap-and-close counts as a session in some listening-session APIs).

**How to avoid:**
- Guard every division explicitly and treat zero-duration as a distinct, filterable case rather than a metric of zero value:
  ```go
  if session.Duration <= 0 {
      return 0, ErrZeroDurationSession // caller decides: skip, log, or store as "instantaneous"
  }
  rate := float64(wordsOrPages) / session.Duration.Minutes()
  ```
- Filter zero-duration sessions out of rate/pace calculations at the query level (`WHERE duration_seconds > 0`) rather than relying on every call site to remember the guard.
- Never use `NaN`/`Inf` as a stored value — SQLite will happily store it as text or a weird float and it will corrupt every later `SUM`/`AVG` that touches that row. Validate before insert.

**Warning signs:**
- CSV exports contain `NaN`, `Inf`, or blank cells in a "words per minute" or "pace" column.
- A single book's stats are wildly (100x+) higher or lower than others for no clear reason — check for a near-zero denominator.

**Phase to address:**
Phase 3 (identity resolution + CSV export), where derived/aggregate metrics are actually computed. Ingestion phases (1, 2) should still validate incoming durations are non-negative, but the division risk lives in the aggregation layer.

---

### Pitfall 6: Re-sync double-counts because ingestion appends instead of upserts

**What goes wrong:**
Running the incremental sync twice (manually, or because the daemon's timer overlaps a slow run) inserts the same session/listening record a second time if the write path is a plain `INSERT` keyed on an autoincrement rowid rather than the source's own stable identifier. Aggregate listening time then silently doubles for any period that was synced more than once — which, with a daemon on a timer, is not a rare accident but an inevitability (every restart, every manual `earworm sync` in between timer ticks, every overlapping run).

**Why it happens:**
It's the path of least resistance to write ingestion as "fetch page, insert rows" without first designing the uniqueness key that makes a second insert of the same logical record a no-op.

**How to avoid:**
- Every ingested record must carry the **source's own stable ID** (session ID, or a deterministic composite like `(source, book_external_id, session_start_epoch_ms)` if the source has no native session ID) as a `UNIQUE` constraint in the schema, enforced by SQLite, not just convention in application code.
- Use `INSERT ... ON CONFLICT(...) DO UPDATE` (upsert) as the only write path for ingested records — never a bare `INSERT`:
  ```sql
  INSERT INTO listening_sessions
      (source, source_session_id, book_id, started_at_ms, duration_seconds, raw_payload_hash)
  VALUES (?, ?, ?, ?, ?, ?)
  ON CONFLICT(source, source_session_id) DO UPDATE SET
      duration_seconds = excluded.duration_seconds,
      raw_payload_hash = excluded.raw_payload_hash,
      updated_at       = CURRENT_TIMESTAMP
  WHERE listening_sessions.raw_payload_hash != excluded.raw_payload_hash;
  ```
- Add a migration-time test: sync the same fixture page twice in a row and assert row counts and aggregate sums are identical after the second run.

**Warning signs:**
- Total listening hours in exports keep creeping up even for months that should be "closed" and unchanging.
- Row counts in the sessions table are roughly double what a manual count from the source UI shows.

**Phase to address:**
Phase 1 and Phase 2 — this must be baked into each source's ingestion schema and write path from the start, not retrofitted. Verify explicitly in Phase 4 once the daemon introduces overlapping-run risk (see Pitfall 15).

---

### Pitfall 7: Treating a mutable in-progress record as a permanent snapshot

**What goes wrong:**
A currently-playing session on Audiobookshelf keeps its `currentTime`/`timeListening` updating until the user stops playback; if earworm ingests it once and marks it "done," the stored duration goes stale and under-reports the actual session the moment the user keeps listening after the sync ran. Conversely, if the reconciliation logic in Phase 3 treats every stored session as final and immutable once written, it will never pick up the corrected/final duration on the next sync — defeating the upsert logic in Pitfall 6 even if the schema is otherwise correct.

**Why it happens:**
Ingestion code often models the API response as read-once, write-once, when active-session data from a streaming media server is fundamentally live until some inactivity timeout closes it out server-side.

**How to avoid:**
- Track a session lifecycle state explicitly (`open`/`closed` or an `updated_at` vs `closed_at` distinction), not just insert-and-forget. Use the source's own signal for "is this still active" if it provides one (an explicit `isActive`/`endTime IS NULL` field); otherwise treat any session whose `updated_at` hasn't advanced across two consecutive syncs as closed.
- The upsert from Pitfall 6 already re-writes `duration_seconds` on conflict — the pitfall here is specifically about downstream code (exports, journaling) treating a still-open session as a finished fact. Gate journaling/export of "today's session" on the session being closed, or clearly label same-day entries as provisional/estimated (ties into Pitfall 12's provenance requirement).

**Warning signs:**
- A journal entry gets written for "finished listening for 12 minutes today" and then the actual session runs another 40 minutes that never gets journaled or corrected.
- Export CSVs for "today" undercount relative to what the user experienced, but past days are accurate.

**Phase to address:**
Phase 2 (Audiobookshelf ingestion), where the live-session behavior originates. Phase 4 (journaling) must respect the open/closed distinction before writing an entry for same-day activity.

---

### Pitfall 8: Historical backfill hammers a third-party API with no resumability

**What goes wrong:**
A first-time backfill against either source can mean months or years of paginated history. Writing this as a naive loop (`for each page: fetch, insert, next page`) with no persisted cursor means any crash, rate-limit rejection, or `Ctrl-C` partway through forces a full restart from page one — and doing that restart by simply re-running the same unthrottled loop is exactly the kind of "hammering" the project's own constraints explicitly warn about for the Audible side, and is equally rude to a self-hosted Audiobookshelf instance running on modest NAS hardware.

**Why it happens:**
Backfill is usually built and tested against a small personal library where it finishes in seconds, so the pagination-cursor/checkpoint logic that matters at scale (years of history, rate-limited APIs) never gets exercised until a real user runs it.

**How to avoid:**
- Persist a `sync_cursor` (per source, per sync type) that records the last successfully-committed page token / timestamp, updated **in the same transaction** as the data it corresponds to, so a crash between "insert rows" and "advance cursor" can never lose or skip a page:
  ```go
  tx, _ := db.BeginTx(ctx, nil)
  // insert this page's rows via upsert...
  _, err = tx.Exec(`UPDATE sync_cursors SET cursor = ?, updated_at = CURRENT_TIMESTAMP WHERE source = ?`, nextCursor, sourceName)
  tx.Commit()
  ```
- Reuse earworm's existing `internal/download` rate-limiting primitives (`RateLimiter.Wait`, `BackoffCalculator.Delay`) for backfill pagination rather than inventing new throttling logic — they already respect `context.Context` cancellation, which backfill needs for graceful interruption.
- Make backfill idempotent-by-construction (Pitfall 6's upsert) so a resumed backfill that slightly overlaps the last completed page is harmless rather than something that needs special-cased dedup logic.
- Default to a conservative fixed delay between backfill pages (configurable, not hardcoded), and treat a rate-limit response as a hard stop-and-backoff, not a "keep retrying immediately" condition — reuse the existing `RateLimitError`-style classification approach from `internal/audible/errors.go`.

**Warning signs:**
- Backfill logs show many consecutive 429/rate-limit responses in a row rather than a clean initial run.
- Interrupting a backfill (SIGINT, network blip) and restarting it produces a visibly different total than an uninterrupted run.

**Phase to address:**
Phase 1 (Audible ingestion) and Phase 2 (Audiobookshelf ingestion) each need their own cursor-checkpointed, rate-limited backfill; the underlying throttling primitives should be shared/reused rather than reimplemented per phase.

---

### Pitfall 9: Subprocess stdout is polluted with human-readable progress text mixed into "JSON" output

**What goes wrong:**
A pre-1.0 external CLI (whether it's the Audible-side tool or a journaling CLI) may write progress bars, warnings, or "upgrade available" banners to stdout even when a `--json`/`--format json` flag is passed, especially across versions where this behavior isn't contractually stable. Naively doing `json.Unmarshal(stdoutBytes, &result)` on the full captured stdout then fails intermittently — "intermittently" because it often only triggers on first-run-after-install messages, expired-cache warnings, or slow-network progress indicators that don't appear in a quiet, already-warmed-up dev environment.

**Why it happens:**
The subprocess wrapper is built and tested against a clean, fast, already-authenticated local environment where the CLI never emits anything extra on stdout.

**How to avoid:**
- Capture stdout and stderr into **separate buffers**, never combined, using `exec.CommandContext` with explicit `cmd.Stdout`/`cmd.Stderr` (this already matches earworm's existing subprocess pattern for `audible-cli`).
- Don't assume the entire stdout buffer is valid JSON. Scan for the JSON payload defensively: find the first line/byte that starts with `{` or `[` and parse from there, or (preferably) pass a flag that suppresses all non-JSON output if the CLI supports one, and treat any non-empty stderr as a signal worth logging even on success (exit 0 with stderr content often means "warning, but succeeded").
- Always use `exec.CommandContext(ctx, ...)` with a bounded timeout, not bare `exec.Command`, so a hung subprocess (e.g., a journaling CLI waiting on an interactive prompt with stdin closed) doesn't block the daemon indefinitely.
- Reuse and extend the `classifyError`-style pattern already established in `internal/audible/errors.go` for the new subprocess(es): inspect stderr text and exit code to produce typed errors (auth failure vs rate limit vs "not found" vs generic), rather than treating every non-zero exit the same way.

**Warning signs:**
- `json.Unmarshal` errors that mention unexpected characters at the start of input, happening only on some machines/some runs.
- A subprocess that "usually" completes in under a second occasionally hangs until the daemon's operator kills it manually.

**Phase to address:**
Phase 1 (Audible ingestion, if history data is pulled via a CLI subprocess) and Phase 4 (journaling CLI subprocess). Both should share a common "run subprocess, capture stdout/stderr separately, classify by exit code + stderr content, enforce a timeout" helper rather than duplicating ad hoc `exec.Command` calls.

---

### Pitfall 10: Pinning to an unstable, pre-1.0 CLI command surface

**What goes wrong:**
A journaling or history CLI that is pre-1.0 can change its flag names, output format, or exit-code semantics between minor versions without a deprecation period. Code written and tested against version `X.Y` silently breaks (wrong flag parsed as an entry, JSON field renamed, exit code meaning flipped) after the user upgrades that external tool independently of upgrading earworm.

**Why it happens:**
Earworm has no control over the external tool's release cadence, and there's no compile-time coupling to catch a breaking change — it only surfaces at runtime, often as a confusing downstream symptom (a malformed journal entry, a silently-empty history parse) rather than a clear error.

**How to avoid:**
- At startup (or first use per daemon cycle), run the subprocess's `--version` and log/compare it against the version earworm was last verified against; treat a mismatch as a warning, not a hard failure, but surface it clearly (e.g., in `earworm status`).
- Record subprocess version alongside any fixture used in golden-file tests, and re-verify those fixtures whenever the pinned/tested external version changes — don't assume old fixtures remain valid forever.
- Prefer the most stable, longest-lived flags/subcommands documented by the external tool, and avoid relying on any behavior explicitly marked experimental/undocumented in its own changelog.
- Fail loudly and specifically (a typed error naming the exact parse step that broke) rather than swallowing a parse error and silently skipping data — a broken external CLI integration should stop the sync/journal write, not quietly produce incomplete data that looks successful.

**Warning signs:**
- A previously-passing integration test starts failing right after `brew upgrade`/`pip install --upgrade` of the external tool, with no earworm code changes.
- Support requests correlate with a specific external-tool version the user has installed.

**Phase to address:**
Phase 1 and Phase 4, wherever an external CLI's output is parsed. Document the tested version range in the phase's own README/CHANGELOG note.

---

### Pitfall 11: An inferred estimate gets materialized and later mistaken for a measurement

**What goes wrong:**
When two sources disagree, or one source only has partial data (e.g., Audiobookshelf knows exact playback position but not exact listening duration for a session that was paused for a long time; or a book's start date has to be inferred from the earliest session rather than reported directly), it's tempting to compute a best-guess value and store it in the same column as directly-measured values. Once that estimate is in the `listening_sessions` table indistinguishable from a real measurement, every later consumer (CSV export, journaling, future aggregate reports) treats it as ground truth, and there is no way to later tighten or correct the estimate without silently rewriting history.

**Why it happens:**
It's simpler to have one column and one code path than to thread an "is this real or inferred" flag through every layer, especially under time pressure to ship the reconciliation logic in Phase 3.

**How to avoid:**
- Add explicit provenance columns to every derived/aggregate table: `source` (which raw source(s) contributed), `is_estimated BOOLEAN`, and ideally `estimation_method TEXT` (e.g., `"interpolated_from_progress_delta"`) so a later reader — human or code — can distinguish "Audible reported this directly" from "we computed this because Audible didn't report it."
- Propagate provenance all the way to the CSV export: include an `is_estimated` (or `data_quality`) column in the export rather than silently blending estimated and measured rows into one indistinguishable number. If the export format is fixed/external (e.g., matching an existing Goodreads-style CSV convention), still preserve provenance in earworm's own DB and consider a secondary "quality" export or footnote column where the target format allows it.
- Journaling entries (Phase 4) that reference derived numbers (e.g., "average pace this month") should be able to say "estimated" in the generated text when the underlying data includes estimated rows, rather than presenting a computed average with false precision.

**Warning signs:**
- A user notices a listening total that doesn't match what they remember, and there's no way to trace whether that number came from raw source data or from an internal estimate.
- CSV exports contain suspiciously round or suspiciously precise numbers with no way to tell which is which.

**Phase to address:**
Phase 3 (identity resolution + CSV export), where cross-source reconciliation and derived metrics are actually computed. Phase 4 (journaling) must read and respect the provenance flag rather than treating all stored numbers as equally authoritative.

---

### Pitfall 12: Secrets leak via config files, argv, logs, or test fixtures

**What goes wrong:**
This milestone adds at least two new credentialed integrations (a second API/source token, and an external journaling CLI that likely also needs its own credential/config). Common leak points: committing a real config file with a token to git (or to a test fixture "for realism"), passing a token as a CLI flag to a subprocess (visible to any other user on the machine via `ps aux` or `/proc/<pid>/cmdline` for the process's lifetime), logging full HTTP request/response bodies or headers at debug level (capturing `Authorization: Bearer ...`), and world-readable config files on shared/NAS-adjacent machines.

**Why it happens:**
Debugging a flaky integration often means "just log everything," and passing a token as `--token=X` to a subprocess is the most obvious way to wire it in, without considering that argv is visible system-wide.

**How to avoid:**
- Never pass secrets as subprocess CLI arguments. Use environment variables scoped to that one `exec.Cmd` (`cmd.Env = append(os.Environ(), "SERVICE_TOKEN="+token)`) or a short-lived credentials file with restrictive permissions, or stdin, depending on what the external CLI supports — check its docs for the least-visible option.
- Store all tokens exclusively via earworm's existing Viper-based config file convention (`~/.config/earworm/config.yaml`), and set/verify the file's permissions to `0600` at write time (`os.Chmod(path, 0600)` after Viper writes, or set `os.OpenFile` mode explicitly) — don't rely on the umask alone, especially since this config lives on or near a NAS mount that may have different default permission behavior.
- Add a `slog` redaction convention for any new logging around HTTP calls: log the request method/URL/status, never headers or bodies wholesale; if a raw payload must be logged for debugging, strip known-sensitive keys first.
- Keep real tokens out of git and test fixtures entirely — use synthetic, obviously-fake tokens (`"test-token-not-real"`) in all committed fixtures, and add `.gitignore` coverage for any local `.env`/credentials files this milestone introduces. Verify no real config file has ever been committed with `git log --all -- '*config.yaml'` before this work merges.

**Warning signs:**
- `ps aux | grep earworm` (or the journaling CLI's process name) shows a token in cleartext while a sync is running.
- Debug-level logs contain an `Authorization` header or a full response body with account-identifying data.
- A fixture file in `testdata/` has a token-shaped string that isn't obviously fake.

**Phase to address:**
Phase 1 (new source token), Phase 4 (journaling CLI credentials) — but the config-file-permission and logging-redaction conventions should be established once, early (Phase 1), and reused rather than re-solved per phase.

---

### Pitfall 13: Automated writes to a real personal journal duplicate entries or corrupt the store

**What goes wrong:**
Phase 4 writes to the user's actual, hand-maintained journal — a fundamentally different risk class from earworm's other outputs (a wrong CSV row is annoying; a corrupted or duplicated personal journal is a trust-destroying failure, especially if the journal is encrypted and a bad write can't be trivially diffed/fixed by hand). Concrete failure modes: re-running the daemon after a crash writes the same day's entry twice; a sync that reconciles/updates a session's duration after the journal entry was already written has no way to *update* an already-written entry (most journaling CLIs append, they don't provide an "edit entry N" API), so either the journal silently goes stale relative to the true data or a second, conflicting entry gets appended; and if the journaling CLI is ever invoked in a way that rewrites the whole file (e.g., a re-encrypt operation) while the daemon is mid-write, concurrent access could corrupt an encrypted journal file beyond recovery.

**Why it happens:**
Journaling entry-append operations look idempotent-adjacent ("just add today's summary") but the underlying store is a personal, largely unstructured document, not a system-owned table with a uniqueness constraint the daemon can rely on.

**How to avoid:**
- Do not rely on being able to read back the journal to detect "did I already write this" — the journal may be encrypted and unreadable without an interactive passphrase prompt. Instead, track "already journaled" state **in earworm's own SQLite DB**, keyed by a deterministic identifier (e.g., a hash of `(book_id, bucket_date, content_version)`), and only invoke the journaling CLI for records not already marked written. Mark as written only after the subprocess exits 0.
- Treat a day's entry as write-once by default: if the daemon detects that a session for an already-journaled day was updated (duration changed after the fact — see Pitfall 7), do not silently re-append a duplicate; either skip with a logged warning, or make correction-entries an explicit, clearly-labeled opt-in feature ("earworm: correction — actual total for 2026-09-15 was 42m, previously journaled as 30m") rather than a silent second entry indistinguishable from the first.
- Make the daemon's journaling step default to a dry-run/preview mode until the user explicitly enables live writes in config, and always log exactly what was (or would be) written, with enough detail to manually undo.
- Before any write, if feasible, take a lightweight backup/snapshot of the journal file (a timestamped copy) so a bad write is recoverable — but do this without ever reading/decrypting an encrypted journal's contents (a plain file copy is safe even for an encrypted store; parsing its contents is not something earworm should attempt).
- Enforce a single in-process writer for the journaling step (see Pitfall 15) so two overlapping daemon ticks can never both invoke the journaling CLI concurrently against the same file.

**Warning signs:**
- The user finds two entries for the same day with slightly different numbers.
- A journal file's modification time changes but its size doesn't grow as expected, or the file becomes unreadable by the journaling tool.
- Daemon logs show two journaling-CLI invocations for the same book/day pair within one sync interval.

**Phase to address:**
Phase 4 (journaling + daemon), specifically. This is the highest-consequence phase in the milestone and should get proportionally more test coverage and a conservative, opt-in-to-live-writes default.

---

### Pitfall 14: Time-dependent tests, golden CSV files, and map-iteration order make CI flaky

**What goes wrong:**
Three related Go-testing failure modes show up specifically in this kind of pipeline: (1) tests that call `time.Now()` internally (e.g., to compute "days since last sync") pass locally and fail once a month near a month boundary, or fail only when CI runs in a different timezone than the developer's machine; (2) golden-file CSV tests that compare exact byte output break whenever row ordering isn't fully deterministic — and in Go, iterating a `map` for the reconciliation/grouping logic in Phase 3 has explicitly randomized order per the language spec, so a test that "usually" passes will intermittently fail in CI; (3) CSV writer output can vary in subtle ways (field quoting rules around embedded commas/newlines, line-ending choice) that a byte-exact golden comparison is sensitive to but a human reviewer wouldn't notice.

**Why it happens:**
Non-determinism in tests is often invisible until CI runs enough times to hit the unlucky map ordering, timezone, or date-boundary case — by which point it looks like a "flaky test" to be muted rather than a real correctness gap in the underlying code (map iteration order affecting output order is also a real bug if the CSV is meant to be reproducible for diffing between runs).

**How to avoid:**
- Never call `time.Now()` directly inside business logic that needs to be tested; inject a clock:
  ```go
  type Clock interface { Now() time.Time }
  type realClock struct{}
  func (realClock) Now() time.Time { return time.Now() }
  ```
  Tests use a fixed/fake clock, so date-boundary and DST-boundary scenarios (Pitfalls 1–2) become deterministic, reproducible unit tests instead of once-a-year CI surprises.
  Also run at least one CI job (or a `TZ=` env wrapper in `go test`) with a non-UTC, non-developer's-local timezone to catch implicit `time.Local` usage (Pitfall 1) that unit tests in a single timezone would never expose.
- Never rely on Go `map` iteration order for anything that produces ordered output (CSV rows, journal entry ordering). Sort explicitly by a stable key (e.g., `(book_id, session_start)`) immediately before writing:
  ```go
  sort.Slice(rows, func(i, j int) bool {
      if rows[i].BookID != rows[j].BookID {
          return rows[i].BookID < rows[j].BookID
      }
      return rows[i].SessionStart.Before(rows[j].SessionStart)
  })
  ```
- Keep golden CSV fixtures under `testdata/golden/`, generated once from known, sorted input, and regenerate them via an explicit `-update` test flag rather than hand-editing — this matches the idiomatic Go golden-file pattern and keeps intent visible in diffs. Normalize on `\n` line endings explicitly (`csv.Writer.UseCRLF = false`, which is also the Go default) so fixtures are stable across platforms.
- Use `testify/require` (already an earworm convention) for the golden-file byte comparison so a mismatch fails fast with a clear diff rather than continuing into confusing follow-on assertions.

**Warning signs:**
- A CSV export test fails only in CI, never locally, or fails roughly 1-in-N runs with no code changes.
- A "days since" or "this week's total" test starts failing on specific calendar dates.

**Phase to address:**
Phase 3 (identity resolution + CSV export) for golden-file and map-ordering discipline; Phase 1/2 for clock injection wherever ingestion logic branches on "now" (e.g., deciding what counts as "incremental" vs needing backfill). Establish the `Clock` interface convention in Phase 1 so later phases reuse it rather than reinventing per-phase.

---

### Pitfall 15: Daemon timer overlap causes concurrent syncs against the same SQLite file

**What goes wrong:**
A daemon that fires the incremental sync on a fixed timer will, sooner or later, have a sync run longer than the interval (a slow API, a large catch-up backfill after downtime, a stalled subprocess). If the timer fires again before the previous run finishes, two sync goroutines/processes can end up writing to the same SQLite database concurrently. SQLite's WAL mode (already enabled per earworm's `internal/db` conventions) tolerates concurrent readers well and serializes writers, but overlapping *logical* sync runs can still interleave in ways that break the cursor-checkpoint invariant from Pitfall 8 (run B advances the cursor based on state that run A hasn't finished committing yet), or cause the journaling step (Pitfall 13) to double-invoke the external CLI for the same record.

**Why it happens:**
A simple `time.Ticker`-driven loop launches work on every tick without checking whether the previous tick's work is still running — fine in testing (fast, small dataset, ticks never overlap) but not fine once a backfill or slow network is in the mix.

**How to avoid:**
- Guard the daemon's sync trigger with an in-process mutex or a "run in progress" flag checked before each tick fires; skip (and log) a tick if the previous run hasn't completed rather than launching a second concurrent run:
  ```go
  var running atomic.Bool
  func (d *Daemon) tick(ctx context.Context) {
      if !running.CompareAndSwap(false, true) {
          slog.Warn("sync tick skipped: previous run still in progress")
          return
      }
      defer running.Store(false)
      d.runSync(ctx)
  }
  ```
- If the daemon can be run as multiple separate OS processes (not just goroutines within one long-lived process), an in-process flag isn't enough — use a持続 lock row in SQLite (`sync_lock` table with a `holder`/`expires_at` column, checked and claimed atomically via a conditional `UPDATE`) with a staleness timeout so a crashed process doesn't leave the lock stuck forever.
- Keep the cursor-advance-with-data-commit transaction (Pitfall 8) as the actual correctness guarantee — the mutex/lock above is about avoiding wasted work and confusing logs, not the last line of defense against data corruption.

**Warning signs:**
- Daemon logs show two sync runs' log lines interleaved.
- Sync duration metrics show a run that took longer than the configured interval, correlating with any data anomalies reported around that time.

**Phase to address:**
Phase 4 (journaling + daemon), where the timer-driven trigger is introduced. The cursor/upsert invariants it depends on (Pitfalls 6, 8) must already be solid from Phases 1–2.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|-----------------|------------------|
| Decode API responses into `map[string]interface{}` instead of typed structs | Faster to prototype against an undocumented API | Silent `float64` precision loss, panics on type assertions, no compile-time field-rename detection | Never in shipped ingestion code; fine for one-off exploratory scripts in `scratch/` that never merge |
| Store bucketed `date` strings directly from a source instead of deriving from epoch | Less code up front | Locks in whatever ambiguous timezone the source assumed; can't be corrected later without re-fetching raw data (which may no longer be available for old history) | Never — always keep the raw epoch/instant alongside any derived bucket |
| Skip the `sync_cursor` checkpoint for "small" libraries during initial development | Simpler backfill loop to write and demo | Breaks the moment a real user's history is large enough to take more than one sitting to backfill, and retrofitting resumability after data is half-loaded is harder than building it in | Acceptable only behind a feature flag during Phase 1/2 development, must be resolved before backfill ships to real users |
| Write journal entries synchronously inline with the sync loop | Simpler control flow | A slow/hung journaling CLI blocks the entire sync (including unrelated ingestion work) and risks Pitfall 15's timer-overlap scenario | Acceptable for v1.2 if bounded by `exec.CommandContext` timeout and isolated to its own step so ingestion still completes even if journaling fails |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|-----------------|-------------------|
| Audiobookshelf listening-sessions/stats API | Trusting the server-computed `date`/`dayOfWeek` field as if it used the same timezone as earworm's host | Recompute the day bucket yourself from the raw `startTime` epoch-ms field, in an explicitly configured canonical timezone |
| Audible-side history/stats (via subprocess or API) | Assuming a bare date string (no timezone) is directly comparable to another source's timezone-aware bucket | Parse with `time.ParseInLocation` into the same canonical timezone as the other source; store a confidence/ambiguity flag if the source's own timezone is genuinely unknown |
| External journaling CLI | Passing an auth token/passphrase as a `--flag` argument | Use environment variables scoped to the one `exec.Cmd`, or the CLI's documented non-argv credential mechanism |
| Any pre-1.0 external CLI | Hardcoding assumptions about flag names/output shape from whatever version was installed during development | Check `--version` at runtime, log a mismatch warning, and keep golden-file fixtures tagged with the CLI version they were captured against |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|-----------------|
| Full re-fetch of entire history on every incremental sync tick | Sync duration grows over time even though "incremental" implies it shouldn't | Use each source's `since`/cursor-based incremental query; only fall back to full re-fetch for the initial backfill | Noticeable once history exceeds a few hundred sessions per source; becomes a real problem (and risks Pitfall 8's rate-limit hammering) once a user has years of history |
| Loading the entire export dataset into memory before writing CSV | Memory spikes proportional to library/history size during export | Stream rows to `csv.Writer` as they're queried (`rows.Next()` loop) rather than materializing a full slice first | Breaks noticeably once a user's session history reaches tens of thousands of rows |
| Per-row identity-resolution lookups against the books table (N+1 queries) inside the reconciliation loop | Phase 3 reconciliation gets slower roughly linearly with session count, disproportionate to row count | Preload/cache the book-identity mapping once per sync run, keyed in memory, rather than querying per session row | Becomes visible once a sync processes thousands of session rows per run |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Logging full HTTP request/response including `Authorization` headers at debug level | Token leaks into log files that may be shared for support/debugging | Redact known-sensitive header/field names in a shared logging helper before any `slog` call touches HTTP data |
| Passing tokens/passphrases as subprocess CLI arguments | Visible to any other local user via `ps`/`/proc` for the process's lifetime | Use environment variables scoped to the single `exec.Cmd`, or stdin/credentials-file mechanisms the external CLI supports |
| Config file written without restrictive permissions | Other users on a shared machine or misconfigured NAS mount can read tokens | Explicitly `chmod 0600` config files containing secrets after write; verify in a test |
| Real tokens or journal content in committed test fixtures | Secrets or personal data end up in git history permanently | Use synthetic, clearly-fake values in all fixtures; add a pre-commit/CI grep for token-shaped strings in `testdata/` |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-------------------|
| Silently skipping sessions that can't be matched to a known book (identity resolution fails) | User's totals look inexplicably low with no indication why | Surface unmatched records explicitly (e.g., `earworm status` or a dedicated report) so the user can see and fix the mismatch |
| Daemon fails a sync tick silently, only visible in a log file the user never checks | Data quietly goes stale, discovered much later when a gap shows up in exports/journal | Track and expose last-successful-sync time and last-error per source in a status command, not just logs |
| Journaling writes go live by default the first time the feature is enabled | A misconfigured mapping or bucketing bug writes garbage into a irreplaceable personal document before the user has any chance to review | Default to dry-run/preview output for at least the first run after enabling journaling; require an explicit opt-in flag for live writes |

## "Looks Done But Isn't" Checklist

- [ ] **Backfill:** Often missing pagination-cursor exhaustion handling — verify it terminates correctly on the *last* page (some APIs return an empty page, others return a `hasMore: false` flag, others just stop changing the cursor; confirm which and test it explicitly)
- [ ] **Identity resolution:** Often works only on exact-match happy paths — verify behavior for a book present in one source but not the other, and for titles/authors that differ slightly in formatting between sources (not necessarily fuzzy-matching, but at least a defined, tested fallback)
- [ ] **CSV export:** Often matches the delimiter/header spec but forgets to escape embedded commas, quotes, or newlines in book titles/notes — verify with `encoding/csv` (which handles this automatically) rather than manual string concatenation, and add a golden-file test with a title containing a comma
- [ ] **Journaling subprocess integration:** Often works interactively but hangs in daemon (non-interactive) mode if the external CLI ever prompts for input (first-run setup, missing config, expired credential) — verify with `stdin` explicitly closed/redirected from `/dev/null` and a bounded `exec.CommandContext` timeout
- [ ] **Timezone bucketing:** Often "works" in the developer's own timezone during manual testing — verify with an explicit non-local `TZ` in at least one test run
- [ ] **Re-sync idempotency:** Often verified only for "run sync twice with no new data" — verify also for "run sync twice where the second run's fetch includes updated/still-open records from the first run" (Pitfall 7)

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|----------------|------------------|
| Double-counted sessions from a missing upsert constraint (Pitfall 6) | MEDIUM | Add the missing `UNIQUE(source, source_session_id)` constraint via migration; write a one-time dedup script that keeps the most-recently-updated row per key and deletes the rest inside a transaction; re-run aggregate exports afterward |
| Wrong timezone bucketing applied historically (Pitfall 1) | MEDIUM (if raw epoch was preserved) / HIGH (if not) | If raw epoch-ms values were stored alongside the derived bucket, write a migration that recomputes every bucket column using the corrected canonical timezone. If only the bucket label was ever stored (no raw instant), the original ambiguity cannot be recovered — this is the strongest argument for always retaining the raw instant per Pitfall 1's prevention |
| Duplicate journal entries already written to the user's real journal (Pitfall 13) | HIGH | Earworm cannot safely auto-delete from a personal journal it doesn't fully control. Cross-reference earworm's own "already journaled" tracking table against what should have been written, generate a clear report of the specific duplicate entries (with enough context — date, content — for the user to identify them), and let the user manually remove them; never attempt an automated rewrite of the journal file |
| Unit-scaling bug (seconds mistaken for minutes) already reflected in exported CSVs (Pitfall 3) | LOW–MEDIUM | Fix the ingestion conversion, add the round-trip regression test, then regenerate exports from the corrected source-of-truth table (SQLite) rather than trying to patch already-exported CSV files by hand |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|-------------------|----------------|
| Server-local vs UTC day bucketing mismatch | Phase 1 (establish rule) / Phase 2 (conform) | Cross-source fixture test asserting identical bucket for a known session near midnight |
| DST transition double-count/skip | Phase 2 | Fixture test spanning a known DST transition date, asserting duration matches source-reported seconds |
| Mixed units (seconds/ms/minutes) | Phase 1 & 2 | Per-source round-trip test with a known raw value asserting exact converted seconds |
| JSON type drift (int/float/string/null) | Phase 1 & 2 | Fixture payloads covering each observed type variant per ambiguous field |
| Division by zero duration | Phase 3 | Unit test with a zero-duration session asserting no `NaN`/`Inf`/panic |
| Append instead of upsert (double-counting) | Phase 1 & 2 | Test: sync same fixture page twice, assert identical row count and aggregate sum |
| Mutable in-progress session treated as final | Phase 2 (ingestion) / Phase 4 (journaling gate) | Test: session updated across two syncs results in updated stored duration, not a stale duplicate |
| Backfill without rate limiting/resumability | Phase 1 & 2 | Test: interrupt backfill mid-run, resume, assert identical final state to an uninterrupted run |
| Subprocess stdout polluted with non-JSON text | Phase 1 & 4 | Fixture with progress-bar/banner text mixed into captured stdout, assert parser still extracts JSON correctly |
| Pre-1.0 CLI surface drift | Phase 1 & 4 | `--version` check at runtime; fixtures tagged with tested CLI version |
| Estimate materialized as measurement | Phase 3 | Export includes `is_estimated`/provenance column; test asserts it's populated correctly for a known estimated row |
| Secrets in argv/config/logs/fixtures | Phase 1, 2, 4 (established once, reused) | Test asserting config file permissions are 0600; grep-based CI check for token-shaped strings in fixtures; log redaction unit test |
| Duplicate/destructive journal writes | Phase 4 | Test: re-run journaling step twice for identical input, assert only one subprocess invocation occurs |
| Flaky time/map-order/golden-file tests | Phase 1 (clock convention) / Phase 3 (golden files, sort order) | CI run with non-UTC `TZ`; golden-file regeneration flag; explicit sort before every ordered-output test |
| Daemon timer overlap | Phase 4 | Test: simulate a sync run exceeding the tick interval, assert the next tick is skipped and logged, not run concurrently |

## Sources

- [Audiobookshelf API Reference](https://api.audiobookshelf.org/) — confirms `GET /api/users/<ID>/listening-sessions` returns `startTime` as epoch milliseconds and `currentTime`/`timeListening` in seconds, plus server-computed `dayOfWeek`/`date` fields (MEDIUM confidence — full field schema not exhaustively documented in fetched content, verify against a live instance before implementation)
- [Audiobookshelf Listening Sessions docs](https://audiobookshelf.org/docs/documentation/server-management/listening-sessions/) — general behavior of session tracking
- [mkb79/audible-cli GitHub](https://github.com/mkb79/audible-cli) — confirms the CLI is actively maintained but pre-1.0; exact history/stats command output schema not found in public docs at research time (LOW confidence — verify directly against the installed CLI version's `--help` and actual output during Phase 1 implementation)
- [jrnl.sh Command Line Reference](https://jrnl.sh/en/stable/reference-command-line/) and [jrnl-org/jrnl GitHub](https://github.com/jrnl-org/jrnl) — confirms a representative CLI journaling tool accepts entry text directly as command-line arguments (`jrnl <entry>`) and supports AES-encrypted journal storage — both facts directly informed the argv-secret-exposure and encrypted-file-can't-be-diffed pitfalls above (MEDIUM confidence — if a different journaling CLI is actually used, verify its own argv/encryption behavior against these same concerns)
- earworm existing codebase conventions: `internal/download/ratelimiter.go`, `internal/download/backoff.go` (context-aware rate limiting and exponential backoff already established — reuse rather than reimplement), `internal/audible/errors.go` (stderr/exit-code error classification pattern to extend to new subprocess integrations), `internal/db/db.go` (WAL-mode SQLite, embedded-migration convention — informs where new schema/constraints should live)
- Go language spec on map iteration order (no guaranteed order; randomized per the spec) — general Go knowledge, HIGH confidence
- General Go `time` package semantics (`time.Parse` defaults to UTC when no offset is present in the layout; `time.ParseInLocation` allows an explicit fallback location; `time.Local` reads the host's local timezone unless overridden) — general Go knowledge, HIGH confidence

---
*Pitfalls research for: cross-source listening analytics + journaling sync (earworm v1.2)*
*Researched: 2026-09-20*
