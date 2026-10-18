package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/pressly/goose/v3"
)

// Migration 16 (#56):
//   - key tables get AUTOINCREMENT, so ids of deleted rows (for example
//     after "Replace the tree") are never handed out again. A stale id in
//     a transcription draft, URL or bookmark then finds nothing instead of
//     another person;
//   - a citation is deleted together with its last link (trigger
//     citation_links_drop_orphans), and citations that already lost all
//     their links are removed.
//
// Rebuilding a table that other tables reference needs foreign keys off:
// with them on, DROP TABLE runs an implicit DELETE that cascades and wipes
// the child rows. SQLite allows that pragma only outside a transaction,
// and goose runs SQL files inside one, so this is a Go migration that runs
// on a dedicated connection.
//
// Edge cases:
//   - An existing database gets AUTOINCREMENT now, and the sequence starts
//     at the current max(id). Ids above it that were deleted earlier can
//     still be reused once; they cannot be recovered.
//   - The first start after the update copies these tables, which takes a
//     few seconds on a big tree.
var autoincrementTables = []string{
	"persons", "person_names", "families", "events", "places",
	"sources", "repositories", "citations", "heirlooms",
}

// orphanCitationTrigger deletes a citation when its last link goes, on every
// path: the *_unlink_citations triggers, cascades, Unlink, heirloom deletion
// and clearTree. A citation removed this way through a cascade gets no
// change_log row; the deletion of its parent is logged.
const orphanCitationTrigger = `CREATE TRIGGER IF NOT EXISTS citation_links_drop_orphans AFTER DELETE ON citation_links
WHEN NOT EXISTS (SELECT 1 FROM citation_links WHERE citation_id = OLD.citation_id)
BEGIN
    DELETE FROM citations WHERE id = OLD.citation_id;
END`

func migration16() *goose.Migration {
	return goose.NewGoMigration(16,
		&goose.GoFunc{RunDB: up16, Mode: goose.TransactionDisabled},
		&goose.GoFunc{RunDB: down16, Mode: goose.TransactionDisabled})
}

// withoutForeignKeys runs fn in a transaction on one pinned connection with
// foreign key enforcement off, and turns it back on before the connection
// returns to the pool.
func withoutForeignKeys(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	c, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := c.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	// The pragma is a silent no-op inside a transaction. If it did not take,
	// DROP TABLE would cascade and wipe the child rows, so stop instead.
	var on int
	if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&on); err != nil {
		return err
	}
	if on != 0 {
		return fmt.Errorf("could not turn foreign keys off")
	}
	// Registered after Close, so it runs before it.
	defer c.ExecContext(context.WithoutCancel(ctx), "PRAGMA foreign_keys = ON")
	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type savedSQL struct{ name, sql string }

// dropTriggers returns all triggers and drops them. Since SQLite 3.26,
// ALTER TABLE ... RENAME re-parses every trigger and fails when one refers
// to a table that is missing at that moment.
func dropTriggers(ctx context.Context, tx *sql.Tx) ([]savedSQL, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name, sql FROM sqlite_master WHERE type = 'trigger' AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var saved []savedSQL
	for rows.Next() {
		var s savedSQL
		if err := rows.Scan(&s.name, &s.sql); err != nil {
			rows.Close()
			return nil, err
		}
		saved = append(saved, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, s := range saved {
		if _, err := tx.ExecContext(ctx, `DROP TRIGGER "`+s.name+`"`); err != nil {
			return nil, err
		}
	}
	return saved, nil
}

func restoreTriggers(ctx context.Context, tx *sql.Tx, saved []savedSQL) error {
	for _, s := range saved {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("recreate trigger %s: %w", s.name, err)
		}
	}
	return nil
}

func up16(ctx context.Context, db *sql.DB) error {
	return withoutForeignKeys(ctx, db, func(tx *sql.Tx) error {
		saved, err := dropTriggers(ctx, tx)
		if err != nil {
			return err
		}
		for _, t := range autoincrementTables {
			if err := rebuild(ctx, tx, t, true); err != nil {
				return err
			}
		}
		if err := restoreTriggers(ctx, tx, saved); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, orphanCitationTrigger); err != nil {
			return err
		}
		// Citations that already lost every link. With foreign keys off this
		// does not cascade, and nothing but citation_links references them.
		// Foreign key violations are deliberately not an error here: copying
		// identical rows cannot create any, and failing would stop an
		// existing install from starting.
		_, err = tx.ExecContext(ctx, `DELETE FROM citations WHERE NOT EXISTS (SELECT 1 FROM citation_links l WHERE l.citation_id = citations.id)`)
		return err
	})
}

