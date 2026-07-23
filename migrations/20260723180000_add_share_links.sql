-- +goose Up
-- +goose StatementBegin

-- A share link lets someone without an account open a read-only view of the
-- owner's board or dashboard. The link itself is the credential, so only a
-- hash of the token is stored: a leaked database must not hand out working
-- links, exactly as with a password.
CREATE TABLE IF NOT EXISTS share_links (
    id             UUID PRIMARY KEY,
    user_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash     TEXT NOT NULL,
    kind           TEXT NOT NULL DEFAULT 'BOARD',
    label          TEXT NOT NULL DEFAULT '',
    expires_at     TIMESTAMPTZ,
    revoked_at     TIMESTAMPTZ,
    view_count     INTEGER NOT NULL DEFAULT 0,
    last_viewed_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT share_links_kind_check CHECK (kind IN ('BOARD', 'DASHBOARD', 'BOTH'))
);

-- The public lookup is by token hash alone, so it must be unique and indexed:
-- resolving a link is a single index hit rather than a scan over every row.
CREATE UNIQUE INDEX IF NOT EXISTS share_links_token_hash_key ON share_links (token_hash);
CREATE INDEX IF NOT EXISTS share_links_user_id_created_at_idx ON share_links (user_id, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS share_links_user_id_created_at_idx;
DROP INDEX IF EXISTS share_links_token_hash_key;
DROP TABLE IF EXISTS share_links;
-- +goose StatementEnd
