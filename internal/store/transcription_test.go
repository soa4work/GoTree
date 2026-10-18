package store

import (
	"errors"
	"strings"
	"testing"
)

func row(line, role string, values map[string]string) TranscriptionRow {
	return TranscriptionRow{Line: line, Role: role, Values: values}
}

func mustSource(t *testing.T, s *Store, a Actor, title string) Source {
	t.Helper()
	src, err := s.CreateSource(ctx, a, SourceInput{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func TestCensusTranscription(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	src := mustSource(t, s, a, "Census 1900, Leipzig")
	// Already in the tree, spelled differently.
	johann, _ := s.CreatePerson(ctx, a, PersonInput{GivenNames: "Johann", Surname: "Maier", Sex: "M"})
	if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &johann.ID, Type: "BIRT", Date: "1861"}); err != nil {
		t.Fatal(err)
	}

	rows := []TranscriptionRow{
		row("12", "head", map[string]string{"given": "Johann", "surname": "Meyer", "sex": "m", "age": "39", "occupation": "Weber", "birthplace": "Leipzig, Sachsen"}),
		row("13", "spouse", map[string]string{"given": "Maria", "surname": "Meyer", "sex": "w", "age": "35", "birthplace": "Halle, Sachsen"}),
		row("14", "child", map[string]string{"given": "Paul", "surname": "Meyer", "sex": "m", "age": "8"}),
	}
	tr, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "census", SourceID: &src.ID, Page: "p. 4", Date: "1 DEC 1900", Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != "draft" || len(tr.Rows) != 3 || tr.Rows[0].Action != RowActionNew || tr.Source.Title != "Census 1900, Leipzig" {
		t.Fatalf("draft: %+v", tr)
	}

	cands, err := s.Match(ctx, a, MatchRequest{Date: tr.Date, Rows: tr.Rows})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 || len(cands[0]) == 0 || cands[0][0].Person.ID != johann.ID || cands[0][0].Score < 0.8 {
		t.Fatalf("candidates for Johann: %+v", cands)
	}
	if len(cands[1]) != 0 {
		t.Errorf("Maria matches nobody: %+v", cands[1])
	}

	rows[0].Action, rows[0].PersonID = RowActionPerson, &johann.ID
	if _, err := s.UpdateTranscription(ctx, a, tr.ID, TranscriptionInput{TemplateKey: "census", SourceID: &src.ID, Page: "p. 4", Date: "1 DEC 1900", Rows: rows}); err != nil {
		t.Fatal(err)
	}
	res, err := s.ApplyTranscription(ctx, a, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 2 || res.Status != "applied" || res.Rows[1].PersonID == nil {
		t.Fatalf("result: %+v", res)
	}

	census, err := s.GetEvent(ctx, a, res.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if census.Type != "CENS" || *census.PersonID != johann.ID || len(census.Participants) != 2 || census.Participants[0].Role != "spouse" {
		t.Errorf("census: %+v", census)
	}
	if len(census.Citations) != 1 || census.Citations[0].Page != "p. 4" {
		t.Errorf("census citation: %+v", census.Citations)
	}

	maria, err := s.GetPersonDetail(ctx, a, res.Created[0])
	if err != nil {
		t.Fatal(err)
	}
	var birth *Event
	for i := range maria.Events {
		if maria.Events[i].Type == "BIRT" {
			birth = &maria.Events[i]
		}
	}
	if maria.Sex != "F" || birth == nil || birth.Date.Normalized != "BET 1864 AND 1865" || birth.Place == nil || birth.Place.FullName != "Halle, Sachsen" {
		t.Fatalf("Maria: %+v, birth %+v", maria.Person, birth)
	}
	// One citation of line 13, for the date and the place.
	if len(birth.Citations) != 2 || birth.Citations[0].Page != "p. 4, line 13" || birth.Citations[0].Field != "date" ||
		birth.Citations[1].Field != "place" || birth.Citations[0].CitationID != birth.Citations[1].CitationID {
		t.Errorf("birth citation: %+v", birth.Citations)
	}

	// Johann already had a birth; his occupation is new.
	jd, _ := s.GetPersonDetail(ctx, a, johann.ID)
	types := map[string]int{}
	for _, e := range jd.Events {
		types[e.Type]++
	}
	if types["BIRT"] != 1 || types["OCCU"] != 1 {
		t.Errorf("Johann's events: %v", types)
	}
	// "Sachsen" was created once and reused.
	var places int
	_ = s.DB.QueryRow(`SELECT count(*) FROM places WHERE name = 'Sachsen'`).Scan(&places)
	if places != 1 {
		t.Errorf("Sachsen created %d times", places)
	}

	if _, err := s.ApplyTranscription(ctx, a, tr.ID); !errors.Is(err, ErrApplied) {
		t.Errorf("applied twice: %v", err)
	}
	if _, err := s.UpdateTranscription(ctx, a, tr.ID, TranscriptionInput{TemplateKey: "census"}); !errors.Is(err, ErrApplied) {
		t.Errorf("changed after applying: %v", err)
	}
}

func TestMarriageAndBaptismTranscription(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	src := mustSource(t, s, a, "Kirchenbuch")

	mar, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "marriage", SourceID: &src.ID, Date: "3 MAY 1890", Rows: []TranscriptionRow{
		row("", "groom", map[string]string{"given": "Karl", "surname": "Weber", "age": "25", "residence": "Leipzig"}),
		row("", "bride", map[string]string{"given": "Anna", "surname": "Schulz", "age": "22"}),
		row("", "witness", map[string]string{"given": "Otto", "surname": "Weber"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.ApplyTranscription(ctx, a, mar.ID)
	if err != nil {
		t.Fatal(err)
	}
	fam, err := s.GetFamily(ctx, a, res.FamilyID)
	if err != nil {
		t.Fatal(err)
	}
	if fam.UnionType != "married" || fam.Partner1.GivenNames != "Karl" || len(fam.Events) != 1 || len(fam.Events[0].Participants) != 1 {
		t.Errorf("family: %+v", fam)
	}
	karl, anna := fam.Partner1.ID, fam.Partner2.ID

	bap, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "baptism", SourceID: &src.ID, Date: "1 FEB 1891", Rows: []TranscriptionRow{
		{Role: "child", Values: map[string]string{"given": "Emil", "surname": "Weber", "sex": "Sohn"}},
		{Role: "father", Action: RowActionPerson, PersonID: &karl},
		{Role: "mother", Action: RowActionPerson, PersonID: &anna},
		{Role: "godparent", Values: map[string]string{"given": "Emil", "surname": "Schulz"}},
		{Role: "godparent", Action: RowActionSkip, Values: map[string]string{"given": "illegible"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	res, err = s.ApplyTranscription(ctx, a, bap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.FamilyID != 0 || len(res.Created) != 2 {
		t.Errorf("baptism: %+v", res)
	}
	emil, _ := s.GetPersonDetail(ctx, a, res.Created[0])
	if emil.Sex != "M" || len(emil.ParentFamilies) != 1 || emil.ParentFamilies[0].ID != fam.ID {
		t.Errorf("Emil's parents are Karl and Anna's family: %+v", emil.ParentFamilies)
	}
	bapm, _ := s.GetEvent(ctx, a, res.EventID)
	roles := map[string]int{}
	for _, p := range bapm.Participants {
		roles[p.Role]++
	}
	if roles["parent"] != 2 || roles["godparent"] != 1 {
		t.Errorf("baptism participants: %v", roles)
	}
}

func TestTranscriptionValidation(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	_, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "nope"})
	validationField(t, err, "templateKey")
	_, err = s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "census", Rows: []TranscriptionRow{row("", "king", nil)}})
	validationField(t, err, "rows.0")

	tr, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "census", Rows: []TranscriptionRow{
		row("", "spouse", map[string]string{"given": "Maria"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ApplyTranscription(ctx, a, tr.ID)
	validationField(t, err, "sourceId")
	validationField(t, err, "rows")
}

func TestCustomTemplates(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	tpl := RecordTemplate{
		Name: "Emigration list", EventType: "EMIG", Columns: []string{"given", "surname", "age", "occupation"},
		Roles: []TemplateRole{
			{Key: "emigrant", Label: "Emigrant", Kind: RoleKindPrincipal},
			{Key: "family", Label: "Travelling with", Kind: RoleKindParticipant, Participant: "relative"},
		},
	}
	saved, err := s.SaveTemplate(ctx, a, 0, tpl)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListTemplates(ctx, a)
	if len(all) != len(builtinTemplates)+1 || all[len(all)-1].Key != saved.Key || all[len(all)-1].Name != "Emigration list" {
		t.Errorf("templates: %+v", all)
	}

	bad := tpl
	bad.Roles = []TemplateRole{{Key: "x", Label: "X", Kind: RoleKindParticipant, Participant: "pirate"}}
	_, err = s.SaveTemplate(ctx, a, 0, bad)
	validationField(t, err, "roles")
	bad = tpl
	bad.Columns = []string{"age", "age"}
	_, err = s.SaveTemplate(ctx, a, 0, bad)
	validationField(t, err, "columns")

	src := mustSource(t, s, a, "Passenger list")
	tr, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: saved.Key, SourceID: &src.ID, Date: "1882", Rows: []TranscriptionRow{
		row("", "emigrant", map[string]string{"given": "Fritz", "surname": "Klein", "age": "30"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	_ = s.DB.QueryRow(`SELECT id FROM record_templates`).Scan(&id)
	if err := s.DeleteTemplate(ctx, a, id); !errors.Is(err, ErrConflict) {
		t.Errorf("deleting a template in use: %v", err)
	}
	res, err := s.ApplyTranscription(ctx, a, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.GetEvent(ctx, a, res.EventID)
	if e.Type != "EMIG" {
		t.Errorf("event: %+v", e)
	}
}

func TestFieldCitationsExportOnce(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	src := mustSource(t, s, a, "Census")
	tr, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "census", SourceID: &src.ID, Date: "1900", Rows: []TranscriptionRow{
		row("1", "head", map[string]string{"given": "Hans", "age": "40", "birthplace": "Leipzig"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTranscription(ctx, a, tr.ID); err != nil {
		t.Fatal(err)
	}
	out := exportText(t, s, a, ExportOptions{})
	// The citation also supports the person's name, which is written on
	// the NAME line; the birth must still cite it once.
	birth := out[strings.Index(out, "\n1 BIRT"):]
	birth = birth[:strings.Index(birth[1:], "\n1 ")+1]
	if n := strings.Count(birth, "3 PAGE line 1"); n != 1 {
		t.Errorf("the birth cites line 1 %d times:\n%s", n, out)
	}
}

func TestTranscriptionReusesPlaceIgnoringCyrillicCase(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	moscow, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Москва"})
	if err != nil {
		t.Fatal(err)
	}
	muller, err := s.CreatePlace(ctx, a, PlaceInput{Name: "Müller"})
	if err != nil {
		t.Fatal(err)
	}
	src := mustSource(t, s, a, "Перепись")
	tr, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "census", SourceID: &src.ID, Rows: []TranscriptionRow{
		row("1", "head", map[string]string{"given": "Иван", "surname": "Петров", "birthplace": "москва"}),
		row("2", "spouse", map[string]string{"given": "Anna", "surname": "Petrov", "birthplace": "Muller"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTranscription(ctx, a, tr.ID); err != nil {
		t.Fatal(err)
	}
	places, err := s.ListPlaces(ctx, a, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range places {
		names = append(names, p.Name)
	}
	if len(places) != 3 {
		t.Fatalf("places %v, want Москва and Müller reused and Muller added", names)
	}
	for _, p := range places {
		if p.ID != moscow.ID && p.ID != muller.ID && p.Name != "Muller" {
			t.Errorf("unexpected place %q", p.Name)
		}
		if p.Name == "москва" {
			t.Errorf("created a second Москва")
		}
	}
}
