-- +goose Up
CREATE TABLE IF NOT EXISTS export_targets
(
    id           UUID PRIMARY KEY                  DEFAULT uuid_generate_v4(),
    user_id      UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         VARCHAR(120) NOT NULL,
    url          TEXT         NOT NULL,
    secret       TEXT,
    headers      JSONB        NOT NULL             DEFAULT '{}'::jsonb,
    enabled      BOOLEAN      NOT NULL             DEFAULT TRUE,
    last_status  INTEGER,
    last_error   TEXT,
    last_sent_at TIMESTAMP WITH TIME ZONE,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_export_targets_user ON export_targets (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS export_deliveries
(
    id           UUID PRIMARY KEY                  DEFAULT uuid_generate_v4(),
    user_id      UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_id    UUID REFERENCES export_targets (id) ON DELETE SET NULL,
    url          TEXT        NOT NULL,
    kind         VARCHAR(40) NOT NULL,
    status       VARCHAR(20) NOT NULL,
    status_code  INTEGER,
    attempts     INTEGER     NOT NULL              DEFAULT 0,
    duration_ms  INTEGER     NOT NULL              DEFAULT 0,
    payload_size INTEGER     NOT NULL              DEFAULT 0,
    error        TEXT,
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT export_deliveries_status_check CHECK (status IN ('SUCCESS', 'FAILED'))
);

CREATE INDEX IF NOT EXISTS idx_export_deliveries_user ON export_deliveries (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS export_deliveries;
DROP TABLE IF EXISTS export_targets;
