-- +goose Up
ALTER TABLE tasks
    ADD COLUMN IF NOT EXISTS priority       VARCHAR(10)              NOT NULL DEFAULT 'MEDIUM',
    ADD COLUMN IF NOT EXISTS reviewer       VARCHAR(255),
    ADD COLUMN IF NOT EXISTS estimate_hours NUMERIC(6, 2),
    ADD COLUMN IF NOT EXISTS buffer_hours   NUMERIC(6, 2),
    ADD COLUMN IF NOT EXISTS spent_hours    NUMERIC(6, 2),
    ADD COLUMN IF NOT EXISTS due_date       TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS started_at     TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS completed_at   TIMESTAMP WITH TIME ZONE,
    ADD COLUMN IF NOT EXISTS updated_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN IF NOT EXISTS position       DOUBLE PRECISION         NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS blockers       TEXT;

UPDATE tasks SET completed_at = created_at WHERE status = 'DONE' AND completed_at IS NULL;
UPDATE tasks SET started_at = created_at WHERE status IN ('IN_PROGRESS', 'DONE') AND started_at IS NULL;
UPDATE tasks SET position = EXTRACT(EPOCH FROM created_at) WHERE position = 0;

ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS tasks_status_check,
    DROP CONSTRAINT IF EXISTS tasks_priority_check,
    DROP CONSTRAINT IF EXISTS tasks_reviewer_required;

ALTER TABLE tasks
    ADD CONSTRAINT tasks_status_check CHECK (status IN ('TODO', 'IN_PROGRESS', 'IN_REVIEW', 'DONE')),
    ADD CONSTRAINT tasks_priority_check CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'URGENT')),
    ADD CONSTRAINT tasks_reviewer_required CHECK (status <> 'IN_REVIEW' OR (reviewer IS NOT NULL AND btrim(reviewer) <> ''));

CREATE INDEX IF NOT EXISTS idx_tasks_user_status ON tasks (user_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_user_created_at ON tasks (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_user_completed_at ON tasks (user_id, completed_at);
CREATE INDEX IF NOT EXISTS idx_tasks_user_priority ON tasks (user_id, priority);
CREATE INDEX IF NOT EXISTS idx_tasks_board_order ON tasks (user_id, status, position);

CREATE TABLE IF NOT EXISTS task_status_history
(
    id          UUID PRIMARY KEY                  DEFAULT uuid_generate_v4(),
    task_id     UUID        NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id     UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    from_status VARCHAR(20),
    to_status   VARCHAR(20) NOT NULL,
    changed_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_status_history_user_changed ON task_status_history (user_id, changed_at DESC);
CREATE INDEX IF NOT EXISTS idx_status_history_task ON task_status_history (task_id, changed_at);

-- +goose Down
DROP TABLE IF EXISTS task_status_history;

DROP INDEX IF EXISTS idx_tasks_board_order;
DROP INDEX IF EXISTS idx_tasks_user_priority;
DROP INDEX IF EXISTS idx_tasks_user_completed_at;
DROP INDEX IF EXISTS idx_tasks_user_created_at;
DROP INDEX IF EXISTS idx_tasks_user_status;

ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS tasks_status_check,
    DROP CONSTRAINT IF EXISTS tasks_priority_check,
    DROP CONSTRAINT IF EXISTS tasks_reviewer_required;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS reviewer,
    DROP COLUMN IF EXISTS estimate_hours,
    DROP COLUMN IF EXISTS buffer_hours,
    DROP COLUMN IF EXISTS spent_hours,
    DROP COLUMN IF EXISTS due_date,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS position,
    DROP COLUMN IF EXISTS blockers;
