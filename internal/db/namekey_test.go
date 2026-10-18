package db

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestNamesKeyMatchesCollation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE n (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	in := []string{"яков", "Яков", "Ұлан", "Zed", "алматы", "Әлия", "Үй", "Уфа", "Ғани", "Гоголь", "Ион", "Іле", "Muller", "Müller", "abc", "Abd",
		"", "Ёлкин", "Елкин", "Иван", "Йорк", "ab", "a"}
	for _, s := range in {
		if _, err := conn.ExecContext(ctx, `INSERT INTO n (name) VALUES (?)`, s); err != nil {
			t.Fatal(err)
		}
	}
	order := func(q string) []int {
		rows, err := conn.QueryContext(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		return ids
	}
	byCollation := order(`SELECT id FROM n ORDER BY name COLLATE NAMES, id`)
	bySQLKey := order(`SELECT id FROM n ORDER BY names_key(name), id`)
	idx := make([]int, len(in))
	for i := range idx {
		idx[i] = i + 1
	}
	sort.SliceStable(idx, func(i, j int) bool {
		return bytes.Compare(NamesKey(in[idx[i]-1]), NamesKey(in[idx[j]-1])) < 0
	})
	if !reflect.DeepEqual(byCollation, idx) {
		t.Errorf("Go keys order %v, collation order %v", idx, byCollation)
	}
	if !reflect.DeepEqual(byCollation, bySQLKey) {
		t.Errorf("names_key order %v, collation order %v", bySQLKey, byCollation)
	}

	if k := NamesKey(""); k == nil || len(k) != 0 {
		t.Errorf("NamesKey(\"\") = %#v, want empty non-nil", k)
	}
	if !bytes.Equal(NamesKey("Muller"), NamesKey("müller")) {
		t.Error("case and diacritics should not change the key")
	}
	for _, s := range []string{"Әлия", "Muller", ""} {
		var got []byte
		if err := conn.QueryRowContext(ctx, `SELECT names_key(?)`, s).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, NamesKey(s)) {
			t.Errorf("names_key(%q) = %x, want %x", s, got, NamesKey(s))
		}
	}
	var got []byte
	if err := conn.QueryRowContext(ctx, `SELECT names_key(NULL)`).Scan(&got); err != nil || len(got) != 0 {
		t.Errorf("names_key(NULL) = %x, %v", got, err)
	}
}

func TestNamesKeyGolden(t *testing.T) {
	t.Parallel()
	// If this fails after changing sortKeyText or upgrading golang.org/x/text,
	// add a migration that recomputes persons.surname_key/given_key with
	// names_key().
	for s, want := range map[string]string{
		"Muller": "1741183617111711164c17bd",
		"Әлия":   "85e00185e00e85e00a85e028",
		"ё":      "85e007",
		"й":      "85e00b",
		"і":      "85e024",
		"ұ":      "85e019",
		"ү":      "85e01a",
		"é":      "164c",
	} {
		if got := fmt.Sprintf("%x", NamesKey(s)); got != want {
			t.Errorf("NamesKey(%q) = %s, want %s", s, got, want)
		}
	}
}

func TestMigrationFillsPersonSortKeys(t *testing.T) {
	t.Parallel()
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
	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO trees (id, name, created_at, updated_at) VALUES (1, 'T', '', '')`,
		`INSERT INTO persons (id, tree_id, given_names, surname, created_at, updated_at) VALUES (1, 1, 'Пётр', 'Ёлкин', '', '')`,
		`INSERT INTO persons (id, tree_id, given_names, surname, created_at, updated_at) VALUES (2, 1, 'Anna', 'Müller', '', '')`,
		`INSERT INTO persons (id, tree_id, given_names, surname, created_at, updated_at) VALUES (3, 1, 'X', '', '', '')`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	for id, names := range map[int][2]string{1: {"Ёлкин", "Пётр"}, 2: {"Müller", "Anna"}, 3: {"", "X"}} {
		var sk, gk []byte
		var nulls int
		if err := conn.QueryRowContext(ctx, `SELECT surname_key, given_key, surname_key IS NULL OR given_key IS NULL FROM persons WHERE id = ?`, id).Scan(&sk, &gk, &nulls); err != nil {
			t.Fatal(err)
		}
		if nulls != 0 || !bytes.Equal(sk, NamesKey(names[0])) || !bytes.Equal(gk, NamesKey(names[1])) {
			t.Errorf("person %d: keys %x %x, want %x %x", id, sk, gk, NamesKey(names[0]), NamesKey(names[1]))
		}
	}
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'persons_tree_sort'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("persons_tree_sort: %d, %v", n, err)
	}
	if _, err := provider.DownTo(ctx, 10); err != nil {
		t.Errorf("down: %v", err)
	}
}

func TestOpenRecomputesStaleNameKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO trees (id, name, created_at, updated_at) VALUES (1, 'T', '', '')`,
		`INSERT INTO persons (id, tree_id, given_names, surname, created_at, updated_at) VALUES (1, 1, 'Anna', 'Müller', '', '')`,
		`UPDATE persons SET surname_key = x'ff', given_key = x'ff'`,
		`UPDATE app_meta SET value = 'stale' WHERE key = 'names_key_fingerprint'`,
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
	var sk, gk []byte
	var fp string
	if err := conn.QueryRowContext(ctx, `SELECT surname_key, given_key FROM persons WHERE id = 1`).Scan(&sk, &gk); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sk, NamesKey("Müller")) || !bytes.Equal(gk, NamesKey("Anna")) {
		t.Errorf("keys not recomputed: %x %x", sk, gk)
	}
	if err := conn.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = 'names_key_fingerprint'`).Scan(&fp); err != nil || fp != NamesKeyFingerprint() {
		t.Errorf("fingerprint %q, %v", fp, err)
	}
}
