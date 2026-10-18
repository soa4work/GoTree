// Package graph holds a family tree as adjacency lists in memory and finds
// how two people are related.
//
// Kinship is found through the closest common ancestors: going up from both
// people at once, the first ancestors reached from both sides decide the
// relationship, named by how many generations each side climbed.
package graph

import (
	"slices"
	"sort"
)

// Family is a couple and its children as loaded from the database.
type Family struct {
	ID                 int64
	Partner1, Partner2 int64
	Children           []Child
}

// Child is a child link; Blood1 and Blood2 say whether the child is a
// birth child of partner 1 and partner 2 (not adopted, foster, step…).
type Child struct {
	PersonID       int64
	Blood1, Blood2 bool
}

type parentLink struct {
	parent, family int64
	blood          bool
}

// Graph is a family tree's people and their links.
type Graph struct {
	parents  map[int64][]parentLink
	children map[int64][]int64
	spouses  map[int64][]int64
	persons  map[int64]bool
}

// New builds a Graph. Persons lists every person, including those without
// any family.
func New(persons []int64, families []Family) *Graph {
	g := &Graph{
		parents:  map[int64][]parentLink{},
		children: map[int64][]int64{},
		spouses:  map[int64][]int64{},
		persons:  map[int64]bool{},
	}
	for _, id := range persons {
		g.persons[id] = true
	}
	for _, f := range families {
		if f.Partner1 != 0 && f.Partner2 != 0 {
			g.spouses[f.Partner1] = append(g.spouses[f.Partner1], f.Partner2)
			g.spouses[f.Partner2] = append(g.spouses[f.Partner2], f.Partner1)
		}
		for _, c := range f.Children {
			if f.Partner1 == 0 && f.Partner2 == 0 {
				g.link(c.PersonID, UnknownParents(f.ID), f.ID, c.Blood1 || c.Blood2)
				continue
			}
			for _, p := range []struct {
				id    int64
				blood bool
			}{{f.Partner1, c.Blood1}, {f.Partner2, c.Blood2}} {
				if p.id == 0 {
					continue
				}
				g.link(c.PersonID, p.id, f.ID, p.blood)
			}
		}
	}
	return g
}

func (g *Graph) link(child, parent, family int64, blood bool) {
	g.parents[child] = append(g.parents[child], parentLink{parent, family, blood})
	g.children[parent] = append(g.children[parent], child)
}

// UnknownParents is the id that stands for the unknown parents of a family
// without partners, so that siblings whose parents are not recorded are
// still related. It is negative and never a person id.
func UnknownParents(familyID int64) int64 { return -familyID }

// Has reports whether id is a person of the graph.
func (g *Graph) Has(id int64) bool { return g.persons[id] }

// Kinship is a relationship through common ancestors.
type Kinship struct {
	// Up is the number of generations from A to the common ancestors,
	// Down from there to B. Up 0 means B descends from A, Down 0 that B is
	// an ancestor of A, 1/1 are siblings, 2/2 first cousins.
	Up   int `json:"up"`
	Down int `json:"down"`
	// Half is set when A and B descend from different partners of the
	// common ancestor, as half-siblings do.
	Half bool `json:"half"`
	// Adoptive is set when the path needs an adoptive, foster or step
	// parent link.
	Adoptive  bool    `json:"adoptive"`
	Ancestors []int64 `json:"ancestors"`
	// Path runs from A up to a common ancestor and down to B.
	Path []int64 `json:"path"`
}

// Relationship kinds.
const (
	KindSelf  = "self"
	KindBlood = "blood"
	// KindSpouse: B is A's spouse or partner.
	KindSpouse = "spouse"
	// KindSpouseOfRelative: B is the spouse of A's relative (sister-in-law,
	// son-in-law, step-parent).
	KindSpouseOfRelative = "spouse_of_relative"
	// KindRelativeOfSpouse: B is a relative of A's spouse (mother-in-law,
	// step-child).
	KindRelativeOfSpouse = "relative_of_spouse"
	// KindSpouseBloodSpouse: B is the spouse of a relative of A's spouse
	// (husbands of sisters, wives of brothers). Via is A's spouse, Via2 the
	// relative; Kinship runs from Via to Via2 and Path is A, Kinship.Path, B.
	KindSpouseBloodSpouse = "spouse_blood_spouse"
	// KindStepSibling: B is a child of the spouse of A's parent. Via is that
	// parent, Via2 the step-parent; Kinship is A to Via and Path is A, Via,
	// Via2, B.
	KindStepSibling = "step_sibling"
	// KindCoParentInLaw: B is a parent of the spouse of A's child. Via is the
	// child, Via2 the child's spouse; Kinship is A to Via and Path is A, Via,
	// Via2, B.
	KindCoParentInLaw = "co_parent_in_law"
	KindNone          = "none"
)

