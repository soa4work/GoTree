package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestSessionIDMigrationKeepsSessions(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO users (id, username, password_hash, created_at, updated_at) VALUES (1, 'u', 'x', '', '')`,
		`INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at) VALUES (x'01', 1, 'a', 'a', 'a')`,
		`INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at) VALUES (x'02', 1, 'b', 'b', 'b')`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	var n, ids int
	if err := conn.QueryRowContext(ctx, `SELECT count(*), count(DISTINCT id) FROM sessions WHERE id IS NOT NULL`).Scan(&n, &ids); err != nil || n != 2 || ids != 2 {
		t.Errorf("sessions after migration: %d rows, %d distinct ids, %v", n, ids, err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at) VALUES (x'01', 1, '', '', '')`); err == nil {
		t.Error("token_hash must be unique")
	}
	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&n); err != nil || n != 2 {
		t.Errorf("sessions after the down migration: %d, %v", n, err)
	}
}
