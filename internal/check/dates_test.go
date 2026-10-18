package check

import (
	"math"
	"testing"

	"github.com/praetorianer777/gotree/internal/gendate"
)

func lifeOf(birth, death *Range) Data {
	return Data{
		Persons: []Person{{ID: 1}},
		Events: []Event{
			{ID: 10, PersonID: 1, Type: "BIRT", Date: birth},
			{ID: 11, PersonID: 1, Type: "DEAT", Date: death},
		},
	}
}

func TestFromSortKeysBC(t *testing.T) {
	if got, want := FromSortKeys(-499685, -499685, ""), (Range{Day(-50, 3, 15), Day(-50, 3, 15)}); got != want {
		t.Errorf("15 MAR 50 B.C.: got %+v, want %+v", got, want)
	}
	if got, want := FromSortKeys(-500000, -500000, ""), (Range{Day(-50, 1, 1), Day(-50, 12, 31)}); got != want {
		t.Errorf("50 B.C.: got %+v, want %+v", got, want)
	}
}

func TestBCLifeIsNotContradictory(t *testing.T) {
	// 15 MAR 50 B.C. to 1 JAN 49 B.C.
	d := lifeOf(date(-499685, ""), date(-489899, ""))
	if _, ok := rules(Run(d, now))[RuleBirthAfterDeath]; ok {
		t.Errorf("a B.C. life flagged as birth after death: %+v", Run(d, now))
	}
}

func TestDualYearRange(t *testing.T) {
	// BET 1 DEC 1750 AND 10 FEB 1750/51, died MAR 1751.
	birth := FromSortKeys(17501201, 17510210, "BET")
	d := lifeOf(&birth, date(17510300, ""))
	if _, ok := rules(Run(d, now))[RuleBirthAfterDeath]; ok {
		t.Errorf("flagged: %+v", Run(d, now))
	}
	// FEB 1750/51 is in 1751, so a death in JUN 1750 is before the birth.
	d = lifeOf(date(17510200, ""), date(17500600, ""))
	if _, ok := rules(Run(d, now))[RuleBirthAfterDeath]; !ok {
		t.Errorf("not flagged: %+v", Run(d, now))
	}
}

func parseDate(t *testing.T, s string) gendate.Date {
	t.Helper()
	d, err := gendate.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFromDateJulianMonth(t *testing.T) {
	r, ok := FromDate(parseDate(t, "@#DJULIAN@ FEB 1700"))
	if want := (Range{Day(1700, 2, 11), Day(1700, 3, 11)}); !ok || r != want {
		t.Fatalf("got %+v, %v, want %+v", r, ok, want)
	}
	// Born 5 MAR 1700 (Gregorian), died in Julian FEB 1700: that is 11 FEB
	// to 11 MAR Gregorian, so the death may be after the birth.
	d := lifeOf(date(17000305, ""), &r)
	if _, ok := rules(Run(d, now))[RuleBirthAfterDeath]; ok {
		t.Errorf("flagged: %+v", Run(d, now))
	}
	// Read by the sort key alone, the death is the whole of Gregorian FEB.
	d = lifeOf(date(17000305, ""), date(17000200, ""))
	if _, ok := rules(Run(d, now))[RuleBirthAfterDeath]; !ok {
		t.Errorf("the key-only reading should flag it: %+v", Run(d, now))
	}
}

func TestFromDateQualifiers(t *testing.T) {
	if r, ok := FromDate(parseDate(t, "BEF @#DJULIAN@ FEB 1700")); !ok || r.Lo != math.MinInt || r.Hi != Day(1700, 3, 11) {
		t.Errorf("BEF: %+v, %v", r, ok)
	}
	if r, ok := FromDate(parseDate(t, "ABT @#DJULIAN@ FEB 1700")); !ok ||
		r.Lo != Day(1700, 2, 11)-2*yearDays || r.Hi != Day(1700, 3, 11)+2*yearDays {
		t.Errorf("ABT: %+v, %v", r, ok)
	}
	if r, ok := FromDate(parseDate(t, "TO @#DJULIAN@ 1700")); !ok || r.Lo != math.MinInt || r.Hi != Day(1701, 1, 11) {
		t.Errorf("TO: %+v, %v", r, ok)
	}
	if _, ok := FromDate(parseDate(t, "(during the war)")); ok {
		t.Error("a phrase has no range")
	}
}
