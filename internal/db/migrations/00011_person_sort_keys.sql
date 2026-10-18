-- +goose Up

-- Sorting the person list with COLLATE NAMES ran a Go callback for every
-- comparison over the whole tree on every page (about 1.2 s for 50,000
-- people). The store now keeps the NAMES sort keys of the surname and the
-- given names (db.NamesKey) and the list reads them in index order.
-- Rows written outside GoTree get empty keys until the next fingerprint
-- change (db.Open recomputes the keys when db.NamesKeyFingerprint changes).
ALTER TABLE persons ADD COLUMN surname_key BLOB NOT NULL DEFAULT x'';
ALTER TABLE persons ADD COLUMN given_key BLOB NOT NULL DEFAULT x'';
UPDATE persons SET surname_key = names_key(surname), given_key = names_key(given_names);
CREATE INDEX persons_tree_sort ON persons (tree_id, surname = '', surname_key, given_key);

-- +goose Down
DROP INDEX persons_tree_sort;
ALTER TABLE persons DROP COLUMN given_key;
ALTER TABLE persons DROP COLUMN surname_key;
