// Package gendate parses genealogical dates in GEDCOM 5.5.1 and 7.0 syntax.
//
// Genealogical dates are often partial ("MAR 1850"), approximate
// ("ABT 1850") or ranges ("BET 1850 AND 1860"). A Date keeps that meaning,
// renders it back to canonical GEDCOM, and yields an integer sort key so
// events can be ordered without losing the original wording.
package gendate

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Qualifier says how the date (or dates) of a Date are to be read.
type Qualifier string

// Qualifiers. Exact dates have QualifierNone.
const (
	QualifierNone        Qualifier = ""
	QualifierAbout       Qualifier = "ABT"
	QualifierCalculated  Qualifier = "CAL"
	QualifierEstimated   Qualifier = "EST"
	QualifierBefore      Qualifier = "BEF"
	QualifierAfter       Qualifier = "AFT"
	QualifierBetween     Qualifier = "BET"
	QualifierFrom        Qualifier = "FROM"
	QualifierTo          Qualifier = "TO"
	QualifierFromTo      Qualifier = "FROMTO"
	QualifierInterpreted Qualifier = "INT"
	// QualifierPhrase is a date given only as free text, e.g. "(during the war)".
	QualifierPhrase Qualifier = "PHRASE"
)

// Calendar is the calendar a SimpleDate is expressed in.
type Calendar string

// Supported calendars.
const (
	Gregorian Calendar = "GREGORIAN"
	Julian    Calendar = "JULIAN"
)

// SimpleDate is a single, possibly partial, calendar date. Month and Day are
// zero when unknown.
type SimpleDate struct {
	Calendar Calendar
	Year     int
	// DualYear is the second year of 5.5.1 dual dating ("1750/51"), or 0.
	// It is the year the date falls in by today's reckoning; validation and
	// sort keys use it.
	DualYear int
	Month    int
	Day      int
	BC       bool
}

// Date is a parsed genealogical date.
type Date struct {
	Qualifier Qualifier
	// Start is the date for single-date forms and the lower bound for
	// ranges and periods. It is nil for phrase-only dates and "TO x".
	Start *SimpleDate
	// End is the upper bound of BET…AND and FROM…TO, and the date of "TO x".
	End *SimpleDate
	// Phrase is free text: the whole date for QualifierPhrase, or the
	// explanation attached to an INT date.
	Phrase string
}

// ErrEmpty is returned by Parse for blank input.
var ErrEmpty = errors.New("empty date")

var months = []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}

func monthNumber(tok string) int {
	for i, m := range months {
		if tok == m {
			return i + 1
		}
	}
	return 0
}

// Parse reads a date in GEDCOM syntax (case-insensitive). For convenience it
// also accepts ISO dates ("1850-03-12", "1850-03") and German-style
// "12.3.1850" / "3.1850", which it normalizes to GEDCOM.
func Parse(s string) (Date, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return Date{}, ErrEmpty
	}

	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		return Date{Qualifier: QualifierPhrase, Phrase: strings.TrimSpace(s[1 : len(s)-1])}, nil
	}

	// The phrase of an INT date may contain anything, so it is split off
	// before the rest is upper-cased and tokenized.
	phrase := ""
	if i := strings.Index(s, "("); i >= 0 {
		if !strings.HasSuffix(s, ")") {
			return Date{}, fmt.Errorf("unterminated phrase in %q", s)
		}
		phrase = strings.TrimSpace(s[i+1 : len(s)-1])
		s = strings.TrimSpace(s[:i])
	}

	toks := strings.Fields(strings.ToUpper(s))
	d, err := parseTokens(toks)
	if err != nil {
		return Date{}, fmt.Errorf("%q: %w", s, err)
	}
	if phrase != "" {
		if d.Qualifier != QualifierInterpreted {
			return Date{}, fmt.Errorf("%q: a phrase is only allowed with INT", s)
		}
		d.Phrase = phrase
	}
	return d, nil
}

func parseTokens(toks []string) (Date, error) {
	switch toks[0] {
	case "ABT", "ABOUT":
		return single(QualifierAbout, toks[1:])
	case "CAL":
		return single(QualifierCalculated, toks[1:])
	case "EST":
		return single(QualifierEstimated, toks[1:])
	case "BEF", "BEFORE":
		return single(QualifierBefore, toks[1:])
	case "AFT", "AFTER":
		return single(QualifierAfter, toks[1:])
	case "INT":
		return single(QualifierInterpreted, toks[1:])
	case "BET":
		left, right, ok := split(toks[1:], "AND")
		if !ok {
			return Date{}, errors.New("BET without AND")
		}
		return pair(QualifierBetween, left, right)
	case "FROM":
		left, right, ok := split(toks[1:], "TO")
		if !ok {
			return single(QualifierFrom, toks[1:])
		}
		return pair(QualifierFromTo, left, right)
	case "TO":
		end, err := parseSimple(toks[1:])
		if err != nil {
			return Date{}, err
		}
		return Date{Qualifier: QualifierTo, End: &end}, nil
	}
	return single(QualifierNone, toks)
}

