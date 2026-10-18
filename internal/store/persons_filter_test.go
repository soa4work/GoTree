package store

import (
	"reflect"
	"testing"
)

type filterFixture struct {
	s        *Store
	a        Actor
	dead1900 int64
	anna1990 int64
	noEvents int64
	undated  int64
	chr1850  int64
}

func newFilterFixture(t *testing.T) filterFixture {
	t.Helper()
	s := newStore(t)
	a := setup(t, s)
	f := filterFixture{s: s, a: a}
	mk := func(in PersonInput) int64 {
		p, err := s.CreatePerson(ctx, a, in)
		if err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	f.dead1900 = mk(PersonInput{GivenNames: "Karl", Surname: "Weber", Sex: "M",
		Birth: &LifeEventInput{Date: "1900"}, Death: &LifeEventInput{Date: "1970"}})
	f.anna1990 = mk(PersonInput{GivenNames: "Anna", Surname: "Weber", Sex: "F", Birth: &LifeEventInput{Date: "1990"}})
	f.noEvents = mk(PersonInput{GivenNames: "Greta", Surname: "Weber", Sex: "U", IsLiving: ptr(false)})
	f.undated = mk(PersonInput{GivenNames: "Hans", Surname: "Weber", Sex: "X"})
	f.chr1850 = mk(PersonInput{GivenNames: "Otto", Surname: "Weber", Sex: "M"})
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &f.chr1850, Type: "CHR", Date: "1850"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f filterFixture) ids(t *testing.T, q string, flt PersonFilter, limit, offset int) []int64 {
	t.Helper()
	list, err := f.s.ListPersonsFiltered(ctx, f.a, q, flt, limit, offset)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, p := range list.Items {
		ids = append(ids, p.ID)
	}
	return ids
}

func TestPersonFilterSortByBirth(t *testing.T) {
	f := newFilterFixture(t)
	got := f.ids(t, "", PersonFilter{Sort: "birth"}, 50, 0)
	if len(got) != 5 || !reflect.DeepEqual(got[:3], []int64{f.chr1850, f.dead1900, f.anna1990}) {
		t.Errorf("birth: %v", got)
	}
	got = f.ids(t, "", PersonFilter{Sort: "-birth"}, 50, 0)
	if len(got) != 5 || !reflect.DeepEqual(got[:3], []int64{f.anna1990, f.dead1900, f.chr1850}) {
		t.Errorf("-birth: %v", got)
	}
}

func TestPersonFilterSexAndTotal(t *testing.T) {
	f := newFilterFixture(t)
	got := f.ids(t, "", PersonFilter{Sex: "F"}, 50, 0)
	if !reflect.DeepEqual(got, []int64{f.anna1990}) {
		t.Errorf("sex F: %v", got)
	}
	list, err := f.s.ListPersonsFiltered(ctx, f.a, "", PersonFilter{Sex: "M"}, 1, 0)
	if err != nil || list.Total != 2 || len(list.Items) != 1 {
		t.Errorf("total: %+v %v", list, err)
	}
	if got := f.ids(t, "anna", PersonFilter{Sex: "F"}, 50, 0); !reflect.DeepEqual(got, []int64{f.anna1990}) {
		t.Errorf("query+sex: %v", got)
	}
	if got := f.ids(t, "anna", PersonFilter{Sex: "M"}, 50, 0); len(got) != 0 {
		t.Errorf("query+other sex: %v", got)
	}
}

func TestPersonFilterPagingIsStable(t *testing.T) {
	f := newFilterFixture(t)
	all := f.ids(t, "", PersonFilter{Sort: "updated"}, 50, 0)
	var paged []int64
	for off := 0; off < len(all); off += 2 {
		paged = append(paged, f.ids(t, "", PersonFilter{Sort: "updated"}, 2, off)...)
	}
	if !reflect.DeepEqual(all, paged) || len(all) != 5 {
		t.Errorf("all %v paged %v", all, paged)
	}
}

// The SQL living rule must agree with the Go rule in personRefs.
func TestPersonFilterLivingMatchesRefs(t *testing.T) {
	f := newFilterFixture(t)
	// Edge cases of the rule: a stated flag beats a death record, and a
	// disproven death does not count.
	ida, err := f.s.CreatePerson(ctx, f.a, PersonInput{GivenNames: "Ida", Surname: "Weber", IsLiving: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateEvent(ctx, f.a, EventInput{PersonID: &ida.ID, Type: "BURI", Date: "1960"}); err != nil {
		t.Fatal(err)
	}
	old, err := f.s.CreatePerson(ctx, f.a, PersonInput{GivenNames: "Emil", Surname: "Weber", Birth: &LifeEventInput{Date: "1980"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateEvent(ctx, f.a, EventInput{PersonID: &old.ID, Type: "DEAT", Date: "2000", Status: "disproven",
		StatusReason: "wrong person"}); err != nil {
		t.Fatal(err)
	}
	all, err := f.s.ListPersons(ctx, f.a, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, living := range []bool{true, false} {
		want := map[int64]bool{}
		for _, p := range all.Items {
			if p.Living == living {
				want[p.ID] = true
			}
		}
		got := map[int64]bool{}
		for _, id := range f.ids(t, "", PersonFilter{Living: &living}, 50, 0) {
			got[id] = true
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("living=%v: got %v want %v", living, got, want)
		}
	}
}

func TestPersonFilterZeroEqualsListPersons(t *testing.T) {
	f := newFilterFixture(t)
	for _, q := range []string{"", "weber"} {
		a, err := f.s.ListPersons(ctx, f.a, q, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := f.s.ListPersonsFiltered(ctx, f.a, q, PersonFilter{}, 50, 0)
		if err != nil || !reflect.DeepEqual(a, b) {
			t.Errorf("q=%q: %+v vs %+v (%v)", q, a, b, err)
		}
	}
	// An explicit name sort matches the default list order.
	def := f.ids(t, "", PersonFilter{}, 50, 0)
	if got := f.ids(t, "", PersonFilter{Sort: "name"}, 50, 0); !reflect.DeepEqual(got, def) {
		t.Errorf("name: %v vs %v", got, def)
	}
}
