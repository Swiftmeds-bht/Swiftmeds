CREATE TABLE IF NOT EXISTS stock_sync_jobs (
    job_id             TEXT PRIMARY KEY,
    pharmacy_id        TEXT NOT NULL,
    uploaded_by_user_id TEXT NOT NULL,
    filename           TEXT NOT NULL DEFAULT '',
    status             SMALLINT NOT NULL CHECK (status BETWEEN 1 AND 4),
    rows_total         INTEGER NOT NULL DEFAULT 0 CHECK (rows_total >= 0),
    rows_applied       INTEGER NOT NULL DEFAULT 0 CHECK (rows_applied >= 0),
    rows_rejected      INTEGER NOT NULL DEFAULT 0 CHECK (rows_rejected >= 0),
    row_errors         JSONB NOT NULL DEFAULT '[]'::jsonb
                       CHECK (jsonb_typeof(row_errors) = 'array'),
    created_at         TIMESTAMPTZ NOT NULL,
    finished_at        TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS stock_sync_jobs_pharmacy_created_idx
    ON stock_sync_jobs (pharmacy_id, created_at DESC, job_id DESC);