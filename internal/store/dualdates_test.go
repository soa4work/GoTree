package store

import (
	"strings"
	"testing"
)

func TestDualAndJulianDatesStored(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Anna", "Weber")
	mk := func(typ, date string) Event {
		t.Helper()
		e, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: typ, Date: date})
		if err != nil {
			t.Fatalf("%s: %v", date, err)
		}
		return e
	}

	leap := mk("BIRT", "29 FEB 1751/52")
	if !leap.Date.Valid || leap.Date.Raw != "29 FEB 1751/52" || leap.Date.SortKey == nil || *leap.Date.SortKey != 17520229 {
		t.Errorf("29 FEB 1751/52 stored as %+v", leap.Date)
	}

	between := mk("BAPM", "BET 1 DEC 1750 AND 10 FEB 1750/51")
	if !between.Date.Valid || between.Date.Qualifier != "BET" {
		t.Errorf("range over the turn of the year: %+v", between.Date)
	}

	// The events share a sort order, so the dates alone decide.
	mk("OCCU", "DEC 1750")
	mk("RESI", "FEB 1750/51")
	mk("CENS", "JAN 1751")
	d, err := s.GetPersonDetail(ctx, a, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, e := range d.Events {
		switch e.Type {
		case "OCCU", "CENS", "RESI":
			order = append(order, e.Date.Raw)
		}
	}
	if got := strings.Join(order, ","); got != "DEC 1750,JAN 1751,FEB 1750/51" {
		t.Errorf("order: %s", got)
	}
}

func TestChecksDualJulianAndBC(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	ev := func(p Person, typ, date string) {
		t.Helper()
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: typ, Date: date}); err != nil {
			t.Fatal(err)
		}
	}
	leap := mustPerson(t, s, a, "Leap", "Year")
	ev(leap, "BIRT", "29 FEB 1751/52")
	julian := mustPerson(t, s, a, "Julian", "Month")
	ev(julian, "BIRT", "5 MAR 1700")
	ev(julian, "DEAT", "@#DJULIAN@ FEB 1700")
	bc := mustPerson(t, s, a, "Roman", "Life")
	ev(bc, "BIRT", "15 MAR 50 B.C.")
	ev(bc, "DEAT", "1 JAN 49 B.C.")
	control := mustPerson(t, s, a, "Control", "Case")
	ev(control, "BIRT", "1850")
	ev(control, "DEAT", "1849")

	rep, err := s.Checks(ctx, a, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		switch {
		case f.PersonID == control.ID && f.Rule == "birth_after_death":
		case f.PersonID == leap.ID || f.PersonID == julian.ID || f.PersonID == bc.ID:
			t.Errorf("unexpected finding %s for person %d", f.Rule, f.PersonID)
		}
	}
	found := false
	for _, f := range rep.Findings {
		if f.PersonID == control.ID && f.Rule == "birth_after_death" {
			found = true
		}
	}
	if !found {
		t.Errorf("control: no birth_after_death in %+v", rep.Findings)
	}
}

func TestRecordYearBC(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int{
		"15 MAR 50 B.C.": -50,
		"FEB 1750/51":    1751,
		"":               0,
		"12 MAR 1850":    1850,
	} {
		if got := recordYear(in); got != want {
			t.Errorf("recordYear(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestDecadeAndAge(t *testing.T) {
	t.Parallel()
	for key, want := range map[int]int{-499685: -50, -489685: -50, -509685: -60, 18550312: 1850} {
		if got := decadeOf(key); got != want {
			t.Errorf("decadeOf(%d) = %d, want %d", key, got, want)
		}
	}
	if got := ageBetween(18500312, 19000311); got != 49 {
		t.Errorf("age before the birthday: %d", got)
	}
	if got := ageBetween(18500312, 19000312); got != 50 {
		t.Errorf("age on the birthday: %d", got)
	}
}

func TestExportDualYear(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Anna", "Weber")
	for _, d := range []string{"12 FEB 1750/51", "29 FEB 1751/52"} {
		if _, err := s.CreateEvent(ctx, a, EventInput{PersonID: &p.ID, Type: "RESI", Date: d}); err != nil {
			t.Fatal(err)
		}
	}
	v5 := exportText(t, s, a, ExportOptions{})
	if !strings.Contains(v5, "2 DATE 29 FEB 1751/52\n") {
		t.Errorf("5.5.1 export lost the leap day:\n%s", v5)
	}
	v7 := exportText(t, s, a, ExportOptions{Version7: true})
	for _, want := range []string{
		"2 DATE 12 FEB 1751\n3 PHRASE 12 FEB 1750/51\n",
		"2 DATE 29 FEB 1752\n3 PHRASE 29 FEB 1751/52\n",
	} {
		if !strings.Contains(v7, want) {
			t.Errorf("7.0 export lacks %q:\n%s", want, v7)
		}
	}
}

func TestImportOldGedcom7DualYear(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"12 FEB 1750", "12 FEB 1751"} {
		text := "0 HEAD\n1 GEDC\n2 VERS 7.0\n" +
			"0 @I1@ INDI\n1 NAME Anna /Weber/\n1 RESI\n2 DATE " + value + "\n3 PHRASE 12 FEB 1750/51\n0 TRLR\n"
		s, a := reimport(t, text)
		got := findPerson(t, s, a, "anna")
		if len(got.Events) != 1 || got.Events[0].Date.Raw != "12 FEB 1750/51" {
			t.Errorf("value %q: %+v", value, got.Events)
		}
	}
}
