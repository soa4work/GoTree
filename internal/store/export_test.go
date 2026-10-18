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

func TestExportGedcom7DatePhraseRoundTrip(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	p, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Anna", Surname: "Weber", Sex: "F"})
	if err != nil {
		t.Fatal(err)
	}
	dates := []struct{ typ, in, want string }{
		{"BIRT", "Frühjahr 1850", "(Frühjahr 1850)"},
		{"BAPM", "INT 1850 (nach dem Kirchenbuch)", "INT 1850 (nach dem Kirchenbuch)"},
		{"DEAT", "12 FEB 1750/51", "12 FEB 1750/51"},
		{"BURI", "BET 1750/51 AND 1760", "BET 1750/51 AND 1760"},
		{"CHR", "INT 1750/51", "1750/51"},
		{"RESI", "29 FEB 1751/52", "29 FEB 1751/52"},
	}
	for i, d := range dates[:3] {
		if _, err := s.CreateHeirloom(ctx, a, HeirloomInput{Name: fmt.Sprintf("Heirloom %d", i), MadeDate: d.in}); err != nil {
			t.Fatalf("heirloom %s: %v", d.in, err)
		}
	}
	for _, d := range dates {
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: d.typ, Date: d.in}); err != nil {
			t.Fatalf("%s: %v", d.in, err)
		}
	}
	text := exportText(t, s, a, ExportOptions{Version7: true})
	if !strings.Contains(text, "3 PHRASE ") {
		t.Fatalf("7.0 export has no DATE.PHRASE:\n%s", text)
	}

	s2, a2 := reimport(t, text)
	got := findPerson(t, s2, a2, "anna")
	byType := map[string]DateValue{}
	for _, e := range got.Events {
		byType[e.Type] = e.Date
	}
	for _, d := range dates {
		if byType[d.typ].Raw != d.want {
			t.Errorf("%s: %q came back as %q, want %q", d.typ, d.in, byType[d.typ].Raw, d.want)
		}
	}
	made := map[string]string{}
	list, err := s2.ListHeirlooms(ctx, a2, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range list {
		made[h.Name] = h.MadeDate
	}
	for i, d := range dates[:3] {
		if got := made[fmt.Sprintf("Heirloom %d", i)]; got != d.want {
			t.Errorf("heirloom %q came back as %q, want %q", d.in, got, d.want)
		}
	}
	if !byType["DEAT"].Valid || byType["BAPM"].Qualifier != "INT" || !byType["CHR"].Valid {
		t.Errorf("dates lost their meaning: %+v", byType)
	}
}

// TestExportIntDualYear551 pins that an interpreted date with a dual year
// keeps both in 5.5.1, and that 7.0 keeps the dual year in a PHRASE.
func TestExportIntDualYear551(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Anna", "Weber")
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "CHR", Date: "INT 1750/51"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateHeirloom(ctx, a, HeirloomInput{Name: "Clock", MadeDate: "INT 1750/51"}); err != nil {
		t.Fatal(err)
	}

	text := exportText(t, s, a, ExportOptions{})
	for _, want := range []string{"2 DATE INT 1750/51\n", "1 DATE INT 1750/51\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("5.5.1 export lacks %q:\n%s", want, text)
		}
	}
	s2, a2 := reimport(t, text)
	got := findPerson(t, s2, a2, "anna")
	var chr *Event
	for i := range got.Events {
		if got.Events[i].Type == "CHR" {
			chr = &got.Events[i]
		}
	}
	if chr == nil || chr.Date.Raw != "INT 1750/51" || chr.Date.Qualifier != "INT" {
		t.Errorf("5.5.1 CHR came back as %+v", chr)
	}
	list, err := s2.ListHeirlooms(ctx, a2, "", 0)
	if err != nil || len(list) != 1 || list[0].MadeDate != "INT 1750/51" {
		t.Errorf("5.5.1 heirloom: %+v %v", list, err)
	}

	text7 := exportText(t, s, a, ExportOptions{Version7: true})
	if !strings.Contains(text7, "1 DATE 1751\n2 PHRASE 1750/51\n") {
		t.Errorf("7.0 heirloom made date lacks its dual year:\n%s", text7)
	}
	s3, a3 := reimport(t, text7)
	list, err = s3.ListHeirlooms(ctx, a3, "", 0)
	if err != nil || len(list) != 1 || list[0].MadeDate != "1750/51" {
		t.Errorf("7.0 heirloom: %+v %v", list, err)
	}
}

