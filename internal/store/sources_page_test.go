package store

import (
	"fmt"
	"testing"
)

func TestListSourcesPages(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	for i := 1; i <= 5; i++ {
		mustSource(t, s, a, fmt.Sprintf("Book %d", i))
	}
	var titles []string
	for _, off := range []int{0, 2, 4} {
		page, err := s.ListSources(ctx, a, "", 2, off)
		if err != nil {
			t.Fatal(err)
		}
		for _, src := range page {
			titles = append(titles, src.Title)
		}
	}
	want := []string{"Book 1", "Book 2", "Book 3", "Book 4", "Book 5"}
	if fmt.Sprint(titles) != fmt.Sprint(want) {
		t.Errorf("titles = %v, want %v", titles, want)
	}
	neg, err := s.ListSources(ctx, a, "", 2, -1)
	if err != nil || len(neg) != 2 || neg[0].Title != "Book 1" {
		t.Errorf("offset -1: %+v, %v", neg, err)
	}
}
