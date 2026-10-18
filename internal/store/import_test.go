package store

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praetorianer777/gotree/internal/gedcom"
)

func parseFixture(t *testing.T, name string) *gedcom.Document {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := gedcom.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func findPerson(t *testing.T, s *Store, a Actor, q string) PersonDetail {
	t.Helper()
	list, err := s.ListPersons(ctx, a, q, 5, 0)
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("find %q: %+v %v", q, list, err)
	}
	d, err := s.GetPersonDetail(ctx, a, list.Items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func tagOutcome(r ImportReport, path string) string {
	for _, t := range r.Tags {
		if t.Path == path {
			return t.Outcome
		}
	}
	return ""
}

func TestImportChildOnlyInFAMC(t *testing.T) {
	// Interpreted strings: the trailing space after Otto's FAMC matters.
	src := strings.Join([]string{
		"0 HEAD", "1 GEDC", "2 VERS 5.5.1", "1 CHAR UTF-8",
		"0 @I1@ INDI", "1 NAME Karl /Weber/", "1 SEX M", "1 FAMS @F1@", "1 FAMC @F1@",
		"0 @I2@ INDI", "1 NAME Maria /Weber/", "1 SEX F", "1 FAMS @F1@",
		"0 @I3@ INDI", "1 NAME Paul /Weber/", "1 FAMC @F1@",
		"0 @I4@ INDI", "1 NAME Lena /Weber/", "1 FAMC @F1@", "2 PEDI adopted", "1 FAMC @F1@",
		"0 @I5@ INDI", "1 NAME Otto /Weber/", "1 FAMC @F1@ ",
		"0 @I7@ INDI", "1 NAME Nora /Weber/", "1 FAMC @F1@", "1 FAMC @F1@", "2 PEDI adopted",
		"0 @I6@ INDI", "1 NAME Emil /Weber/", "1 FAMC @F9@",
		"0 @F1@ FAM", "1 HUSB @I1@", "1 WIFE @I2@", "1 CHIL @I3@",
		"0 TRLR",
	}, "\n")
	doc, err := gedcom.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	a := setup(t, s)
	report, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts["families"] != 1 {
		t.Errorf("families: %v", report.Counts)
	}
	karl := findPerson(t, s, a, "karl")
	if len(karl.PartnerFamilies) != 1 {
		t.Fatalf("Karl's families: %+v", karl.PartnerFamilies)
	}
	var names []string
	rels := map[string][2]string{}
	for _, c := range karl.PartnerFamilies[0].Children {
		names = append(names, c.Person.GivenNames)
		rels[c.Person.GivenNames] = [2]string{c.RelationPartner1, c.RelationPartner2}
	}
	if got := strings.Join(names, ","); got != "Paul,Lena,Otto,Nora" {
		t.Errorf("children %q, want Paul,Lena,Otto,Nora", got)
	}
	if rels["Lena"] != [2]string{"adopted", "adopted"} || rels["Nora"] != [2]string{"adopted", "adopted"} || rels["Otto"] != [2]string{"birth", "birth"} {
		t.Errorf("relations: %v", rels)
	}
	if lena := findPerson(t, s, a, "lena"); len(lena.ParentFamilies) != 1 {
		t.Errorf("Lena's parent families: %+v", lena.ParentFamilies)
	}
	if report.ChildrenFromFAMC != 3 || len(report.ChildrenFromFAMCExamples) != 3 {
		t.Fatalf("FAMC children: %d %v", report.ChildrenFromFAMC, report.ChildrenFromFAMCExamples)
	}
	ex := strings.Join(report.ChildrenFromFAMCExamples, "\n")
	for _, want := range []string{"@I4@", "@I5@", "@F1@"} {
		if !strings.Contains(ex, want) {
			t.Errorf("examples miss %s: %s", want, ex)
		}
	}
	if strings.Contains(ex, "@I3@") {
		t.Errorf("Paul is listed under CHIL and must not be reported: %s", ex)
	}
	broken := strings.Join(report.BrokenReferences, "\n")
	if !strings.Contains(broken, "@F9@") || !strings.Contains(broken, "both a partner and a child of family @F1@") {
		t.Errorf("broken references: %s", broken)
	}
	if tagOutcome(report, "INDI.FAMC") != TagMapped {
		t.Errorf("INDI.FAMC outcome %q", tagOutcome(report, "INDI.FAMC"))
	}
}

func TestImportAncestry551(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	report, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "ancestry-551.ged"), ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"persons": 4, "families": 1, "events": 7, "sources": 2, "repositories": 1, "citations": 3, "places": 3}
	for k, v := range want {
		if report.Counts[k] != v {
			t.Errorf("count %s = %d, want %d (all: %v)", k, report.Counts[k], v, report.Counts)
		}
	}
	if report.Source != "Ancestry.com Member Trees" || report.Version != "5.5.1" {
		t.Errorf("header: %+v", report)
	}
	if report.ChildrenFromFAMC != 0 || report.ChildrenFromFAMCExamples == nil {
		t.Errorf("FAMC children: %d %v", report.ChildrenFromFAMC, report.ChildrenFromFAMCExamples)
	}

	johann := findPerson(t, s, a, "johann weber")
	if johann.Sex != "M" || !strings.Contains(johann.Notes, "Zweite Zeile der Notiz") {
		t.Errorf("Johann: sex %q notes %q", johann.Sex, johann.Notes)
	}
	var birth, occu, even, death *Event
	for i := range johann.Events {
		e := &johann.Events[i]
		switch e.Type {
		case "BIRT":
			birth = e
		case "OCCU":
			occu = e
		case "EVEN":
			even = e
		case "DEAT":
			death = e
		}
	}
	if birth == nil || birth.Date.Normalized != "3 FEB 1855" || birth.Place == nil || birth.Place.FullName != "Leipzig, Sachsen, Deutschland" {
		t.Fatalf("birth: %+v", birth)
	}
	if len(birth.Citations) != 1 || birth.Citations[0].Page != "S. 12, Nr. 34" || *birth.Citations[0].Quality != 3 {
		t.Errorf("birth citation: %+v", birth.Citations)
	}
	if occu == nil || occu.Description != "Weber" || occu.Date.Normalized != "ABT 1880" {
		t.Errorf("occupation: %+v", occu)
	}
	if even == nil || even.CustomLabel != "Auswanderungsversuch" {
		t.Errorf("custom event: %+v", even)
	}
	// "12 Dezember 1920" is not GEDCOM: kept as written and reported.
	if death == nil || death.Date.Raw != "12 Dezember 1920" || death.Date.Valid {
		t.Errorf("death: %+v", death)
	}
	if report.InvalidDates != 1 || !strings.Contains(report.InvalidDateExamples[0], "12 Dezember 1920") {
		t.Errorf("invalid dates: %d %v", report.InvalidDates, report.InvalidDateExamples)
	}

	place, err := s.GetPlace(ctx, a, birth.Place.ID)
	if err != nil || place.Lat == nil || *place.Lat != 51.3397 || *place.Lng != 12.3731 {
		t.Errorf("place coordinates: %+v %v", place, err)
	}

	src, err := s.GetSource(ctx, a, birth.Citations[0].SourceID)
	if err != nil || src.Repository == nil || src.Repository.Name != "Stadtarchiv Leipzig" || src.CallNumber != "KB 12" {
		t.Errorf("source: %+v %v", src, err)
	}
	if src.Repository != nil && (src.Repository.URL != "https://www.leipzig.de/stadtarchiv" || !strings.Contains(src.Repository.Address, "Leipzig")) {
		t.Errorf("repository: %+v", src.Repository)
	}

	maria := findPerson(t, s, a, "maria")
	if len(maria.AlternateNames) != 1 || maria.AlternateNames[0].Type != "married" || maria.AlternateNames[0].Surname != "Weber" {
		t.Errorf("married name: %+v", maria.AlternateNames)
	}
	// The inline source of her birth became a source of its own.
	if len(maria.Events[0].Citations) != 1 || !strings.Contains(maria.Events[0].Citations[0].SourceTitle, "Familienbibel") {
		t.Errorf("inline source: %+v", maria.Events[0].Citations)
	}

	fam := johann.PartnerFamilies[0]
	if fam.UnionType != "married" || fam.Partner2 == nil || fam.Partner2.GivenNames != "Maria" || len(fam.Citations) != 0 {
		t.Errorf("family: %+v", fam)
	}
	if len(fam.Events) != 1 || len(fam.Events[0].Citations) != 1 {
		t.Errorf("marriage with citation: %+v", fam.Events)
	}
	rel := map[string][2]string{}
	for _, c := range fam.Children {
		rel[c.Person.GivenNames] = [2]string{c.RelationPartner1, c.RelationPartner2}
	}
	if rel["Paul"] != [2]string{"birth", "step"} || rel["Lena"] != [2]string{"adopted", "adopted"} {
		t.Errorf("child relations (_FREL/_MREL and PEDI): %v", rel)
	}

	for path, outcome := range map[string]string{
		"INDI.BIRT.DATE":       TagMapped,
		"INDI._UID":            TagKept,
		"INDI.OBJE":            TagKept,
		"INDI.CHAN":            TagDropped,
		"INDI.BIRT.SOUR._APID": TagKept,
		"SOUR._APID":           TagKept,
		"SUBM":                 TagDropped,
		"FAM.CHIL._FREL":       TagMapped,
		"INDI.BIRT.PLAC.MAP":   TagMapped,
	} {
		if got := tagOutcome(report, path); got != outcome {
			t.Errorf("tag %s: %q, want %q", path, got, outcome)
		}
	}

	// Kept lines are stored verbatim with the record.
	var extra string
	if err := s.DB.QueryRow(`SELECT extra_json FROM persons WHERE gedcom_xref = 'I1'`).Scan(&extra); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(extra, `"t":"_UID"`) || !strings.Contains(extra, "6B5C0D1E2F3A4B5C") || !strings.Contains(extra, "abc.jpg") {
		t.Errorf("extra_json: %s", extra)
	}
}

