// Package db opens the SQLite database and applies the embedded migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedded embed.FS

var migrations = func() fs.FS {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}()

// Open opens (creating if needed) the database at path and migrates it to the
// latest schema.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	// Pragmas go into the DSN so every pooled connection gets them, not just
	// the first one.
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	// The path is part of a URI: '#' and '?' in a directory name would
	// otherwise cut it short.
	dsn := "file:" + strings.NewReplacer("%", "%25", "#", "%23", "?", "%3F").Replace(path) + "?" + q.Encode()

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := Migrate(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := refreshNameKeys(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := refreshDateKeys(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

const fingerprintKey = "names_key_fingerprint"

// refreshNameKeys recomputes persons.surname_key/given_key when the stored
// fingerprint of NamesKey is missing or differs from the current one, so a
// change of the key function (or of golang.org/x/text) cannot leave stale
// keys behind. The update and the new fingerprint are one transaction.
func refreshNameKeys(ctx context.Context, conn *sql.DB) error {
	want := NamesKeyFingerprint()
	var have string
	err := conn.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = ?`, fingerprintKey).Scan(&have)
	if err == nil && have == want {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("name key fingerprint: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE persons SET surname_key = names_key(surname), given_key = names_key(given_names)`); err != nil {
		return fmt.Errorf("recompute name keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fingerprintKey, want); err != nil {
		return err
	}
	return tx.Commit()
}

// Migrate applies all pending migrations.
func Migrate(ctx context.Context, conn *sql.DB) error {
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// SchemaVersion reports the version of the newest applied migration.
func SchemaVersion(ctx context.Context, conn *sql.DB) (int64, error) {
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		return 0, err
	}
	return provider.GetDBVersion(ctx)
}
