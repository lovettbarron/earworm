---
quick_id: 260925-r3m
status: complete
date: 2026-09-27
commits: [1ed6b27, "cluster detection", "series test", "30s window"]
---

# Summary

A Komga library re-import on 2026-09-22 exposed two defects and cost 309 wrong
Day One entries. Both are fixed, the database is repaired, and the entries are
deleted.

## What was wrong

1. **Flags were recomputed from the response.** `MarkBulkStatus` clears the flag
   on every row a run did not name. Komga lists only what currently holds a read
   status, so the re-import's 210 orphaned rows were read as "no longer
   unreliable" while keeping their 2024 dates. 205 entries followed, back to 2014.
2. **Nothing caught the re-mark itself.** 103 books were marked read at ~12:17
   on 2026-09-22, after the configured cutoff, producing 102 "Finished" entries.
3. **The flag had a way around it.** `BuildFinishEntries` refuses a recovered
   date that is not independent of the discredited timestamp, but only in the
   bulk branch. A flagged book above 95% and unfinished took the near-complete
   branch and was dated from `LastPositionAt`, which Komga copies from the very
   status timestamp the flag rejected. Two volumes reached the journal that way
   after the flags were restored, which is how the bug was found.

## What changed

- `db.MarkBulkStatusBefore` decides every row from its stored status timestamp.
  Absent from a response no longer means reliable.
- `db.AddBulkStatus` layers extra keys on without clearing.
- `listening.DetectBulkClusterGroups` hands back the clusters so a caller can
  judge one whole.
- Komga runs cluster detection with a 30-second window, flagging a cluster only
  when it spans `MinReMarkSeries` (3) distinct series, or holds more than
  `MaxReadingBatch` (15) books however narrow it is.
- The near-complete branch in `BuildFinishEntries` applies the same
  independence test as the bulk branch.

## Why the series test exists

The plain one-second rule flagged five Tokyo Ghoul volumes marked together on
2026-09-20 and would have deleted a real journal entry. Breadth separates the
two: on this library every 3+ series cluster was the re-import and every
single-series cluster was genuine.

## Result on the real library

| | Books |
|---|---|
| Flagged by cutoff | 202 |
| Flagged as re-marking (2026-09-22) | 97 |
| Genuine binges still unflagged | 2026-06-02, 06-07, 09-04, 09-19, 09-20 |

Day One: 301 entries deleted in total — 200 for 2024-03-10/11/12, 80 orphaned
finish entries, 19 from the narrow Solo Leveling batch the user confirmed was
the re-import, and 2 that the near-complete branch had written back.

A dry run over the whole history now reports 633 unchanged, 0 created,
0 updated: the journal matches the data exactly.

## Notes

- `TestKomgaSyncFlagsCompletionsBeforeCutoff` had asserted the wholesale
  clearing as intended, so the first bug was locked in by its own test. The new
  tests were verified to fail against the previous ingestor.
- The daemon runs the installed binary, so a fix in the working tree does not
  reach it. A repair sync was undone by the stale binary until `go install`.
- The 19-book Solo Leveling batch was confirmed by the user as the re-import,
  which is what `MaxReadingBatch` now catches. Genuine sessions on this library
  never exceeded nine books.
- Writes were paused (`journal.daemon_write: false`) while the fixes were made,
  then re-enabled and verified against a live cycle.
- `internal/cli` sits at 74.4%, below the project bar, because the merged
  `convert` command arrived without CLI tests. Unrelated to this fix.
