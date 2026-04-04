-- Add download tracking columns to books table
ALTER TABLE books ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN download_started_at DATETIME;
ALTER TABLE books ADD COLUMN download_completed_at DATETIME;
