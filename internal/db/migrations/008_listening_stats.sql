-- Listening statistics ingestion (milestone v1.2, phase 19).
--
-- Three tables with deliberately different semantics:
--   listening_days   -- per-source daily totals. Mutable: a source may revise
--                       a past day, so writes upsert with DO UPDATE. Rows are
--                       never summed blindly across sources; callers decide.
--   book_listening   -- per-source, per-book state snapshot. Mutable.
--   stats_sync_state -- opaque key/value watermarks for resumable sync.
--
-- Day keys are always 'YYYY-MM-DD' rendered in the configured stats timezone,
-- never in the process-local zone and never taken from a source's own
-- precomputed date field.

CREATE TABLE listening_days (
    day        TEXT    NOT NULL,           -- YYYY-MM-DD in the configured timezone
    source     TEXT    NOT NULL,           -- 'audible' | 'abs'
    seconds    INTEGER NOT NULL DEFAULT 0, -- total listening seconds for that day
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (day, source)
);

CREATE INDEX idx_listening_days_source ON listening_days(source);

CREATE TABLE book_listening (
    source            TEXT    NOT NULL,           -- 'audible' | 'abs'
    source_key        TEXT    NOT NULL,           -- ASIN (audible) or library item id (abs)
    asin              TEXT    NOT NULL DEFAULT '',
    title             TEXT    NOT NULL DEFAULT '',
    author            TEXT    NOT NULL DEFAULT '',
    narrator          TEXT    NOT NULL DEFAULT '',
    series            TEXT    NOT NULL DEFAULT '',
    series_position   TEXT    NOT NULL DEFAULT '',
    genres            TEXT    NOT NULL DEFAULT '', -- comma separated
    runtime_seconds   INTEGER NOT NULL DEFAULT 0,
    percent_complete  REAL    NOT NULL DEFAULT 0,
    is_finished       INTEGER NOT NULL DEFAULT 0,
    purchase_date     TEXT    NOT NULL DEFAULT '', -- RFC3339 or YYYY-MM-DD as supplied
    added_date        TEXT    NOT NULL DEFAULT '',
    -- status_changed_at is the source's last STATUS CHANGE timestamp. It is not
    -- proof of a finish: it is also set when a book is marked unfinished.
    status_changed_at TEXT    NOT NULL DEFAULT '',
    -- status_is_bulk marks rows whose status_changed_at belongs to a cluster of
    -- many books sharing a sub-second timestamp, i.e. a bulk marking or
    -- migration artifact rather than a genuine per-book event.
    status_is_bulk    INTEGER NOT NULL DEFAULT 0,
    last_position_at  TEXT    NOT NULL DEFAULT '', -- last playback position sync
    last_position_ms  INTEGER NOT NULL DEFAULT 0,
    seconds_listened  INTEGER NOT NULL DEFAULT 0, -- measured only; 0 when unknown
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source, source_key)
);

CREATE INDEX idx_book_listening_asin ON book_listening(asin);
CREATE INDEX idx_book_listening_source ON book_listening(source);

CREATE TABLE stats_sync_state (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
