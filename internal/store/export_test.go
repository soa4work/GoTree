package store

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/gedcom"
)

func exportText(t *testing.T, s *Store, a Actor, opt ExportOptions) string {
	t.Helper()
	ex, err := s.ExportGEDCOM(ctx, a, opt)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := gedcom.Write(&buf, ex.Records, gedcom.WriteOptions{Version7: opt.Version7}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// summary is what must survive a round trip.
type summary struct {
	counts map[string]int
	names  []string
}

func summarize(t *testing.T, s *Store, a Actor) summary {
	t.Helper()
	sm := summary{counts: map[string]int{}}
	for table, q := range map[string]string{
		"persons":      `SELECT count(*) FROM persons WHERE tree_id = ?`,
		"families":     `SELECT count(*) FROM families WHERE tree_id = ?`,
		"events":       `SELECT count(*) FROM events WHERE tree_id = ?`,
		"sources":      `SELECT count(*) FROM sources WHERE tree_id = ?`,
		"repositories": `SELECT count(*) FROM repositories WHERE tree_id = ?`,
		"citations":    `SELECT count(*) FROM citations WHERE tree_id = ?`,
		"places":       `SELECT count(*) FROM places WHERE tree_id = ?`,
		"children":     `SELECT count(*) FROM family_children c JOIN families f ON f.id = c.family_id WHERE f.tree_id = ?`,
		"names":        `SELECT count(*) FROM person_names n JOIN persons p ON p.id = n.person_id WHERE p.tree_id = ?`,
		"participants": `SELECT count(*) FROM event_participants x JOIN events e ON e.id = x.event_id WHERE e.tree_id = ?`,
	} {
		var n int
		if err := s.DB.QueryRow(q, a.TreeID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		sm.counts[table] = n
	}
	rows, err := s.DB.Query(`SELECT given_names || ' ' || surname || ' ' || sex FROM persons WHERE tree_id = ?`, a.TreeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		sm.names = append(sm.names, n)
	}
	sort.Strings(sm.names)
	return sm
}

func reimport(t *testing.T, text string) (*Store, Actor) {
	t.Helper()
	doc, err := gedcom.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	a := setup(t, s)
	if _, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	return s, a
}

func TestExportRoundTrip(t *testing.T) {
	for _, fixture := range []string{"ancestry-551.ged", "gedcom7.ged"} {
		for _, v7 := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s v7=%v", fixture, v7), func(t *testing.T) {
				s := newStore(t)
				a := setup(t, s)
				if _, err := s.ImportGEDCOM(ctx, a, parseFixture(t, fixture), ImportIntoEmpty); err != nil {
					t.Fatal(err)
				}
				before := summarize(t, s, a)
				text := exportText(t, s, a, ExportOptions{Version7: v7, AppVersion: "1.2.3"})

				s2, a2 := reimport(t, text)
				after := summarize(t, s2, a2)
				for k, v := range before.counts {
					// GEDCOM 5.5.1 has no sex X; it comes back as U.
					if after.counts[k] != v {
						t.Errorf("v7=%v %s: %d before, %d after", v7, k, v, after.counts[k])
					}
				}
				if !v7 {
					for i := range before.names {
						before.names[i] = strings.Replace(before.names[i], " X", " U", 1)
					}
				}
				if strings.Join(before.names, "|") != strings.Join(after.names, "|") {
					t.Errorf("names changed:\n%v\n%v", before.names, after.names)
				}
			})
		}
	}
}