func TestImportGedcom7(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	report, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "gedcom7.ged"), ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if report.Version != "7.0" || len(report.BrokenReferences) != 0 {
		t.Errorf("report: %+v", report)
	}
	alex := findPerson(t, s, a, "alex")
	if alex.Sex != "X" || alex.Notes != "Shared note from GEDCOM 7." {
		t.Errorf("Alex: %+v", alex.Person)
	}
	if len(alex.AlternateNames) != 1 || alex.AlternateNames[0].Type != "birth" || alex.AlternateNames[0].GivenNames != "Alexandra" {
		t.Errorf("names: %+v", alex.AlternateNames)
	}
	bapm := alex.Events[0]
	if bapm.Date.Normalized != "@#DJULIAN@ 4 OCT 1582" || len(bapm.Participants) != 1 || bapm.Participants[0].Role != "godparent" {
		t.Errorf("baptism: %+v", bapm)
	}
	if tagOutcome(report, "INDI._HAIR") != TagKept || tagOutcome(report, "INDI.EXID") != TagKept {
		t.Errorf("extension and EXID should be kept: %+v", report.Tags)
	}
}

func TestImportBrokenFile(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	report, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "broken.ged"), ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if report.InvalidDates != 2 {
		t.Errorf("invalid dates: %d", report.InvalidDates)
	}
	joined := strings.Join(report.BrokenReferences, "\n")
	if !strings.Contains(joined, "@I999@") || !strings.Contains(joined, "both a partner and a child") {
		t.Errorf("broken references: %v", report.BrokenReferences)
	}
	if tagOutcome(report, "_WEIRD") != TagDropped || tagOutcome(report, "FAM._CUSTOM") != TagKept {
		t.Errorf("tags: %+v", report.Tags)
	}
}

