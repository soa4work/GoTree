package store

import (
	"errors"
	"testing"
)

// Ids of deleted rows are never handed out again, so a stale id in a draft,
// a URL or a bookmark finds nothing instead of someone else.
func TestIDsAreNotReused(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)

	p := mustPerson(t, s, a, "Last", "")
	if err := s.DeletePerson(ctx, a, p.ID); err != nil {
		t.Fatal(err)
	}
	q := mustPerson(t, s, a, "Next", "")
	if q.ID <= p.ID {
		t.Errorf("person id %d reused after deleting %d", q.ID, p.ID)
	}
	if _, err := s.GetPerson(ctx, a, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted person: %v", err)
	}

	src, err := s.CreateSource(ctx, a, SourceInput{Title: "One"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSource(ctx, a, src.ID); err != nil {
		t.Fatal(err)
	}
	src2, err := s.CreateSource(ctx, a, SourceInput{Title: "Two"})
	if err != nil {
		t.Fatal(err)
	}
	if src2.ID <= src.ID {
		t.Errorf("source id %d reused after deleting %d", src2.ID, src.ID)
	}
}
