package store

import "testing"

func TestSetupDefaultTreeName(t *testing.T) {
	s := newStore(t)
	a := setup(t, s) // no tree name given
	name, err := s.TreeName(ctx, a.TreeID)
	if err != nil {
		t.Fatal(err)
	}
	if name != DefaultTreeName {
		t.Errorf("tree name %q, want %q", name, DefaultTreeName)
	}
}
