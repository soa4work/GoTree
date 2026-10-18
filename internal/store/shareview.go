package store

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"unicode"

	"github.com/praetorianer777/gotree/internal/db"
	"golang.org/x/text/unicode/norm"
)

// shareScope decides, per person, what a share link reveals. Everything a
// visitor sees passes through it: people outside the scope do not exist
// for them, and with living_names, living people keep their name and sex
// but lose dates, events, notes and other names.
type shareScope struct {
	// visible is nil when the whole tree is in scope.
	visible   map[int64]bool
	living    map[int64]bool
	namesOnly bool
	deceased  bool
}

func (sc *shareScope) sees(id int64) bool {
	if sc.visible != nil && !sc.visible[id] {
		return false
	}
	return !(sc.deceased && sc.living[id])
}

// reduced reports whether only the name of a visible person may be shown.
func (sc *shareScope) reduced(id int64) bool { return sc.namesOnly && sc.living[id] }

func (sc *shareScope) full(id int64) bool { return sc.sees(id) && !sc.reduced(id) }

func (s *Store) scopeOf(ctx context.Context, sh Share) (*shareScope, error) {
	living, err := s.livingAll(ctx, sh.Actor)
	if err != nil {
		return nil, err
	}
	sc := &shareScope{
		living:    living,
		namesOnly: sh.Link.Privacy == SharePrivacyLivingNames,
		deceased:  sh.Link.Privacy == SharePrivacyDeceased,
	}
	if sh.Link.Scope == ShareScopeDescendants && sh.Link.Root != nil {
		sc.visible, err = s.descendantsWithPartners(ctx, sh.Actor, sh.Link.Root.ID)
		if err != nil {
			return nil, err
		}
	}
	return sc, nil
}

// descendantsWithPartners is root, everyone descending from root, and the
// partners of all of them. A partner who is also a descendant (cousins who
// married) is expanded when reached as a child.
func (s *Store) descendantsWithPartners(ctx context.Context, a Actor, root int64) (map[int64]bool, error) {
	fams, err := s.familyLinks(ctx, a)
	if err != nil {
		return nil, err
	}
	byPartner := map[int64][]familyRow{}
	for _, f := range fams {
		for _, p := range []int64{f.p1, f.p2} {
			if p != 0 {
				byPartner[p] = append(byPartner[p], f)
			}
		}
	}
	out := map[int64]bool{root: true}      // shown: descendants and partners
	expanded := map[int64]bool{root: true} // descendants whose own families were followed
	queue := []int64{root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, f := range byPartner[id] {
			for _, p := range []int64{f.p1, f.p2} {
				if p != 0 {
					out[p] = true // a partner is shown, not expanded
				}
			}
			for _, c := range f.children {
				out[c.id] = true
				if !expanded[c.id] {
					expanded[c.id] = true
					queue = append(queue, c.id)
				}
			}
		}
	}
	return out, nil
}

func (sc *shareScope) ref(r PersonRef) PersonRef {
	r.Portrait = nil
	if sc.reduced(r.ID) {
		r.BirthDate, r.DeathDate, r.BirthSort = "", "", 0
	}
	return r
}

func (sc *shareScope) refPtr(r *PersonRef) *PersonRef {
	if r == nil || !sc.sees(r.ID) {
		return nil
	}
	v := sc.ref(*r)
	return &v
}

func (sc *shareScope) family(f Family) Family {
	whole := true
	for _, p := range []*PersonRef{f.Partner1, f.Partner2} {
		if p != nil && !sc.full(p.ID) {
			whole = false
		}
	}
	f.Partner1, f.Partner2 = sc.refPtr(f.Partner1), sc.refPtr(f.Partner2)
	children := []ChildLink{}
	for _, c := range f.Children {
		if sc.sees(c.Person.ID) {
			c.Person = sc.ref(c.Person)
			children = append(children, c)
		}
	}
	f.Children = children
	// A marriage date or note says as much about a hidden partner as about
	// the visible one.
	if !whole {
		f.Events, f.Notes, f.Citations = []Event{}, "", []CitationRef{}
	} else {
		f.Events = sc.events(f.Events)
	}
	return f
}

