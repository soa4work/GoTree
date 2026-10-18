package store

import "testing"

// cousinMarriage is a tree where two cousins marry and one of them has a
// second partner:
//
//	R -> A -> X --+-- Y <- C <- B <- R
//	              |   |
//	              Z   +-- W (second partner), child Q
type cousinMarriage struct {
	r, a, b, c, x, y, z, w, q Person
	f1, f2                    int64
}

func buildCousinMarriage(t *testing.T, s *Store, a Actor) cousinMarriage {
	t.Helper()
	var m cousinMarriage
	m.r = mustPerson(t, s, a, "R", "Weber")
	m.a = newRelative(t, s, a, m.r.ID, RelationChild, "A").Person
	m.b = newRelative(t, s, a, m.r.ID, RelationChild, "B").Person
	m.x = newRelative(t, s, a, m.a.ID, RelationChild, "X").Person
	m.c = newRelative(t, s, a, m.b.ID, RelationChild, "C").Person
	// X and Y marry first, then Y is made C's child, so the family of X comes
	// before the family of C and Y is met as X's partner before C's child.
	m.y = mustPerson(t, s, a, "Y", "Weber")
	f1, err := s.AddRelative(ctx, a, m.x.ID, RelativeInput{Relation: RelationPartner, PersonID: &m.y.ID})
	if err != nil {
		t.Fatal(err)
	}
	m.f1 = f1.Family.ID
	if _, err := s.AddRelative(ctx, a, m.c.ID, RelativeInput{Relation: RelationChild, PersonID: &m.y.ID}); err != nil {
		t.Fatal(err)
	}
	m.z = newRelative(t, s, a, m.x.ID, RelationChild, "Z", func(in *RelativeInput) { in.FamilyID = &m.f1 }).Person
	wRes := newRelative(t, s, a, m.y.ID, RelationPartner, "W")
	m.w, m.f2 = wRes.Person, wRes.Family.ID
	if m.f2 == m.f1 {
		t.Fatal("W must be in a second family")
	}
	m.q = newRelative(t, s, a, m.y.ID, RelationChild, "Q", func(in *RelativeInput) { in.FamilyID = &m.f2 }).Person
	return m
}

func TestTreeCousinMarriage(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	m := buildCousinMarriage(t, s, a)

	g, err := s.Tree(ctx, a, m.r.ID, TreeOptions{Down: 6})
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]Person{"W": m.w, "Q": m.q, "Z": m.z, "Y": m.y} {
		if _, ok := g.Persons[p.ID]; !ok {
			t.Errorf("%s is missing from the descendants", name)
		}
	}
	var hasF2 bool
	for _, f := range g.Families {
		hasF2 = hasF2 || f.ID == m.f2
	}
	if !hasF2 {
		t.Error("the second marriage of the cousin is missing")
	}

	// Depth is still respected: Y is a child of C at generation 3 and is not
	// followed further with Down = 3.
	g, err = s.Tree(ctx, a, m.r.ID, TreeOptions{Down: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Persons[m.q.ID]; ok {
		t.Error("Q is beyond three generations")
	}
	if _, ok := g.Persons[m.y.ID]; !ok {
		t.Error("Y is within three generations")
	}
}

func TestShareDescendantsFollowsCousinMarriage(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	m := buildCousinMarriage(t, s, a)

	got, err := s.descendantsWithPartners(ctx, a, m.r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]Person{"W": m.w, "Q": m.q, "Z": m.z, "Y": m.y} {
		if !got[p.ID] {
			t.Errorf("%s is missing from the branch", name)
		}
	}

	sh := shareFor(t, s, a, ShareLinkInput{Label: "R's line", Scope: ShareScopeDescendants, RootPersonID: &m.r.ID, Privacy: SharePrivacyLivingNames})
	list, err := s.SharePersons(ctx, sh, "")
	if err != nil {
		t.Fatal(err)
	}
	visible := listIDs(list)
	if !visible[m.w.ID] || !visible[m.q.ID] {
		t.Errorf("share shows %+v", list.Items)
	}
}
