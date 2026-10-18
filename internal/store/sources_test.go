package store

import (
	"errors"
	"testing"
)

func TestSourcesAndCitations(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)

	repo, err := s.CreateRepository(ctx, a, RepositoryInput{Name: "Stadtarchiv Leipzig", URL: "https://example.org"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateRepository(ctx, a, RepositoryInput{Name: "X", URL: "ftp://nope"})
	validationField(t, err, "url")

	src, err := s.CreateSource(ctx, a, SourceInput{Title: "  Kirchenbuch   St. Thomas ", Author: "Ev.-luth. Gemeinde", RepositoryID: &repo.ID})
	if err != nil {
		t.Fatal(err)
	}
	if src.Title != "Kirchenbuch St. Thomas" || src.Repository == nil || src.Repository.ID != repo.ID {
		t.Fatalf("source: %+v", src)
	}
	_, err = s.CreateSource(ctx, a, SourceInput{Title: ""})
	validationField(t, err, "title")

	anna := mustPerson(t, s, a, "Anna", "Müller")
	birth, err := s.CreateEvent(ctx, a, EventInput{
		PersonID: &anna.ID, Type: "BIRT", Date: "1850",
		AddCitations: []NewCitation{{SourceID: src.ID, Page: "S. 12, Nr. 34", Quality: ptr(3)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(birth.Citations) != 1 || birth.Citations[0].SourceTitle != "Kirchenbuch St. Thomas" || *birth.Citations[0].Quality != 3 {
		t.Fatalf("event citations: %+v", birth.Citations)
	}

	// One citation supporting the birth and the person, with a field.
	c, err := s.CreateCitation(ctx, a, CitationInput{
		SourceID: src.ID, Page: "S. 13",
		Links: []LinkInput{{EntityType: "person", EntityID: anna.ID}, {EntityType: "event", EntityID: birth.ID, Field: "date"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Links) != 2 {
		t.Fatalf("links: %+v", c.Links)
	}
	for _, l := range c.Links {
		if l.PersonID == nil || *l.PersonID != anna.ID {
			t.Errorf("link %+v should point to Anna", l)
		}
	}

	_, err = s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID, Quality: ptr(7)})
	validationField(t, err, "quality")
	validationField(t, err, "links")
	_, err = s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID})
	validationField(t, err, "links")
	_, err = s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID, Links: []LinkInput{{EntityType: "event", EntityID: 999}}})
	validationField(t, err, "links")
	_, err = s.CreateEvent(ctx, a, EventInput{PersonID: &anna.ID, Type: "DEAT", AddCitations: []NewCitation{{SourceID: 999}}})
	validationField(t, err, "citations.sourceId")

	detail, err := s.GetSource(ctx, a, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.CitationCount != 2 || len(detail.Citations) != 2 {
		t.Errorf("source detail: %+v", detail)
	}

	person, err := s.GetPersonDetail(ctx, a, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(person.Citations) != 1 || len(person.Events[0].Citations) != 2 {
		t.Errorf("person %d citations, event %d citations", len(person.Citations), len(person.Events[0].Citations))
	}

	// Unlinking the person keeps the citation for the birth; unlinking the
	// last fact deletes it.
	if err := s.Unlink(ctx, a, c.ID, "person", anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCitation(ctx, a, c.ID); err != nil {
		t.Errorf("citation still supports the birth: %v", err)
	}
	if err := s.Unlink(ctx, a, c.ID, "event", birth.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCitation(ctx, a, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("unlinked citation: got %v", err)
	}

	// Deleting the person deletes the birth, and the trigger removes the
	// links; the birth's own citation lost its last link and goes too.
	if err := s.DeletePerson(ctx, a, anna.ID); err != nil {
		t.Fatal(err)
	}
	var cits int
	if err := s.DB.QueryRow(`SELECT count(*) FROM citations`).Scan(&cits); err != nil {
		t.Fatal(err)
	}
	if cits != 0 {
		t.Errorf("%d citations left without a fact", cits)
	}
	var links int
	if err := s.DB.QueryRow(`SELECT count(*) FROM citation_links`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Errorf("%d links survived their targets", links)
	}

	if err := s.DeleteRepository(ctx, a, repo.ID); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetSource(ctx, a, src.ID)
	if err != nil || after.Repository != nil {
		t.Errorf("source after repository delete: %+v, %v", after, err)
	}
}

func TestNameCitationsSurviveEdits(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	src, err := s.CreateSource(ctx, a, SourceInput{Title: "Heiratsregister"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Anna", AlternateNames: []AlternateNameInput{{Type: "married", Surname: "Weber"}}})
	if err != nil {
		t.Fatal(err)
	}
	nameID := p.AlternateNames[0].ID
	if _, err := s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID, Links: []LinkInput{{EntityType: "name", EntityID: nameID}}}); err != nil {
		t.Fatal(err)
	}

	p, err = s.UpdatePerson(ctx, a, p.ID, PersonInput{GivenNames: "Anna Maria", AlternateNames: []AlternateNameInput{
		{ID: &nameID, Type: "married", Surname: "Weber-Schulz"},
		{Type: "aka", Nickname: "Anni"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if p.AlternateNames[0].ID != nameID || len(p.AlternateNames[0].Citations) != 1 || p.AlternateNames[0].Surname != "Weber-Schulz" {
		t.Errorf("edited name lost its id or citation: %+v", p.AlternateNames[0])
	}

	// Dropping the name removes its citation link.
	if _, err := s.UpdatePerson(ctx, a, p.ID, PersonInput{GivenNames: "Anna Maria"}); err != nil {
		t.Fatal(err)
	}
	var links int
	if err := s.DB.QueryRow(`SELECT count(*) FROM citation_links WHERE entity_type = 'name'`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Errorf("link to a deleted name survived")
	}
}

func TestRelativeWithCitation(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	src, err := s.CreateSource(ctx, a, SourceInput{Title: "Taufregister"})
	if err != nil {
		t.Fatal(err)
	}
	paul := mustPerson(t, s, a, "Paul", "")
	res, err := s.AddRelative(ctx, a, paul.ID, RelativeInput{
		Relation: RelationParent, Person: &PersonInput{GivenNames: "Hans"},
		Citation: &NewCitation{SourceID: src.ID, Page: "1890/17"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Family.Citations) != 1 || res.Family.Citations[0].Page != "1890/17" {
		t.Errorf("the relationship's family should carry the citation: %+v", res.Family.Citations)
	}
}

func TestRemoveCitationsOnSave(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	src, err := s.CreateSource(ctx, a, SourceInput{Title: "Register"})
	if err != nil {
		t.Fatal(err)
	}
	p := mustPerson(t, s, a, "Anna", "")
	e, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "BIRT", AddCitations: []NewCitation{{SourceID: src.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	cid := e.Citations[0].CitationID
	e, err = s.UpdateEvent(ctx, a, e.ID, EventInput{PersonID: &p.ID, Type: "BIRT", RemoveCitations: []int64{cid}})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Citations) != 0 {
		t.Errorf("citation still attached: %+v", e.Citations)
	}
	_, err = s.UpdateEvent(ctx, a, e.ID, EventInput{PersonID: &p.ID, Type: "BIRT", RemoveCitations: []int64{cid}})
	validationField(t, err, "removeCitations")
}
