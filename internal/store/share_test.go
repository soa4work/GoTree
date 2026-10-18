package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type shareFamily struct {
	grandpa, hans, erika, lisa, max, kid, other Person
}

// shareTree: grandpa → Hans (∞ Erika) → Lisa (∞ Max) → Kid, plus Other.
// Grandpa, Hans, Erika and Other are deceased; Lisa, Max and Kid living.
func shareTree(t *testing.T, s *Store, a Actor) shareFamily {
	t.Helper()
	var f shareFamily
	event := func(p Person, typ, date string) {
		t.Helper()
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: typ, Date: date, Description: "about " + p.GivenNames}); err != nil {
			t.Fatal(err)
		}
	}
	relative := func(of Person, rel, given string) Person {
		t.Helper()
		r, err := s.AddRelative(ctx, a, of.ID, RelativeInput{Relation: rel, Person: &PersonInput{GivenNames: given, Surname: "Weber", Notes: "notes on " + given}})
		if err != nil {
			t.Fatal(err)
		}
		return r.Person
	}
	f.grandpa = mustPerson(t, s, a, "Grandpa", "Weber")
	event(f.grandpa, "BIRT", "1850")
	event(f.grandpa, "DEAT", "1920")
	f.hans = relative(f.grandpa, RelationChild, "Hans")
	event(f.hans, "BIRT", "3 FEB 1880")
	event(f.hans, "DEAT", "1950")
	f.erika = relative(f.hans, RelationPartner, "Erika")
	event(f.erika, "DEAT", "1960")
	f.lisa = relative(f.hans, RelationChild, "Lisa")
	event(f.lisa, "BIRT", "3 FEB 1990")
	f.max = relative(f.lisa, RelationPartner, "Max")
	f.kid = relative(f.lisa, RelationChild, "Kid")
	f.other = mustPerson(t, s, a, "Other", "Schulz")
	event(f.other, "DEAT", "1900")
	return f
}

