package store

import (
	"testing"
)

func newRelative(t *testing.T, s *Store, a Actor, anchor int64, relation, given string, extra ...func(*RelativeInput)) RelativeResult {
	t.Helper()
	in := RelativeInput{Relation: relation, Person: &PersonInput{GivenNames: given, Surname: "Weber"}}
	for _, f := range extra {
		f(&in)
	}
	res, err := s.AddRelative(ctx, a, anchor, in)
	if err != nil {
		t.Fatalf("add %s %s: %v", relation, given, err)
	}
	return res
}

func TestAddRelativesInAnyOrder(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	paul := mustPerson(t, s, a, "Paul", "Weber")

	// A sibling first: Paul and Lena share a family with unknown parents.
	lena := newRelative(t, s, a, paul.ID, RelationSibling, "Lena")
	if lena.Family.Partner1 != nil || lena.Family.Partner2 != nil || len(lena.Family.Children) != 2 {
		t.Fatalf("sibling family: %+v", lena.Family)
	}

	// Then the father: he fills the first free slot of that same family,
	// so he is Lena's father too.
	hans := newRelative(t, s, a, paul.ID, RelationParent, "Hans")
	if hans.Family.ID != lena.Family.ID || hans.Family.Partner1 == nil || hans.Family.Partner1.ID != hans.Person.ID {
		t.Fatalf("parent family: %+v", hans.Family)
	}
	// An adoptive mother: the relation is recorded on her side only.
	maria := newRelative(t, s, a, paul.ID, RelationParent, "Maria", func(in *RelativeInput) { in.ChildRelation = "adopted" })
	for _, c := range maria.Family.Children {
		if c.Person.ID == paul.ID && (c.RelationPartner1 != "birth" || c.RelationPartner2 != "adopted") {
			t.Errorf("Paul's relations: %+v", c)
		}
	}
	// A third parent in the same family is refused.
	_, err := s.AddRelative(ctx, a, paul.ID, RelativeInput{Relation: RelationParent, Person: &PersonInput{GivenNames: "X"}})
	validationField(t, err, "relation")

	// A child before any partner: the other parent is unknown.
	tom := newRelative(t, s, a, paul.ID, RelationChild, "Tom")
	if tom.Family.Partner1 == nil || tom.Family.Partner1.ID != paul.ID || tom.Family.Partner2 != nil {
		t.Fatalf("child family: %+v", tom.Family)
	}

	// A married partner gets a marriage event to fill in.
	eva := newRelative(t, s, a, paul.ID, RelationPartner, "Eva")
	if eva.Family.UnionType != "married" || len(eva.Family.Events) != 1 || eva.Family.Events[0].Type != "MARR" {
		t.Fatalf("partner family: %+v", eva.Family)
	}
	unmarried := newRelative(t, s, a, paul.ID, RelationPartner, "Ida", func(in *RelativeInput) { in.UnionType = "partners" })
	if len(unmarried.Family.Events) != 0 {
		t.Errorf("unmarried partners must not get a marriage event: %+v", unmarried.Family.Events)
	}

	// Now Paul has three partner families, so a new child needs a choice.
	_, err = s.AddRelative(ctx, a, paul.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Y"}})
	validationField(t, err, "familyId")
	mia := newRelative(t, s, a, paul.ID, RelationChild, "Mia", func(in *RelativeInput) { in.FamilyID = &eva.Family.ID })
	if mia.Family.ID != eva.Family.ID {
		t.Errorf("child went into family %d, want %d", mia.Family.ID, eva.Family.ID)
	}
	_, err = s.AddRelative(ctx, a, paul.ID, RelativeInput{Relation: RelationChild, FamilyID: &lena.Family.ID, Person: &PersonInput{GivenNames: "Z"}})
	validationField(t, err, "familyId")

	// Linking an existing person, and the ancestry guard on that path.
	_, err = s.AddRelative(ctx, a, tom.Person.ID, RelativeInput{Relation: RelationChild, PersonID: &hans.Person.ID})
	validationField(t, err, "childId")
	_, err = s.AddRelative(ctx, a, paul.ID, RelativeInput{Relation: RelationChild, PersonID: &paul.ID})
	validationField(t, err, "personId")

	detail, err := s.GetPersonDetail(ctx, a, paul.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ParentFamilies) != 1 || len(detail.PartnerFamilies) != 3 {
		t.Errorf("Paul: %d parent families, %d partner families", len(detail.ParentFamilies), len(detail.PartnerFamilies))
	}
}

func TestAddRelativeIsAtomic(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	paul := mustPerson(t, s, a, "Paul", "")
	before, err := s.ListPersons(ctx, a, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The new person is valid but the family choice is not, so nothing
	// may be left behind.
	_, err = s.AddRelative(ctx, a, paul.ID, RelativeInput{Relation: RelationChild, FamilyID: ptr(int64(999)), Person: &PersonInput{GivenNames: "Ghost"}})
	validationField(t, err, "familyId")
	after, err := s.ListPersons(ctx, a, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if after.Total != before.Total {
		t.Errorf("a failed add left %d new people behind", after.Total-before.Total)
	}
}
