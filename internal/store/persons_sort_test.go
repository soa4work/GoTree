package store

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/db"
	"github.com/praetorianer777/gotree/internal/gedcom"
)

func TestListPersonsUsesSortIndex(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	rows, err := s.DB.QueryContext(ctx, `EXPLAIN QUERY PLAN `+listPersonsSQL, 1, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	all := strings.Join(plan, "; ")
	if strings.Contains(all, "TEMP B-TREE") || !strings.Contains(all, "persons_tree_sort") {
		t.Errorf("query plan: %s", all)
	}
}

func TestUpdatePersonMovesInList(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	mustPerson(t, s, a, "A", "Борисов")
	yakov := mustPerson(t, s, a, "B", "Яков")
	if _, err := s.UpdatePerson(ctx, a, yakov.ID, PersonInput{GivenNames: "B", Surname: "Абай"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListPersons(ctx, a, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if list.Items[0].ID != yakov.ID {
		t.Errorf("first: %+v", list.Items)
	}
}

func TestSearchPagingIsStable(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	surnames := []string{"Muller", "Müller", "MULLER", "muller"}
	var created []int64
	for i := 0; i < 25; i++ {
		created = append(created, mustPerson(t, s, a, "Anna", surnames[i%len(surnames)]).ID)
	}
	for _, q := range []string{"muller", ""} {
		var got []int64
		total := 25
		for off := 0; off < total; off += 4 {
			page, err := s.ListPersons(ctx, a, q, 4, off)
			if err != nil {
				t.Fatal(err)
			}
			total = page.Total
			for _, p := range page.Items {
				got = append(got, p.ID)
			}
		}
		if total != 25 || !reflect.DeepEqual(got, created) {
			t.Errorf("query %q: total %d, ids %v, want %v", q, total, got, created)
		}
	}
}

func TestImportFillsSortKeys(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	doc, err := gedcom.Parse([]byte("0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" +
		"0 @I1@ INDI\n1 NAME Анна /Ёлкина/\n" +
		"0 @I2@ INDI\n1 NAME Яков /Абаев/\n0 TRLR\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListPersons(ctx, a, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Items[0].Surname != "Абаев" || list.Items[1].Surname != "Ёлкина" {
		t.Errorf("order: %+v", list.Items)
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM persons WHERE tree_id = ? AND surname <> '' AND surname_key = x''`, a.TreeID).Scan(&n); err != nil || n != 0 {
		t.Errorf("persons without a key: %d, %v", n, err)
	}
}

// BenchmarkListPersons documents the fix: a page deep in a 20,000-person tree
// is read from the sort index. Run with -bench; it is not a timing test.
func BenchmarkListPersons(b *testing.B) {
	conn, err := db.Open(ctx, filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	s := New(conn)
	if _, err := conn.Exec(`INSERT INTO trees (id, name, created_at, updated_at) VALUES (1, 'T', '', '')`); err != nil {
		b.Fatal(err)
	}
	tx, err := conn.Begin()
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 20000; i++ {
		surname, given := fmt.Sprintf("Фамилия%05d", (i*7919)%20000), fmt.Sprintf("Имя%d", i%50)
		if _, err := tx.Exec(`INSERT INTO persons (tree_id, given_names, surname, surname_key, given_key, created_at, updated_at)
			VALUES (1, ?, ?, ?, ?, '', '')`, given, surname, db.NamesKey(surname), db.NamesKey(given)); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
	a := Actor{TreeID: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ListPersons(ctx, a, "", 50, 10000); err != nil {
			b.Fatal(err)
		}
	}
}
