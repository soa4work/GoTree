package store

import (
	"testing"
)

func TestChecks(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	mother := mustPerson(t, s, a, "Anna", "Weber")
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &mother.ID, Type: "BIRT", Date: "1800"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &mother.ID, Type: "DEAT", Date: "1790"}); err != nil {
		t.Fatal(err)
	}
	disproven, err := s.CreateEvent(ctx, a, EventInput{PersonID: &mother.ID, Type: "RESI", Date: "1700", Status: StatusDisproven, StatusReason: "other Anna"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.AddRelative(ctx, a, mother.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Paul"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &child.Person.ID, Type: "BIRT", Date: "1805"}); err != nil {
		t.Fatal(err)
	}
	other := mustPerson(t, s, a, "Otto", "")
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &other.ID, Type: "OCCU", Date: "sometime"}); err != nil {
		t.Fatal(err)
	}

	rep, err := s.Checks(ctx, a, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, f := range rep.Findings {
		got[f.Rule] = f.PersonID
		if f.EventID == disproven.ID {
			t.Error("disproven events are not checked")
		}
	}
	if got["birth_after_death"] != mother.ID || got["parent_too_young"] != child.Person.ID || got["invalid_date"] != other.ID {
		t.Errorf("findings: %+v", rep.Findings)
	}
	if rep.Persons[mother.ID].GivenNames != "Anna" {
		t.Errorf("persons: %+v", rep.Persons)
	}

	mine, err := s.Checks(ctx, a, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine.Findings) != 1 || mine.Findings[0].Rule != "invalid_date" {
		t.Errorf("filtered: %+v", mine.Findings)
	}
	if _, err := s.Checks(ctx, a, 9999); err != ErrNotFound {
		t.Errorf("unknown person: %v", err)
	}
}

func TestNormalizeDates(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Anna", "Weber")
	mk := func(date string) Event {
		e, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "RESI", Date: date})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	german := mk("12.3.1850") // read by Parse, so never proposed
	// Saving tidies loose dates itself, so legacy loose text is written directly.
	loose := mk("1851")
	if _, err := s.DB.Exec(`UPDATE events SET date_raw = 'March 12, 1851', date = '', date_sort = NULL, date_sort_end = NULL WHERE id = ?`, loose.ID); err != nil {
		t.Fatal(err)
	}
	mk("ABT 1852")
	mk("whenever")
	edited := mk("abt 1853")

	prop, err := s.ProposeDates(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Items) != 1 || prop.Unreadable != 1 {
		t.Fatalf("proposals: %+v", prop)
	}
	byID := map[int64]DateProposal{}
	for _, it := range prop.Items {
		byID[it.EventID] = it
	}
	if _, ok := byID[german.ID]; ok {
		t.Errorf("a date Parse reads was proposed: %+v", byID[german.ID])
	}
	if l := byID[loose.ID]; l.Proposed != "12 MAR 1851" || l.WasValid || l.PersonIDs[0] != p.ID {
		t.Errorf("loose: %+v", l)
	}
	if prop.Persons[p.ID].Surname != "Weber" {
		t.Errorf("persons: %+v", prop.Persons)
	}

	if _, err := s.UpdateEvent(ctx, a, edited.ID, EventInput{PersonID: &p.ID, Type: "RESI", Date: "1853"}); err != nil {
		t.Fatal(err)
	}
	res, err := s.ApplyDates(ctx, a, []DateChange{
		{EventID: loose.ID, Raw: "March 12, 1851", Date: "12 MAR 1851"},
		{EventID: edited.ID, Raw: "abt 1853", Date: "ABT 1853"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || res.Skipped != 1 {
		t.Errorf("result: %+v", res)
	}
	e, _ := s.GetEvent(ctx, a, loose.ID)
	if e.Date.Raw != "12 MAR 1851" || !e.Date.Valid || e.Date.SortKey == nil || *e.Date.SortKey != 18510312 {
		t.Errorf("loose after: %+v", e.Date)
	}
	var logged int
	_ = s.DB.QueryRow(`SELECT count(*) FROM change_log WHERE entity_type = 'event' AND action = 'update' AND after_json = '{"date":"12 MAR 1851"}'`).Scan(&logged)
	if logged != 1 {
		t.Error("the change is logged")
	}

	if _, err := s.ApplyDates(ctx, a, []DateChange{{EventID: loose.ID, Raw: "12 MAR 1851", Date: "12.3.1851"}}); err == nil {
		t.Error("non-canonical target accepted")
	}
}

func TestRelationship(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Weber")
	sib, err := s.AddRelative(ctx, a, anna.ID, RelativeInput{Relation: RelationSibling, Person: &PersonInput{GivenNames: "Berta"}})
	if err != nil {
		t.Fatal(err)
	}
	niece, err := s.AddRelative(ctx, a, sib.Person.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Clara"}})
	if err != nil {
		t.Fatal(err)
	}
	// The sibling was added without parents; their placeholder family is
	// the unknown common ancestor, with a negative id in the path.
	rep, err := s.Relationship(ctx, a, anna.ID, niece.Person.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Kind != "blood" || rep.Kinship.Up != 1 || rep.Kinship.Down != 2 || len(rep.Kinship.Path) != 4 || rep.Kinship.Path[1] >= 0 {
		t.Errorf("niece: %+v %+v", rep, rep.Kinship)
	}
	if rep.Persons[niece.Person.ID].GivenNames != "Clara" {
		t.Errorf("persons: %+v", rep.Persons)
	}
	if _, err := s.Relationship(ctx, a, anna.ID, 9999); err != ErrNotFound {
		t.Errorf("unknown person: %v", err)
	}
}