func single(q Qualifier, toks []string) (Date, error) {
	sd, err := parseSimple(toks)
	if err != nil {
		return Date{}, err
	}
	return Date{Qualifier: q, Start: &sd}, nil
}

func pair(q Qualifier, left, right []string) (Date, error) {
	start, err := parseSimple(left)
	if err != nil {
		return Date{}, err
	}
	end, err := parseSimple(right)
	if err != nil {
		return Date{}, err
	}
	if start.compareKey() > end.compareKey() {
		return Date{}, errors.New("range ends before it starts")
	}
	return Date{Qualifier: q, Start: &start, End: &end}, nil
}

func split(toks []string, sep string) ([]string, []string, bool) {
	for i, t := range toks {
		if t == sep {
			return toks[:i], toks[i+1:], true
		}
	}
	return nil, nil, false
}

// parseSimple reads [calendar] [[day] month] year [BC].
func parseSimple(toks []string) (SimpleDate, error) {
	sd := SimpleDate{Calendar: Gregorian}
	if len(toks) == 0 {
		return sd, errors.New("missing date")
	}

	switch toks[0] {
	case "@#DGREGORIAN@", "GREGORIAN":
		toks = toks[1:]
	case "@#DJULIAN@", "JULIAN":
		sd.Calendar = Julian
		toks = toks[1:]
	default:
		if strings.HasPrefix(toks[0], "@#D") || toks[0] == "HEBREW" || toks[0] == "FRENCH_R" {
			return sd, fmt.Errorf("calendar %s is not supported", toks[0])
		}
	}

	if n := len(toks); n > 0 && (toks[n-1] == "B.C." || toks[n-1] == "BC" || toks[n-1] == "BCE") {
		sd.BC = true
		toks = toks[:n-1]
	}

	if len(toks) == 1 && sd.Calendar == Gregorian && !sd.BC {
		if num, ok := parseNumeric(toks[0]); ok {
			return num, nil
		}
	}

	switch len(toks) {
	case 1:
		// year only
	case 2:
		if sd.Month = monthNumber(toks[0]); sd.Month == 0 {
			return sd, fmt.Errorf("unknown month %q", toks[0])
		}
	case 3:
		day, err := strconv.Atoi(toks[0])
		if err != nil {
			return sd, fmt.Errorf("invalid day %q", toks[0])
		}
		sd.Day = day
		if sd.Month = monthNumber(toks[1]); sd.Month == 0 {
			return sd, fmt.Errorf("unknown month %q", toks[1])
		}
	default:
		return sd, errors.New("expected [[day] month] year")
	}

	yearTok := toks[len(toks)-1]
	yearPart, dualPart, dual := strings.Cut(yearTok, "/")
	year, err := strconv.Atoi(yearPart)
	if err != nil || year <= 0 || len(yearPart) > 4 {
		return sd, fmt.Errorf("invalid year %q", yearTok)
	}
	sd.Year = year
	if dual {
		if sd.BC {
			return sd, errors.New("dual years cannot be B.C.")
		}
		d, err := strconv.Atoi(dualPart)
		if err != nil || len(dualPart) != 2 || (year+1)%100 != d {
			return sd, fmt.Errorf("invalid dual year %q", yearTok)
		}
		sd.DualYear = year + 1
	}
	if err := sd.validate(); err != nil {
		return sd, err
	}
	return sd, nil
}

// parseNumeric accepts the non-GEDCOM inputs people type: ISO 8601
// (YYYY-MM-DD, YYYY-MM) and German day.month.year (D.M.YYYY, M.YYYY).
func parseNumeric(tok string) (SimpleDate, bool) {
	var parts []string
	iso := strings.Contains(tok, "-")
	if iso {
		parts = strings.Split(tok, "-")
	} else if strings.Contains(tok, ".") {
		parts = strings.Split(tok, ".")
	} else {
		return SimpleDate{}, false
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || p == "" {
			return SimpleDate{}, false
		}
		nums[i] = n
	}

	sd := SimpleDate{Calendar: Gregorian}
	switch {
	case iso && len(nums) == 3 && len(parts[0]) == 4:
		sd.Year, sd.Month, sd.Day = nums[0], nums[1], nums[2]
	case iso && len(nums) == 2 && len(parts[0]) == 4:
		sd.Year, sd.Month = nums[0], nums[1]
	case !iso && len(nums) == 3 && len(parts[2]) == 4:
		sd.Day, sd.Month, sd.Year = nums[0], nums[1], nums[2]
	case !iso && len(nums) == 2 && len(parts[1]) == 4:
		sd.Month, sd.Year = nums[0], nums[1]
	default:
		return SimpleDate{}, false
	}
	if sd.Year <= 0 || sd.validate() != nil {
		return SimpleDate{}, false
	}
	return sd, true
}

