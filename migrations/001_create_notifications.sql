CREATE TYPE notification_status AS ENUM ('sent', 'failed', 'rate_limited', 'duplicate');

CREATE TABLE IF NOT EXISTS notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  TEXT NOT NULL UNIQUE,
    user_id     TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    status      notification_status NOT NULL,
    error       TEXT,
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user_id    ON notifications (user_id);
CREATE INDEX idx_notifications_event_type ON notifications (event_type);
CREATE INDEX idx_notifications_sent_at    ON notifications (sent_at DESC);
