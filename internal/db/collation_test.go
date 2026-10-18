package db

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

func TestNamesCollation(t *testing.T) {
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE n (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	in := []string{"яков", "Яков", "Ұлан", "Zed", "алматы", "Әлия", "Үй", "Уфа", "Ғани", "Гоголь", "Ион", "Іле", "Muller", "Müller", "abc", "Abd"}
	for _, s := range in {
		if _, err := conn.ExecContext(ctx, `INSERT INTO n (name) VALUES (?)`, s); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := conn.QueryContext(ctx, `SELECT name FROM n ORDER BY name COLLATE NAMES, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	want := []string{"abc", "Abd", "Muller", "Müller", "Zed", "алматы", "Әлия", "Гоголь", "Ғани", "Ион", "Іле", "Уфа", "Үй", "Ұлан", "яков", "Яков"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// Sorting a big list must not be noticeably slower than a binary sort.
func TestNamesCollationSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE n (name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 50000)
		INSERT INTO n SELECT char(1040 + i % 32, 1072 + (i / 32) % 32, 1072 + (i / 7) % 32) || i FROM c`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var last string
	if err := conn.QueryRowContext(ctx, `SELECT name FROM (SELECT name FROM n ORDER BY name COLLATE NAMES LIMIT 10 OFFSET 49990) LIMIT 1`).Scan(&last); err != nil {
		t.Fatal(err)
	}
	t.Logf("sorted 50000 names in %s", time.Since(start))
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("sorting 50000 names took %s", d)
	}
}

func TestMigrationFoldsExistingSearchIndex(t *testing.T) {
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
	if _, err := provider.DownTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO trees (id, name, created_at, updated_at) VALUES (1, 'T', '', '')`,
		`INSERT INTO persons (id, tree_id, given_names, surname, created_at, updated_at) VALUES (1, 1, 'Пётр', 'Ёлкин', '', '')`,
		`INSERT INTO persons_fts (rowid, names) VALUES (1, 'Пётр Ёлкин  ')`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM persons_fts WHERE persons_fts MATCH ?`, `"елкин"*`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d matches for елкин after migrating, want 1", n)
	}
}
