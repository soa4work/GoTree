package store

import (
	"errors"
	"testing"
)

func TestImportReplaceNeedsOwnerInStore(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	doc := parseFixture(t, "ancestry-551.ged")
	if _, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	before, _ := s.ListPersons(ctx, a, "", 50, 0)

	editor := a
	editor.Role = RoleEditor
	if _, err := s.ImportGEDCOM(ctx, editor, parseFixture(t, "gedcom7.ged"), ImportReplace); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("an editor replacing the tree: %v, want ErrOwnerOnly", err)
	}
	after, _ := s.ListPersons(ctx, a, "", 50, 0)
	if before.Total == 0 || after.Total != before.Total {
		t.Errorf("people before %d, after %d: the tree must be untouched", before.Total, after.Total)
	}
}

func TestImportNeedsEditor(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	viewer := a
	viewer.Role = RoleViewer
	if _, err := s.ImportGEDCOM(ctx, viewer, parseFixture(t, "ancestry-551.ged"), ImportIntoEmpty); !errors.Is(err, ErrViewOnly) {
		t.Fatalf("a viewer importing: %v, want ErrViewOnly", err)
	}
	list, _ := s.ListPersons(ctx, a, "", 50, 0)
	if list.Total != 0 {
		t.Errorf("%d people created by a viewer", list.Total)
	}
}
