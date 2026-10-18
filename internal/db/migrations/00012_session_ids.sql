-- +goose Up

-- Sessions get a numeric id so the account page can list and end them
-- without exposing anything derived from the token.
CREATE TABLE sessions_new (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash   BLOB NOT NULL UNIQUE,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tree_id      INTEGER REFERENCES trees (id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    user_agent   TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sessions_new (token_hash, user_id, tree_id, created_at, last_seen_at, expires_at, user_agent)
SELECT token_hash, user_id, tree_id, created_at, last_seen_at, expires_at, user_agent
FROM sessions ORDER BY created_at;
DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);

-- +goose Down
CREATE TABLE sessions_old (
    token_hash   BLOB PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tree_id      INTEGER REFERENCES trees (id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    user_agent   TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO sessions_old SELECT token_hash, user_id, tree_id, created_at, last_seen_at, expires_at, user_agent FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_old RENAME TO sessions;
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX sessions_expires ON sessions (expires_at);
