package store

import (
	"fmt"
	"strings"
	"testing"
)

// indiBlock returns the record of one person in an exported file.
func indiBlock(t *testing.T, text string, id int64) string {
	t.Helper()
	head := fmt.Sprintf("@I%d@ INDI", id)
	for _, block := range strings.Split(text, "\n0 ") {
		if strings.HasPrefix(block, head) {
			return block
		}
	}
	t.Fatalf("no record for I%d in:\n%s", id, text)
	return ""
}

// A 5.5.1 export writes an association on the person whose event it is
// and nowhere else; GEDCOM 7 has it on the event.
func TestExportAssociationsOnlyOnThePrincipal(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	pa := mustPerson(t, s, a, "Anna", "Weber")
	pb := mustPerson(t, s, a, "Berta", "Weber")
	pg := mustPerson(t, s, a, "Greta", "Weber")
	if _, err := s.CreateEvent(ctx, a, EventInput{
		PersonID: &pa.ID, Type: "BAPM", Date: "1850",
		Participants: []ParticipantInput{{PersonID: pg.ID, Role: "godparent"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &pb.ID, Type: "BIRT", Date: "1852"}); err != nil {
		t.Fatal(err)
	}

	text := exportText(t, s, a, ExportOptions{Privacy: PrivacyAll})
	block := indiBlock(t, text, pa.ID)
	want := fmt.Sprintf("1 ASSO @I%d@\n2 RELA godparent (BAPM)", pg.ID)
	if strings.Count(block, "1 ASSO") != 1 || !strings.Contains(block, want) {
		t.Errorf("Anna's record should hold exactly one association %q:\n%s", want, block)
	}
	for name, id := range map[string]int64{"Berta": pb.ID, "Greta": pg.ID} {
		if strings.Contains(indiBlock(t, text, id), "ASSO") {
			t.Errorf("%s's record has an association", name)
		}
	}

	v7 := exportText(t, s, a, ExportOptions{Privacy: PrivacyAll, Version7: true})
	for _, id := range []int64{pa.ID, pb.ID, pg.ID} {
		if strings.Contains(indiBlock(t, v7, id), "\n1 ASSO") {
			t.Errorf("GEDCOM 7 person I%d has an association of its own", id)
		}
	}
	if !strings.Contains(v7, "2 ASSO @I") {
		t.Errorf("GEDCOM 7 event lost its association:\n%s", v7)
	}
}
