CREATE TABLE IF NOT EXISTS notifications (
    notification_id TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL UNIQUE,
    user_id         TEXT NOT NULL,
    channel         INTEGER NOT NULL,
    template        INTEGER NOT NULL,
    params          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL,
    read_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS notifications_user_created_idx
    ON notifications (user_id, created_at DESC, notification_id DESC);
