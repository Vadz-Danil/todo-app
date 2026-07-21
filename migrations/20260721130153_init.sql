-- +goose Up
CREATE
EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users
(
    id            UUID PRIMARY KEY         DEFAULT uuid_generate_v4(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255),
    google_id     VARCHAR(255) UNIQUE,
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tasks
(
    id         UUID PRIMARY KEY         DEFAULT uuid_generate_v4(),
    user_id    UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      VARCHAR(255) NOT NULL,
    status     VARCHAR(20)  NOT NULL    DEFAULT 'TODO',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO users (id, email, password_hash)
VALUES ('11111111-1111-1111-1111-111111111111',
        'admin@example.com',
        '$2a$10$w8T0sN9V8mH3oG.QzO2j5.Xq9g/4Yx2Y6Z4b0V3u9O2K3L5M6N7O8') ON CONFLICT (email) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS users;