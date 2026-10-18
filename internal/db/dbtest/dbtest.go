// Package dbtest opens migrated databases for tests from a template built once per test binary.
package dbtest

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/praetorianer777/gotree/internal/db"
)

var tmpl struct {
	once sync.Once
	data []byte
	err  error
}

// template migrates a database once and keeps its bytes. VACUUM INTO writes
// one consistent file without -wal and -shm side files.
func template() ([]byte, error) {
	tmpl.once.Do(func() {
		dir, err := os.MkdirTemp("", "gotree-dbtest-")
		if err != nil {
			tmpl.err = err
			return
		}
		defer os.RemoveAll(dir)
		conn, err := db.Open(context.Background(), filepath.Join(dir, "migrated.db"))
		if err != nil {
			tmpl.err = err
			return
		}
		out := filepath.Join(dir, "template.db")
		_, err = conn.ExecContext(context.Background(), `VACUUM INTO ?`, out)
		conn.Close()
		if err != nil {
			tmpl.err = err
			return
		}
		tmpl.data, tmpl.err = os.ReadFile(out)
	})
	return tmpl.data, tmpl.err
}

// Open returns a migrated database of its own in t.TempDir(). It is closed
// when the test ends.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	data, err := template()
	if err != nil {
		t.Fatal(err)
	}
	// TempDir is registered before the Close cleanup, so the connection is
	// closed first (cleanups run last-in, first-out).
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}
