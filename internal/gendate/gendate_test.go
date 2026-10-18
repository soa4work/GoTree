package gendate

import (
	"errors"
	"testing"
)

func TestParseCanonical(t *testing.T) {
	tests := []struct {
		in, want string
		key      int
		keyEnd   int
		q        Qualifier
	}{
		{"1850", "1850", 18500000, 18500000, QualifierNone},
		{"mar 1850", "MAR 1850", 18500300, 18500300, QualifierNone},
		{"12 Mar 1850", "12 MAR 1850", 18500312, 18500312, QualifierNone},
		{"  12   MAR   1850 ", "12 MAR 1850", 18500312, 18500312, QualifierNone},
		{"ABT 1850", "ABT 1850", 18500000, 18500000, QualifierAbout},
		{"about 1850", "ABT 1850", 18500000, 18500000, QualifierAbout},
		{"CAL 1850", "CAL 1850", 18500000, 18500000, QualifierCalculated},
		{"EST 1850", "EST 1850", 18500000, 18500000, QualifierEstimated},
		{"BEF 3 JUN 1901", "BEF 3 JUN 1901", 19010603, 19010603, QualifierBefore},
		{"AFT 1901", "AFT 1901", 19010000, 19010000, QualifierAfter},
		{"BET 1850 AND 1860", "BET 1850 AND 1860", 18500000, 18600000, QualifierBetween},
		{"FROM 1914 TO 1918", "FROM 1914 TO 1918", 19140000, 19180000, QualifierFromTo},
		{"FROM 1914", "FROM 1914", 19140000, 19140000, QualifierFrom},
		{"TO 1918", "TO 1918", 19180000, 19180000, QualifierTo},
		{"INT 1850 (about the time of the fire)", "INT 1850 (about the time of the fire)", 18500000, 18500000, QualifierInterpreted},
		{"29 FEB 2000", "29 FEB 2000", 20000229, 20000229, QualifierNone},
		{"44 B.C.", "44 B.C.", -440000, -440000, QualifierNone},
		{"11 FEB 1731/32", "11 FEB 1731/32", 17310211, 17310211, QualifierNone},
		{"1799/00", "1799/00", 17990000, 17990000, QualifierNone},
		// GEDCOM 7 spelling of the calendar escape.
		{"JULIAN 4 OCT 1582", "@#DJULIAN@ 4 OCT 1582", 15821014, 15821014, QualifierNone},
		{"@#DJULIAN@ 1 JAN 1700", "@#DJULIAN@ 1 JAN 1700", 17000111, 17000111, QualifierNone},
		{"@#DGREGORIAN@ 15 OCT 1582", "15 OCT 1582", 15821015, 15821015, QualifierNone},
		// Julian 29 Feb 1700 exists (every fourth year is a leap year).
		{"@#DJULIAN@ 29 FEB 1700", "@#DJULIAN@ 29 FEB 1700", 17000311, 17000311, QualifierNone},
		// Friendly input normalized to GEDCOM.
		{"1850-03-12", "12 MAR 1850", 18500312, 18500312, QualifierNone},
		{"1850-03", "MAR 1850", 18500300, 18500300, QualifierNone},
		{"12.3.1850", "12 MAR 1850", 18500312, 18500312, QualifierNone},
		{"03.1850", "MAR 1850", 18500300, 18500300, QualifierNone},
		{"abt 12.3.1850", "ABT 12 MAR 1850", 18500312, 18500312, QualifierAbout},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			d, err := Parse(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got := d.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if d.Qualifier != tt.q {
				t.Errorf("Qualifier = %q, want %q", d.Qualifier, tt.q)
			}
			if key, ok := d.SortKey(); !ok || key != tt.key {
				t.Errorf("SortKey() = %d, %v, want %d", key, ok, tt.key)
			}
			if key, ok := d.SortKeyEnd(); !ok || key != tt.keyEnd {
				t.Errorf("SortKeyEnd() = %d, %v, want %d", key, ok, tt.keyEnd)
			}
			// The canonical form must parse back to itself.
			again, err := Parse(d.String())
			if err != nil {
				t.Fatalf("canonical form does not re-parse: %v", err)
			}
			if again.String() != d.String() {
				t.Errorf("round trip %q -> %q", d.String(), again.String())
			}
		})
	}
}

func TestParsePhrase(t *testing.T) {
	d, err := Parse("(during the war)")
	if err != nil {
		t.Fatal(err)
	}
	if d.Qualifier != QualifierPhrase || d.Phrase != "during the war" {
		t.Errorf("got %+v", d)
	}
	if d.String() != "(during the war)" {
		t.Errorf("String() = %q", d.String())
	}
	if _, ok := d.SortKey(); ok {
		t.Error("a phrase-only date must have no sort key")
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"yesterday",
		"32 JAN 1850",
		"29 FEB 1900",
		"31 APR 1850",
		"12 FOO 1850",
		"BET 1860 AND 1850",
		"BET 1850",
		"ABT",
		"ABT 1850 (phrase)",
		"INT 1850 (unterminated",
		"@#DHEBREW@ 5600",
		"1850/52",
		"0",
		"12345",
		"2020-13-01",
		"31.02.1850",
		"1 2 3 1850",
	} {
		t.Run(in, func(t *testing.T) {
			if d, err := Parse(in); err == nil {
				t.Errorf("Parse(%q) = %q, want error", in, d.String())
			}
		})
	}
	if _, err := Parse(" "); !errors.Is(err, ErrEmpty) {
		t.Errorf("blank input: got %v, want ErrEmpty", err)
	}
}

func TestSortOrder(t *testing.T) {
	ordered := []string{"44 B.C.", "1850", "JAN 1850", "1 JAN 1850", "2 JAN 1850", "FEB 1850", "1851"}
	prev := -1 << 62
	for _, s := range ordered {
		d, err := Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		key, _ := d.SortKey()
		if key <= prev {
			t.Errorf("%q (key %d) does not sort after its predecessor (key %d)", s, key, prev)
		}
		prev = key
	}
}

func TestGEDCOM7(t *testing.T) {
	tests := []struct{ in, value, phrase string }{
		{"12 MAR 1850", "12 MAR 1850", ""},
		{"ABT 1850", "ABT 1850", ""},
		{"@#DJULIAN@ 4 OCT 1582", "JULIAN 4 OCT 1582", ""},
		{"44 B.C.", "44 BCE", ""},
		{"BET 1850 AND 1860", "BET 1850 AND 1860", ""},
		{"INT 1850 (the year of the fire)", "1850", "the year of the fire"},
		{"(during the war)", "", "during the war"},
		{"11 FEB 1731/32", "11 FEB 1731", "11 FEB 1731/32"},
		{"INT 1750/51", "1750", "1750/51"},
		{"INT 12 FEB 1750/51", "12 FEB 1750", "12 FEB 1750/51"},
		{"INT 1750/51 (as written)", "1750", "as written"},
		{"INT 1850", "1850", ""},
	}
	for _, tt := range tests {
		d, err := Parse(tt.in)
		if err != nil {
			t.Fatal(err)
		}
		value, phrase := d.GEDCOM7()
		if value != tt.value || phrase != tt.phrase {
			t.Errorf("%q: got %q / %q, want %q / %q", tt.in, value, phrase, tt.value, tt.phrase)
		}
	}
}
