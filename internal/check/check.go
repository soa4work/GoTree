// Package check finds implausible data in a family tree: people born after
// they died, parents who would have been children themselves, events after
// a burial. The rules work on a plain snapshot so they need no database.
//
// Dates are compared as ranges, and a rule only fires when every reading of
// the dates involved is implausible: "ABT 1850" and "1849" never contradict
// each other, "1850" and "1849" do.
package check

import (
	"math"
	"sort"
	"time"

	"github.com/praetorianer777/gotree/internal/gendate"
)

// Severity tells how sure a rule is that something is wrong.
type Severity string

// Severities.
const (
	// Error is impossible, such as a birth after the death.
	Error Severity = "error"
	// Warning is very unlikely, such as a mother aged 60.
	Warning Severity = "warning"
)

// Rule names; the UI translates them.
const (
	RuleBirthAfterDeath    = "birth_after_death"
	RuleBurialBeforeDeath  = "burial_before_death"
	RuleEventBeforeBirth   = "event_before_birth"
	RuleEventAfterDeath    = "event_after_death"
	RuleTooOld             = "too_old"
	RuleLivingTooOld       = "living_too_old"
	RuleParentTooYoung     = "parent_too_young"
	RuleMotherTooOld       = "mother_too_old"
	RuleBornAfterMotherDie = "born_after_mother_death"
	RuleBornAfterFatherDie = "born_after_father_death"
	RuleMarriedBeforeBirth = "married_before_birth"
	RuleMarriedAfterDeath  = "married_after_death"
	RuleInvalidDate        = "invalid_date"
)

// Limits used by the rules, in years.
const (
	MaxAge          = 110
	MinParentAge    = 12
	MaxMotherAge    = 55
	posthumousYears = 1
)

// Range is the earliest and latest day something can have happened, as
// day numbers from Day. Open ends are math.MinInt and math.MaxInt.
type Range struct {
	Lo, Hi int
}

func (r Range) known() bool { return r.Lo != math.MinInt || r.Hi != math.MaxInt }

// Person is the part of a person the rules look at.
type Person struct {
	ID  int64
	Sex string
	// Living is the explicit flag, nil when not stated.
	Living *bool
}

// Event is a dated event of a person or a family. Disproven events are
// left out by the caller.
type Event struct {
	ID       int64
	PersonID int64
	FamilyID int64
	Type     string
	// Date is nil when the event has no usable date.
	Date *Range
	// Invalid is set when the event has date text that is not a date.
	Invalid bool
}

// Child is a child link of a family.
type Child struct {
	PersonID int64
	// Birth1 and Birth2 say whether the child is a birth child of
	// partner 1 and partner 2.
	Birth1, Birth2 bool
}

// Family is a couple and its children.
type Family struct {
	ID                 int64
	Partner1, Partner2 int64
	Children           []Child
}

// Data is the snapshot the rules run on.
type Data struct {
	Persons  []Person
	Events   []Event
	Families []Family
}

// Finding is one implausibility.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	// PersonID is the person the finding is about.
	PersonID int64 `json:"personId"`
	// OtherPersonID is the second person involved, such as the parent.
	OtherPersonID int64  `json:"otherPersonId,omitempty"`
	FamilyID      int64  `json:"familyId,omitempty"`
	EventID       int64  `json:"eventId,omitempty"`
	EventType     string `json:"eventType,omitempty"`
	// Years is the age or gap the rule measured, where it has one.
	Years int `json:"years,omitempty"`
}

// Run applies every rule. now is the reference for "living" checks.
func Run(d Data, now time.Time) []Finding {
	c := checker{now: Day(now.Year(), int(now.Month()), now.Day())}
	c.index(d)
	for _, p := range d.Persons {
		c.person(p)
	}
	for _, f := range d.Families {
		c.family(f)
	}
	for _, e := range d.Events {
		if e.Invalid {
			c.add(Finding{Rule: RuleInvalidDate, Severity: Warning, PersonID: e.PersonID, FamilyID: e.FamilyID, EventID: e.ID, EventType: e.Type})
		}
	}
	sort.SliceStable(c.out, func(i, j int) bool {
		if c.out[i].Severity != c.out[j].Severity {
			return c.out[i].Severity == Error
		}
		return c.out[i].PersonID < c.out[j].PersonID
	})
	return c.out
}

type life struct {
	birth, death Range
	events       []Event
}

type checker struct {
	now      int
	lives    map[int64]*life
	sex      map[int64]string
	families map[int64][]Event
	out      []Finding
}

