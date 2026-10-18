package store

import (
	"context"
	"database/sql"
	"sort"
)

// TreeChild is a child link inside a TreeFamily.
type TreeChild struct {
	PersonID         int64  `json:"personId"`
	RelationPartner1 string `json:"relationPartner1"`
	RelationPartner2 string `json:"relationPartner2"`
}

// TreeFamily is a family in a tree graph, referring to people by id.
type TreeFamily struct {
	ID         int64       `json:"id"`
	Partner1ID *int64      `json:"partner1Id"`
	Partner2ID *int64      `json:"partner2Id"`
	UnionType  string      `json:"unionType"`
	Children   []TreeChild `json:"children"`
}

// TreeGraph is the neighbourhood of a person as a flat graph; the client
// lays it out.
type TreeGraph struct {
	RootID   int64               `json:"rootId"`
	Persons  map[int64]PersonRef `json:"persons"`
	Families []TreeFamily        `json:"families"`
	// Truncated is set when MaxTreePersons was reached before the requested
	// generations were complete.
	Truncated bool `json:"truncated"`
}

// MaxTreePersons bounds a tree response so huge imports stay usable.
const MaxTreePersons = 2000

// MaxTreeGenerations bounds the generations in each direction.
const MaxTreeGenerations = 12

// TreeOptions selects what Tree collects around the root.
type TreeOptions struct {
	// Up is the number of ancestor generations.
	Up int
	// Down is the number of descendant generations; partners of
	// descendants are included.
	Down int
	// Siblings includes the root's siblings (the other children of the
	// root's parent families).
	Siblings bool
}

// Tree collects ancestors and descendants of rootID. Ancestors are followed
// through every parent family (birth, adoptive, …); the client decides
// which to draw.
func (s *Store) Tree(ctx context.Context, a Actor, rootID int64, opt TreeOptions) (TreeGraph, error) {
	opt.Up = clamp(opt.Up, 0, MaxTreeGenerations)
	opt.Down = clamp(opt.Down, 0, MaxTreeGenerations)
	if err := requireInTree(ctx, s.DB, "persons", rootID, a.TreeID); err != nil {
		return TreeGraph{}, err
	}

	g := TreeGraph{RootID: rootID}
	people := map[int64]bool{rootID: true}
	families := map[int64]TreeFamily{}
	full := func() bool { return len(people) >= MaxTreePersons }

	// Ancestors: the parent families of each generation, then their partners.
	frontier := []int64{rootID}
	for gen := 0; gen < opt.Up && len(frontier) > 0; gen++ {
		fams, err := s.treeFamilies(ctx, a, parentFamiliesOf, frontier)
		if err != nil {
			return TreeGraph{}, err
		}
		var next []int64
		for _, f := range fams {
			families[f.ID] = f
			for _, p := range []*int64{f.Partner1ID, f.Partner2ID} {
				if p != nil && !people[*p] {
					if full() {
						g.Truncated = true
						continue
					}
					people[*p] = true
					next = append(next, *p)
				}
			}
			if gen == 0 && opt.Siblings {
				for _, c := range f.Children {
					if !people[c.PersonID] && !full() {
						people[c.PersonID] = true
					}
				}
			}
		}
		frontier = next
	}
	if opt.Up == 0 && opt.Siblings {
		fams, err := s.treeFamilies(ctx, a, parentFamiliesOf, []int64{rootID})
		if err != nil {
			return TreeGraph{}, err
		}
		for _, f := range fams {
			families[f.ID] = f
			for _, c := range f.Children {
				people[c.PersonID] = true
			}
		}
	}

	// Descendants: the partner families of each generation, their partners,
	// then their children. people also holds ancestors, siblings and
	// partners, so it cannot tell whom the walk has followed: expanded does.
	// A partner who is also a descendant (cousins who married) is expanded
	// when reached as a child.
	expanded := map[int64]bool{rootID: true}
	frontier = []int64{rootID}
	for gen := 0; gen < opt.Down && len(frontier) > 0; gen++ {
		fams, err := s.treeFamilies(ctx, a, partnerFamiliesOf, frontier)
		if err != nil {
			return TreeGraph{}, err
		}
		var next []int64
		for _, f := range fams {
			families[f.ID] = f
			for _, p := range []*int64{f.Partner1ID, f.Partner2ID} {
				if p != nil && !people[*p] {
					if full() {
						g.Truncated = true
						continue
					}
					people[*p] = true
				}
			}
			for _, c := range f.Children {
				if !people[c.PersonID] {
					if full() {
						g.Truncated = true
						continue
					}
					people[c.PersonID] = true
				}
				if !expanded[c.PersonID] {
					expanded[c.PersonID] = true
					next = append(next, c.PersonID)
				}
			}
		}
		frontier = next
	}

	ids := make([]int64, 0, len(people))
	for id := range people {
		ids = append(ids, id)
	}
	refs, err := s.personRefs(ctx, s.DB, a, ids)
	if err != nil {
		return TreeGraph{}, err
	}
	g.Persons = refs

	g.Families = make([]TreeFamily, 0, len(families))
	for _, f := range families {
		// Children outside the collected set are dropped so the graph only
		// refers to people it contains.
		kept := f.Children[:0:0]
		for _, c := range f.Children {
			if people[c.PersonID] {
				kept = append(kept, c)
			}
		}
		f.Children = kept
		g.Families = append(g.Families, f)
	}
	sort.Slice(g.Families, func(i, j int) bool { return g.Families[i].ID < g.Families[j].ID })
	return g, nil
}

