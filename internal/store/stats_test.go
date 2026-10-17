package store

import "testing"

func TestStats(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	ev := func(p Person, typ, date string) {
		t.Helper()
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: typ, Date: date}); err != nil {
			t.Fatal(err)
		}
	}
	hans, _ := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Johann Georg", Surname: "Weber", Sex: "M"})
	ev(hans, "BIRT", "10 MAR 1850")
	ev(hans, "DEAT", "9 MAR 1920") // a day before his 70th birthday
	anna, _ := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Anna", Surname: "Weber", Sex: "F"})
	ev(anna, "BIRT", "1855")
	ev(anna, "DEAT", "1935")
	erna, _ := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Erna", Surname: "Schulz", Sex: "F"})
	ev(erna, "BIRT", "1858")
	ev(erna, "DEAT", "BEF 1900")

	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &hans.ID, Partner2ID: &anna.ID, UnionType: "married"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam.ID, Type: "MARR", Date: "1876"}); err != nil {
		t.Fatal(err)
	}
	fam2, _ := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &hans.ID, Partner2ID: &erna.ID, UnionType: "married"})
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam2.ID, Type: "MARR", Date: "1890"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Paul", "Otto", "Anna"} {
		if _, err := s.AddRelative(ctx, a, hans.ID, RelativeInput{Relation: RelationChild, FamilyID: &fam.ID, Person: &PersonInput{GivenNames: name, Surname: "Weber"}}); err != nil {
			t.Fatal(err)
		}
	}

	st, err := s.Stats(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if st.Persons != 6 || st.WithLifespan != 2 || len(st.Lifespans) != 1 {
		t.Fatalf("stats: %+v", st)
	}
	if l := st.Lifespans[0]; l.Decade != 1850 || l.Min != 69 || l.Max != 80 || l.Average != 74.5 {
		t.Errorf("lifespans: %+v", l)
	}
	if len(st.Marriages) != 1 {
		t.Fatalf("marriages: %+v", st.Marriages)
	}
	m := st.Marriages[0]
	// Hans's second marriage counts as a later one; both wives' as first.
	if m.FirstMen.Count != 1 || m.FirstWomen.Count != 2 || m.Later.Count != 1 {
		t.Errorf("marriage ages: %+v", m)
	}
	if st.Families != 2 || st.ChildrenHistogram[3] != 1 || st.ChildrenHistogram[0] != 1 || st.ChildrenAverage != 1.5 {
		t.Errorf("children: %v avg %v families %d", st.ChildrenHistogram, st.ChildrenAverage, st.Families)
	}
	if st.Surnames[0] != (NameCount{"Weber", 5}) || st.GivenNames[0] != (NameCount{"Anna", 2}) {
		t.Errorf("names: %+v %+v", st.Surnames, st.GivenNames)
	}
	if len(st.GivenNameTrends) != 1 || st.GivenNameTrends[0].Men[0].Name != "Johann" {
		t.Errorf("trends: %+v", st.GivenNameTrends)
	}
}

func TestMapData(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	saxony, _ := s.CreatePlace(ctx, a, PlaceInput{Name: "Sachsen", Lat: ptr(51.0), Lng: ptr(13.3)})
	village, _ := s.CreatePlace(ctx, a, PlaceInput{Name: "Kleindorf", ParentID: &saxony.ID})
	leipzig, _ := s.CreatePlace(ctx, a, PlaceInput{Name: "Leipzig", ParentID: &saxony.ID, Lat: ptr(51.34), Lng: ptr(12.37)})
	nowhere, _ := s.CreatePlace(ctx, a, PlaceInput{Name: "Atlantis"})

	hans := mustPerson(t, s, a, "Hans", "Weber")
	for _, e := range []EventInput{
		{PersonID: &hans.ID, Type: "BIRT", Date: "1850", PlaceID: &village.ID},
		{PersonID: &hans.ID, Type: "RESI", Date: "1880", PlaceID: &leipzig.ID},
		{PersonID: &hans.ID, Type: "RESI", Date: "1890", PlaceID: &nowhere.ID},
		{PersonID: &hans.ID, Type: "DEAT", Date: "1920"},
	} {
		if _, err := s.CreateEvent(ctx, a, e); err != nil {
			t.Fatal(err)
		}
	}
	child, _ := s.AddRelative(ctx, a, hans.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Paul"}})
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &child.Person.ID, Type: "BIRT", Date: "1885", PlaceID: &leipzig.ID}); err != nil {
		t.Fatal(err)
	}

	d, err := s.MapData(ctx, a, MapScopeAll, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tracks) != 2 || d.Unmapped != 1 || d.Tracks[0].PersonID != hans.ID || len(d.Tracks[0].Points) != 2 || d.Tracks[0].Death != 19200000 {
		t.Fatalf("map: %+v", d)
	}
	if p := d.Places[village.ID]; !p.Approximate || p.Lat != 51.0 || p.Name != "Kleindorf, Sachsen" {
		t.Errorf("village takes Saxony's coordinates: %+v", p)
	}
	if d.Persons[child.Person.ID].GivenNames != "Paul" {
		t.Errorf("persons: %+v", d.Persons)
	}

	// Only Atlantis lacks coordinates for good; Kleindorf inherits Saxony's.
	um, err := s.UnmappedPlaces(ctx, a)
	if err != nil || len(um) != 1 || um[0].ID != nowhere.ID || um[0].Events != 1 || um[0].FullName != "Atlantis" {
		t.Errorf("unmapped places: %+v %v", um, err)
	}

	anc, err := s.MapData(ctx, a, MapScopeAncestors, child.Person.ID)
	if err != nil || len(anc.Tracks) != 2 {
		t.Errorf("ancestors of Paul include Paul and Hans: %+v %v", anc.Tracks, err)
	}
	desc, err := s.MapData(ctx, a, MapScopeDescendants, child.Person.ID)
	if err != nil || len(desc.Tracks) != 1 {
		t.Errorf("descendants of Paul: %+v %v", desc.Tracks, err)
	}
}
