-- +goose Up
ALTER TABLE tasks ADD COLUMN description TEXT;

-- +goose Down
ALTER TABLE tasks DROP COLUMN description;