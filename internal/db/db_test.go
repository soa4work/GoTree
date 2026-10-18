package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMigratesAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	conn, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v, err := SchemaVersion(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if v < 1 {
		t.Fatalf("schema version %d, want >= 1", v)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO app_meta (key, value) VALUES ('k', 'v')`); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	// A second open must find the schema in place and the data intact.
	conn, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var got string
	if err := conn.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = 'k'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "v" {
		t.Errorf("got %q, want %q", got, "v")
	}
}

func TestPragmasApplyToEveryConnection(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(3)

	// Hold several connections at once so the pool has to open new ones.
	for i := range 3 {
		c, err := conn.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		var fk int
		if err := c.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if fk != 1 {
			t.Errorf("connection %d: foreign_keys = %d, want 1", i, fk)
		}
		var mode string
		if err := c.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if mode != "wal" {
			t.Errorf("connection %d: journal_mode = %q, want wal", i, mode)
		}
	}
}

func TestOpenPathWithSpecialCharacters(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	a, err := Open(ctx, filepath.Join(base, "family #1", "tree?.db"))
	if err == nil {
		t.Fatal("a missing directory should fail")
	}
	_ = a
	dir := filepath.Join(base, "family #1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	conn, err := Open(ctx, filepath.Join(dir, "tree?.db"))
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if _, err := os.Stat(filepath.Join(dir, "tree?.db")); err != nil {
		t.Errorf("database not created at the exact path: %v", err)
	}
}

// Migration 9 rebuilds media_links and citation_links; links made before
// must survive it.
func TestHeirloomMigrationKeepsLinks(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	provider, err := newProvider(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO trees (id, name, created_at, updated_at) VALUES (1, 't', '', '')`,
		`INSERT INTO persons (id, tree_id, created_at, updated_at) VALUES (1, 1, '', '')`,
		`INSERT INTO sources (id, tree_id, title, created_at, updated_at) VALUES (1, 1, 's', '', '')`,
		`INSERT INTO citations (id, tree_id, source_id, created_at, updated_at) VALUES (1, 1, 1, '', '')`,
		`INSERT INTO citation_links (citation_id, entity_type, entity_id, field) VALUES (1, 'person', 1, 'name')`,
		`INSERT INTO media (id, tree_id, sha256, mime, kind, size, original_name, created_at, updated_at) VALUES (1, 1, printf('%064d', 0), 'image/jpeg', 'image', 1, 'a.jpg', '', '')`,
		`INSERT INTO media_links (media_id, entity_type, entity_id, sort_order) VALUES (1, 'person', 1, 3)`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	var field string
	var sort int
	if err := conn.QueryRowContext(ctx, `SELECT field FROM citation_links WHERE citation_id = 1`).Scan(&field); err != nil || field != "name" {
		t.Errorf("citation link: %q %v", field, err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT sort_order FROM media_links WHERE media_id = 1`).Scan(&sort); err != nil || sort != 3 {
		t.Errorf("media link: %d %v", sort, err)
	}
	// The triggers are back: deleting the person removes both links.
	if _, err := conn.ExecContext(ctx, `DELETE FROM persons WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = conn.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM citation_links) + (SELECT count(*) FROM media_links)`).Scan(&n)
	if n != 0 {
		t.Errorf("%d links left after deleting the person", n)
	}
}
