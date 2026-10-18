package store

import (
	"errors"
	"sort"
	"testing"
)

func ids(g TreeGraph) []string {
	var names []string
	for _, p := range g.Persons {
		names = append(names, p.GivenNames)
	}
	sort.Strings(names)
	return names
}

func TestTree(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)

	// Grandparents → Hans → Paul (+ sibling Lena) → Paul & Eva → Mia → Mia & Jon → Ben
	paul := mustPerson(t, s, a, "Paul", "")
	hans := newRelative(t, s, a, paul.ID, RelationParent, "Hans")
	newRelative(t, s, a, paul.ID, RelationSibling, "Lena")
	newRelative(t, s, a, hans.Person.ID, RelationParent, "Opa")
	newRelative(t, s, a, hans.Person.ID, RelationParent, "Oma")
	eva := newRelative(t, s, a, paul.ID, RelationPartner, "Eva")
	mia := newRelative(t, s, a, paul.ID, RelationChild, "Mia", func(in *RelativeInput) { in.FamilyID = &eva.Family.ID })
	newRelative(t, s, a, mia.Person.ID, RelationPartner, "Jon")
	newRelative(t, s, a, mia.Person.ID, RelationChild, "Ben")

	tree := func(opt TreeOptions) TreeGraph {
		t.Helper()
		g, err := s.Tree(ctx, a, paul.ID, opt)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	check := func(name string, g TreeGraph, want ...string) {
		t.Helper()
		sort.Strings(want)
		got := ids(g)
		if len(got) != len(want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: got %v, want %v", name, got, want)
				return
			}
		}
	}

	check("pedigree 1", tree(TreeOptions{Up: 1}), "Paul", "Hans")
	check("pedigree 2", tree(TreeOptions{Up: 2}), "Paul", "Hans", "Opa", "Oma")
	check("descendants 1", tree(TreeOptions{Down: 1}), "Paul", "Eva", "Mia")
	check("descendants 2", tree(TreeOptions{Down: 2}), "Paul", "Eva", "Mia", "Jon", "Ben")
	check("family group", tree(TreeOptions{Up: 1, Down: 1, Siblings: true}), "Paul", "Hans", "Lena", "Eva", "Mia")
	check("siblings only", tree(TreeOptions{Siblings: true}), "Paul", "Lena")

	// Families only refer to people in the graph.
	g := tree(TreeOptions{Up: 1})
	for _, f := range g.Families {
		for _, c := range f.Children {
			if _, ok := g.Persons[c.PersonID]; !ok {
				t.Errorf("family %d refers to child %d outside the graph", f.ID, c.PersonID)
			}
		}
	}

	if _, err := s.Tree(ctx, otherTree(t, s), paul.ID, TreeOptions{Up: 1}); !errors.Is(err, ErrNotFound) {
		t.Errorf("tree across trees: got %v", err)
	}
}

func TestTreeIsCapped(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	root := mustPerson(t, s, a, "Root", "")
	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &root.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Bulk insert children directly; the store would be too slow for this.
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := range MaxTreePersons + 10 {
		res, err := tx.Exec(`INSERT INTO persons (tree_id, given_names, created_at, updated_at) VALUES (?, ?, '', '')`, a.TreeID, "C")
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := tx.Exec(`INSERT INTO family_children (family_id, child_id, sort_order) VALUES (?, ?, ?)`, fam.ID, id, i); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	g, err := s.Tree(ctx, a, root.ID, TreeOptions{Down: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !g.Truncated || len(g.Persons) != MaxTreePersons {
		t.Errorf("truncated=%v persons=%d, want truncated at %d", g.Truncated, len(g.Persons), MaxTreePersons)
	}
}