func TestExportKeptDatePhrase(t *testing.T) {
	src := "0 HEAD\n1 GEDC\n2 VERS 7.0\n0 @I1@ INDI\n1 NAME Anna /Weber/\n1 BIRT\n2 DATE ABT 1900\n3 PHRASE during the war\n0 TRLR\n"
	s, a := reimport(t, src)
	v7 := exportText(t, s, a, ExportOptions{Version7: true})
	if !strings.Contains(v7, "2 DATE ABT 1900\n3 PHRASE during the war\n") || strings.Contains(v7, "\n2 PHRASE") {
		t.Errorf("7.0 export:\n%s", v7)
	}
	v5 := exportText(t, s, a, ExportOptions{})
	if !strings.Contains(v5, "2 NOTE Date phrase: during the war\n") || strings.Contains(v5, "PHRASE") {
		t.Errorf("5.5.1 export:\n%s", v5)
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
	pa, pb, pw := mustPerson(t, s, a, "Anna", "Verify"), mustPerson(t, s, a, "Hans", "Verify"), mustPerson(t, s, a, "Karl", "Verify")
	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &pa.ID, Partner2ID: &pb.ID, UnionType: "married"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam.ID, Type: "MARR", Date: "1880",
		Participants: []ParticipantInput{{PersonID: pw.ID, Role: "witness", Notes: "best man"}}}); err != nil {
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

func TestExportStatusRoundTrip(t *testing.T) {
	for _, v7 := range []bool{false, true} {
		t.Run(fmt.Sprintf("v7=%v", v7), func(t *testing.T) {
			s := newStore(t)
			a := setup(t, s)
			anna, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Anna", Surname: "Weber", Sex: "F",
				AlternateNames: []AlternateNameInput{{Type: "aka", GivenNames: "Hanna", Surname: "Weber",
					Status: "disputed", StatusReason: "only in one letter"}}})
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range []EventInput{
				{Type: "BIRT", Date: "1850", Status: "disproven", StatusReason: "wrong parish"},
				{Type: "DEAT", Date: "1900", Status: "disputed", StatusReason: "line one\nline two"},
				{Type: "OCCU", Description: "Weber"},
			} {
				e.PersonID = &anna.ID
				if _, err := s.CreateEvent(ctx, a, e); err != nil {
					t.Fatal(err)
				}
			}
			res, err := s.AddRelative(ctx, a, anna.ID, RelativeInput{Relation: RelationPartner,
				Person: &PersonInput{GivenNames: "Karl", Surname: "Weber"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &res.Family.ID, Type: "MARR", Date: "1875",
				Status: "disputed", StatusReason: "no record"}); err != nil {
				t.Fatal(err)
			}

			text := exportText(t, s, a, ExportOptions{Version7: v7})
			for _, want := range []string{"_STAT disproven", "_STAT disputed", "2 _STAT disputed\n3 NOTE only in one letter"} {
				if !strings.Contains(text, want) {
					t.Errorf("export lacks %q:\n%s", want, text)
				}
			}
			count := strings.Count(text, "\n2 _STAT ")
			if count != 4 {
				t.Errorf("export has %d _STAT lines, want 4:\n%s", count, text)
			}

			doc, err := gedcom.Parse([]byte(text))
			if err != nil {
				t.Fatal(err)
			}
			s2 := newStore(t)
			a2 := setup(t, s2)
			report, err := s2.ImportGEDCOM(ctx, a2, doc, ImportIntoEmpty)
			if err != nil {
				t.Fatal(err)
			}
			got := findPerson(t, s2, a2, "anna")
			type st struct{ status, reason string }
			byType := map[string]st{}
			for _, e := range got.Events {
				byType[e.Type] = st{e.Status, e.StatusReason}
			}
			for typ, want := range map[string]st{
				"BIRT": {"disproven", "wrong parish"},
				"DEAT": {"disputed", "line one\nline two"},
				"OCCU": {"accepted", ""},
			} {
				if byType[typ] != want {
					t.Errorf("%s came back as %+v, want %+v", typ, byType[typ], want)
				}
			}
			if len(got.AlternateNames) != 1 || got.AlternateNames[0].Status != "disputed" || got.AlternateNames[0].StatusReason != "only in one letter" {
				t.Errorf("alternate names: %+v", got.AlternateNames)
			}
			if len(got.PartnerFamilies) != 1 {
				t.Fatalf("families: %+v", got.PartnerFamilies)
			}
			marr := ""
			for _, e := range got.PartnerFamilies[0].Events {
				if e.Type == "MARR" && e.Date.Raw == "1875" {
					marr = e.Status + "/" + e.StatusReason
				}
			}
			if marr != "disputed/no record" {
				t.Errorf("MARR came back as %q", marr)
			}
			for _, path := range []string{"INDI.BIRT._STAT", "INDI.NAME._STAT", "FAM.MARR._STAT"} {
				if tagOutcome(report, path) != TagMapped {
					t.Errorf("%s: %q, want mapped", path, tagOutcome(report, path))
				}
			}

			again := exportText(t, s2, a2, ExportOptions{Version7: v7})
			if n := strings.Count(again, "\n2 _STAT "); n != count {
				t.Errorf("re-export has %d _STAT lines, want %d:\n%s", n, count, again)
			}
			if strings.Contains(again, "\n1 _STAT") {
				t.Errorf("a status moved to the INDI level:\n%s", again)
			}

			r, err := s.VerifyExport(ctx, a, v7, "test")
			if err != nil {
				t.Fatal(err)
			}
			if !r.OK {
				t.Errorf("verify: %+v %+v", r.Rows, r.Dropped)
			}
		})
	}
}