const yearDays = 372

// Day numbers a calendar day so ranges compare by subtraction; every month
// counts 31 days, which is close enough for the age limits used here.
func Day(year, month, day int) int {
	return year*yearDays + (month-1)*31 + (day - 1)
}

// FromSortKeys turns the stored YYYYMMDD sort keys of a date (negative
// years for B.C.) into a Range; a 00 month or day widens the range to the
// whole year or month, and the qualifier opens or widens it.
func FromSortKeys(start, end int, qualifier string) Range {
	lo := func(k int) int {
		y, m, d := gendate.SplitKey(k)
		if m == 0 {
			m = 1
		}
		if d == 0 {
			d = 1
		}
		return Day(y, m, d)
	}
	hi := func(k int) int {
		y, m, d := gendate.SplitKey(k)
		if m == 0 {
			m = 12
		}
		if d == 0 {
			d = 31
		}
		return Day(y, m, d)
	}
	return widen(Range{lo(start), hi(end)}, qualifier)
}

// FromDate is FromSortKeys for a parsed date. It is exact for partial
// Julian dates, whose month or year does not line up with the Gregorian one.
// ok is false for a date without a calendar date (a phrase).
func FromDate(d gendate.Date) (r Range, ok bool) {
	first, last := d.Start, d.End
	if first == nil {
		first = last
	}
	if last == nil {
		last = first
	}
	if first == nil {
		return Range{}, false
	}
	lo, _ := first.Bounds()
	_, hi := last.Bounds()
	ly, lm, ld := gendate.SplitKey(lo)
	hy, hm, hd := gendate.SplitKey(hi)
	return widen(Range{Day(ly, lm, ld), Day(hy, hm, hd)}, string(d.Qualifier)), true
}

// widen opens or widens a range by the qualifier of its date.
func widen(r Range, qualifier string) Range {
	switch qualifier {
	case "BEF", "TO":
		r.Lo = math.MinInt
	case "AFT", "FROM":
		r.Hi = math.MaxInt
	case "ABT", "EST":
		r.Lo -= 2 * yearDays
		r.Hi += 2 * yearDays
	}
	return r
}

var open = Range{math.MinInt, math.MaxInt}

func (c *checker) index(d Data) {
	c.lives = map[int64]*life{}
	c.sex = map[int64]string{}
	c.families = map[int64][]Event{}
	for _, p := range d.Persons {
		c.lives[p.ID] = &life{birth: open, death: open}
		c.sex[p.ID] = p.Sex
	}

	// Birth and death come from BIRT and DEAT; without them a baptism or
	// burial still bounds them from above.
	type pick struct{ exact, proxy *Range }
	births, deaths := map[int64]*pick{}, map[int64]*pick{}
	set := func(m map[int64]*pick, id int64, r Range, exact bool) {
		p := m[id]
		if p == nil {
			p = &pick{}
			m[id] = p
		}
		if exact && p.exact == nil {
			p.exact = &r
		} else if !exact && (p.proxy == nil || r.Hi < p.proxy.Hi) {
			p.proxy = &Range{math.MinInt, r.Hi}
		}
	}
	for _, e := range d.Events {
		if e.FamilyID != 0 {
			c.families[e.FamilyID] = append(c.families[e.FamilyID], e)
			continue
		}
		l := c.lives[e.PersonID]
		if l == nil {
			continue
		}
		l.events = append(l.events, e)
		if e.Date == nil {
			continue
		}
		switch e.Type {
		case "BIRT":
			set(births, e.PersonID, *e.Date, true)
		case "CHR", "BAPM":
			set(births, e.PersonID, *e.Date, false)
		case "DEAT":
			set(deaths, e.PersonID, *e.Date, true)
		case "BURI", "CREM":
			set(deaths, e.PersonID, *e.Date, false)
		}
	}
	for id, p := range births {
		if p.exact != nil {
			c.lives[id].birth = *p.exact
		} else {
			c.lives[id].birth = *p.proxy
		}
	}
	for id, p := range deaths {
		if p.exact != nil {
			c.lives[id].death = *p.exact
		} else {
			c.lives[id].death = *p.proxy
		}
	}
}

func (c *checker) add(f Finding) { c.out = append(c.out, f) }

// before reports whether a certainly happened before b.
func before(a, b Range) bool {
	return a.Hi != math.MaxInt && b.Lo != math.MinInt && a.Hi < b.Lo
}