func TestExportContent(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	if _, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "ancestry-551.ged"), ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	v5 := exportText(t, s, a, ExportOptions{AppVersion: "1.2.3"})
	for _, want := range []string{
		"2 VERS 5.5.1", "1 CHAR UTF-8", "2 VERS 1.2.3", "0 @U1@ SUBM",
		"1 NAME Johann /Weber/", "2 DATE 3 FEB 1855", "2 PLAC Leipzig, Sachsen, Deutschland", "4 LATI N51.3397",
		"3 PAGE S. 12, Nr. 34", "3 QUAY 3", "4 TEXT Johann, Sohn des Karl Weber",
		"2 DATE (12 Dezember 1920)",       // unreadable date kept as a phrase
		"1 _UID 6B5C0D1E2F3A4B5C",         // kept verbatim
		"2 _FREL Natural", "2 _MREL Step", // relations that differ per parent
		"2 PEDI adopted", // the same for both parents
		"1 OCCU Weber", "1 EVEN\n2 TYPE Auswanderungsversuch",
		"1 WWW https://www.leipzig.de/stadtarchiv",
	} {
		if !strings.Contains(v5, want+"\n") {
			t.Errorf("5.5.1 export lacks %q", want)
		}
	}
	if strings.Contains(v5, "CHAN") {
		t.Error("dropped tags must not come back")
	}

	v7 := exportText(t, s, a, ExportOptions{Version7: true})
	for _, want := range []string{"2 VERS 7.0", "1 SCHMA", "2 TAG _FREL " + extensionDocs + "_frel", "2 DATE\n3 PHRASE 12 Dezember 1920"} {
		if !strings.Contains(v7, want) {
			t.Errorf("7.0 export lacks %q", want)
		}
	}
	if strings.Contains(v7, "CHAR") || strings.Contains(v7, "CONC") {
		t.Error("GEDCOM 7 has neither CHAR nor CONC")
	}

	s7 := newStore(t)
	a7 := setup(t, s7)
	if _, err := s7.ImportGEDCOM(ctx, a7, parseFixture(t, "gedcom7.ged"), ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	out7 := exportText(t, s7, a7, ExportOptions{Version7: true})
	for _, want := range []string{"2 DATE JULIAN 4 OCT 1582", "2 ASSO @I", "3 ROLE GODP", "1 SEX X", "1 _HAIR brown"} {
		if !strings.Contains(out7, want) {
			t.Errorf("7.0 export of the 7.0 fixture lacks %q", want)
		}
	}
	out5 := exportText(t, s7, a7, ExportOptions{})
	if !strings.Contains(out5, "2 DATE @#DJULIAN@ 4 OCT 1582") || !strings.Contains(out5, "1 ASSO @I") || !strings.Contains(out5, "1 SEX U") {
		t.Errorf("5.5.1 export of the 7.0 fixture:\n%s", out5)
	}
}

func TestExportPrivacy(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	old := mustPerson(t, s, a, "Old", "Weber")
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &old.ID, Type: "BIRT", Date: "1850"}); err != nil {
		t.Fatal(err)
	}
	young, err := s.AddRelative(ctx, a, old.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Young", Surname: "Weber", Notes: "private note"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &young.Person.ID, Type: "BIRT", Date: "1990", Description: "secret"}); err != nil {
		t.Fatal(err)
	}

	all := exportText(t, s, a, ExportOptions{Privacy: PrivacyAll})
	if !strings.Contains(all, "Young /Weber/") || !strings.Contains(all, "private note") {
		t.Error("full export must contain everyone")
	}

	excluded := exportText(t, s, a, ExportOptions{Privacy: PrivacyExcludeLiving})
	if strings.Contains(excluded, "Young") || strings.Contains(excluded, "CHIL") {
		t.Errorf("living person leaked:\n%s", excluded)
	}

	nameOnly := exportText(t, s, a, ExportOptions{Privacy: PrivacyLivingNameOnly})
	if !strings.Contains(nameOnly, "Young /Weber/") || strings.Contains(nameOnly, "private note") || strings.Contains(nameOnly, "1990") {
		t.Errorf("name-only export:\n%s", nameOnly)
	}
	if !strings.Contains(nameOnly, "1 CHIL @I") {
		t.Error("name-only keeps the family link")
	}

	if _, err := s.ExportGEDCOM(ctx, a, ExportOptions{Privacy: "everyone"}); err == nil {
		t.Error("unknown privacy mode accepted")
	}
}