func TestExportParticipantRolesRoundTrip(t *testing.T) {
	roles := []struct{ given, role, custom string }{
		{"Bert", "head", ""}, {"Clara", "sibling", ""}, {"Dora", "relative", ""}, {"Emil", "informant", ""},
		{"Fritz", "other", "lodger"}, {"Gerd", "godparent", ""}, {"Hans", "witness", ""},
	}
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Weber")
	var parts []ParticipantInput
	for _, r := range roles {
		p := mustPerson(t, s, a, r.given, "Weber")
		parts = append(parts, ParticipantInput{PersonID: p.ID, Role: r.role, CustomRole: r.custom})
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &anna.ID, Type: "CENS", Date: "1900", Participants: parts}); err != nil {
		t.Fatal(err)
	}

	participants := func(s *Store, a Actor) map[string]Participant {
		got := map[string]Participant{}
		for _, e := range findPerson(t, s, a, "anna").Events {
			if e.Type == "CENS" {
				for _, p := range e.Participants {
					got[p.Person.GivenNames] = p
				}
			}
		}
		return got
	}

	t551 := exportText(t, s, a, ExportOptions{})
	for _, want := range []string{"2 RELA head (CENS)", "2 RELA sibling (CENS)", "2 RELA relative (CENS)", "2 RELA informant (CENS)", "2 RELA godparent (CENS)"} {
		if !strings.Contains(t551, want) {
			t.Errorf("5.5.1 export lacks %q:\n%s", want, t551)
		}
	}
	s1, a1 := reimport(t, t551)
	got551 := participants(s1, a1)
	for _, r := range roles {
		if got551[r.given].Role != r.role {
			t.Errorf("5.5.1: %s came back as %q, want %q", r.given, got551[r.given].Role, r.role)
		}
	}
	if got551["Fritz"].CustomRole != "lodger" {
		t.Errorf("5.5.1 lost the custom role: %+v", got551["Fritz"])
	}

	t7 := exportText(t, s, a, ExportOptions{Version7: true})
	for _, want := range []string{
		"3 ROLE OTHER\n4 PHRASE head", "3 ROLE OTHER\n4 PHRASE sibling", "3 ROLE OTHER\n4 PHRASE relative",
		"3 ROLE OTHER\n4 PHRASE informant", "3 ROLE OTHER\n4 PHRASE lodger", "3 ROLE GODP", "3 ROLE WITN",
	} {
		if !strings.Contains(t7, want) {
			t.Errorf("7.0 export lacks %q:\n%s", want, t7)
		}
	}
	s2, a2 := reimport(t, t7)
	got7 := participants(s2, a2)
	for _, r := range roles {
		if got7[r.given].Role != r.role || got7[r.given].CustomRole != r.custom {
			t.Errorf("7.0: %s came back as %q / %q, want %q / %q", r.given, got7[r.given].Role, got7[r.given].CustomRole, r.role, r.custom)
		}
	}
	if again := exportText(t, s2, a2, ExportOptions{Version7: true}); strings.Count(again, "ROLE OTHER\n4 PHRASE") != strings.Count(t7, "ROLE OTHER\n4 PHRASE") ||
		!strings.Contains(again, "3 ROLE OTHER\n4 PHRASE lodger") || !strings.Contains(again, "3 ROLE OTHER\n4 PHRASE head") {
		t.Errorf("7.0 round trip is not stable:\n%s", again)
	}
}

