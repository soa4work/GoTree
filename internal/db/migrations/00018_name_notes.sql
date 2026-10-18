-- +goose Up
-- Notes on a name (GEDCOM NAME.NOTE): for the primary name on the person,
-- for the other names on the name.
ALTER TABLE persons ADD COLUMN name_notes TEXT NOT NULL DEFAULT '';
ALTER TABLE person_names ADD COLUMN notes TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE person_names DROP COLUMN notes;
ALTER TABLE persons DROP COLUMN name_notes;