func (sc *shareScope) events(events []Event) []Event {
	out := []Event{}
	for _, e := range events {
		parts := []Participant{}
		for _, p := range e.Participants {
			if sc.sees(p.Person.ID) {
				p.Person = sc.ref(p.Person)
				parts = append(parts, p)
			}
		}
		e.Participants = parts
		out = append(out, e)
	}
	return out
}

// ShareInfo describes a share link to its visitor.
type ShareInfo struct {
	TreeName string     `json:"treeName"`
	Label    string     `json:"label"`
	Scope    string     `json:"scope"`
	Privacy  string     `json:"privacy"`
	Root     *PersonRef `json:"root"`
	// StartID is the person to open first: the root, or else the first
	// visible person.
	StartID *int64 `json:"startId"`
}

// ShareInfo returns what the link shows.
func (s *Store) ShareInfo(ctx context.Context, sh Share) (ShareInfo, error) {
	sc, err := s.scopeOf(ctx, sh)
	if err != nil {
		return ShareInfo{}, err
	}
	info := ShareInfo{TreeName: sh.TreeName, Label: sh.Link.Label, Scope: sh.Link.Scope, Privacy: sh.Link.Privacy}
	if sh.Link.Root != nil && sc.sees(sh.Link.Root.ID) {
		info.Root = sc.refPtr(sh.Link.Root)
		info.StartID = &sh.Link.Root.ID
		return info, nil
	}
	ids, err := personIDs(ctx, s.DB, sh.Actor, "", -1, 0)
	if err != nil {
		return ShareInfo{}, err
	}
	for _, id := range ids {
		if sc.sees(id) {
			info.StartID = &id
			break
		}
	}
	return info, nil
}

// maxShareResults bounds a share link's person list.
const maxShareResults = 100

// SharePersons searches the people a link shows: the first maxShareResults in
// list order, and how many there are. Only those are loaded.
func (s *Store) SharePersons(ctx context.Context, sh Share, query string) (PersonList, error) {
	sc, err := s.scopeOf(ctx, sh)
	if err != nil {
		return PersonList{}, err
	}
	// The search index also holds nicknames and alternate names (a maiden
	// name, say). A person shown by name only must not be found by those,
	// or the link would reveal names it hides. Relevance is computed over
	// all names too, so such links list matches in name order instead.
	var ids []int64
	var words []string
	var given, surname map[int64]string
	if sc.namesOnly {
		words = searchWords(query)
	}
	if len(words) > 0 {
		if given, surname, err = s.primaryNames(ctx, sh.Actor); err != nil {
			return PersonList{}, err
		}
		ids, err = queryIDs(ctx, s.DB, searchPersonsByNameSQL, ftsQuery(query), sh.Actor.TreeID, -1, 0)
	} else {
		ids, err = personIDs(ctx, s.DB, sh.Actor, ftsQuery(query), -1, 0)
	}
	if err != nil {
		return PersonList{}, err
	}
	out := PersonList{Items: []PersonRef{}}
	var shown []int64
	for _, id := range ids {
		if !sc.sees(id) {
			continue
		}
		if len(words) > 0 && sc.reduced(id) && !primaryNameMatches(given[id], surname[id], words) {
			continue
		}
		out.Total++
		if len(shown) < maxShareResults {
			shown = append(shown, id)
		}
	}
	refs, err := s.personRefs(ctx, s.DB, sh.Actor, shown)
	if err != nil {
		return PersonList{}, err
	}
	for _, id := range shown {
		out.Items = append(out.Items, sc.ref(refs[id]))
	}
	return out, nil
}

// searchPersonsByNameSQL is searchPersonsSQL in list order, without the
// relevance that hidden names would sway.
const searchPersonsByNameSQL = `
	SELECT p.id FROM persons_fts f JOIN persons p ON p.id = f.rowid
	WHERE persons_fts MATCH ? AND p.tree_id = ?
	ORDER BY p.surname = '', p.surname_key, p.given_key, p.id
	LIMIT ? OFFSET ?`

