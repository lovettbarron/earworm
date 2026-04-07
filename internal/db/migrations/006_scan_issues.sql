-- Scan issues: detected problems during library scanning
CREATE TABLE IF NOT EXISTS scan_issues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    issue_type TEXT NOT NULL,
    severity TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    suggested_action TEXT NOT NULL DEFAULT '',
    scan_run_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scan_issues_path ON scan_issues(path);
CREATE INDEX IF NOT EXISTS idx_scan_issues_type ON scan_issues(issue_type);
CREATE INDEX IF NOT EXISTS idx_scan_issues_run ON scan_issues(scan_run_id);