// Relationship says what B is to A.
type Relationship struct {
	Kind string `json:"kind"`
	// Kinship is the blood part; for the in-law kinds it is the kinship
	// between the relative and A, or B and the spouse.
	Kinship *Kinship `json:"kinship,omitempty"`
	// Via is the spouse that links the in-law kinds.
	Via int64 `json:"via,omitempty"`
	// Via2 is the second person on the way for the kinds that go through two
	// marriages or a step-parent.
	Via2 int64 `json:"via2,omitempty"`
	// Others are further blood relationships, e.g. through pedigree
	// collapse when cousins married.
	Others []Kinship `json:"others"`
}

// maxOthers bounds the extra relationships reported.
const maxOthers = 5

// Relate finds what b is to a.
func (g *Graph) Relate(a, b int64) Relationship {
	switch {
	case a == b:
		return Relationship{Kind: KindSelf, Others: []Kinship{}}
	case slices.Contains(g.spouses[a], b):
		r := Relationship{Kind: KindSpouse, Others: []Kinship{}}
		if ks := g.kinships(a, b); len(ks) > 0 {
			r.Others = limit(ks)
		}
		return r
	}
	if ks := g.kinships(a, b); len(ks) > 0 {
		return Relationship{Kind: KindBlood, Kinship: &ks[0], Others: limit(ks[1:])}
	}

	best := Relationship{Kind: KindNone, Others: []Kinship{}}
	bestLen := -1
	consider := func(kind string, via int64, k Kinship) {
		if n := k.Up + k.Down; bestLen < 0 || n < bestLen {
			kk := k
			best, bestLen = Relationship{Kind: kind, Kinship: &kk, Via: via, Others: []Kinship{}}, n
		}
	}
	for _, s := range g.spouses[b] {
		if ks := g.kinships(a, s); len(ks) > 0 {
			k := ks[0]
			k.Path = append(k.Path, b)
			consider(KindSpouseOfRelative, s, k)
		}
	}
	for _, s := range g.spouses[a] {
		if ks := g.kinships(s, b); len(ks) > 0 {
			k := ks[0]
			k.Path = append([]int64{a}, k.Path...)
			consider(KindRelativeOfSpouse, s, k)
		}
	}
	if best.Kind == KindNone {
		return g.viaTwoLinks(a, b)
	}
	return best
}

// viaTwoLinks finds the relations that need two marriage or step links. It
// only runs when nothing closer exists and is bounded by the number of
// spouses, parents and children of the two people (plus one kinship search
// per pair of spouses).
func (g *Graph) viaTwoLinks(a, b int64) Relationship {
	none := Relationship{Kind: KindNone, Others: []Kinship{}}
	// A's spouse's relative's spouse.
	var best Relationship
	bestLen := -1
	for _, s1 := range g.spouses[a] {
		for _, s2 := range g.spouses[b] {
			if s1 == s2 || s1 == b || s2 == a {
				continue
			}
			ks := g.kinships(s1, s2)
			if len(ks) == 0 {
				continue
			}
			k := ks[0]
			if n := k.Up + k.Down; bestLen < 0 || n < bestLen {
				k.Path = append(append([]int64{a}, k.Path...), b)
				best, bestLen = Relationship{Kind: KindSpouseBloodSpouse, Kinship: &k, Via: s1, Via2: s2, Others: []Kinship{}}, n
			}
		}
	}
	if bestLen >= 0 {
		return best
	}
	// A's parent's spouse's child.
	for _, pl := range g.parents[a] {
		for _, s := range g.spouses[pl.parent] {
			if s == a || s == b {
				continue
			}
			for _, bl := range g.parents[b] {
				if bl.parent == s {
					return Relationship{
						Kind:    KindStepSibling,
						Kinship: &Kinship{Up: 1, Adoptive: !pl.blood, Ancestors: []int64{pl.parent}, Path: []int64{a, pl.parent, s, b}},
						Via:     pl.parent, Via2: s, Others: []Kinship{},
					}
				}
			}
		}
	}
	// A's child's spouse's parent.
	for _, c := range g.children[a] {
		for _, s := range g.spouses[c] {
			if s == a || s == b {
				continue
			}
			for _, pl := range g.parents[s] {
				if pl.parent == b {
					return Relationship{
						Kind:    KindCoParentInLaw,
						Kinship: &Kinship{Down: 1, Ancestors: []int64{}, Path: []int64{a, c, s, b}},
						Via:     c, Via2: s, Others: []Kinship{},
					}
				}
			}
		}
	}
	return none
}

