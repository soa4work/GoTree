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
		{"11 FEB 1731/32", "11 FEB 1731/32", 17320211, 17320211, QualifierNone},
		{"1799/00", "1799/00", 18000000, 18000000, QualifierNone},
		// A dual date counts by its second year.
		{"29 FEB 1751/52", "29 FEB 1751/52", 17520229, 17520229, QualifierNone},
		{"FEB 1750/51", "FEB 1750/51", 17510200, 17510200, QualifierNone},
		{"1750/51", "1750/51", 17510000, 17510000, QualifierNone},
		{"BET 1 DEC 1750 AND 10 FEB 1750/51", "BET 1 DEC 1750 AND 10 FEB 1750/51", 17501201, 17510210, QualifierBetween},
		{"@#DJULIAN@ 29 FEB 1751/52", "@#DJULIAN@ 29 FEB 1751/52", 17520311, 17520311, QualifierNone},
		{"@#DJULIAN@ 10 FEB 1750/51", "@#DJULIAN@ 10 FEB 1750/51", 17510221, 17510221, QualifierNone},
		// Julian dates without a day are converted too.
		{"@#DJULIAN@ FEB 1700", "@#DJULIAN@ FEB 1700", 17000200, 17000200, QualifierNone},
		{"@#DJULIAN@ 1700", "@#DJULIAN@ 1700", 17000000, 17000000, QualifierNone},
		// B.C. dates, also in the Julian calendar.
		{"15 MAR 50 B.C.", "15 MAR 50 B.C.", -499685, -499685, QualifierNone},
		{"@#DJULIAN@ 1 JAN 44 B.C.", "@#DJULIAN@ 1 JAN 44 B.C.", -448770, -448770, QualifierNone},
		{"@#DJULIAN@ 15 MAR 44 B.C.", "@#DJULIAN@ 15 MAR 44 B.C.", -439687, -439687, QualifierNone},
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
		"29 FEB 1750/51",
		"BET 10 FEB 1750/51 AND 1 DEC 1750",
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
	for _, ordered := range [][]string{
		{"44 B.C.", "1850", "JAN 1850", "1 JAN 1850", "2 JAN 1850", "FEB 1850", "1851"},
		{"DEC 1750", "1 DEC 1750", "1751", "JAN 1750/51", "FEB 1750/51", "10 FEB 1750/51", "MAR 1751"},
		{"51 B.C.", "DEC 51 B.C.", "50 B.C.", "15 MAR 50 B.C.", "DEC 50 B.C.", "1 B.C.", "1"},
		// Julian 1 JAN 44 B.C. is Gregorian 30 DEC 45 B.C.
		{"@#DJULIAN@ 1 JAN 44 B.C.", "1 JAN 44 B.C."},
	} {
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
		{"11 FEB 1731/32", "11 FEB 1732", "11 FEB 1731/32"},
		{"29 FEB 1751/52", "29 FEB 1752", "29 FEB 1751/52"},
		{"1799/00", "1800", "1799/00"},
		{"@#DJULIAN@ 10 FEB 1750/51", "JULIAN 10 FEB 1751", "@#DJULIAN@ 10 FEB 1750/51"},
		{"BET 1 DEC 1750 AND 10 FEB 1750/51", "BET 1 DEC 1750 AND 10 FEB 1751", "10 FEB 1750/51"},
		{"INT 1750/51", "1751", "1750/51"},
		{"INT 12 FEB 1750/51", "12 FEB 1751", "12 FEB 1750/51"},
		{"INT 1750/51 (as written)", "1751", "as written"},
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
		// The value must be a date 7.0 readers (and Parse) can read back.
		if value != "" {
			if _, err := Parse(value); err != nil {
				t.Errorf("%q: value %q does not parse: %v", tt.in, value, err)
			}
		}
	}
}

func TestSplitKey(t *testing.T) {
	tests := []struct{ key, y, m, d int }{
		{-499685, -50, 3, 15},
		{-500000, -50, 0, 0},
		{-499700, -50, 3, 0},
		{-8769, -1, 12, 31},
		{18500312, 1850, 3, 12},
		{17510200, 1751, 2, 0},
	}
	for _, tt := range tests {
		y, m, d := SplitKey(tt.key)
		if y != tt.y || m != tt.m || d != tt.d {
			t.Errorf("SplitKey(%d) = %d, %d, %d, want %d, %d, %d", tt.key, y, m, d, tt.y, tt.m, tt.d)
		}
	}
	if y := KeyYear(-499685); y != -50 {
		t.Errorf("KeyYear(-499685) = %d, want -50", y)
	}
	for _, in := range []string{"15 MAR 50 B.C.", "50 B.C.", "DEC 50 B.C.", "31 DEC 1 B.C.", "1 B.C.", "1850", "12 MAR 1850", "FEB 1750/51", "@#DJULIAN@ 1 JAN 44 B.C."} {
		d, err := Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		k, _ := d.SortKey()
		y, m, day := SplitKey(k)
		if m < 0 || m > 12 || day < 0 || day > 31 || y*10000+m*100+day != k {
			t.Errorf("%q: key %d splits into %d, %d, %d", in, k, y, m, day)
		}
		if y == 0 {
			t.Errorf("%q: key %d has year 0", in, k)
		}
	}
}

func TestBounds(t *testing.T) {
	tests := []struct {
		in          string
		first, last int
	}{
		{"@#DJULIAN@ FEB 1700", 17000211, 17000311},
		{"FEB 1700", 17000201, 17000228},
		{"1750/51", 17510101, 17511231},
		{"29 FEB 1751/52", 17520229, 17520229},
		{"@#DJULIAN@ 1700", 17000111, 17010111},
		{"50 B.C.", -499899, -498769},
	}
	for _, tt := range tests {
		d, err := Parse(tt.in)
		if err != nil {
			t.Fatal(err)
		}
		first, last := d.Start.Bounds()
		if first != tt.first || last != tt.last {
			t.Errorf("%q: Bounds() = %d, %d, want %d, %d", tt.in, first, last, tt.first, tt.last)
		}
		if first > last {
			t.Errorf("%q: first %d is after last %d", tt.in, first, last)
		}
	}
}