func TestExportNameTypeRoundTrip(t *testing.T) {
	for _, v7 := range []bool{false, true} {
		t.Run(fmt.Sprintf("v7=%v", v7), func(t *testing.T) {
			s := newStore(t)
			a := setup(t, s)
			if _, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Anna", Surname: "Weber",
				AlternateNames: []AlternateNameInput{
					{Type: "religious", GivenNames: "Anna Maria"},
					{Type: "other", GivenNames: "Anni"},
					{Type: "immigrant", GivenNames: "Ann", Surname: "Weaver"},
				}}); err != nil {
				t.Fatal(err)
			}
			s2, a2 := reimport(t, exportText(t, s, a, ExportOptions{Version7: v7}))
			got := map[string]string{}
			for _, n := range findPerson(t, s2, a2, "anna").AlternateNames {
				got[n.GivenNames] = n.Type
			}
			want := map[string]string{"Anna Maria": "religious", "Anni": "other", "Ann": "immigrant"}
			for name, typ := range want {
				if got[name] != typ {
					t.Errorf("%s came back as %q, want %q (all: %v)", name, got[name], typ, got)
				}
			}
		})
	}
}

func TestDatePhraseKeepsParentheses(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Frühjahr 1850", "(Frühjahr 1850)"},
		{"Frühjahr (1850)", "(Frühjahr (1850))"},
		{"(a) b", "((a) b)"},
		{"(during the war)", "(during the war)"},
	}
	for _, v7 := range []bool{false, true} {
		ex := &exporter{opt: ExportOptions{Version7: v7}}
		for _, tt := range tests {
			if got := ex.textDate(tt.in); got != tt.want {
				t.Errorf("v7=%v textDate(%q) = %q, want %q", v7, tt.in, got, tt.want)
			}
		}
	}
	n := node("BIRT", "")
	(&exporter{}).date(n, "Frühjahr (1850)")
	if got := n.Text("DATE"); got != "(Frühjahr (1850))" {
		t.Errorf("5.5.1 DATE = %q", got)
	}
}

