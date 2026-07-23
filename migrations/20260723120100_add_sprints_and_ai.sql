-- +goose Up
CREATE TABLE IF NOT EXISTS sprints
(
    id             UUID PRIMARY KEY                  DEFAULT uuid_generate_v4(),
    user_id        UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name           VARCHAR(255) NOT NULL,
    goal           TEXT,
    starts_on      DATE         NOT NULL,
    ends_on        DATE         NOT NULL,
    capacity_hours NUMERIC(7, 2),
    status         VARCHAR(20)  NOT NULL             DEFAULT 'PLANNED',
    ai_rationale   TEXT,
    ai_model       VARCHAR(100),
    created_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT sprints_status_check CHECK (status IN ('PLANNED', 'ACTIVE', 'COMPLETED', 'ARCHIVED')),
    CONSTRAINT sprints_range_check CHECK (ends_on >= starts_on)
);

CREATE INDEX IF NOT EXISTS idx_sprints_user_starts ON sprints (user_id, starts_on DESC);

ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS sprint_id UUID REFERENCES sprints (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_sprint ON tasks (sprint_id);

CREATE TABLE IF NOT EXISTS planning_sessions
(
    id                       UUID PRIMARY KEY                  DEFAULT uuid_generate_v4(),
    user_id                  UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    state                    VARCHAR(20) NOT NULL              DEFAULT 'COLLECTING',
    horizon_weeks            INTEGER     NOT NULL              DEFAULT 1,
    capacity_hours_per_week  NUMERIC(6, 2) NOT NULL            DEFAULT 40,
    starts_on                DATE,
    payload                  JSONB       NOT NULL              DEFAULT '{}'::jsonb,
    sprint_id                UUID REFERENCES sprints (id) ON DELETE SET NULL,
    ai_model                 VARCHAR(100),
    created_at               TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT planning_sessions_state_check CHECK (state IN ('COLLECTING', 'READY', 'PLANNED', 'COMMITTED')),
    CONSTRAINT planning_sessions_horizon_check CHECK (horizon_weeks BETWEEN 1 AND 8)
);

CREATE INDEX IF NOT EXISTS idx_planning_sessions_user ON planning_sessions (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS ai_summaries
(
    id           UUID PRIMARY KEY                  DEFAULT uuid_generate_v4(),
    user_id      UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    period       VARCHAR(30) NOT NULL,
    range_start  TIMESTAMP WITH TIME ZONE NOT NULL,
    range_end    TIMESTAMP WITH TIME ZONE NOT NULL,
    fingerprint  VARCHAR(64) NOT NULL,
    content      JSONB       NOT NULL,
    ai_model     VARCHAR(100),
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_summaries_fingerprint ON ai_summaries (user_id, fingerprint);
CREATE INDEX IF NOT EXISTS idx_ai_summaries_user_created ON ai_summaries (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS ai_summaries;
DROP TABLE IF EXISTS planning_sessions;

DROP INDEX IF EXISTS idx_tasks_sprint;
ALTER TABLE tasks DROP COLUMN IF EXISTS sprint_id;

DROP TABLE IF EXISTS sprints;
