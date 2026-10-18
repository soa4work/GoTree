package graph

import (
	"fmt"
	"testing"
)

// Two marriages and step links:
//
//	sisters 1 and 2 (daughters of 20 ∞ 21) with husbands 3 and 4
//	brothers 5 and 6 (sons of 22 ∞ 23) with wives 7 and 8
//	9 of 3+1 marries 10, whose parents are 11 and 12
//	15 of 13+14; 14 remarries 16, who has 17 from another union
func twoMarriages() *Graph {
	both := func(id int64) Child { return Child{PersonID: id, Blood1: true, Blood2: true} }
	return New(
		[]int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 20, 21, 22, 23},
		[]Family{
			{ID: 1, Partner1: 20, Partner2: 21, Children: []Child{both(1), both(2)}},
			{ID: 2, Partner1: 22, Partner2: 23, Children: []Child{both(5), both(6)}},
			{ID: 3, Partner1: 3, Partner2: 1, Children: []Child{both(9)}},
			{ID: 4, Partner1: 4, Partner2: 2},
			{ID: 5, Partner1: 5, Partner2: 7},
			{ID: 6, Partner1: 6, Partner2: 8},
			{ID: 7, Partner1: 9, Partner2: 10},
			{ID: 8, Partner1: 11, Partner2: 12, Children: []Child{both(10)}},
			{ID: 9, Partner1: 13, Partner2: 14, Children: []Child{both(15)}},
			{ID: 10, Partner1: 14, Partner2: 16},
			{ID: 11, Partner1: 16, Partner2: 18, Children: []Child{both(17)}},
		},
	)
}

func TestRelateSpouseBloodSpouse(t *testing.T) {
	g := twoMarriages()
	tests := []struct {
		a, b      int64
		via, via2 int64
		path      string
	}{
		{3, 4, 1, 2, "[3 1 20 2 4]"}, // husbands of sisters
		{4, 3, 2, 1, "[4 2 20 1 3]"},
		{7, 8, 5, 6, "[7 5 22 6 8]"}, // wives of brothers
	}
	for _, tt := range tests {
		r := g.Relate(tt.a, tt.b)
		if r.Kind != KindSpouseBloodSpouse || r.Via != tt.via || r.Via2 != tt.via2 ||
			fmt.Sprint(r.Kinship.Path) != tt.path || r.Kinship.Up != 1 || r.Kinship.Down != 1 {
			t.Errorf("%d→%d: %+v kinship %+v", tt.a, tt.b, r, r.Kinship)
		}
	}
}

func TestRelateStepSibling(t *testing.T) {
	g := twoMarriages()
	r := g.Relate(15, 17)
	if r.Kind != KindStepSibling || r.Via != 14 || r.Via2 != 16 || fmt.Sprint(r.Kinship.Path) != "[15 14 16 17]" || r.Kinship.Adoptive {
		t.Errorf("step-sibling: %+v %+v", r, r.Kinship)
	}
	r = g.Relate(17, 15)
	if r.Kind != KindStepSibling || r.Via != 16 || r.Via2 != 14 {
		t.Errorf("step-sibling, reversed: %+v", r)
	}
	// Full siblings stay blood.
	if got := describe(g.Relate(1, 2)); got != "blood 1/1" {
		t.Errorf("siblings: %s", got)
	}
}

func TestRelateCoParentInLaw(t *testing.T) {
	g := twoMarriages()
	// 3 and 1 are the parents of 9, whose wife 10 has parents 11 and 12.
	for _, tt := range []struct{ a, b, c, s int64 }{{3, 11, 9, 10}, {1, 12, 9, 10}, {11, 3, 10, 9}, {12, 1, 10, 9}} {
		r := g.Relate(tt.a, tt.b)
		if r.Kind != KindCoParentInLaw || r.Via != tt.c || r.Via2 != tt.s ||
			fmt.Sprint(r.Kinship.Path) != fmt.Sprint([]int64{tt.a, tt.c, tt.s, tt.b}) {
			t.Errorf("%d→%d: %+v", tt.a, tt.b, r)
		}
	}
}

func TestTwoLinksKeepExistingAndStayNone(t *testing.T) {
	g := twoMarriages()
	if got := describe(g.Relate(3, 10)); got != "spouse_of_relative 0/1" {
		t.Errorf("child's wife: %s", got)
	}
	if r := g.Relate(15, 18); r.Kind != KindNone {
		t.Errorf("15→18: %+v", r)
	}
}

func TestTwoLinksTerminateOnLoops(t *testing.T) {
	// 2 is the spouse of 1 and of their own child 3.
	g := New([]int64{1, 2, 3}, []Family{
		{ID: 1, Partner1: 1, Partner2: 2, Children: []Child{{PersonID: 3, Blood1: true, Blood2: true}}},
		{ID: 2, Partner1: 2, Partner2: 3},
	})
	for a := int64(1); a <= 3; a++ {
		for b := int64(1); b <= 3; b++ {
			g.Relate(a, b)
		}
	}
}
