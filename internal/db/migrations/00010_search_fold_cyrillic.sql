-- +goose Up

-- The unicode61 tokenizer does not fold Cyrillic: "Пётр" was not found by
-- "петр". The store now indexes names with ё/й folded to е/и (and folds the
-- query the same way); rebuild the existing entries accordingly.
DELETE FROM persons_fts;
INSERT INTO persons_fts (rowid, names)
SELECT p.id, replace(replace(replace(replace(
        p.given_names || ' ' || p.surname || ' ' || p.nickname || ' ' ||
        coalesce((SELECT group_concat(n.given_names || ' ' || n.surname || ' ' || n.nickname, ' ')
            FROM person_names n WHERE n.person_id = p.id), ''),
    'ё', 'е'), 'Ё', 'Е'), 'й', 'и'), 'Й', 'И')
FROM persons p;

-- +goose Down
DELETE FROM persons_fts;
INSERT INTO persons_fts (rowid, names)
SELECT p.id, p.given_names || ' ' || p.surname || ' ' || p.nickname || ' ' ||
    coalesce((SELECT group_concat(n.given_names || ' ' || n.surname || ' ' || n.nickname, ' ')
        FROM person_names n WHERE n.person_id = p.id), '')
FROM persons p;
