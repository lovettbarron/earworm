-- Audiobookshelf playback sessions (milestone v1.2, phase 20).
--
-- Sessions are the only source of exact per-book, per-day listening time, and
-- unlike Audible's daily totals they name the book.
--
-- They are MUTABLE. A session stays open while playback continues, so
-- time_listening and current_time grow between syncs, and offline clients
-- upsert sessions by their own id long after started_at. Writes therefore use
-- ON CONFLICT DO UPDATE keyed on the server's session id; inserting blindly
-- would double-count every session observed while still open.

CREATE TABLE listening_sessions (
    id              TEXT PRIMARY KEY,           -- server-assigned session id
    user_id         TEXT NOT NULL DEFAULT '',
    library_item_id TEXT NOT NULL DEFAULT '',
    book_id         TEXT NOT NULL DEFAULT '',
    episode_id      TEXT NOT NULL DEFAULT '',
    media_type      TEXT NOT NULL DEFAULT '',
    asin            TEXT NOT NULL DEFAULT '',   -- from the session metadata snapshot, often empty
    title           TEXT NOT NULL DEFAULT '',
    author          TEXT NOT NULL DEFAULT '',
    -- day is derived from started_at in the configured stats timezone, never
    -- from the server's own precomputed date field.
    day             TEXT NOT NULL DEFAULT '',
    seconds         INTEGER NOT NULL DEFAULT 0, -- time actually listened
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    start_seconds   INTEGER NOT NULL DEFAULT 0,
    current_seconds INTEGER NOT NULL DEFAULT 0,
    started_at      TEXT NOT NULL DEFAULT '',   -- RFC3339
    updated_at      TEXT NOT NULL DEFAULT '',   -- RFC3339; the sync watermark field
    device          TEXT NOT NULL DEFAULT '',
    synced_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_listening_sessions_day ON listening_sessions(day);
CREATE INDEX idx_listening_sessions_item ON listening_sessions(library_item_id);
CREATE INDEX idx_listening_sessions_updated ON listening_sessions(updated_at);