func TestImportModes(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	mustPerson(t, s, a, "Existing", "")
	doc := parseFixture(t, "gedcom7.ged")
	if _, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty); !errors.Is(err, ErrTreeNotEmpty) {
		t.Errorf("import into a non-empty tree: %v", err)
	}
	if _, err := s.ImportGEDCOM(ctx, a, doc, "merge"); err == nil {
		t.Error("unknown mode accepted")
	}
	if _, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "ancestry-551.ged"), ImportIntoEmpty); !errors.Is(err, ErrTreeNotEmpty) {
		t.Error("expected refusal")
	}
	if _, err := s.ImportGEDCOM(ctx, a, parseFixture(t, "ancestry-551.ged"), ImportReplace); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportGEDCOM(ctx, a, doc, ImportReplace); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListPersons(ctx, a, "", 50, 0)
	if list.Total != 2 {
		t.Errorf("after replacing twice: %d people", list.Total)
	}
	var places, sources int
	_ = s.DB.QueryRow(`SELECT (SELECT count(*) FROM places), (SELECT count(*) FROM sources)`).Scan(&places, &sources)
	if places != 0 || sources != 0 {
		t.Errorf("replace left %d places and %d sources", places, sources)
	}
	var logged int
	_ = s.DB.QueryRow(`SELECT count(*) FROM change_log WHERE entity_type = 'import'`).Scan(&logged)
	if logged != 2 {
		t.Errorf("imports logged: %d", logged)
	}
}

func TestImportLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("large import")
	}
	var b strings.Builder
	b.WriteString("0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n")
	const people = 10000
	for i := 1; i <= people; i++ {
		fmt.Fprintf(&b, "0 @I%d@ INDI\n1 NAME Person%d /Family%d/\n1 SEX %s\n1 BIRT\n2 DATE %d\n2 PLAC Town%d, Region%d, Country\n1 DEAT\n2 DATE %d\n",
			i, i, i%300, []string{"M", "F"}[i%2], 1700+i%250, i%400, i%20, 1760+i%250)
	}
	for i := 1; i+2 <= people; i += 3 {
		fmt.Fprintf(&b, "0 @F%d@ FAM\n1 HUSB @I%d@\n1 WIFE @I%d@\n1 CHIL @I%d@\n1 MARR\n2 DATE 1750\n", i, i, i+1, i+2)
	}
	b.WriteString("0 TRLR\n")
	doc, err := gedcom.Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}

	s := newStore(t)
	s.Now = time.Now
	a := setup(t, s)
	started := time.Now()
	report, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	took := time.Since(started)
	t.Logf("imported %d people, %d events, %d places in %v", report.Counts["persons"], report.Counts["events"], report.Counts["places"], took)
	if report.Counts["persons"] != people || report.Counts["events"] != 2*people+people/3 {
		t.Errorf("counts: %v", report.Counts)
	}
	if took > 30*time.Second {
		t.Errorf("import too slow: %v", took)
	}
	if res, _ := s.ListPersons(ctx, a, "person4242", 5, 0); res.Total != 1 {
		t.Errorf("search after import: %+v", res)
	}
}

