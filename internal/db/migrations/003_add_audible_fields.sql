-- Add Audible remote metadata fields to books table
ALTER TABLE books ADD COLUMN audible_status TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN purchase_date TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN runtime_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE books ADD COLUMN narrators TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_name TEXT NOT NULL DEFAULT '';
ALTER TABLE books ADD COLUMN series_position TEXT NOT NULL DEFAULT '';
