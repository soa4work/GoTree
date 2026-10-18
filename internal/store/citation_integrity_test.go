package store

import (
	"errors"
	"testing"
)

func citationExists(t *testing.T, s *Store, a Actor, id int64) bool {
	t.Helper()
	_, err := s.GetCitation(ctx, a, id)
	if errors.Is(err, ErrNotFound) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func TestCitationDeletedWithItsLastTarget(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	src, err := s.CreateSource(ctx, a, SourceInput{Title: "Register"})
	if err != nil {
		t.Fatal(err)
	}
	p := mustPerson(t, s, a, "Anna", "")
	q := mustPerson(t, s, a, "Karl", "")
	ev, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "BIRT"})
	if err != nil {
		t.Fatal(err)
	}
	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &p.ID, Partner2ID: &q.ID})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID, Links: []LinkInput{
		{EntityType: "event", EntityID: ev.ID}, {EntityType: "family", EntityID: fam.ID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEvent(ctx, a, ev.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCitation(ctx, a, c.ID)
	if err != nil || len(got.Links) != 1 {
		t.Fatalf("the citation still supports the family: %+v, %v", got, err)
	}
	if err := s.DeleteFamily(ctx, a, fam.ID); err != nil {
		t.Fatal(err)
	}
	if citationExists(t, s, a, c.ID) {
		t.Error("citation survived its last fact")
	}

	// Through a cascade: deleting the person deletes their event.
	e2, err := s.CreateEvent(ctx, a, EventInput{PersonID: &q.ID, Type: "BIRT", AddCitations: []NewCitation{{SourceID: src.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	id := e2.Citations[0].CitationID
	if err := s.DeletePerson(ctx, a, q.ID); err != nil {
		t.Fatal(err)
	}
	if citationExists(t, s, a, id) {
		t.Error("citation survived the deletion of its person's event")
	}

	// A heirloom.
	h, err := s.CreateHeirloom(ctx, a, HeirloomInput{Name: "Ring", AddCitations: []NewCitation{{SourceID: src.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	var hc int64
	if err := s.DB.QueryRow(`SELECT citation_id FROM citation_links WHERE entity_type = 'heirloom' AND entity_id = ?`, h.ID).Scan(&hc); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHeirloom(ctx, a, h.ID); err != nil {
		t.Fatal(err)
	}
	if citationExists(t, s, a, hc) {
		t.Error("citation survived the deletion of its heirloom")
	}

	// An alternate name removed by an edit.
	np, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Eva", AlternateNames: []AlternateNameInput{{Type: "married", Surname: "Roth"}}})
	if err != nil {
		t.Fatal(err)
	}
	nc, err := s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID, Links: []LinkInput{{EntityType: "name", EntityID: np.AlternateNames[0].ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdatePerson(ctx, a, np.ID, PersonInput{GivenNames: "Eva"}); err != nil {
		t.Fatal(err)
	}
	if citationExists(t, s, a, nc.ID) {
		t.Error("citation survived the removal of its name")
	}
}

func TestUpdateCitationKeepsAndReplacesLinks(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	src, err := s.CreateSource(ctx, a, SourceInput{Title: "Register"})
	if err != nil {
		t.Fatal(err)
	}
	p := mustPerson(t, s, a, "Anna", "")
	e1, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "BIRT"})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "DEAT"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID, Links: []LinkInput{
		{EntityType: "person", EntityID: p.ID}, {EntityType: "event", EntityID: e1.ID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	up, err := s.UpdateCitation(ctx, a, c.ID, CitationInput{SourceID: src.ID, Links: []LinkInput{
		{EntityType: "person", EntityID: p.ID, Status: "disputed", StatusReason: "two Annas"},
		{EntityType: "event", EntityID: e2.ID},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if up.ID != c.ID || len(up.Links) != 2 {
		t.Fatalf("updated citation: %+v", up)
	}
	var sawPerson, sawDeath bool
	for _, l := range up.Links {
		switch {
		case l.EntityType == "person" && l.EntityID == p.ID:
			sawPerson = true
			if l.Status != "disputed" || l.StatusReason != "two Annas" {
				t.Errorf("person link: %+v", l)
			}
		case l.EntityType == "event" && l.EntityID == e2.ID:
			sawDeath = true
		default:
			t.Errorf("unexpected link %+v", l)
		}
	}
	if !sawPerson || !sawDeath {
		t.Errorf("links: %+v", up.Links)
	}

	_, err = s.UpdateCitation(ctx, a, c.ID, CitationInput{SourceID: src.ID})
	validationField(t, err, "links")
	again, err := s.GetCitation(ctx, a, c.ID)
	if err != nil || len(again.Links) != 2 {
		t.Errorf("citation changed by a rejected update: %+v, %v", again, err)
	}
}