func TestExportMedia(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Anna", "")
	m, _, err := s.CreateMedia(ctx, a, newMedia(0), &MediaLinkInput{EntityType: "person", EntityID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	ex, err := s.ExportGEDCOM(ctx, a, ExportOptions{Version7: true, WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Media) != 1 || ex.Media[0].Path != "media/"+m.SHA256+".jpg" {
		t.Fatalf("media files: %+v", ex.Media)
	}
	var buf bytes.Buffer
	_ = gedcom.Write(&buf, ex.Records, gedcom.WriteOptions{Version7: true})
	out := buf.String()
	if !strings.Contains(out, "1 OBJE @O") || !strings.Contains(out, "1 FILE media/"+m.SHA256+".jpg") || !strings.Contains(out, "2 FORM image/jpeg") {
		t.Errorf("media records:\n%s", out)
	}
	if plain := exportText(t, s, a, ExportOptions{}); strings.Contains(plain, "OBJE") {
		t.Error("a plain GEDCOM file has no media records")
	}
}

// deadPerson creates a deceased person born in 1850.
func deadPerson(t *testing.T, s *Store, a Actor, given string) Person {
	t.Helper()
	p := mustPerson(t, s, a, given, "Weber")
	for _, in := range []EventInput{{Type: "BIRT", Date: "1850"}, {Type: "DEAT", Date: "1910"}} {
		in.PersonID = &p.ID
		if _, err := s.CreateEvent(ctx, a, in); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestExportExcludeLivingHidesFamilyEvents(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	old := deadPerson(t, s, a, "Old")
	rel, err := s.AddRelative(ctx, a, old.ID, RelativeInput{Relation: RelationPartner, Person: &PersonInput{GivenNames: "Young", Surname: "Weber"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &rel.Person.ID, Type: "BIRT", Date: "1990"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &rel.Family.ID, Type: "MARR", Date: "1 JAN 2015", Description: "Secret Town"}); err != nil {
		t.Fatal(err)
	}

	if all := exportText(t, s, a, ExportOptions{Privacy: PrivacyAll}); !strings.Contains(all, "Secret Town") {
		t.Fatalf("full export must contain the marriage:\n%s", all)
	}
	for _, mode := range []string{PrivacyExcludeLiving, PrivacyLivingNameOnly} {
		out := exportText(t, s, a, ExportOptions{Privacy: mode})
		if strings.Contains(out, "MARR") || strings.Contains(out, "Secret Town") || strings.Contains(out, "2015") {
			t.Errorf("%s: family event of a living partner leaked:\n%s", mode, out)
		}
	}
}

func TestExportExcludeLivingSkipsMedia(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	old := deadPerson(t, s, a, "Old")
	rel, err := s.AddRelative(ctx, a, old.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Young", Surname: "Weber"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &rel.Person.ID, Type: "BIRT", Date: "1990"}); err != nil {
		t.Fatal(err)
	}
	kept, _, err := s.CreateMedia(ctx, a, newMedia(1), &MediaLinkInput{EntityType: "person", EntityID: old.ID})
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := s.CreateMedia(ctx, a, newMedia(2), &MediaLinkInput{EntityType: "person", EntityID: rel.Person.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Linked to both: stays, an exported record references it.
	shared, _, err := s.CreateMedia(ctx, a, newMedia(3), &MediaLinkInput{EntityType: "person", EntityID: old.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkMedia(ctx, a, shared.ID, MediaLinkInput{EntityType: "person", EntityID: rel.Person.ID}); err != nil {
		t.Fatal(err)
	}

	ex, err := s.ExportGEDCOM(ctx, a, ExportOptions{Privacy: PrivacyExcludeLiving, WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range ex.Media {
		got[m.SHA256] = true
	}
	if !got[kept.SHA256] || !got[shared.SHA256] || got[secret.SHA256] || len(ex.Media) != 2 {
		t.Errorf("media files: %+v", ex.Media)
	}
	var buf bytes.Buffer
	_ = gedcom.Write(&buf, ex.Records, gedcom.WriteOptions{})
	if strings.Contains(buf.String(), secret.SHA256) || strings.Count(buf.String(), "@ OBJE") != 2 {
		t.Errorf("media records leaked:\n%s", buf.String())
	}
}

func TestExportExcludeLivingSkipsPortraitAndEventMedia(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	old := deadPerson(t, s, a, "Old")
	rel, err := s.AddRelative(ctx, a, old.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Young", Surname: "Weber"}})
	if err != nil {
		t.Fatal(err)
	}
	young := rel.Person.ID
	birth, err := s.CreateEvent(ctx, a, EventInput{PersonID: &young, Type: "BIRT", Date: "1990"})
	if err != nil {
		t.Fatal(err)
	}
	// A portrait that was removed from the gallery but is still the portrait.
	portrait, _, err := s.CreateMedia(ctx, a, newMedia(1), &MediaLinkInput{EntityType: "person", EntityID: young})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPortrait(ctx, a, young, PortraitInput{MediaID: &portrait.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.UnlinkMedia(ctx, a, portrait.ID, "person", young); err != nil {
		t.Fatal(err)
	}
	// Media of an event of the living person.
	scan, _, err := s.CreateMedia(ctx, a, newMedia(2), &MediaLinkInput{EntityType: "event", EntityID: birth.ID})
	if err != nil {
		t.Fatal(err)
	}
	// Media of the deceased person's portrait stays.
	kept, _, err := s.CreateMedia(ctx, a, newMedia(3), &MediaLinkInput{EntityType: "person", EntityID: old.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetPortrait(ctx, a, old.ID, PortraitInput{MediaID: &kept.ID}); err != nil {
		t.Fatal(err)
	}

	ex, err := s.ExportGEDCOM(ctx, a, ExportOptions{Privacy: PrivacyExcludeLiving, WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Media) != 1 || ex.Media[0].SHA256 != kept.SHA256 {
		t.Errorf("media files: %+v", ex.Media)
	}
	all, err := s.ExportGEDCOM(ctx, a, ExportOptions{Privacy: PrivacyAll, WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Media) != 3 {
		t.Errorf("full export media: %+v (portrait %s, scan %s)", all.Media, portrait.SHA256, scan.SHA256)
	}
}

func TestExportNameOnlySkipsPortraitMedia(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	old := deadPerson(t, s, a, "Old")
	rel, err := s.AddRelative(ctx, a, old.ID, RelativeInput{Relation: RelationChild, Person: &PersonInput{GivenNames: "Young", Surname: "Weber"}})
	if err != nil {
		t.Fatal(err)
	}
	young := rel.Person.ID
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &young, Type: "BIRT", Date: "1990"}); err != nil {
		t.Fatal(err)
	}
	// One portrait still in the gallery, one removed from it.
	for i, unlink := range []bool{false, true} {
		m, _, err := s.CreateMedia(ctx, a, newMedia(byte(i+1)), &MediaLinkInput{EntityType: "person", EntityID: young})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.SetPortrait(ctx, a, young, PortraitInput{MediaID: &m.ID}); err != nil {
			t.Fatal(err)
		}
		if unlink {
			if err := s.UnlinkMedia(ctx, a, m.ID, "person", young); err != nil {
				t.Fatal(err)
			}
		}
	}
	kept, _, err := s.CreateMedia(ctx, a, newMedia(3), &MediaLinkInput{EntityType: "person", EntityID: old.ID})
	if err != nil {
		t.Fatal(err)
	}

	ex, err := s.ExportGEDCOM(ctx, a, ExportOptions{Privacy: PrivacyLivingNameOnly, WithMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Media) != 1 || ex.Media[0].SHA256 != kept.SHA256 {
		t.Errorf("media files: %+v", ex.Media)
	}
}

func TestVerifyExport(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	if _, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "ancestry-551.ged"), ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	for _, v7 := range []bool{false, true} {
		r, err := s.VerifyExport(ctx, a, v7, "test")
		if err != nil {
			t.Fatal(err)
		}
		if !r.OK {
			t.Errorf("v7=%v not lossless: %+v dropped %+v", v7, r.Rows, r.Dropped)
		}
	}
}