func TestExportNamesRoundTrip(t *testing.T) {
	for _, v7 := range []bool{false, true} {
		t.Run(fmt.Sprintf("v7=%v", v7), func(t *testing.T) {
			s := newStore(t)
			a := setup(t, s)
			p, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Anna", Surname: "Weber", NameNotes: "spelled Maier",
				AlternateNames: []AlternateNameInput{{Type: "married", GivenNames: "Anna", Surname: "Schmidt", Notes: "after 1890"}}})
			if err != nil {
				t.Fatal(err)
			}
			src, err := s.CreateSource(ctx, a, SourceInput{Title: "Parish book"})
			if err != nil {
				t.Fatal(err)
			}
			cite := func(page string, links ...LinkInput) {
				t.Helper()
				if _, err := s.CreateCitation(ctx, a, CitationInput{SourceID: src.ID, Page: page, Links: links}); err != nil {
					t.Fatal(err)
				}
			}
			cite("main name", LinkInput{EntityType: "person", EntityID: p.ID, Field: "name"})
			cite("other name", LinkInput{EntityType: "name", EntityID: p.AlternateNames[0].ID})
			// Cites the name and the person: written once, on the person.
			cite("both", LinkInput{EntityType: "person", EntityID: p.ID, Field: "name"}, LinkInput{EntityType: "person", EntityID: p.ID})
			cite("whole", LinkInput{EntityType: "person", EntityID: p.ID})

			text := exportText(t, s, a, ExportOptions{Version7: v7})
			for _, want := range []string{"\n2 NOTE spelled Maier", "\n2 NOTE after 1890"} {
				if strings.Count(text, want) != 1 {
					t.Errorf("%q not written once:\n%s", want, text)
				}
			}
			first := text[strings.Index(text, "1 NAME Anna /Weber/"):]
			first = first[:strings.Index(first, "\n1 NAME Anna /Schmidt/")]
			if !strings.Contains(first, "2 NOTE spelled Maier") || strings.Count(first, "2 SOUR") != 1 {
				t.Errorf("the first NAME should have the note and one SOUR:\n%s", first)
			}
			if strings.Count(text, "\n1 SOUR @S") != 2 {
				t.Errorf("want two person-level SOUR lines:\n%s", text)
			}

			s2, a2 := reimport(t, text)
			got := findPerson(t, s2, a2, "anna")
			if got.NameNotes != "spelled Maier" || len(got.AlternateNames) != 1 || got.AlternateNames[0].Notes != "after 1890" {
				t.Errorf("notes after reimport: %q %+v", got.NameNotes, got.AlternateNames)
			}
			count := func(q string) int {
				var n int
				if err := s2.DB.QueryRow(q).Scan(&n); err != nil {
					t.Fatal(err)
				}
				return n
			}
			if n := count(`SELECT count(*) FROM citation_links WHERE entity_type = 'person' AND field = 'name'`); n != 1 {
				t.Errorf("person/name links: %d", n)
			}
			if n := count(`SELECT count(*) FROM citation_links WHERE entity_type = 'name' AND field = ''`); n != 1 {
				t.Errorf("name links: %d", n)
			}
			if n := count(`SELECT count(*) FROM citation_links WHERE entity_type = 'person' AND field = ''`); n != 2 {
				t.Errorf("person links: %d", n)
			}
			if n := count(`SELECT count(*) FROM citations`); n != 4 {
				t.Errorf("citations: %d, want 4", n)
			}

			// A living person exported as name only keeps neither.
			if _, err := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Living", Surname: "Person", NameNotes: "secret name note"}); err != nil {
				t.Fatal(err)
			}
			text = exportText(t, s, a, ExportOptions{Version7: v7, Privacy: PrivacyLivingNameOnly})
			if !strings.Contains(text, "Living /Person/") || strings.Contains(text, "secret name note") {
				t.Errorf("name-only export:\n%s", text)
			}
		})
	}
}