// primaryNames loads the given names and surname of every person of the tree.
func (s *Store) primaryNames(ctx context.Context, a Actor) (given, surname map[int64]string, err error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, given_names, surname FROM persons WHERE tree_id = ?`, a.TreeID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	given, surname = map[int64]string{}, map[int64]string{}
	for rows.Next() {
		var id int64
		var g, sn string
		if err := rows.Scan(&id, &g, &sn); err != nil {
			return nil, nil, err
		}
		given[id], surname[id] = g, sn
	}
	return given, surname, rows.Err()
}

// nameTokens splits a name the way the search index does: folded like
// searchFold, lower-cased, accents of Latin letters removed, split at
// non-letters/digits. The index keeps other accents (ї stays ї), so they
// stay here too: dropping them would let «і» match «Їжак» while the index
// matched a hidden «Іван».
func nameTokens(s string) []string {
	var b strings.Builder
	latin := false
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			if latin {
				continue
			}
		} else {
			latin = unicode.Is(unicode.Latin, r)
		}
		b.WriteRune(r)
	}
	return strings.FieldsFunc(db.SearchFold(b.String()), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// primaryNameMatches reports whether every query word starts a word of the
// person's primary given names or surname. It only ever removes search
// results, so a difference from the index's tokenizer hides a person
// instead of revealing one.
func primaryNameMatches(given, surname string, words []string) bool {
	tokens := append(nameTokens(given), nameTokens(surname)...)
	for _, w := range words {
		w = strings.Join(nameTokens(w), "")
		found := false
		for _, t := range tokens {
			if strings.HasPrefix(t, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// SharePerson returns a person as the link may show them.
func (s *Store) SharePerson(ctx context.Context, sh Share, id int64) (PersonDetail, error) {
	sc, err := s.scopeOf(ctx, sh)
	if err != nil {
		return PersonDetail{}, err
	}
	if !sc.sees(id) {
		return PersonDetail{}, ErrNotFound
	}
	d, err := s.GetPersonDetail(ctx, sh.Actor, id)
	if err != nil {
		return PersonDetail{}, err
	}
	d.Portrait = nil
	d.CreatedAt, d.UpdatedAt = "", ""
	if sc.reduced(id) {
		d.NamePrefix, d.NameSuffix, d.Nickname, d.Notes, d.NameNotes = "", "", "", "", ""
		d.IsLiving = nil
		d.AlternateNames, d.Citations, d.Events = []AlternateName{}, []CitationRef{}, []Event{}
	} else {
		var events []Event
		for _, e := range d.Events {
			// Taking part in someone else's event reveals that event; it
			// stays only when its principal could be shown in full.
			if e.Role != "" && (e.PersonID == nil || !sc.full(*e.PersonID)) {
				continue
			}
			events = append(events, e)
		}
		d.Events = sc.events(events)
	}
	for i := range d.ParentFamilies {
		d.ParentFamilies[i] = sc.family(d.ParentFamilies[i])
	}
	for i := range d.PartnerFamilies {
		d.PartnerFamilies[i] = sc.family(d.PartnerFamilies[i])
	}
	return d, nil
}

// ShareTree is Tree limited to what the link shows.
func (s *Store) ShareTree(ctx context.Context, sh Share, rootID int64, opt TreeOptions) (TreeGraph, error) {
	sc, err := s.scopeOf(ctx, sh)
	if err != nil {
		return TreeGraph{}, err
	}
	if !sc.sees(rootID) {
		return TreeGraph{}, ErrNotFound
	}
	g, err := s.Tree(ctx, sh.Actor, rootID, opt)
	if err != nil {
		return TreeGraph{}, err
	}
	persons := map[int64]PersonRef{}
	for id, p := range g.Persons {
		if sc.sees(id) {
			persons[id] = sc.ref(p)
		}
	}
	keep := func(p *int64) *int64 {
		if p != nil && !sc.sees(*p) {
			return nil
		}
		return p
	}
	fams := []TreeFamily{}
	for _, f := range g.Families {
		f.Partner1ID, f.Partner2ID = keep(f.Partner1ID), keep(f.Partner2ID)
		var children []TreeChild
		for _, c := range f.Children {
			if sc.sees(c.PersonID) {
				children = append(children, c)
			}
		}
		f.Children = children
		if f.Partner1ID == nil && f.Partner2ID == nil && len(f.Children) == 0 {
			continue
		}
		if f.Children == nil {
			f.Children = []TreeChild{}
		}
		fams = append(fams, f)
	}
	g.Persons, g.Families = persons, fams
	return g, nil
}

// DayEvent is a birth, marriage or death on a given day of the year.
type DayEvent struct {
	EventID   int64   `json:"eventId"`
	Type      string  `json:"type"`
	Year      int     `json:"year"`
	PersonIDs []int64 `json:"personIds"`
}

// DayReport lists what happened on one day of the year.
type DayReport struct {
	Events  []DayEvent          `json:"events"`
	Persons map[int64]PersonRef `json:"persons"`
}

// OnThisDay lists births, marriages and deaths of deceased people on the
// given month and day, oldest first. Only exact dates count; for a share
// link, only people the link shows in full.
func (s *Store) OnThisDay(ctx context.Context, a Actor, month, day int) (DayReport, error) {
	living, err := s.livingAll(ctx, a)
	if err != nil {
		return DayReport{}, err
	}
	return s.onThisDay(ctx, a, month, day, func(id int64) bool { return !living[id] })
}

// ShareOnThisDay is OnThisDay for a share link.
func (s *Store) ShareOnThisDay(ctx context.Context, sh Share, month, day int) (DayReport, error) {
	sc, err := s.scopeOf(ctx, sh)
	if err != nil {
		return DayReport{}, err
	}
	return s.onThisDay(ctx, sh.Actor, month, day, func(id int64) bool { return sc.full(id) && !sc.living[id] })
}

func (s *Store) onThisDay(ctx context.Context, a Actor, month, day int, show func(int64) bool) (DayReport, error) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return DayReport{}, &ValidationError{Fields: map[string]string{"date": "month and day are out of range"}}
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.id, e.type, e.date_sort / 10000, e.person_id, f.partner1_id, f.partner2_id
		FROM events e LEFT JOIN families f ON f.id = e.family_id
		WHERE e.tree_id = ? AND e.type IN ('BIRT', 'MARR', 'DEAT') AND e.status <> 'disproven'
			AND e.date_qualifier = '' AND e.date_sort > 0 AND e.date_sort % 10000 = ?
		ORDER BY e.date_sort, e.id`, a.TreeID, month*100+day)
	if err != nil {
		return DayReport{}, err
	}
	defer rows.Close()
	rep := DayReport{Events: []DayEvent{}}
	var ids []int64
	for rows.Next() {
		var e DayEvent
		var person, p1, p2 sql.NullInt64
		if err := rows.Scan(&e.EventID, &e.Type, &e.Year, &person, &p1, &p2); err != nil {
			return DayReport{}, err
		}
		for _, p := range []sql.NullInt64{person, p1, p2} {
			if p.Valid {
				e.PersonIDs = append(e.PersonIDs, p.Int64)
			}
		}
		ok := len(e.PersonIDs) > 0
		for _, id := range e.PersonIDs {
			ok = ok && show(id)
		}
		if ok {
			rep.Events = append(rep.Events, e)
			ids = append(ids, e.PersonIDs...)
		}
	}
	if err := rows.Err(); err != nil {
		return DayReport{}, err
	}
	rows.Close()
	rep.Persons, err = s.personRefsChunked(ctx, a, ids)
	for id, r := range rep.Persons {
		r.Portrait = nil
		rep.Persons[id] = r
	}
	sort.SliceStable(rep.Events, func(i, j int) bool { return rep.Events[i].Year < rep.Events[j].Year })
	return rep, err
}
