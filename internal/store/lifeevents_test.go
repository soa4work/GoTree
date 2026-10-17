package store

import "testing"

func TestCreatePersonWithBirthAndDeath(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	place, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Leipzig"})
	if err != nil {
		t.Fatal(err)
	}

	p, err := s.CreatePerson(ctx, a, PersonInput{
		GivenNames: "Paul",
		Birth:      &LifeEventInput{Date: "12.3.1850", PlaceID: &place.ID},
		Death:      &LifeEventInput{Date: "1920"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.GetPersonDetail(ctx, a, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Event{}
	for _, e := range d.Events {
		got[e.Type] = e
	}
	if len(d.Events) != 2 || got["BIRT"].Date.Normalized != "12 MAR 1850" || got["BIRT"].Place == nil || got["DEAT"].Date.Normalized != "1920" {
		t.Errorf("events: %+v", d.Events)
	}
	if p.Living {
		t.Errorf("a person with a death is not living: %+v", p)
	}

	// Empty birth/death create nothing.
	q, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Eva", Birth: &LifeEventInput{}})
	if err != nil {
		t.Fatal(err)
	}
	if d, _ = s.GetPersonDetail(ctx, a, q.ID); len(d.Events) != 0 {
		t.Errorf("events: %+v", d.Events)
	}

	// An invalid date, a missing place or a death for a living person
	// rejects everything.
	_, err = s.CreatePerson(ctx, a, PersonInput{GivenNames: "Bad", Birth: &LifeEventInput{Date: "31 FEB 1850"}})
	validationField(t, err, "birth.date")
	_, err = s.CreatePerson(ctx, a, PersonInput{GivenNames: "Bad", Death: &LifeEventInput{PlaceID: ptr(int64(9999))}})
	validationField(t, err, "placeId")
	_, err = s.CreatePerson(ctx, a, PersonInput{GivenNames: "Bad", IsLiving: ptr(true), Death: &LifeEventInput{Date: "1900"}})
	validationField(t, err, "death")
	list, err := s.ListPersons(ctx, a, "Bad", 10, 0)
	if err != nil || list.Total != 0 {
		t.Errorf("rejected person was saved: %+v %v", list, err)
	}
}

func TestAddRelativeWithBirth(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	paul := mustPerson(t, s, a, "Paul", "Weber")

	res := newRelative(t, s, a, paul.ID, RelationParent, "Hans", func(in *RelativeInput) {
		in.Person.Birth = &LifeEventInput{Date: "1850"}
	})
	d, err := s.GetPersonDetail(ctx, a, res.Person.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Events) != 1 || d.Events[0].Type != "BIRT" || d.Events[0].Date.Raw != "1850" {
		t.Errorf("events: %+v", d.Events)
	}

	// A bad date creates no relative at all.
	_, err = s.AddRelative(ctx, a, paul.ID, RelativeInput{Relation: RelationSibling,
		Person: &PersonInput{GivenNames: "Lena", Birth: &LifeEventInput{Date: "nonsense"}}})
	validationField(t, err, "birth.date")
	if list, _ := s.ListPersons(ctx, a, "Lena", 10, 0); list.Total != 0 {
		t.Errorf("rolled-back relative exists: %+v", list)
	}
}