func limit(ks []Kinship) []Kinship {
	if len(ks) > maxOthers {
		ks = ks[:maxOthers]
	}
	if ks == nil {
		return []Kinship{}
	}
	return ks
}

// kinships finds blood relationships first and falls back to paths that
// include adoptive links.
func (g *Graph) kinships(a, b int64) []Kinship {
	if ks := g.commonAncestry(a, b, true); len(ks) > 0 {
		return ks
	}
	ks := g.commonAncestry(a, b, false)
	for i := range ks {
		ks[i].Adoptive = true
	}
	return ks
}

type visit struct {
	gen int
	// child is the person one generation below on the way up, family the
	// family linking them; zero for the start person.
	child, family int64
}

// ancestors climbs from start breadth-first. Nodes in blocked other than
// target are not passed through, so with a target the result tells whether
// it is reachable without going through another common ancestor.
func (g *Graph) ancestors(start int64, bloodOnly bool, blocked map[int64]bool, target int64) map[int64]visit {
	seen := map[int64]visit{start: {}}
	frontier := []int64{start}
	for gen := 1; len(frontier) > 0; gen++ {
		var next []int64
		for _, p := range frontier {
			for _, l := range g.parents[p] {
				if bloodOnly && !l.blood {
					continue
				}
				if _, ok := seen[l.parent]; ok {
					continue
				}
				seen[l.parent] = visit{gen: gen, child: p, family: l.family}
				if !blocked[l.parent] || l.parent == target {
					next = append(next, l.parent)
				}
			}
		}
		frontier = next
	}
	return seen
}

func (g *Graph) commonAncestry(a, b int64, bloodOnly bool) []Kinship {
	upA, upB := g.ancestors(a, bloodOnly, nil, 0), g.ancestors(b, bloodOnly, nil, 0)
	common := map[int64]bool{}
	var ids []int64
	for id := range upA {
		if _, ok := upB[id]; ok {
			common[id] = true
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	type key struct {
		up, down int
		family   int64
	}
	byKey := map[key]*Kinship{}
	var order []key
	for _, x := range ids {
		// x is a separate relationship only when both sides reach it
		// without passing another common ancestor; otherwise it is just an
		// ancestor of a closer one. With pedigree collapse, an ancestor can
		// be both.
		viaA := g.ancestors(a, bloodOnly, common, x)
		viaB := g.ancestors(b, bloodOnly, common, x)
		va, okA := viaA[x]
		vb, okB := viaB[x]
		if !okA || !okB {
			continue
		}
		// A couple of common ancestors reached through the same family is
		// one relationship; the family on A's side tells couples apart.
		k := key{va.gen, vb.gen, va.family}
		if existing, ok := byKey[k]; ok {
			existing.Ancestors = append(existing.Ancestors, x)
			continue
		}
		kin := &Kinship{
			Up: va.gen, Down: vb.gen,
			Half:      va.gen > 0 && vb.gen > 0 && va.family != vb.family,
			Ancestors: []int64{x},
			Path:      path(viaA, viaB, x),
		}
		byKey[k] = kin
		order = append(order, k)
	}
	out := make([]Kinship, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := out[i].Up+out[i].Down, out[j].Up+out[j].Down
		if di != dj {
			return di < dj
		}
		return out[i].Half != out[j].Half && !out[i].Half
	})
	return out
}

// path walks from a up to x and down to b.
func path(upA, upB map[int64]visit, x int64) []int64 {
	var left []int64
	for id := x; ; id = upA[id].child {
		left = append([]int64{id}, left...)
		if upA[id].gen == 0 {
			break
		}
	}
	p := left
	for id := x; upB[id].gen != 0; {
		id = upB[id].child
		p = append(p, id)
	}
	return p
}