func shareFor(t *testing.T, s *Store, a Actor, in ShareLinkInput) Share {
	t.Helper()
	link, err := s.CreateShareLink(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	sh, err := s.ShareByToken(ctx, link.Token)
	if err != nil {
		t.Fatal(err)
	}
	return sh
}

func listIDs(list PersonList) map[int64]bool {
	out := map[int64]bool{}
	for _, p := range list.Items {
		out[p.ID] = true
	}
	return out
}

func TestShareDeceasedOnly(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	f := shareTree(t, s, a)
	sh := shareFor(t, s, a, ShareLinkInput{Label: "Cousins", Scope: ShareScopeTree, Privacy: SharePrivacyDeceased})

	list, err := s.SharePersons(ctx, sh, "")
	if err != nil {
		t.Fatal(err)
	}
	got := listIDs(list)
	if len(got) != 4 || !got[f.grandpa.ID] || !got[f.hans.ID] || !got[f.erika.ID] || !got[f.other.ID] || list.Total != 4 {
		t.Errorf("visible: %+v", list.Items)
	}
	for _, p := range []Person{f.lisa, f.max, f.kid} {
		if _, err := s.SharePerson(ctx, sh, p.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", p.GivenNames, err)
		}
	}

	hans, err := s.SharePerson(ctx, sh, f.hans.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hans.PartnerFamilies) != 1 || len(hans.PartnerFamilies[0].Children) != 0 || hans.Notes != "notes on Hans" {
		t.Errorf("Hans: %+v", hans.PartnerFamilies)
	}

	g, err := s.ShareTree(ctx, sh, f.grandpa.ID, TreeOptions{Down: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Persons) != 3 {
		t.Errorf("tree persons: %v", g.Persons)
	}
	raw, _ := json.Marshal(g)
	for _, name := range []string{"Lisa", "Max", "Kid"} {
		if strings.Contains(string(raw), name) {
			t.Errorf("%s leaked into the tree", name)
		}
	}
	if _, err := s.ShareTree(ctx, sh, f.lisa.ID, TreeOptions{Up: 2}); !errors.Is(err, ErrNotFound) {
		t.Errorf("tree from a living person: %v", err)
	}

	day, err := s.ShareOnThisDay(ctx, sh, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Events) != 1 || day.Events[0].Year != 1880 || day.Persons[f.hans.ID].GivenNames != "Hans" {
		t.Errorf("on this day: %+v", day)
	}
}

func TestShareDescendantsWithLivingNames(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	f := shareTree(t, s, a)
	fams, err := s.GetPersonDetail(ctx, a, f.lisa.ID)
	if err != nil {
		t.Fatal(err)
	}
	famID := fams.PartnerFamilies[0].ID
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &famID, Type: "MARR", Date: "2015"}); err != nil {
		t.Fatal(err)
	}

	sh := shareFor(t, s, a, ShareLinkInput{Label: "Hans' line", Scope: ShareScopeDescendants, RootPersonID: &f.hans.ID, Privacy: SharePrivacyLivingNames})
	list, err := s.SharePersons(ctx, sh, "")
	if err != nil {
		t.Fatal(err)
	}
	got := listIDs(list)
	if len(got) != 5 || got[f.grandpa.ID] || got[f.other.ID] {
		t.Errorf("visible: %+v", list.Items)
	}
	for _, p := range list.Items {
		if p.ID == f.lisa.ID && (p.BirthDate != "" || p.BirthSort != 0) {
			t.Errorf("living person's birth date: %+v", p)
		}
	}

	lisa, err := s.SharePerson(ctx, sh, f.lisa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lisa.GivenNames != "Lisa" || lisa.Notes != "" || len(lisa.Events) != 0 {
		t.Errorf("Lisa: %+v", lisa)
	}
	if len(lisa.PartnerFamilies) != 1 || len(lisa.PartnerFamilies[0].Events) != 0 || len(lisa.PartnerFamilies[0].Children) != 1 {
		t.Errorf("Lisa's family: %+v", lisa.PartnerFamilies)
	}
	raw, _ := json.Marshal(lisa)
	for _, secret := range []string{"1990", "2015", "about Lisa", "notes on"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("%q leaked: %s", secret, raw)
		}
	}

	hans, err := s.SharePerson(ctx, sh, f.hans.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hans.ParentFamilies) != 1 || hans.ParentFamilies[0].Partner1 != nil || hans.ParentFamilies[0].Partner2 != nil {
		t.Errorf("Hans' parents are outside the scope: %+v", hans.ParentFamilies)
	}
	if _, err := s.SharePerson(ctx, sh, f.grandpa.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("grandpa: %v", err)
	}
	info, err := s.ShareInfo(ctx, sh)
	if err != nil {
		t.Fatal(err)
	}
	if info.StartID == nil || *info.StartID != f.hans.ID || info.TreeName == "" {
		t.Errorf("info: %+v", info)
	}
	day, err := s.ShareOnThisDay(ctx, sh, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(day.Events) != 1 {
		t.Errorf("living Lisa's birthday must not show: %+v", day.Events)
	}
}

func TestShareLinkLifecycle(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Anna", "")

	_, err := s.CreateShareLink(ctx, a, ShareLinkInput{Label: "", Scope: "branch", Privacy: SharePrivacyDeceased})
	validationField(t, err, "label")
	_, err = s.CreateShareLink(ctx, a, ShareLinkInput{Label: "x", Scope: ShareScopeDescendants, Privacy: SharePrivacyDeceased})
	validationField(t, err, "rootPersonId")
	_, err = s.CreateShareLink(ctx, a, ShareLinkInput{Label: "x", Scope: ShareScopeTree, Privacy: SharePrivacyDeceased, ExpiresOn: "2026-10-01"})
	validationField(t, err, "expiresOn")

	link, err := s.CreateShareLink(ctx, a, ShareLinkInput{Label: "Family", Scope: ShareScopeDescendants, RootPersonID: &p.ID, Privacy: SharePrivacyLivingNames, ExpiresOn: "2026-10-05"})
	if err != nil {
		t.Fatal(err)
	}
	if len(link.Token) < 40 || link.Root.GivenNames != "Anna" {
		t.Errorf("link: %+v", link)
	}
	if _, err := s.ShareByToken(ctx, link.Token); err != nil {
		t.Errorf("the link works through its last day: %v", err)
	}
	list, err := s.ListShareLinks(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Token != "" || list[0].LastUsedAt == nil {
		t.Errorf("list: %+v", list)
	}

	viewer := a
	viewer.Role = RoleViewer
	if _, err := s.CreateShareLink(ctx, viewer, ShareLinkInput{Label: "x", Scope: ShareScopeTree, Privacy: SharePrivacyDeceased}); !errors.Is(err, ErrOwnerOnly) {
		t.Errorf("viewer: %v", err)
	}

	if err := s.RevokeShareLink(ctx, a, link.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ShareByToken(ctx, link.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoked: %v", err)
	}

	expiring, _ := s.CreateShareLink(ctx, a, ShareLinkInput{Label: "Short", Scope: ShareScopeTree, Privacy: SharePrivacyDeceased, ExpiresOn: "2026-10-05"})
	s.Now = func() time.Time { return time.Date(2026, 10, 6, 0, 0, 1, 0, time.UTC) }
	if _, err := s.ShareByToken(ctx, expiring.Token); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired: %v", err)
	}
	if _, err := s.ShareByToken(ctx, "nonsense"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestOnThisDay(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	f := shareTree(t, s, a)
	rep, err := s.OnThisDay(ctx, a, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Events) != 1 || rep.Events[0].PersonIDs[0] != f.hans.ID {
		t.Errorf("only the deceased: %+v", rep.Events)
	}
	if _, err := s.OnThisDay(ctx, a, 13, 1); err == nil {
		t.Error("month 13 accepted")
	}
}
