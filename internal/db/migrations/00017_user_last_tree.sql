-- +goose Up

-- The tree a user last switched to; a new session opens in it while the user
-- is still a member (store.CreateSession). No foreign key: membership is
-- checked whenever the value is read, a stale id is simply ignored, and
-- SQLite cannot drop a column that is part of a foreign key (see Down).
ALTER TABLE users ADD COLUMN last_tree_id INTEGER;

-- +goose Down
ALTER TABLE users DROP COLUMN last_tree_id;