func (sd SimpleDate) validate() error {
	if sd.Month < 0 || sd.Month > 12 {
		return fmt.Errorf("invalid month %d", sd.Month)
	}
	if sd.Day != 0 {
		if sd.Month == 0 {
			return errors.New("day without month")
		}
		if sd.Day < 1 || sd.Day > daysIn(sd.Calendar, sd.leapYear(), sd.Month) {
			return fmt.Errorf("day %d does not exist in %s %d", sd.Day, months[sd.Month-1], sd.year())
		}
	}
	return nil
}

func daysIn(cal Calendar, year, month int) int {
	switch month {
	case 2:
		if isLeap(cal, year) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	default:
		return 31
	}
}

func isLeap(cal Calendar, year int) bool {
	if cal == Julian {
		return year%4 == 0
	}
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// String renders the SimpleDate in canonical GEDCOM 5.5.1 form.
func (sd SimpleDate) String() string {
	var b strings.Builder
	if sd.Calendar == Julian {
		b.WriteString("@#DJULIAN@ ")
	}
	if sd.Day != 0 {
		fmt.Fprintf(&b, "%d ", sd.Day)
	}
	if sd.Month != 0 {
		b.WriteString(months[sd.Month-1])
		b.WriteByte(' ')
	}
	b.WriteString(strconv.Itoa(sd.Year))
	if sd.DualYear != 0 {
		fmt.Fprintf(&b, "/%02d", sd.DualYear%100)
	}
	if sd.BC {
		b.WriteString(" B.C.")
	}
	return b.String()
}

// String renders the Date in canonical GEDCOM 5.5.1 form.
func (d Date) String() string {
	switch d.Qualifier {
	case QualifierPhrase:
		return "(" + d.Phrase + ")"
	case QualifierBetween:
		return "BET " + d.Start.String() + " AND " + d.End.String()
	case QualifierFromTo:
		return "FROM " + d.Start.String() + " TO " + d.End.String()
	case QualifierTo:
		return "TO " + d.End.String()
	case QualifierInterpreted:
		s := "INT " + d.Start.String()
		if d.Phrase != "" {
			s += " (" + d.Phrase + ")"
		}
		return s
	case QualifierNone:
		return d.Start.String()
	default:
		return string(d.Qualifier) + " " + d.Start.String()
	}
}

// SortKey orders dates chronologically as YYYYMMDD in the Gregorian
// calendar, with unknown month or day as 00 so "1850" sorts before
// "1 JAN 1850". ok is false for phrase-only dates.
func (d Date) SortKey() (key int, ok bool) {
	switch {
	case d.Start != nil:
		return d.Start.compareKey(), true
	case d.End != nil:
		return d.End.compareKey(), true
	default:
		return 0, false
	}
}

// SortKeyEnd is the key of the upper bound of a range or period, or the
// same as SortKey for single dates.
func (d Date) SortKeyEnd() (key int, ok bool) {
	if d.End != nil {
		return d.End.compareKey(), true
	}
	return d.SortKey()
}

// year is the historical year: the second year of a dual date
// ("FEB 1750/51" happened in 1751), else Year.
func (sd SimpleDate) year() int {
	if sd.DualYear != 0 {
		return sd.DualYear
	}
	return sd.Year
}

// leapYear is the astronomical year that decides whether February has a
// 29th: 1 B.C. is year 0, so 1, 5, 9 B.C. are leap years, not 4 B.C.
func (sd SimpleDate) leapYear() int {
	if sd.BC {
		return 1 - sd.year()
	}
	return sd.year()
}

func (sd SimpleDate) compareKey() int {
	y, m, d := sd.year(), sd.Month, sd.Day
	if sd.Calendar != Julian {
		return key(y, m, d, sd.BC)
	}
	// A partial date is converted at its middle, so a month or year stays
	// what it is in the Gregorian calendar unless the shift moves it.
	cm, cd := m, d
	switch {
	case m == 0:
		cm, cd = 7, 1
	case d == 0:
		cd = 15
	}
	gy, gm, gd, bc := julianToGregorian(y, cm, cd, sd.BC)
	if m == 0 {
		gm = 0
	}
	if d == 0 {
		gd = 0
	}
	return key(gy, gm, gd, bc)
}

// Bounds returns the sort keys of the first and the last day the date can
// mean, converted to the Gregorian calendar: "@#DJULIAN@ FEB 1700" gives
// 17000211 and 17000311. Both keys have a month and a day.
func (sd SimpleDate) Bounds() (first, last int) {
	y := sd.year()
	fm, fd := sd.Month, sd.Day
	if fm == 0 {
		fm = 1
	}
	if fd == 0 {
		fd = 1
	}
	lm, ld := sd.Month, sd.Day
	if lm == 0 {
		lm = 12
	}
	if ld == 0 {
		ld = daysIn(sd.Calendar, sd.leapYear(), lm)
	}
	if sd.Calendar != Julian {
		return key(y, fm, fd, sd.BC), key(y, lm, ld, sd.BC)
	}
	fy, fmo, fday, fbc := julianToGregorian(y, fm, fd, sd.BC)
	ly, lmo, lday, lbc := julianToGregorian(y, lm, ld, sd.BC)
	return key(fy, fmo, fday, fbc), key(ly, lmo, lday, lbc)
}

func key(y, m, d int, bc bool) int {
	if bc {
		y = -y
	}
	return y*10000 + m*100 + d
}

// SplitKey is the inverse of a sort key: year (negative for B.C.), month
// and day, 0 when unknown. It divides with rounding down, so the key
// -499685 of "15 MAR 50 B.C." gives -50, 3, 15.
func SplitKey(key int) (year, month, day int) {
	year = floorDiv(key, 10000)
	rest := key - year*10000
	return year, rest / 100, rest % 100
}

// KeyYear is the year of a sort key, negative for B.C.
func KeyYear(key int) int {
	y, _, _ := SplitKey(key)
	return y
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// julianToGregorian converts via the Julian Day Number. bc says y is a
// B.C. year (1 B.C. is the astronomical year 0).
func julianToGregorian(y, m, d int, bc bool) (year, month, day int, isBC bool) {
	ay := y
	if bc {
		ay = 1 - y
	}
	// The algorithm needs astronomical years above -4800; GoTree's four
	// digit years can reach 9999 B.C., which is left as it is.
	if ay <= -4800 {
		return y, m, d, bc
	}
	a := (14 - m) / 12
	yy := ay + 4800 - a
	mm := m + 12*a - 3
	jdn := d + (153*mm+2)/5 + 365*yy + yy/4 - 32083

	a = jdn + 32044
	b := (4*a + 3) / 146097
	c := a - 146097*b/4
	dd := (4*c + 3) / 1461
	e := c - 1461*dd/4
	mo := (5*e + 2) / 153
	day = e - (153*mo+2)/5 + 1
	month = mo + 3 - 12*(mo/10)
	year = 100*b + dd - 4800 + mo/10
	if year <= 0 {
		return 1 - year, month, day, true
	}
	return year, month, day, false
}

// GEDCOM7 renders the date for GEDCOM 7, which spells calendars as
// keywords ("JULIAN 4 OCT 1582"), writes "BCE" and has neither INT nor
// dual years. The value of a dual date carries its later year, as in the 7.0
// specification's own example ("30 JAN 1650" with the phrase
// "30 JAN 1649/50"), and the phrase the 5.5.1 form. What 7.0 cannot express goes into phrase, to be written as a
// DATE.PHRASE substructure; value may then be empty.
//
// A Date has one phrase, so two known limits apply: an INT date with a dual
// year and an explanation keeps only the explanation (without an
// explanation it keeps the dual year), and a range with dual years on both
// ends keeps only the last end's.
func (d Date) GEDCOM7() (value, phrase string) {
	simple := func(sd *SimpleDate) string {
		c := *sd
		if c.DualYear != 0 {
			phrase = sd.String()
			c.Year, c.DualYear = c.DualYear, 0
		}
		s := strings.Replace(c.String(), "@#DJULIAN@ ", "JULIAN ", 1)
		return strings.Replace(s, " B.C.", " BCE", 1)
	}
	switch d.Qualifier {
	case QualifierPhrase:
		return "", d.Phrase
	case QualifierBetween:
		value = "BET " + simple(d.Start) + " AND " + simple(d.End)
	case QualifierFromTo:
		value = "FROM " + simple(d.Start) + " TO " + simple(d.End)
	case QualifierTo:
		value = "TO " + simple(d.End)
	case QualifierInterpreted:
		value = simple(d.Start)
		if d.Phrase != "" {
			phrase = d.Phrase
		}
	case QualifierNone:
		value = simple(d.Start)
	default:
		value = string(d.Qualifier) + " " + simple(d.Start)
	}
	return value, phrase
}

// KeyVersion changes whenever Parse, String or the sort keys change for
// text that was already accepted or rejected, so stored keys can be
// recomputed (see db.Open). 2: dual years and Julian dates without a day (#55).
const KeyVersion = "2"