func down16(ctx context.Context, db *sql.DB) error {
	return withoutForeignKeys(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DROP TRIGGER IF EXISTS citation_links_drop_orphans`); err != nil {
			return err
		}
		saved, err := dropTriggers(ctx, tx)
		if err != nil {
			return err
		}
		for _, t := range autoincrementTables {
			if err := rebuild(ctx, tx, t, false); err != nil {
				return err
			}
		}
		if err := restoreTriggers(ctx, tx, saved); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence'`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			marks := strings.TrimSuffix(strings.Repeat("?,", len(autoincrementTables)), ",")
			args := make([]any, len(autoincrementTables))
			for i, t := range autoincrementTables {
				args[i] = t
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM sqlite_sequence WHERE name IN (`+marks+`)`, args...); err != nil {
				return err
			}
		}
		return nil
	})
}

var (
	reAutoincrement = regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`)
	rePK            = regexp.MustCompile(`(?i)\bid\s+INTEGER\s+PRIMARY\s+KEY\b`)
	rePKAuto        = regexp.MustCompile(`(?i)\bid\s+INTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT\b`)
)

// rebuild recreates table with or without AUTOINCREMENT on its id column,
// following SQLite's documented procedure: create the new table, copy,
// drop the old one, rename, recreate the indexes. It does nothing when the
// table is already in the wanted shape, so Up and Down can be repeated.
func rebuild(ctx context.Context, tx *sql.Tx, table string, auto bool) error {
	var create string
	if err := tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&create); err != nil {
		return fmt.Errorf("read schema of %s: %w", table, err)
	}
	if reAutoincrement.MatchString(create) == auto {
		return nil
	}
	if auto {
		if len(rePK.FindAllStringIndex(create, -1)) != 1 {
			return fmt.Errorf("table %s: cannot find a single id INTEGER PRIMARY KEY", table)
		}
		create = rePK.ReplaceAllString(create, "id INTEGER PRIMARY KEY AUTOINCREMENT")
	} else {
		if len(rePKAuto.FindAllStringIndex(create, -1)) != 1 {
			return fmt.Errorf("table %s: cannot find a single id INTEGER PRIMARY KEY AUTOINCREMENT", table)
		}
		create = rePKAuto.ReplaceAllString(create, "id INTEGER PRIMARY KEY")
	}
	// After an earlier rename SQLite stores the name quoted.
	reHead := regexp.MustCompile(`(?is)^\s*CREATE\s+TABLE\s+"?` + regexp.QuoteMeta(table) + `"?\s*\(`)
	if !reHead.MatchString(create) {
		return fmt.Errorf("table %s: unexpected CREATE TABLE header", table)
	}
	create = reHead.ReplaceAllLiteralString(create, "CREATE TABLE "+table+"_rebuild (")

	rows, err := tx.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL`, table)
	if err != nil {
		return err
	}
	var idx []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		idx = append(idx, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	steps := []string{
		create,
		`INSERT INTO ` + table + `_rebuild SELECT * FROM ` + table,
		`DROP TABLE ` + table,
		`ALTER TABLE ` + table + `_rebuild RENAME TO ` + table,
	}
	steps = append(steps, idx...)
	for _, q := range steps {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("rebuild %s: %w", table, err)
		}
	}
	return nil
}
