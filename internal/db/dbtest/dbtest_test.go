package dbtest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/praetorianer777/gotree/internal/db"
)

func TestOpenIsMigrated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fresh, err := db.Open(ctx, filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	want, err := db.SchemaVersion(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.SchemaVersion(ctx, Open(t))
	if err != nil {
		t.Fatal(err)
	}
	if got != want || got == 0 {
		t.Errorf("schema version = %d, want %d", got, want)
	}
}

func TestOpenIsolated(t *testing.T) {
	t.Parallel()
	a, b := Open(t), Open(t)
	if _, err := a.Exec(`INSERT INTO trees (name, created_at, updated_at) VALUES ('A', '', '')`); err != nil {
		t.Fatal(err)
	}
	var inA, inB int
	if err := a.QueryRow(`SELECT count(*) FROM trees`).Scan(&inA); err != nil {
		t.Fatal(err)
	}
	if err := b.QueryRow(`SELECT count(*) FROM trees`).Scan(&inB); err != nil {
		t.Fatal(err)
	}
	if inA != 1 || inB != 0 {
		t.Errorf("trees: a = %d, b = %d; want 1 and 0", inA, inB)
	}
}

func TestOpenPragmas(t *testing.T) {
	t.Parallel()
	conn := Open(t)
	var fk int
	if err := conn.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, %v", fk, err)
	}
	var mode string
	if err := conn.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v", mode, err)
	}
}
