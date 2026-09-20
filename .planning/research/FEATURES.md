# Feature Research

**Domain:** Personal listening/reading analytics + LLM-oriented data export + automated journaling
**Researched:** 2026-09-20
**Confidence:** MEDIUM

Scope: earworm v1.2 — extract listening history from Audible and Audiobookshelf, merge it, export
CSVs meant as an LLM analysis dataset, and write journal entries to Day One, with a historical
backfill plus a daily incremental sync.

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist for any "my listening/reading history" tool. Missing these makes the
export feel incomplete or untrustworthy for LLM analysis.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Per-session listening facts (start time, duration, book) | Every scrobble-style tool (Last.fm, ListenBrainz, Trakt) is built on a timestamped event log, not just totals. Audiobookshelf exposes this via `GET /api/me/listening-sessions` and `GET /api/users/{id}/listening-sessions` (playback session objects with device + progress data). [Audiobookshelf API Reference](https://api.audiobookshelf.org/) | LOW | earworm already talks to the ABS REST API for scan-trigger; this is a new read-only endpoint on the same client. |
| Coarse per-book completion facts from Audible | Audible has no public listening-history export. `audible-cli` documents this directly: "listening history data cannot be downloaded... according to Audible's official policy." The only usable signal is per-item `percent_complete` / `listening_status` via the library response groups on the reverse-engineered API. [audible-cli GitHub](https://github.com/mkb79/audible-cli), [audible package docs](https://audible.readthedocs.io/en/latest/misc/external_api.html) | LOW-MEDIUM | This is coarse (a %, not a session log) and undocumented/unofficial — treat as a secondary, lower-confidence source, not equal to ABS session data. |
| Historical backfill (full history, one-time) | Every comparable tool (Last.fm scrobble history, Trakt "Get History", StoryGraph stats) lets a user see their *entire* recorded history, not just going forward. The user explicitly asked for "a historical overview... and a daily sync." | MEDIUM | Needs pagination handling for ABS sessions and idempotent upsert so re-running backfill doesn't duplicate rows. |
| Daily/periodic incremental sync | Trakt's Streaming Scrobbler re-syncs roughly every 24h; ListenBrainz and Last.fm are near-real-time but consumers of the *data* (dashboards, wrapped-style tools) typically batch daily. Matches earworm's existing daemon polling model. | MEDIUM | Should reuse earworm's existing sync-state / upsert pattern (see `internal/db`), not a bespoke mechanism. |
| Merge/dedupe across sources into one canonical per-book timeline | Both Trakt and ListenBrainz have to reconcile multiple watch/listen sources (streaming scrobbler + manual + Kodi plugin) into one history; the join key is stable (Trakt uses IMDb/TMDb IDs, ListenBrainz uses MBIDs). For earworm the natural join key is ASIN, which is already the primary key in the existing schema. | MEDIUM | Two sources disagree in kind, not just in value — ABS gives timestamps/durations, Audible gives a %. Don't silently average them; see Anti-Features. |
| Basic aggregate stats (total time, books completed/in-progress, pace) | Every one of the comparable tools leads with these: ListenBrainz stats page, StoryGraph "Books & Pages," Audiobookshelf's own stats page (introduced v1.6.12, shows top authors/genres + a Daily Listening Chart), Trakt "Get Watched." | LOW-MEDIUM | Straightforward SQL aggregation once the merged timeline exists. |
| Time-of-day / day-of-week distribution | Standard in music/video scrobble dashboards (ListenBrainz community has published hour-of-day breakdowns from its own dumps; Tautulli — the de facto Plex stats companion, since Plex's own Dashboard doesn't do this — offers "weekly & hourly patterns to identify peak viewing days and hours"). [Tautulli](https://tautulli.com/) | LOW | **Can only be computed from ABS session data.** Audible's %-complete signal has no timestamp granularity, so this dimension is source-limited — flag it as such in the dataset. |
| Author / narrator / genre breakdowns (top-N) | StoryGraph's stats page has dedicated Authors, Genres, Format sections; Audiobookshelf's stats page ships "top authors and genres" out of the box. Users expect their own metadata (already tracked in earworm) surfaced this way. | LOW | earworm already stores author/narrator/genre in its book metadata — this is a query, not new ingestion. |
| CSV export | earworm already ships a CSV export pattern (Goodreads). Users of StoryGraph/ListenBrainz/Trakt all expect *some* export path even if the native UI is a dashboard. | LOW | Reuse the existing `internal/cli` export command pattern (see `earworm goodreads`). |

### Differentiators (Competitive Advantage)

Features that set this apart from Audiobookshelf's own stats page, Audible's app, or a manual
Goodreads export — and align with the stated core value ("feed to an LLM," "link to journaling").

| Feature | Value Proposition | Complexity | Notes |
|---------|--------------------|------------|-------|
| Export shaped specifically for LLM consumption (not a human dashboard) | Every comparable tool (Audiobookshelf stats, StoryGraph, Spotify Wrapped) builds a *visual dashboard for a human*, not a machine-readable dataset meant to be handed to an LLM for open-ended querying. No named competitor targets this use case directly. See "LLM Dataset Design Guidance" below for concrete shape. | MEDIUM | This is the actual differentiator of the milestone — not the merge itself, but designing the merged output for LLM ingestion rather than for a chart. |
| Explicit source/confidence tagging per fact | No comparable tool needs this because they each own a single authoritative source. earworm must reconcile a rich source (ABS sessions) with a coarse, unofficial one (Audible %). Tagging provenance (`source: abs_session \| audible_percent \| inferred`, `confidence: high \| low`) lets the LLM (and the user prompting it) weight claims appropriately instead of the tool silently averaging or picking one. | LOW-MEDIUM | Directly answers the "how to encode uncertainty" question — see dataset section. |
| Re-listen detection | Audiobooks are commonly relistened (comfort reads, reference books); Trakt's "watched" endpoint sorts by play count for exactly this reason on the video side ("Get Watched returns all movies or shows... sorted by most plays"). No audiobook-specific tool surfaces this well today. | LOW-MEDIUM | Detect via multiple completion events (percent_complete resets to near-0 then climbs again, or a new session cluster long after a prior completion) for the same ASIN. |
| Series completion tracking | StoryGraph and Goodreads both track series membership but not authoritative completion percentage across a series; Hardcover's "Wrapped"-style yearly recap leans on this too. earworm already has metadata plumbing (title/series columns exist in CSV import) to build "N of M books in series X finished" cheaply. | MEDIUM | Depends on series metadata being present/clean in Audible's own metadata, which is inconsistent — expect partial coverage, not universal. |
| Streak calculation (consecutive days with any listening) | Present in Hardcover ("streak tracking gauge... resembles the GitLab contribution map") and implicitly in Goodreads community "reading streak" threads. Users expect it because Duolingo/Spotify-adjacent apps trained the expectation, even though audiobook listening is naturally less daily than music. | LOW-MEDIUM | Pure derived stat off the merged daily timeline — no new ingestion needed once sessions exist. |
| Automated Day One journal entries with narrative highlights | This is unique among the comparables — none of Last.fm/ListenBrainz/Trakt/StoryGraph/Audiobookshelf write to a journaling app. Value is turning passive data collection into a low-effort personal record ("finished X today," "50% through Y this week") without the user writing it. | MEDIUM-HIGH | See "Journaling Integration Conventions" below — entry cadence and "nothing happened" handling are the hard design decisions, not the Day One plumbing itself. |
| Multi-year, full-corpus export (not an annual "Wrapped" snapshot) | Spotify Wrapped and Hardcover's "Wrapped" clone are deliberately backward-looking annual snapshots optimized for shareability, not analysis. An LLM-oriented export benefits from the full multi-year raw history so trend questions ("did I read more fantasy after 2024?") are answerable without waiting for a yearly recap. | LOW | Just means "don't artificially window the export to the last 12 months" — a scoping decision, not new engineering. |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|------------------|-------------|
| Building charts/visual dashboards inside earworm (Wrapped-style graphics, Plex-Dashboard-style UI) | Every comparable tool's headline feature is a pretty chart (Spotify Wrapped, Audiobookshelf stats page, Tautulli graphs) — feels like "what these tools do." | earworm is a CLI with no persistent web UI or rendering surface; building and maintaining chart rendering is a large surface area disconnected from the stated goal (LLM analysis + journaling), and duplicates what Audiobookshelf's own stats page already partially provides. | Export clean CSVs; let the LLM or the user's own notebook/BI tool render charts. |
| Precomputing subjective "insights" (mood tags, sentiment scores, taste clusters, ML recommendations) | StoryGraph's mood/pace tags and Spotify Wrapped's ML-driven "personality" categories look appealing and are explicitly cited by users as the fun part. | These require training/maintaining classification logic in Go with no ground truth, can't be verified, and duplicate exactly the kind of open-ended interpretation an LLM does better and more transparently from raw facts. Baking a "mood" column into the export also constrains what the LLM concludes instead of letting it reason from evidence. | Export only unambiguous arithmetic aggregates (sums, counts, rates, %) computed directly from raw facts; leave interpretation to the LLM prompt. |
| Real-time / per-event webhook-driven journal entries | Feels like "complete" integration — write to Day One the instant a session ends. | Already explicitly out of scope for earworm generally ("Real-time webhook notifications — users can wrap CLI with their own scripts," per PROJECT.md), and journaling research confirms per-event auto-entries read as noisy/robotic rather than valuable — "the template fills itself with metadata, not narrative" is the exact failure mode users abandon. | Batch to a daily (or weekly) digest entry written once per sync cycle, only summarizing what happened. |
| Treating Audible's `percent_complete` as equal-confidence to Audiobookshelf session data | Simplifies the merge — "just take the max % or most recent value." | Audible's signal has no timestamps and is on an undocumented, unofficial endpoint that can change without notice; presenting it with the same confidence as ABS's real session log produces confidently wrong time-of-day/streak/pace numbers when ABS data is missing (e.g., books played outside ABS, like the Audible mobile app itself). | Explicit source/confidence column per record (see Differentiators); never merge two different-precision facts into one number without flagging it. |
| Gamified streak pressure (push notifications, "don't break your streak!") | Streaks and habit apps (Habitica, Streaks, Duolingo) show this drives engagement. | earworm's whole ethos is "zero manual intervention," not user engagement/retention — a personal analytics CLI has no reason to nudge behavior, and it conflicts with the already-out-of-scope real-time-notifications decision. | Report streak length as a passive stat in the export/journal; no reminders, no gamification UI. |
| Auto-generating a journal entry for every single book completion or every session | Feels maximally complete/granular — "never miss a moment." | Directly contradicts journaling-UX findings: entry-per-trivial-event is the exact pattern quantified-self/auto-journal users cite as "shallow," cluttered, and eventually ignored/disabled. | Default to one digest entry per day-with-activity; let a book-finished *milestone* earn a mention inside that digest rather than its own entry. |
| Writing a "nothing happened" entry every day to preserve an unbroken journal | Some quantified-self users want a continuous daily log for visual streak continuity (mirrors Hardcover's GitLab-style contribution map). | For most journaling users this is pure noise — an empty templated entry every day is the "metadata, not narrative" complaint researchers found causes people to abandon auto-daily-note templates. | Skip silent zero-activity days by default; offer a config flag for users who explicitly want a minimal one-line "no listening today" entry instead of a padded fake-narrative one. |
| Two-way sync / reading journal content back out of Day One | Feels like a "complete" integration once you're already writing to it. | Day One's automation surface is one-way by design (URL scheme, JSON import/export as a manual bulk operation, third-party CLI) — there is no incremental read API to safely reconcile against; building this is scope creep with no path to the stated goal. | One-way write only (via Day One's CLI/JSON-import mechanism), matching the "wrap an external tool as a clean subprocess boundary" pattern earworm already uses for audible-cli. |

## LLM Dataset Design Guidance

This directly answers "what makes an exported dataset good as LLM input" — concrete, not general.

**Shape: default to a small set of purpose-built files, not one giant flat file, but denormalize
the join key's human-readable attributes.**

- LLMs are unreliable at mentally joining two separate CSVs pasted into a chat (no code execution)
  — a bare `asin` column with no title/author is close to meaningless as a token to the model. But
  full denormalization (repeating narrator bios, cover URLs, full genre lists on every session row)
  wastes tokens badly, and CSV/JSONL serialization is already one of the more token-expensive
  formats to begin with — one head-to-head test of 11 tabular formats found CSV among the *worst*
  performing for LLM accuracy (44.3%), behind Markdown-KV (60.7%), XML (56%), and INI (55.7%).
  [Best input format for LLMs (11 formats compared)](https://www.improvingagents.com/blog/best-input-data-format-for-llms/)
- **Recommendation for earworm:** ship 2-3 files per export, not one:
  1. `sessions.csv` (or `daily_activity.csv`) — the fact table, one row per listening event or
     per book-per-day aggregate. Inline only the handful of dimensions users will actually ask
     about in-line: `asin`, `title`, `author`, `narrator`, `series`, `genre`. Leave out rarely-used
     metadata (cover URL, full description, ISBN).
  2. `books.csv` — a dimension table, one row per book, with everything else (full metadata,
     completion status, first/last listened dates, total time, source(s) seen in).
  3. Optionally, `daily_summary.csv` — one row per calendar day (date, minutes_listened,
     books_touched, primary_book) for quick day-of-week/streak questions without scanning raw
     sessions.
- If the tool also wants a single-file "paste into chat" option, offer it as an explicit
  alternate `--flat` export mode that denormalizes `sessions.csv` + the inline `books.csv` columns
  into one file, understanding this trades token efficiency for one-file convenience — make this a
  deliberate, named choice, not the only option.
- **Prefer long/tidy format over wide.** One row per observation (a session, or a book-day), not
  one row per book with 40 monthly columns. Tidy data ("each column a variable, each row an
  observation") is what most analysis tooling and LLM code-execution environments expect; wide
  format is for human dashboards/snapshots, which is explicitly not the goal here.
  [Wide vs long data](https://anvil.works/blog/tidy-data)
- **Column naming:** `snake_case`, units in the name (`duration_minutes`, not `duration`),
  unambiguous booleans (`is_completed`, not `status` with mixed string/bool semantics).
- **Dates/timestamps:** ISO-8601 dates (`2026-03-14`) and RFC3339 timestamps with explicit UTC
  offset (`2026-03-14T21:05:00-04:00`) — never locale-ambiguous formats like `3/14/26`. LLMs parse
  ISO-8601 far more reliably than ambiguous locale formats, and offset-less timestamps make
  time-of-day/day-of-week analysis silently wrong across DST boundaries or multi-timezone users.
- **Encoding uncertainty/confidence (the specific ask):** add explicit `source` and `confidence`
  columns to every row derived from a single-source or inferred fact — e.g.
  `source=audiobookshelf_session|audible_percent_complete|inferred`,
  `confidence=high|low`. Never silently average or overwrite a high-confidence session-derived
  value with a low-confidence percent-complete value; keep both and let the LLM/user decide. This
  is the direct fix for the Audible-vs-ABS asymmetry described in Table Stakes above.
- **Missing vs. zero:** leave a cell empty (or explicit `NA`) for "we don't know," never `0` —
  `0` minutes listened and "no data for this source" are materially different facts and must stay
  distinguishable, especially with Audible's coarse-only data.
- **Derived aggregates:** include simple, verifiable arithmetic aggregates (totals, counts, rates,
  streak lengths) as convenience files, but do not include subjective/derived judgments (mood,
  "taste profile," recommendation scores) in the exported dataset — see Anti-Features. The LLM
  should compute interpretation from facts; earworm should compute only arithmetic.
- **Practical size guidance:** for a "paste into chat" flat file, keep it well under typical
  context-window-friendly sizes (roughly low tens of thousands of rows / a few MB) since CSV/JSONL
  serialization is token-expensive per cell; for larger multi-year histories, the multi-file /
  code-execution-oriented export (sessions + books + daily_summary) scales better than one giant
  flat file. [TOON vs JSON vs CSV for LLM prompts](https://medium.com/data-science-in-your-pocket/toon-vs-json-vs-csv-9cbfbb9a93f8)

## Journaling Integration Conventions

Answers "what makes automated journal entries valuable vs. noisy" and how to handle Day One
specifically.

- **Day One's automation surface is one-way and mechanism-specific.** There is no live read/write
  REST API for third-party incremental sync; the supported automation paths are the bundled
  `dayone2` CLI tool (macOS, scriptable — the natural fit for a Go CLI shelling out, matching
  earworm's existing subprocess-wrapper pattern for `audible-cli`), JSON bulk import/export (manual,
  not incremental), and URL-scheme/Shortcuts (mobile-first, not CLI-friendly).
  [Day One Tools](http://help.dayoneapp.com/en/articles/440580-day-one-tools),
  [Day One JSON import/export](https://dayoneapp.com/releases/import-and-export-web/)
- **Entry frequency: daily digest, not per-event.** Auto-journaling research is consistent that
  granular, metadata-heavy auto-entries read as robotic/noisy and get abandoned — "many
  Obsidian daily-notes users eventually stop because the template fills itself with metadata, not
  narrative." The value comes from batching into one digest per day (or week) with a short
  narrative synthesis ("Finished *Book A* today; 45 min into *Book B*"), not a raw stats dump.
- **"Nothing happened" handling: skip by default, minimal opt-in otherwise.** Default behavior
  should be to write no entry on a day with zero listening activity, avoiding the padded/fake
  narrative failure mode. For users who explicitly want continuity (visible unbroken log, mirroring
  Hardcover's contribution-map streak visual), offer a config flag for a one-line
  "no listening today" entry — never auto-generate filler narrative to avoid an empty day.
- **Idempotency matters for a daemon-polled tool.** Because earworm already runs on a polling
  interval, entry-writing needs a persisted "last journaled date" marker so repeated daemon cycles
  in the same day don't create duplicate entries — same pattern as earworm's existing sync-state
  tracking.
- **Keep journal content and CSV export non-overlapping in purpose.** The journal is for a short,
  human-readable highlight (what happened, milestones crossed); the CSV export is the exhaustive
  machine-readable substrate. Don't try to reproduce full stats tables inside a Day One entry —
  that duplicates the export's job and creates two sources of truth for the same numbers.
- **Tagging is a cheap, valuable extra.** Day One supports entry tags; tagging automated entries
  (e.g., `audiobook`, `earworm`) makes them filterable/searchable/excludable within the app without
  any new earworm-side complexity.

## Feature Dependencies

```
Historical backfill (ABS sessions + Audible percent_complete)
    └──requires──> ABS listening-sessions client + Audible library response-group client

Daily incremental sync
    └──requires──> Historical backfill (baseline/cursor state must exist first)

Merge/dedupe with source+confidence tagging
    └──requires──> Historical backfill (both sources ingested)

Basic aggregate stats (totals, pace, top authors/narrators/genres)
    └──requires──> Merge/dedupe

Time-of-day / day-of-week distribution
    └──requires──> ABS session data specifically (Audible source has no timestamps)

Streak calculation
    └──requires──> Basic aggregate stats (per-day activity flag)

Re-listen detection
    └──requires──> Merge/dedupe (multiple completion events per ASIN over time)

Series completion tracking
    └──requires──> Merge/dedupe + existing series metadata

LLM-ready CSV/multi-file export
    └──requires──> Merge/dedupe + Basic aggregate stats

Day One journal entries (daily digest)
    └──requires──> Basic aggregate stats (source of narrative content)
    └──enhances──> nothing downstream; terminal feature

Gamified streak notifications ──conflicts──> earworm's "zero manual intervention" design ethos
Per-event journal entries ──conflicts──> daily-digest journaling convention (noise vs. signal)
```

### Dependency Notes

- **Daily incremental sync requires historical backfill:** without a baseline, "daily sync" has
  nothing to diff against and would either re-import everything or miss the historical record the
  user explicitly asked for ("a historical overview of and a daily sync with updates").
- **Time-of-day/day-of-week requires ABS specifically:** this is a hard source constraint, not an
  implementation choice — Audible's only available signal is a coarse percent-complete with no
  session timestamps, so these dimensions are structurally unavailable from Audible alone. Any
  roadmap phase covering these dimensions must sequence after ABS session ingestion, not after
  Audible ingestion.
- **Day One entries require basic aggregate stats, not raw merge output:** journal narrative
  synthesis needs "what changed today" (a book finished, a milestone crossed), which is a derived
  fact, not a raw session row — so this phase must come after the aggregation logic exists.
- **Gamification/per-event entries conflict with stated design ethos:** both are listed as explicit
  anti-features above; a roadmap should not schedule them at all rather than deferring them.

## MVP Definition

### Launch With (v1.2)

Minimum viable product — validates "give me a dataset I can feed to an LLM."

- [ ] Historical backfill of ABS listening sessions — the only source with the granularity (time,
      duration, day-of-week) the user's stated goal needs
- [ ] Historical backfill of Audible per-book percent_complete/listening_status — completes
      coverage for books not tracked in ABS at all
- [ ] Daily incremental sync for both sources, idempotent against the backfill
- [ ] Merge into one canonical per-book/per-day timeline with explicit `source`/`confidence` tagging
- [ ] Basic aggregate stats: totals, pace, time-of-day/day-of-week, top authors/narrators/genres
- [ ] CSV export in the multi-file, tidy, ISO-8601-dated shape described above

### Add After Validation (v1.x)

- [ ] Streak calculation — trigger once basic daily-activity aggregation is proven correct
- [ ] Series completion tracking — trigger once series metadata coverage is assessed as "good
      enough" across the real library
- [ ] Re-listen detection — trigger once merge/dedupe logic has enough real completions to validate
      the reset-and-reclimb heuristic
- [ ] Day One journal entries (daily digest, skip-by-default on empty days) — trigger once the
      aggregate stats used as narrative source are trusted

### Future Consideration (v2+)

- [ ] Abandonment/DNF inference — heuristic-heavy (no-activity-for-N-days + <threshold% complete),
      high false-positive risk; defer until there's enough historical data to tune thresholds
- [ ] Multi-year trend visualization — better served once the raw multi-file export has been used
      long enough to know what trend questions users actually ask an LLM
- [ ] Any mood/sentiment/taste-cluster tagging — explicitly anti-feature territory; only reconsider
      if a concrete, verifiable data source for it appears

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|----------------------|----------|
| ABS session backfill + daily sync | HIGH | MEDIUM | P1 |
| Audible percent_complete backfill + daily sync | MEDIUM | LOW-MEDIUM | P1 |
| Merge/dedupe with source+confidence tagging | HIGH | MEDIUM | P1 |
| Basic aggregate stats | HIGH | LOW-MEDIUM | P1 |
| LLM-shaped CSV export (multi-file, tidy) | HIGH | MEDIUM | P1 |
| Streak calculation | MEDIUM | LOW-MEDIUM | P2 |
| Series completion tracking | MEDIUM | MEDIUM | P2 |
| Re-listen detection | MEDIUM | LOW-MEDIUM | P2 |
| Day One daily-digest journal entries | HIGH | MEDIUM-HIGH | P2 |
| Abandonment/DNF inference | LOW-MEDIUM | MEDIUM-HIGH | P3 |
| Mood/taste/sentiment tagging | LOW | HIGH | Excluded (anti-feature) |
| Real-time per-event journal entries | LOW | MEDIUM | Excluded (anti-feature) |
| Gamified streak notifications | LOW | LOW | Excluded (anti-feature) |

**Priority key:**
- P1: Must have for v1.2 launch
- P2: Should have, add once P1 is validated against real library data
- P3: Nice to have, revisit after real usage data exists

## Competitor Feature Analysis

| Feature | Audiobookshelf (native) | Trakt / ListenBrainz | Our Approach |
|---------|--------------------------|------------------------|--------------|
| Per-session event log | Yes, via `/api/me/listening-sessions`; earworm's actual raw-data source | Yes (their whole model) | Ingest directly from ABS's existing API; no new tracking needed |
| Dashboard/visual stats | Yes — stats page with Daily Listening Chart, top authors/genres | Yes — Trakt profile stats, ListenBrainz stats page | Explicitly not building a competing dashboard; export data instead |
| Cross-source merge | No — single source of truth (its own DB) | Partial — Trakt merges scrobbler + manual + plugin sources via one ID scheme | Necessary because Audible has no exportable session log; must reconcile two asymmetric sources |
| LLM/analysis-oriented export | No | No | Primary differentiator — purpose-built tidy multi-file CSV export |
| Journaling integration | No | No | Primary differentiator — one-way Day One digest entries |
| Streak/gamification | No | No (ListenBrainz), N/A (Trakt) | Report as passive stat only, no notifications (see anti-features) |

## Sources

- [Audiobookshelf API Reference](https://api.audiobookshelf.org/) — listening-sessions and
  listening-stats endpoints
- [Audiobookshelf Statistics discussion #167](https://github.com/advplyr/audiobookshelf/discussions/167) —
  stats page history, Daily Listening Chart
- [audible-cli GitHub](https://github.com/mkb79/audible-cli) — confirms no official listening
  history export; library export/response-group capabilities
- [audible (Python) docs — External API](https://audible.readthedocs.io/en/latest/misc/external_api.html) —
  reverse-engineered endpoint notes, `percent_complete`/`listening_status` response groups
- [Audible Help — View listening log](https://help.audible.com/s/article/view-listening-log?language=en_US) —
  confirms Audible's own in-app listening log is mobile-only, not exportable
- [ListenBrainz stats discussions](https://community.metabrainz.org/t/statistics-in-listenbrainz/212010) —
  time-of-day/hour breakdown community analysis
- [Last.fm API — track.scrobble](https://www.last.fm/api/show/track.scrobble) — scrobble data model
- [Trakt API docs](https://docs.trakt.tv/docs/getting-started) — scrobble/history/watched model,
  most-plays sorting
- [Trakt Streaming Scrobbler launch](https://alternativeto.net/news/2024/12/trakt-tv-launches-streaming-scrobbler-to-sync-viewing-history-across-streaming-platforms/) —
  ~24h sync cadence precedent
- [The StoryGraph reading stats](https://app.thestorygraph.com/stats/readingstatus) — mood/pace/DNF
  stats categories
- [StoryGraph DNF roadmap discussion](https://roadmap.thestorygraph.com/requests-ideas/posts/mark-dnf-on-reading-challenge) —
  DNF as explicit user action, not inferred
- [Hardcover.app](https://docs.hardcover.app/) — streak visualization, "Wrapped" yearly recap
- [Tautulli](https://tautulli.com/) — day-of-week/hour-of-day pattern stats as the actual source of
  this dimension in the Plex ecosystem (native Plex Dashboard doesn't provide it)
- [Spotify Wrapped methodology](https://newsroom.spotify.com/2025-12-05/wrapped-methodology-explained/) —
  annual-snapshot framing, 30-second "listen" threshold
- [Best input data format for LLMs (11 formats)](https://www.improvingagents.com/blog/best-input-data-format-for-llms/) —
  CSV underperforms Markdown-KV/XML/INI for LLM accuracy
- [Wide vs. long data / tidy data](https://anvil.works/blog/tidy-data) — long format for
  machine/analysis consumption vs. wide for human dashboards
- [TOON vs JSON vs CSV for LLM prompts](https://medium.com/data-science-in-your-pocket/toon-vs-json-vs-csv-9cbfbb9a93f8) —
  token-cost tradeoffs of serialization formats
- [Day One Tools](http://help.dayoneapp.com/en/articles/440580-day-one-tools) — CLI/URL-scheme/
  Zapier automation surface, one-way by design
- [Day One JSON import/export release notes](https://dayoneapp.com/releases/import-and-export-web/)
- [deariary blog — "AI journaling is a spectrum"](https://blog.deariary.com/posts/2026-04-13-ai-journaling-is-a-spectrum) —
  granular/metadata-only auto-entries cited as the abandonment failure mode
- earworm's own `.planning/PROJECT.md` and `README.md` — existing architecture/patterns
  (subprocess wrapping, CSV export precedent, daemon polling model, "zero manual intervention" and
  "no real-time webhooks" scope decisions)

---
*Feature research for: personal listening/reading analytics + LLM dataset export + journaling
integration (earworm v1.2)*
*Researched: 2026-09-20*
