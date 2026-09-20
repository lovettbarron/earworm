-- Journal entry ledger (milestone v1.2, phase 22).
--
-- Records what earworm has written to an external journal, so a re-run updates
-- the existing entry instead of adding a duplicate.
--
-- Idempotency is tracked here rather than by reading the journal back: the
-- journal is someone else's store, may be encrypted, and is not guaranteed to
-- be queryable. Entry ids are derived deterministically from the entry key, so
-- this table is a record of what happened rather than the source of the id.

CREATE TABLE journal_entries (
    entry_key   TEXT PRIMARY KEY,           -- e.g. 'day:2026-09-20' or 'finish:<identity>'
    kind        TEXT NOT NULL DEFAULT '',   -- 'day' | 'finish'
    entry_id    TEXT NOT NULL DEFAULT '',   -- deterministic id supplied to the journal
    journal_id  TEXT NOT NULL DEFAULT '',   -- destination journal
    entry_date  TEXT NOT NULL DEFAULT '',   -- the date the entry is filed under
    -- content_hash lets a sync skip entries whose rendered text has not
    -- changed, so an unchanged day is not rewritten on every run.
    content_hash TEXT NOT NULL DEFAULT '',
    written_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_journal_entries_date ON journal_entries(entry_date);
CREATE INDEX idx_journal_entries_kind ON journal_entries(kind);
