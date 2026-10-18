package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/praetorianer777/gotree/internal/gendate"
)

func TestOpenRecomputesDateKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO trees (id, name, created_at, updated_at) VALUES (1, 't', '', '')`,
		`INSERT INTO persons (id, tree_id, created_at, updated_at) VALUES (1, 1, '', '')`,
		`INSERT INTO events (id, tree_id, person_id, type, date_raw, date, date_sort, date_sort_end, created_at, updated_at)
			VALUES (1, 1, 1, 'BIRT', 'FEB 1750/51', 'FEB 1750/51', 17500200, 17500200, 'then', 'then')`,
		`INSERT INTO events (id, tree_id, person_id, type, date_raw, date, date_sort, date_sort_end, date_qualifier, created_at, updated_at)
			VALUES (2, 1, 1, 'DEAT', '29 FEB 1751/52', '', NULL, NULL, '', '', '')`,
		`INSERT INTO events (id, tree_id, person_id, type, date_raw, created_at, updated_at)
			VALUES (3, 1, 1, 'RESI', 'yesterday', '', '')`,
		`INSERT INTO heirlooms (id, tree_id, name, made_date_raw, made_date_sort, created_at, updated_at)
			VALUES (1, 1, 'Clock', 'FEB 1750/51', 17500200, '', '')`,
		`DELETE FROM app_meta WHERE key = 'date_key_version'`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	conn.Close()

	conn, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	type ev struct {
		raw, date, updated string
		sort, end          sql.NullInt64
	}
	read := func(id int) ev {
		var e ev
		if err := conn.QueryRowContext(ctx, `SELECT date_raw, date, updated_at, date_sort, date_sort_end FROM events WHERE id = ?`, id).
			Scan(&e.raw, &e.date, &e.updated, &e.sort, &e.end); err != nil {
			t.Fatal(err)
		}
		return e
	}
	if e := read(1); e.sort.Int64 != 17510200 || e.end.Int64 != 17510200 || e.raw != "FEB 1750/51" || e.updated != "then" {
		t.Errorf("FEB 1750/51: %+v", e)
	}
	if e := read(2); e.sort.Int64 != 17520229 || e.end.Int64 != 17520229 || e.date != "29 FEB 1751/52" || e.raw != "29 FEB 1751/52" {
		t.Errorf("29 FEB 1751/52: %+v", e)
	}
	if e := read(3); e.date != "" || e.sort.Valid || e.end.Valid || e.raw != "yesterday" {
		t.Errorf("yesterday: %+v", e)
	}
	var made sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT made_date_sort FROM heirlooms WHERE id = 1`).Scan(&made); err != nil || made.Int64 != 17510200 {
		t.Errorf("heirloom: %v %v", made, err)
	}
	var version string
	if err := conn.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = 'date_key_version'`).Scan(&version); err != nil || version != gendate.KeyVersion {
		t.Errorf("version: %q %v", version, err)
	}

	// With the version present, nothing is recomputed.
	if _, err := conn.ExecContext(ctx, `UPDATE events SET date_sort = 1 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	conn, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if e := read(1); e.sort.Int64 != 1 {
		t.Errorf("a reopen with the current version rewrote the keys: %+v", e)
	}
}