func years(days int) int { return days / yearDays }

// eventsAfterDeath may legitimately be dated after the death.
var eventsAfterDeath = map[string]bool{"BURI": true, "CREM": true, "PROB": true, "_FUN": true, "OBIT": true}

func (c *checker) person(p Person) {
	l := c.lives[p.ID]
	if before(l.death, l.birth) {
		c.add(Finding{Rule: RuleBirthAfterDeath, Severity: Error, PersonID: p.ID})
	}
	for _, e := range l.events {
		if e.Date == nil {
			continue
		}
		switch {
		case e.Type == "BIRT" || e.Type == "DEAT":
		case (e.Type == "BURI" || e.Type == "CREM") && before(*e.Date, l.death):
			c.add(Finding{Rule: RuleBurialBeforeDeath, Severity: Error, PersonID: p.ID, EventID: e.ID, EventType: e.Type})
		case before(*e.Date, l.birth):
			c.add(Finding{Rule: RuleEventBeforeBirth, Severity: Error, PersonID: p.ID, EventID: e.ID, EventType: e.Type})
		case !eventsAfterDeath[e.Type] && before(l.death, *e.Date):
			c.add(Finding{Rule: RuleEventAfterDeath, Severity: Warning, PersonID: p.ID, EventID: e.ID, EventType: e.Type})
		}
	}
	if l.birth.Hi != math.MaxInt && l.death.Lo != math.MinInt && l.death.Lo-l.birth.Hi > MaxAge*yearDays {
		c.add(Finding{Rule: RuleTooOld, Severity: Warning, PersonID: p.ID, Years: years(l.death.Lo - l.birth.Hi)})
	}
	if p.Living != nil && *p.Living && l.birth.Hi != math.MaxInt && c.now-l.birth.Hi > MaxAge*yearDays {
		c.add(Finding{Rule: RuleLivingTooOld, Severity: Warning, PersonID: p.ID, Years: years(c.now - l.birth.Hi)})
	}
}

func (c *checker) family(f Family) {
	partners := []int64{f.Partner1, f.Partner2}
	for _, id := range partners {
		l := c.lives[id]
		if l == nil {
			continue
		}
		for _, e := range c.families[f.ID] {
			if e.Date == nil || !isUnion(e.Type) {
				continue
			}
			if before(*e.Date, l.birth) {
				c.add(Finding{Rule: RuleMarriedBeforeBirth, Severity: Error, PersonID: id, FamilyID: f.ID, EventID: e.ID, EventType: e.Type})
			} else if before(l.death, *e.Date) {
				c.add(Finding{Rule: RuleMarriedAfterDeath, Severity: Error, PersonID: id, FamilyID: f.ID, EventID: e.ID, EventType: e.Type})
			}
		}
	}

	for _, ch := range f.Children {
		child := c.lives[ch.PersonID]
		if child == nil || !child.birth.known() {
			continue
		}
		for i, parentID := range partners {
			parent := c.lives[parentID]
			if parent == nil || !(i == 0 && ch.Birth1 || i == 1 && ch.Birth2) {
				continue
			}
			base := Finding{Severity: Warning, PersonID: ch.PersonID, OtherPersonID: parentID, FamilyID: f.ID}
			if parent.birth.Lo != math.MinInt && child.birth.Hi != math.MaxInt && child.birth.Hi-parent.birth.Lo < MinParentAge*yearDays {
				base.Rule, base.Years = RuleParentTooYoung, max(0, years(child.birth.Hi-parent.birth.Lo))
				c.add(base)
				continue
			}
			mother := c.sex[parentID] == "F"
			if mother && parent.birth.Hi != math.MaxInt && child.birth.Lo != math.MinInt && child.birth.Lo-parent.birth.Hi > MaxMotherAge*yearDays {
				base.Rule, base.Years = RuleMotherTooOld, years(child.birth.Lo-parent.birth.Hi)
				c.add(base)
				continue
			}
			grace := 0
			rule := RuleBornAfterMotherDie
			if !mother {
				grace, rule = posthumousYears*yearDays, RuleBornAfterFatherDie
			}
			if parent.death.Hi != math.MaxInt && child.birth.Lo != math.MinInt && child.birth.Lo > parent.death.Hi+grace {
				base.Rule, base.Severity = rule, Error
				c.add(base)
			}
		}
	}
}

func isUnion(t string) bool {
	return t == "MARR" || t == "ENGA" || t == "MARB" || t == "MARC" || t == "MARL"
}
