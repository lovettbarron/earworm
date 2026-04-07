-- Library items: path-keyed table for all library content (ASIN and non-ASIN)
CREATE TABLE IF NOT EXISTS library_items (
    path TEXT PRIMARY KEY,
    item_type TEXT NOT NULL DEFAULT 'unknown',
    title TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    asin TEXT NOT NULL DEFAULT '',
    folder_name TEXT NOT NULL DEFAULT '',
    file_count INTEGER NOT NULL DEFAULT 0,
    total_size_bytes INTEGER NOT NULL DEFAULT 0,
    has_cover INTEGER NOT NULL DEFAULT 0,
    metadata_source TEXT NOT NULL DEFAULT '',
    last_scanned_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_library_items_asin ON library_items(asin);
CREATE INDEX IF NOT EXISTS idx_library_items_type ON library_items(item_type);

-- Plans: named containers for a set of operations
CREATE TABLE IF NOT EXISTS plans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_plans_status ON plans(status);

-- Plan operations: individual actions within a plan
CREATE TABLE IF NOT EXISTS plan_operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id INTEGER NOT NULL REFERENCES plans(id),
    seq INTEGER NOT NULL,
    op_type TEXT NOT NULL,
    source_path TEXT NOT NULL,
    dest_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT NOT NULL DEFAULT '',
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_plan_operations_plan_id ON plan_operations(plan_id);
CREATE INDEX IF NOT EXISTS idx_plan_operations_status ON plan_operations(status);

-- Audit log: immutable record of all plan mutations
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    action TEXT NOT NULL,
    before_state TEXT NOT NULL DEFAULT '',
    after_state TEXT NOT NULL DEFAULT '',
    success INTEGER NOT NULL DEFAULT 1,
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_log_entity ON audit_log(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