func TestImportStatusFromFile(t *testing.T) {
	src := strings.Join([]string{
		"0 HEAD", "1 GEDC", "2 VERS 5.5.1", "1 CHAR UTF-8",
		"0 @I1@ INDI",
		"1 NAME Anna /Weber/", "2 _STAT disputed", "3 NOTE main name doubted",
		"1 NAME Hanna /Weber/", "2 TYPE aka", "2 _STAT DISPROVEN",
		"1 BIRT", "2 DATE 1850", "2 _STAT disputed",
		"1 DEAT", "2 DATE 1900", "2 _STAT probable",
		"1 BURI", "2 _STAT disputed", "3 NOTE @N1@",
		"0 @N1@ NOTE Shared reason",
		"0 TRLR", "",
	}, "\n")
	doc, err := gedcom.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	a := setup(t, s)
	report, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	got := findPerson(t, s, a, "anna")
	type st struct{ status, reason string }
	byType := map[string]st{}
	for _, e := range got.Events {
		byType[e.Type] = st{e.Status, e.StatusReason}
	}
	want := map[string]st{"BIRT": {"disputed", "?"}, "DEAT": {"accepted", ""}, "BURI": {"disputed", "Shared reason"}}
	for typ, w := range want {
		if byType[typ] != w {
			t.Errorf("%s: %+v, want %+v", typ, byType[typ], w)
		}
	}
	if tagOutcome(report, "INDI.DEAT._STAT") != TagKept {
		t.Errorf("unknown status: %q, want kept", tagOutcome(report, "INDI.DEAT._STAT"))
	}
	if len(got.AlternateNames) != 1 || got.AlternateNames[0].Status != "disproven" || got.AlternateNames[0].StatusReason != "?" {
		t.Errorf("alternate names: %+v", got.AlternateNames)
	}
	dropped := false
	for _, tg := range report.Tags {
		if tg.Path == "INDI.NAME._STAT" && tg.Outcome == TagDropped {
			dropped = true
		}
	}
	if !dropped {
		t.Errorf("the main name's status is not reported as dropped: %+v", report.Tags)
	}
	text := exportText(t, s, a, ExportOptions{})
	if strings.Contains(text, "main name doubted") || strings.Contains(text, "\n1 _STAT") {
		t.Errorf("the main name's status moved to INDI:\n%s", text)
	}
	if !strings.Contains(text, "1 DEAT\n2 DATE 1900\n2 _STAT probable") {
		t.Errorf("the unknown status was not kept under DEAT:\n%s", text)
	}
}

func TestImportRolePhrase(t *testing.T) {
	src := strings.Join([]string{
		"0 HEAD", "1 GEDC", "2 VERS 7.0",
		"0 @I1@ INDI", "1 NAME Anna /Weber/",
		"1 BIRT",
		"2 ASSO @I2@", "3 ROLE OTHER", "4 PHRASE Informant",
		"2 ASSO @I3@", "3 ROLE OTHER", "4 PHRASE Lodger",
		"2 ASSO @I4@", "3 ROLE OTHER",
		"2 ASSO @I5@", "3 ROLE WITN", "4 PHRASE chief witness",
		"0 @I2@ INDI", "1 NAME Bert /Weber/",
		"0 @I3@ INDI", "1 NAME Clara /Weber/",
		"0 @I4@ INDI", "1 NAME Dora /Weber/",
		"0 @I5@ INDI", "1 NAME Emil /Weber/",
		"0 TRLR", "",
	}, "\n")
	doc, err := gedcom.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	a := setup(t, s)
	if _, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	type rc struct{ role, custom string }
	got := map[string]rc{}
	for _, e := range findPerson(t, s, a, "anna").Events {
		for _, p := range e.Participants {
			got[p.Person.GivenNames] = rc{p.Role, p.CustomRole}
		}
	}
	want := map[string]rc{
		"Bert": {"informant", ""}, "Clara": {"other", "Lodger"}, "Dora": {"other", ""}, "Emil": {"witness", ""},
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s: %+v, want %+v", name, got[name], w)
		}
	}
}
