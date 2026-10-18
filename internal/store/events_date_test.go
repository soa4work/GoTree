package store

import "testing"

func TestEventPlainLanguageDates(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Plain", "Dates")

	for _, tc := range []struct{ in, raw, normalized string }{
		{"12 March 1850", "12 MAR 1850", "12 MAR 1850"},
		{"about 1850", "about 1850", "ABT 1850"},
		{"between 1850 and 1860", "BET 1850 AND 1860", "BET 1850 AND 1860"},
		{"before 1900", "before 1900", "BEF 1900"},
		{"March 12, 1850", "12 MAR 1850", "12 MAR 1850"},
		{"sometime", "sometime", ""},
	} {
		e, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "BIRT", Date: tc.in})
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if e.Date.Raw != tc.raw || e.Date.Normalized != tc.normalized {
			t.Errorf("%q: raw %q normalized %q, want %q / %q", tc.in, e.Date.Raw, e.Date.Normalized, tc.raw, tc.normalized)
		}
	}
}

// Imports stay strict: loose heirloom dates are kept as written and only
// text Parse reads is made canonical, as before storedDate existed.
func TestHeirloomImportDatesStayStrict(t *testing.T) {
	t.Parallel()
	text := "0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n" +
		"0 @H1@ _HEIRLOOM\n1 NAME Loose\n1 DATE March 12, 1850\n" +
		"0 @H2@ _HEIRLOOM\n1 NAME Abbreviated\n1 DATE abt 1880\n" +
		"0 @H3@ _HEIRLOOM\n1 NAME Between\n1 DATE between 1850 and 1860\n0 TRLR\n"
	s, a := reimport(t, text)
	list, err := s.ListHeirlooms(ctx, a, "", 0)
	if err != nil || len(list) != 3 {
		t.Fatalf("heirlooms: %+v %v", list, err)
	}
	want := map[string]string{"Loose": "March 12, 1850", "Abbreviated": "ABT 1880", "Between": "between 1850 and 1860"}
	for _, h := range list {
		if h.MadeDate != want[h.Name] {
			t.Errorf("%s: made date %q, want %q", h.Name, h.MadeDate, want[h.Name])
		}
	}
}

func TestStoredDateEverywhere(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Weber")
	const typed, want = "between 1850 and 1860", "BET 1850 AND 1860"

	for in, out := range map[string]string{
		"about 1850": "about 1850", "12.3.1850": "12.3.1850", "March 12, 1850": "12 MAR 1850", "sometime": "sometime", "": "",
	} {
		if got := storedDate(in); got != out {
			t.Errorf("storedDate(%q) = %q, want %q", in, got, out)
		}
	}
	if y := recordYear(typed); y != 1850 {
		t.Errorf("recordYear(%q) = %d", typed, y)
	}

	h, err := s.CreateHeirloom(ctx, a, HeirloomInput{
		Name: "Watch", MadeDate: typed, Custody: []CustodyInput{{PersonID: &anna.ID, FromDate: typed, ToDate: "March 12, 1900", How: "inherited"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if h.MadeDate != want || h.Custody[0].FromDate != want || h.Custody[0].ToDate != "12 MAR 1900" {
		t.Errorf("heirloom dates: %+v", h)
	}
	if k := madeSort(h.MadeDate); !k.Valid || k.Int64 != 18500000 {
		t.Errorf("made sort key: %+v", k)
	}

	m, _, err := s.CreateMedia(ctx, a, newMedia(0), nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err = s.UpdateMedia(ctx, a, m.ID, MediaUpdate{Title: m.Title, Date: typed})
	if err != nil {
		t.Fatal(err)
	}
	if m.DateRaw != want {
		t.Errorf("media date %q, want %q", m.DateRaw, want)
	}

	tr, err := s.CreateTranscription(ctx, a, TranscriptionInput{TemplateKey: "census", Date: typed})
	if err != nil {
		t.Fatal(err)
	}
	if tr.Date != want {
		t.Errorf("transcription date %q, want %q", tr.Date, want)
	}
}
