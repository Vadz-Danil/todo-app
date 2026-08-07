-- +goose Up
-- +goose StatementBegin

-- A subtask is a checklist item inside a task: a title and a done flag. It
-- carries user_id (denormalised from the parent) so ownership can be enforced
-- with a single indexed predicate instead of a join on every read.
CREATE TABLE IF NOT EXISTS subtasks
(
    id         UUID PRIMARY KEY                  DEFAULT uuid_generate_v4(),
    task_id    UUID         NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id    UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      VARCHAR(500) NOT NULL,
    done       BOOLEAN      NOT NULL             DEFAULT FALSE,
    position   DOUBLE PRECISION NOT NULL         DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT subtasks_title_not_blank CHECK (btrim(title) <> '')
);

-- The list-for-a-task read is (task_id, position); the ownership guard filters
-- by user_id. One composite index serves the common ordered listing.
CREATE INDEX IF NOT EXISTS idx_subtasks_task_position ON subtasks (task_id, position);
CREATE INDEX IF NOT EXISTS idx_subtasks_user ON subtasks (user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_subtasks_user;
DROP INDEX IF EXISTS idx_subtasks_task_position;
DROP TABLE IF EXISTS subtasks;
-- +goose StatementEnd