// participantSet lists a tree's participants as "event type|person|role|custom|notes".
func participantSet(t *testing.T, s *Store, a Actor) []string {
	t.Helper()
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.type, p.given_names, x.role, x.custom_role, x.notes
		FROM event_participants x JOIN events e ON e.id = x.event_id JOIN persons p ON p.id = x.person_id
		WHERE e.tree_id = ?`, a.TreeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ, given, role, custom, notes string
		if err := rows.Scan(&typ, &given, &role, &custom, &notes); err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Join([]string{typ, given, role, custom, notes}, "|"))
	}
	sort.Strings(out)
	return out
}

func TestExportParticipantsRoundTrip(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Weber")
	hans := mustPerson(t, s, a, "Hans", "Weber")
	person := func(given string) int64 { return mustPerson(t, s, a, given, "Weber").ID }
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &anna.ID, Type: "BAPM", Date: "1850", Participants: []ParticipantInput{
		{PersonID: person("Greta"), Role: "godparent", CustomRole: "Taufpate", Notes: "uncle"},
		{PersonID: person("Fritz"), Role: "other", CustomRole: "Trauzeuge"},
		{PersonID: person("Bert"), Role: "head"},
		{PersonID: person("Clara"), Role: "sibling", CustomRole: "twin"},
	}}); err != nil {
		t.Fatal(err)
	}
	for i, w := range []string{"Ida", "Jonas"} {
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &anna.ID, Type: "RESI", Date: fmt.Sprintf("%d", 1860+i*10),
			Participants: []ParticipantInput{{PersonID: person(w), Role: "witness"}}}); err != nil {
			t.Fatal(err)
		}
	}
	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &anna.ID, Partner2ID: &hans.ID, UnionType: "married"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam.ID, Type: "MARR", Date: "1880", Participants: []ParticipantInput{
		{PersonID: person("Karl"), Role: "witness", Notes: "best man"},
		{PersonID: person("Lena"), Role: "witness"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam.ID, Type: "MARR", Date: "1881", Participants: []ParticipantInput{
		{PersonID: person("Max"), Role: "officiant"},
	}}); err != nil {
		t.Fatal(err)
	}
	want := participantSet(t, s, a)

	for _, v7 := range []bool{false, true} {
		text := exportText(t, s, a, ExportOptions{Version7: v7})
		var must []string
		if v7 {
			must = []string{"3 ROLE GODP\n4 PHRASE Taufpate", "3 NOTE uncle", "3 ROLE OTHER\n4 PHRASE Trauzeuge", "4 PHRASE sibling: twin"}
		} else {
			must = []string{
				"RELA godparent: Taufpate (BAPM)", "2 NOTE uncle", "RELA other: Trauzeuge (BAPM)", "RELA witness (RESI 2)",
				fmt.Sprintf("RELA witness (MARR F%d)", fam.ID), fmt.Sprintf("RELA officiant (MARR F%d 2)", fam.ID), "RELA sibling: twin (BAPM)",
			}
		}
		for _, m := range must {
			if !strings.Contains(text, m) {
				t.Errorf("v7=%v: export lacks %q:\n%s", v7, m, text)
			}
		}
		s2, a2 := reimport(t, text)
		if got := participantSet(t, s2, a2); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("v7=%v: participants after the round trip:\n%s\nwant:\n%s", v7, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestExportFamilyWitnessesOnFirstPartner(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Weber")
	hans := mustPerson(t, s, a, "Hans", "Weber")
	karl := mustPerson(t, s, a, "Karl", "Weber")
	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &anna.ID, Partner2ID: &hans.ID, UnionType: "married"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam.ID, Type: "MARR", Date: "1880",
		Participants: []ParticipantInput{{PersonID: karl.ID, Role: "witness"}}}); err != nil {
		t.Fatal(err)
	}
	text := exportText(t, s, a, ExportOptions{})
	if strings.Count(text, "1 ASSO") != 1 || strings.Count(indiBlock(t, text, anna.ID), "1 ASSO") != 1 || strings.Contains(indiBlock(t, text, hans.ID), "ASSO") {
		t.Errorf("the witness belongs on the first partner only:\n%s", text)
	}
}

func TestExportFamilyWitnessesPrivacy(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	born := func(given string) Person {
		p := mustPerson(t, s, a, given, "Weber")
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "BIRT", Date: "1850"}); err != nil {
			t.Fatal(err)
		}
		return p
	}
	anna, karl := born("Anna"), born("Karl")
	hans := mustPerson(t, s, a, "Hans", "Weber") // no dates: living
	lena := mustPerson(t, s, a, "Lena", "Weber") // living
	fam, err := s.CreateFamily(ctx, a, FamilyInput{Partner1ID: &anna.ID, Partner2ID: &hans.ID, UnionType: "married"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{FamilyID: &fam.ID, Type: "MARR", Date: "1880",
		Participants: []ParticipantInput{{PersonID: karl.ID, Role: "witness", Notes: "best man"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &anna.ID, Type: "BAPM", Date: "1850",
		Participants: []ParticipantInput{{PersonID: lena.ID, Role: "godparent"}}}); err != nil {
		t.Fatal(err)
	}
	marr := fmt.Sprintf("(MARR F%d)", fam.ID)
	if text := exportText(t, s, a, ExportOptions{Privacy: PrivacyAll}); !strings.Contains(indiBlock(t, text, anna.ID), marr) {
		t.Errorf("full export lacks the family witness:\n%s", text)
	}
	for _, mode := range []string{PrivacyExcludeLiving, PrivacyLivingNameOnly} {
		text := exportText(t, s, a, ExportOptions{Privacy: mode})
		// The living partner takes the family's events, and so their witnesses.
		if strings.Contains(text, marr) || strings.Contains(text, "best man") {
			t.Errorf("%s: the witness of a hidden family event leaked:\n%s", mode, text)
		}
		lenaRef := fmt.Sprintf("ASSO @I%d@", lena.ID)
		if mode == PrivacyExcludeLiving && strings.Contains(text, lenaRef) {
			t.Errorf("%s: an association with an excluded person:\n%s", mode, text)
		}
	}
}
