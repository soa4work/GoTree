package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSearchFold(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{"Москва", "москва"},
		{"МОСКВА", "москва"},
		{"Ёлкино", "елкино"},
		{"Йошкар-Ола", "иошкар-ола"},
		{"ӘУЛИЕАТА", "әулиеата"},
		{"ҚАРАҒАНДЫ", "қарағанды"},
		{"ӨСКЕМЕН", "өскемен"},
		{"ҰЛЫТАУ", "ұлытау"},
		{"ҮШТӨБЕ", "үштөбе"},
		{"ІЛЕ", "іле"},
		{"ҢҺ", "ңһ"},
		{"йё", "ие"}, // decomposed й and ё
		{"Müller", "müller"},
		{"BERLIN 100%_x", "berlin 100%_x"},
		{"", ""},
	} {
		if got := SearchFold(c.in); got != c.want {
			t.Errorf("SearchFold(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSearchFoldSQL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	conn, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, s := range []string{"Москва", "ҚАРАҒАНДЫ", "Ёлкин", ""} {
		var got string
		if err := conn.QueryRowContext(ctx, `SELECT search_fold(?)`, s).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != SearchFold(s) {
			t.Errorf("search_fold(%q) = %q, want %q", s, got, SearchFold(s))
		}
	}
	var got string
	if err := conn.QueryRowContext(ctx, `SELECT search_fold(NULL)`).Scan(&got); err != nil || got != "" {
		t.Errorf("search_fold(NULL) = %q, %v", got, err)
	}
	var like int
	if err := conn.QueryRowContext(ctx, `SELECT search_fold('Алматы, Казахстан') LIKE '%казахстан%'`).Scan(&like); err != nil || like != 1 {
		t.Errorf("LIKE = %d, %v", like, err)
	}
}