type familyLink int

const (
	parentFamiliesOf familyLink = iota
	partnerFamiliesOf
)

// treeFamilies loads the parent or partner families of ids, with children
// ordered by birth date.
func (s *Store) treeFamilies(ctx context.Context, a Actor, link familyLink, ids []int64) ([]TreeFamily, error) {
	ph := placeholders(len(ids))
	args := []any{a.TreeID}
	var cond string
	switch link {
	case parentFamiliesOf:
		cond = `f.id IN (SELECT family_id FROM family_children WHERE child_id IN (` + ph + `))`
		args = append(args, int64Args(ids)...)
	case partnerFamiliesOf:
		cond = `(f.partner1_id IN (` + ph + `) OR f.partner2_id IN (` + ph + `))`
		args = append(append(args, int64Args(ids)...), int64Args(ids)...)
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.id, f.partner1_id, f.partner2_id, f.union_type
		FROM families f WHERE f.tree_id = ? AND `+cond+`
		ORDER BY (SELECT min(e.date_sort) FROM events e WHERE e.family_id = f.id AND e.type = 'MARR') IS NULL,
			(SELECT min(e.date_sort) FROM events e WHERE e.family_id = f.id AND e.type = 'MARR'), f.id`, args...)
	if err != nil {
		return nil, err
	}
	var fams []TreeFamily
	index := map[int64]int{}
	for rows.Next() {
		var f TreeFamily
		var p1, p2 sql.NullInt64
		if err := rows.Scan(&f.ID, &p1, &p2, &f.UnionType); err != nil {
			rows.Close()
			return nil, err
		}
		f.Partner1ID, f.Partner2ID = ptrID(p1), ptrID(p2)
		f.Children = []TreeChild{}
		index[f.ID] = len(fams)
		fams = append(fams, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(fams) == 0 {
		return fams, err
	}

	famIDs := make([]int64, len(fams))
	for i, f := range fams {
		famIDs[i] = f.ID
	}
	crows, err := s.DB.QueryContext(ctx, `
		SELECT c.family_id, c.child_id, c.relation_partner1, c.relation_partner2
		FROM family_children c
		LEFT JOIN events b ON b.person_id = c.child_id AND b.type = 'BIRT'
		WHERE c.family_id IN (`+placeholders(len(famIDs))+`)
		GROUP BY c.family_id, c.child_id
		ORDER BY c.sort_order, min(b.date_sort) IS NULL, min(b.date_sort), c.child_id`, int64Args(famIDs)...)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var famID int64
		var c TreeChild
		if err := crows.Scan(&famID, &c.PersonID, &c.RelationPartner1, &c.RelationPartner2); err != nil {
			return nil, err
		}
		f := &fams[index[famID]]
		f.Children = append(f.Children, c)
	}
	return fams, crows.Err()
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
