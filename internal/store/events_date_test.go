package store

import "testing"

func TestEventPlainLanguageDates(t *testing.T) {
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
