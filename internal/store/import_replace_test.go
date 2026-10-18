package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/praetorianer777/gotree/internal/gedcom"
)

func parseText(t *testing.T, text string) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

const gedHead = "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n"

// A file with Anna Weber under the given xref and her child.
func annaFile(t *testing.T, xref, surname string) *gedcom.Document {
	t.Helper()
	return parseText(t, gedHead+fmt.Sprintf(
		"0 @%s@ INDI\n1 NAME Anna /%s/\n1 FAMS @F1@\n"+
			"0 @I9001@ INDI\n1 NAME Ben /%s/\n1 FAMC @F1@\n"+
			"0 @F1@ FAM\n1 WIFE @%s@\n1 CHIL @I9001@\n0 TRLR\n", xref, surname, surname, xref))
}

func TestReplaceKeepsShareLinksWhosePersonIsInTheFile(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	root := mustPerson(t, s, a, "Anna", "Weber")
	newRelative(t, s, a, root.ID, RelationChild, "Ben")
	carl := mustPerson(t, s, a, "Carl", "Roth")

	branch := func(label string, p Person) ShareLink {
		t.Helper()
		l, err := s.CreateShareLink(ctx, a, ShareLinkInput{Label: label, Scope: ShareScopeDescendants, RootPersonID: &p.ID, Privacy: SharePrivacyLivingNames})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	kept := branch("Weber branch", root)
	lost := branch("Roth branch", carl)
	revoked := branch("Old Roth branch", carl)
	if err := s.RevokeShareLink(ctx, a, revoked.ID); err != nil {
		t.Fatal(err)
	}
	whole, err := s.CreateShareLink(ctx, a, ShareLinkInput{Label: "Everyone", Scope: ShareScopeTree, Privacy: SharePrivacyDeceased})
	if err != nil {
		t.Fatal(err)
	}

	// The file looks like an export of this tree that someone edited: Anna
	// keeps her id, Carl is gone.
	report, err := s.ImportGEDCOM(ctx, a, annaFile(t, fmt.Sprintf("I%d", root.ID), "Weber"), ImportReplace)
	if err != nil {
		t.Fatal(err)
	}
	if report.ShareLinksKept != 1 || len(report.ShareLinksLost) != 1 || report.ShareLinksLost[0] != "Roth branch" {
		t.Errorf("kept %d, lost %v (a revoked link is not counted)", report.ShareLinksKept, report.ShareLinksLost)
	}

	sh, err := s.ShareByToken(ctx, kept.Token)
	if err != nil {
		t.Fatalf("the kept link stopped working: %v", err)
	}
	if sh.Link.ID != kept.ID || sh.Link.Root == nil || sh.Link.Root.GivenNames != "Anna" {
		t.Fatalf("kept link: %+v", sh.Link)
	}
	if sh.Link.Root.ID == root.ID {
		t.Error("the root was not re-created under a new id")
	}
	list, err := s.SharePersons(ctx, sh, "")
	if err != nil || list.Total != 2 {
		t.Errorf("the branch shows %d people, %v", list.Total, err)
	}
	if _, err := s.ShareByToken(ctx, lost.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("link to a missing person: %v", err)
	}
	if _, err := s.ShareByToken(ctx, whole.Token); err != nil {
		t.Errorf("tree link: %v", err)
	}
	if _, err := s.ShareByToken(ctx, revoked.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoked link: %v", err)
	}
}

func TestReplaceDropsShareLinkWhenTheNameDiffers(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	root := mustPerson(t, s, a, "Anna", "Weber")
	l, err := s.CreateShareLink(ctx, a, ShareLinkInput{Label: "Weber branch", Scope: ShareScopeDescendants, RootPersonID: &root.ID, Privacy: SharePrivacyDeceased})
	if err != nil {
		t.Fatal(err)
	}
	// The same id, another person.
	report, err := s.ImportGEDCOM(ctx, a, annaFile(t, fmt.Sprintf("I%d", root.ID), "Schmidt"), ImportReplace)
	if err != nil {
		t.Fatal(err)
	}
	if report.ShareLinksKept != 0 || len(report.ShareLinksLost) != 1 {
		t.Errorf("kept %d, lost %v", report.ShareLinksKept, report.ShareLinksLost)
	}
	if _, err := s.ShareByToken(ctx, l.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("link into an unrelated person's branch: %v", err)
	}
}

func TestReplaceKeepsShareLinkFoundByGedcomID(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	// The tree came from a file with its own ids.
	if _, err := s.ImportGEDCOM(ctx, a, annaFile(t, "P7", "Weber"), ImportReplace); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.DB.QueryRow(`SELECT id FROM persons WHERE gedcom_xref = 'P7'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	l, err := s.CreateShareLink(ctx, a, ShareLinkInput{Label: "Anna", Scope: ShareScopeDescendants, RootPersonID: &id, Privacy: SharePrivacyDeceased})
	if err != nil {
		t.Fatal(err)
	}
	// A newer version of the same file.
	report, err := s.ImportGEDCOM(ctx, a, annaFile(t, "P7", "Weber"), ImportReplace)
	if err != nil {
		t.Fatal(err)
	}
	if report.ShareLinksKept != 1 || len(report.ShareLinksLost) != 0 {
		t.Errorf("kept %d, lost %v", report.ShareLinksKept, report.ShareLinksLost)
	}
	sh, err := s.ShareByToken(ctx, l.Token)
	if err != nil || sh.Link.Root == nil || sh.Link.Root.ID == id {
		t.Fatalf("link after the second import: %+v, %v", sh.Link, err)
	}
}

func TestReplaceClearsTranscriptionPeople(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	src := mustSource(t, s, a, "Census 1900")
	p := mustPerson(t, s, a, "Johann", "Maier")
	rows := []TranscriptionRow{
		{Line: "1", Role: "head", Action: RowActionPerson, PersonID: &p.ID, Values: map[string]string{"given": "Johann", "surname": "Maier"}},
		row("2", "spouse", map[string]string{"given": "Maria", "surname": "Maier"}),
	}
	in := TranscriptionInput{TemplateKey: "census", SourceID: &src.ID, Date: "1 DEC 1900", Rows: rows}
	tr, err := s.CreateTranscription(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportGEDCOM(ctx, a, annaFile(t, "I1", "Weber"), ImportReplace); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTranscription(ctx, a, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rows[0].PersonID != nil || got.Rows[0].Action != RowActionPerson {
		t.Errorf("row 0: %+v (the person goes, the action stays)", got.Rows[0])
	}
	if got.Rows[1].Action != RowActionNew {
		t.Errorf("row 1: %+v", got.Rows[1])
	}
	if len(got.Persons) != 0 || got.Source != nil {
		t.Errorf("draft still names the old tree: %d people, source %v", len(got.Persons), got.Source)
	}
	if got.UpdatedAt != tr.UpdatedAt {
		t.Errorf("updated_at changed: %s -> %s", tr.UpdatedAt, got.UpdatedAt)
	}
	in.SourceID = nil
	in.Rows = got.Rows
	_, err = s.UpdateTranscription(ctx, a, tr.ID, in)
	validationField(t, err, "rows.0")
}

func TestIDsAreNotReusedByReplaceImport(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	old := []int64{mustPerson(t, s, a, "A", "").ID, mustPerson(t, s, a, "B", "").ID}
	src, err := s.CreateSource(ctx, a, SourceInput{Title: "S"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportGEDCOM(ctx, a, annaFile(t, "I1", "Weber"), ImportReplace); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListPersons(ctx, a, "", 50, 0)
	if err != nil || list.Total != 2 {
		t.Fatalf("people after import: %d, %v", list.Total, err)
	}
	for _, p := range list.Items {
		for _, o := range old {
			if p.ID <= o {
				t.Errorf("new person %d does not come after old person %d", p.ID, o)
			}
		}
	}
	for _, o := range old {
		if _, err := s.GetPerson(ctx, a, o); !errors.Is(err, ErrNotFound) {
			t.Errorf("old person %d: %v", o, err)
		}
	}
	if _, err := s.GetSource(ctx, a, src.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("old source: %v", err)
	}
}
