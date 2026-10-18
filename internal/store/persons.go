package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/praetorianer777/gotree/internal/db"
	"github.com/praetorianer777/gotree/internal/gendate"
)

// AlternateName is a further name of a person: birth name, married name, …
type AlternateName struct {
	ID           int64         `json:"id"`
	Type         string        `json:"type"`
	GivenNames   string        `json:"givenNames"`
	Surname      string        `json:"surname"`
	NamePrefix   string        `json:"namePrefix"`
	NameSuffix   string        `json:"nameSuffix"`
	Nickname     string        `json:"nickname"`
	Status       string        `json:"status"`
	StatusReason string        `json:"statusReason"`
	SortOrder    int           `json:"sortOrder"`
	Notes        string        `json:"notes"`
	Citations    []CitationRef `json:"citations"`
}

// Person is a person's own record.
type Person struct {
	ID         int64  `json:"id"`
	GivenNames string `json:"givenNames"`
	Surname    string `json:"surname"`
	NamePrefix string `json:"namePrefix"`
	NameSuffix string `json:"nameSuffix"`
	Nickname   string `json:"nickname"`
	Sex        string `json:"sex"`
	// IsLiving is what was stated; nil means "infer from the events".
	IsLiving *bool `json:"isLiving"`
	// Living is the effective value, stated or inferred.
	Living bool   `json:"living"`
	Notes  string `json:"notes"`
	// NameNotes are notes on the main name (GEDCOM NAME.NOTE).
	NameNotes      string          `json:"nameNotes"`
	AlternateNames []AlternateName `json:"alternateNames"`
	// Citations support the person as a whole.
	Citations []CitationRef `json:"citations"`
	Portrait  *PortraitRef  `json:"portrait"`
	CreatedAt string        `json:"createdAt"`
	UpdatedAt string        `json:"updatedAt"`
}

// PersonRef is the short form of a person used in lists and relations.
type PersonRef struct {
	ID         int64  `json:"id"`
	GivenNames string `json:"givenNames"`
	Surname    string `json:"surname"`
	Sex        string `json:"sex"`
	BirthDate  string `json:"birthDate"`
	// BirthSort is the birth date as YYYYMMDD for ordering people by age
	// (relationship names by seniority); 0 when no dated birth is known.
	BirthSort int64        `json:"birthSort,omitempty"`
	DeathDate string       `json:"deathDate"`
	Living    bool         `json:"living"`
	Portrait  *PortraitRef `json:"portrait"`
}

// AlternateNameInput is the editable part of an AlternateName. ID keeps an
// existing name (and the citations attached to it); without it a new name
// is created.
type AlternateNameInput struct {
	ID           *int64 `json:"id,omitempty"`
	Type         string `json:"type"`
	GivenNames   string `json:"givenNames"`
	Surname      string `json:"surname"`
	NamePrefix   string `json:"namePrefix"`
	NameSuffix   string `json:"nameSuffix"`
	Nickname     string `json:"nickname"`
	Status       string `json:"status"`
	StatusReason string `json:"statusReason"`
	Notes        string `json:"notes"`
}

// PersonInput is the editable part of a Person. AlternateNames replaces the
// existing list.
type PersonInput struct {
	GivenNames     string               `json:"givenNames"`
	Surname        string               `json:"surname"`
	NamePrefix     string               `json:"namePrefix"`
	NameSuffix     string               `json:"nameSuffix"`
	Nickname       string               `json:"nickname"`
	Sex            string               `json:"sex"`
	IsLiving       *bool                `json:"isLiving"`
	Notes          string               `json:"notes"`
	NameNotes      string               `json:"nameNotes"`
	AlternateNames []AlternateNameInput `json:"alternateNames"`
	// Birth and Death record a BIRT or DEAT event together with a new
	// person. Only creating a person reads them; UpdatePerson ignores them
	// because existing events are edited as events.
	Birth *LifeEventInput `json:"birth,omitempty"`
	Death *LifeEventInput `json:"death,omitempty"`
	// AddCitations cites sources for the person as a whole.
	AddCitations []NewCitation `json:"addCitations,omitempty"`
	// RemoveCitations detaches citations from the person.
	RemoveCitations []int64 `json:"removeCitations,omitempty"`
}

// LifeEventInput is the date and place of a birth or death entered with a new
// person. When both are empty no event is created.
type LifeEventInput struct {
	Date    string `json:"date"`
	PlaceID *int64 `json:"placeId"`
}

func (l *LifeEventInput) dateOrEmpty() string {
	if l == nil {
		return ""
	}
	return l.Date
}

func (l *LifeEventInput) empty() bool {
	return l == nil || (strings.TrimSpace(l.Date) == "" && l.PlaceID == nil)
}

// PersonDetail is everything the person page shows.
type PersonDetail struct {
	Person
	// Events are the person's own events plus those they take part in
	// (with Role set).
	Events []Event `json:"events"`
	// ParentFamilies are the families the person is a child of.
	ParentFamilies []Family `json:"parentFamilies"`
	// PartnerFamilies are the families the person is a partner in.
	PartnerFamilies []Family `json:"partnerFamilies"`
}

// PersonList is one page of a person listing.
type PersonList struct {
	Items []PersonRef `json:"items"`
	Total int         `json:"total"`
}

var nameTypes = []string{"birth", "married", "aka", "religious", "immigrant", "other"}

// livingCutoffYears: without a death record, someone born longer ago than
// this is taken to be deceased.
const livingCutoffYears = 110

func (in *PersonInput) normalize() {
	trim := strings.TrimSpace
	in.GivenNames, in.Surname = collapse(in.GivenNames), collapse(in.Surname)
	in.NamePrefix, in.NameSuffix, in.Nickname = trim(in.NamePrefix), trim(in.NameSuffix), trim(in.Nickname)
	in.Sex = strings.ToUpper(trim(in.Sex))
	if in.Sex == "" {
		in.Sex = "U"
	}
	in.Notes, in.NameNotes = trim(in.Notes), trim(in.NameNotes)
	for i := range in.AlternateNames {
		n := &in.AlternateNames[i]
		n.Type = strings.ToLower(trim(n.Type))
		n.GivenNames, n.Surname = collapse(n.GivenNames), collapse(n.Surname)
		n.NamePrefix, n.NameSuffix, n.Nickname = trim(n.NamePrefix), trim(n.NameSuffix), trim(n.Nickname)
		n.Status = defaultStatus(n.Status)
		n.StatusReason = trim(n.StatusReason)
		n.Notes = trim(n.Notes)
	}
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (in PersonInput) validate() error {
	var v validator
	long := func(field, val string, max int) { v.check(utf8.RuneCountInString(val) <= max, field, "is too long") }
	long("givenNames", in.GivenNames, 200)
	long("surname", in.Surname, 200)
	long("namePrefix", in.NamePrefix, 50)
	long("nameSuffix", in.NameSuffix, 50)
	long("nickname", in.Nickname, 100)
	long("notes", in.Notes, 100_000)
	long("nameNotes", in.NameNotes, 100_000)
	v.check(oneOf(in.Sex, "M", "F", "U", "X"), "sex", "must be M, F, U or X")
	v.check(len(in.AlternateNames) <= 50, "alternateNames", "too many names")
	for _, n := range in.AlternateNames {
		v.check(oneOf(n.Type, nameTypes...), "alternateNames", "unknown name type "+n.Type)
		v.check(n.GivenNames != "" || n.Surname != "" || n.Nickname != "", "alternateNames", "a name needs at least a given name, surname or nickname")
		v.check(validStatus(n.Status), "alternateNames", "unknown status "+n.Status)
		v.check(n.Status == StatusAccepted || n.StatusReason != "", "alternateNames", "say why the name is disputed or disproven")
		long("alternateNames", n.GivenNames+n.Surname, 400)
		long("alternateNames", n.Notes, 100_000)
	}
	return v.err()
}

// validateLife checks the birth and death entered with a new person. Dates
// are kept as typed like event dates, so only their length is limited.
func (in PersonInput) validateLife() error {
	var v validator
	v.check(utf8.RuneCountInString(collapse(in.Birth.dateOrEmpty())) <= 120, "birth.date", "is too long")
	v.check(utf8.RuneCountInString(collapse(in.Death.dateOrEmpty())) <= 120, "death.date", "is too long")
	v.check(in.Death.empty() || in.IsLiving == nil || !*in.IsLiving, "death", "a living person has no death")
	return v.err()
}

// CreatePerson adds a person.
func (s *Store) CreatePerson(ctx context.Context, a Actor, in PersonInput) (Person, error) {
	var p Person
	err := s.tx(ctx, func(tx *sql.Tx) (err error) {
		p, err = s.createPerson(ctx, tx, a, in)
		return err
	})
	return p, err
}

func (s *Store) createPerson(ctx context.Context, tx *sql.Tx, a Actor, in PersonInput) (Person, error) {
	in.normalize()
	if err := in.validate(); err != nil {
		return Person{}, err
	}
	if err := in.validateLife(); err != nil {
		return Person{}, err
	}
	var p Person
	err := func() error {
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO persons (tree_id, given_names, surname, name_prefix, name_suffix, nickname, sex, is_living, notes,
				name_notes, created_at, updated_at, created_by, updated_by, surname_key, given_key)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, in.GivenNames, in.Surname, in.NamePrefix, in.NameSuffix, in.Nickname, in.Sex, in.IsLiving, in.Notes,
			in.NameNotes,
			now, now, nullID(a.UserID), nullID(a.UserID),
			db.NamesKey(in.Surname), db.NamesKey(in.GivenNames))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if err := writeAlternateNames(ctx, tx, id, in.AlternateNames); err != nil {
			return err
		}
		if err := s.addCitations(ctx, tx, a, "person", id, in.AddCitations); err != nil {
			return err
		}
		if err := reindexPerson(ctx, tx, id); err != nil {
			return err
		}
		if p, err = s.getPerson(ctx, tx, a, id); err != nil {
			return err
		}
		if err := s.logChange(ctx, tx, a, "person", id, "create", nil, p); err != nil {
			return err
		}
		for _, ev := range []struct {
			typ, field string
			in         *LifeEventInput
		}{{"BIRT", "birth", in.Birth}, {"DEAT", "death", in.Death}} {
			if ev.in.empty() {
				continue
			}
			if _, err := s.createEvent(ctx, tx, a, EventInput{PersonID: &id, Type: ev.typ, Date: ev.in.Date, PlaceID: ev.in.PlaceID}); err != nil {
				return prefixFields(err, ev.field)
			}
		}
		// Re-read: a death or an old birth changes whether the person is living.
		p, err = s.getPerson(ctx, tx, a, id)
		return err
	}()
	return p, err
}

// prefixFields names the form section an event error came from, so
// "placeId" becomes "birth.placeId".
func prefixFields(err error, prefix string) error {
	var ve *ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	fields := make(map[string]string, len(ve.Fields))
	for k, v := range ve.Fields {
		fields[prefix+"."+k] = v
	}
	return &ValidationError{Fields: fields}
}

// UpdatePerson replaces a person's own fields and alternate names.
func (s *Store) UpdatePerson(ctx context.Context, a Actor, id int64, in PersonInput) (Person, error) {
	in.normalize()
	if err := in.validate(); err != nil {
		return Person{}, err
	}
	var p Person
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getPerson(ctx, tx, a, id)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE persons SET given_names = ?, surname = ?, name_prefix = ?, name_suffix = ?, nickname = ?, sex = ?,
				is_living = ?, notes = ?, name_notes = ?, updated_at = ?, updated_by = ?, surname_key = ?, given_key = ?
			WHERE id = ? AND tree_id = ?`,
			in.GivenNames, in.Surname, in.NamePrefix, in.NameSuffix, in.Nickname, in.Sex, in.IsLiving, in.Notes, in.NameNotes,
			s.now(), nullID(a.UserID), db.NamesKey(in.Surname), db.NamesKey(in.GivenNames), id, a.TreeID)
		if err != nil {
			return err
		}
		if err := writeAlternateNames(ctx, tx, id, in.AlternateNames); err != nil {
			return err
		}
		if err := s.addCitations(ctx, tx, a, "person", id, in.AddCitations); err != nil {
			return err
		}
		if err := s.removeCitations(ctx, tx, a, "person", id, in.RemoveCitations); err != nil {
			return err
		}
		if err := reindexPerson(ctx, tx, id); err != nil {
			return err
		}
		if p, err = s.getPerson(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "person", id, "update", before, p)
	})
	return p, err
}

// DeletePerson removes a person with their events. Families they were a
// partner in keep the other partner and the children; a family left with no
// one in it is removed too.
func (s *Store) DeletePerson(ctx context.Context, a Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getPerson(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM persons WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM persons_fts WHERE rowid = ?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM families
			WHERE tree_id = ? AND partner1_id IS NULL AND partner2_id IS NULL
				AND NOT EXISTS (SELECT 1 FROM family_children c WHERE c.family_id = families.id)
				AND NOT EXISTS (SELECT 1 FROM events e WHERE e.family_id = families.id)`, a.TreeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "person", id, "delete", before, nil)
	})
}

// writeAlternateNames makes the person's names match names: names with an
// ID of this person are updated in place, so citations on them survive;
// the rest are inserted, and names no longer listed are deleted.
func writeAlternateNames(ctx context.Context, tx *sql.Tx, personID int64, names []AlternateNameInput) error {
	existing, err := queryIDs(ctx, tx, `SELECT id FROM person_names WHERE person_id = ?`, personID)
	if err != nil {
		return err
	}
	keep := map[int64]bool{}
	for _, id := range existing {
		keep[id] = false
	}
	for i, n := range names {
		if n.ID != nil {
			if _, ok := keep[*n.ID]; ok {
				keep[*n.ID] = true
				if _, err := tx.ExecContext(ctx, `
					UPDATE person_names SET type = ?, given_names = ?, surname = ?, name_prefix = ?, name_suffix = ?,
						nickname = ?, status = ?, status_reason = ?, sort_order = ?, notes = ?
					WHERE id = ?`,
					n.Type, n.GivenNames, n.Surname, n.NamePrefix, n.NameSuffix, n.Nickname, n.Status, n.StatusReason, i, n.Notes, *n.ID); err != nil {
					return err
				}
				continue
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO person_names (person_id, type, given_names, surname, name_prefix, name_suffix, nickname, status, status_reason, sort_order, notes)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			personID, n.Type, n.GivenNames, n.Surname, n.NamePrefix, n.NameSuffix, n.Nickname, n.Status, n.StatusReason, i, n.Notes); err != nil {
			return err
		}
	}
	for id, kept := range keep {
		if !kept {
			if _, err := tx.ExecContext(ctx, `DELETE FROM person_names WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// reindexPerson rewrites the search entry of a person from all their names.
func reindexPerson(ctx context.Context, tx *sql.Tx, id int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM persons_fts WHERE rowid = ?`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO persons_fts (rowid, names)
		SELECT p.id, `+foldSQL(`p.given_names || ' ' || p.surname || ' ' || p.nickname || ' ' ||
			coalesce((SELECT group_concat(n.given_names || ' ' || n.surname || ' ' || n.nickname, ' ')
				FROM person_names n WHERE n.person_id = p.id), '')`)+`
		FROM persons p WHERE p.id = ?`, id)
	return err
}

// GetPerson returns a person's own record.
func (s *Store) GetPerson(ctx context.Context, a Actor, id int64) (Person, error) {
	return s.getPerson(ctx, s.DB, a, id)
}

func (s *Store) getPerson(ctx context.Context, q queryer, a Actor, id int64) (Person, error) {
	var p Person
	var living sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT id, given_names, surname, name_prefix, name_suffix, nickname, sex, is_living, notes, name_notes, created_at, updated_at
		FROM persons WHERE id = ? AND tree_id = ?`, id, a.TreeID).Scan(
		&p.ID, &p.GivenNames, &p.Surname, &p.NamePrefix, &p.NameSuffix, &p.Nickname, &p.Sex, &living, &p.Notes, &p.NameNotes,
		&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Person{}, ErrNotFound
	}
	if err != nil {
		return Person{}, err
	}
	if living.Valid {
		b := living.Int64 == 1
		p.IsLiving = &b
	}

	rows, err := q.QueryContext(ctx, `
		SELECT id, type, given_names, surname, name_prefix, name_suffix, nickname, status, status_reason, sort_order, notes
		FROM person_names WHERE person_id = ? ORDER BY sort_order, id`, id)
	if err != nil {
		return Person{}, err
	}
	defer rows.Close()
	p.AlternateNames = []AlternateName{}
	for rows.Next() {
		var n AlternateName
		if err := rows.Scan(&n.ID, &n.Type, &n.GivenNames, &n.Surname, &n.NamePrefix, &n.NameSuffix, &n.Nickname,
			&n.Status, &n.StatusReason, &n.SortOrder, &n.Notes); err != nil {
			return Person{}, err
		}
		p.AlternateNames = append(p.AlternateNames, n)
	}
	if err := rows.Err(); err != nil {
		return Person{}, err
	}
	rows.Close()

	refs, err := s.personRefs(ctx, q, a, []int64{id})
	if err != nil {
		return Person{}, err
	}
	p.Living = refs[id].Living
	p.Portrait = refs[id].Portrait

	cits, err := citationRefs(ctx, q, a, "person", []int64{id})
	if err != nil {
		return Person{}, err
	}
	p.Citations = orEmpty(cits[id])
	nameIDs := make([]int64, len(p.AlternateNames))
	for i, n := range p.AlternateNames {
		nameIDs[i] = n.ID
	}
	nameCits, err := citationRefs(ctx, q, a, "name", nameIDs)
	if err != nil {
		return Person{}, err
	}
	for i := range p.AlternateNames {
		p.AlternateNames[i].Citations = orEmpty(nameCits[p.AlternateNames[i].ID])
	}
	return p, nil
}

// GetPersonDetail returns a person with events and families.
func (s *Store) GetPersonDetail(ctx context.Context, a Actor, id int64) (PersonDetail, error) {
	p, err := s.getPerson(ctx, s.DB, a, id)
	if err != nil {
		return PersonDetail{}, err
	}
	d := PersonDetail{Person: p}

	if d.Events, err = s.loadEvents(ctx, s.DB, a, `e.person_id = ?`, id); err != nil {
		return PersonDetail{}, err
	}
	shared, err := s.loadEvents(ctx, s.DB, a, `e.id IN (SELECT event_id FROM event_participants WHERE person_id = ?)`, id)
	if err != nil {
		return PersonDetail{}, err
	}
	for _, e := range shared {
		for _, part := range e.Participants {
			if part.Person.ID == id {
				e.Role = part.Role
				break
			}
		}
		d.Events = append(d.Events, e)
	}

	if d.ParentFamilies, err = s.loadFamilies(ctx, s.DB, a,
		`f.id IN (SELECT family_id FROM family_children WHERE child_id = ?)`, id); err != nil {
		return PersonDetail{}, err
	}
	if d.PartnerFamilies, err = s.loadFamilies(ctx, s.DB, a, `(f.partner1_id = ? OR f.partner2_id = ?)`, id, id); err != nil {
		return PersonDetail{}, err
	}
	return d, nil
}

// ListPersons lists the tree's people alphabetically, or by relevance when
// searching. The search matches the start of any name word, ignores case
// and diacritics ("muller" finds "Müller") and covers alternate names.
func (s *Store) ListPersons(ctx context.Context, a Actor, query string, limit, offset int) (PersonList, error) {
	return s.ListPersonsFiltered(ctx, a, query, PersonFilter{}, limit, offset)
}

// ListPersonsFiltered is ListPersons narrowed and ordered by f. The zero
// filter takes the plain list path, which the persons_tree_sort index serves.
func (s *Store) ListPersonsFiltered(ctx context.Context, a Actor, query string, f PersonFilter, limit, offset int) (PersonList, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	list := PersonList{Items: []PersonRef{}}

	match := ftsQuery(query)
	var ids []int64
	if f.zero(match) {
		var err error
		if match == "" {
			err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM persons WHERE tree_id = ?`, a.TreeID).Scan(&list.Total)
		} else {
			err = s.DB.QueryRowContext(ctx, `
				SELECT count(*) FROM persons_fts f JOIN persons p ON p.id = f.rowid
				WHERE persons_fts MATCH ? AND p.tree_id = ?`, match, a.TreeID).Scan(&list.Total)
		}
		if err != nil {
			return list, err
		}
		if ids, err = personIDs(ctx, s.DB, a, match, limit, offset); err != nil {
			return list, err
		}
	} else {
		from, where, args := s.filteredPersonsQuery(a, match, f)
		if err := s.DB.QueryRowContext(ctx, `SELECT count(*) `+from+where, args...).Scan(&list.Total); err != nil {
			return list, err
		}
		var err error
		ids, err = queryIDs(ctx, s.DB, `SELECT p.id `+from+where+` ORDER BY `+personOrderSQL(match, f.Sort)+` LIMIT ? OFFSET ?`,
			append(args, limit, offset)...)
		if err != nil {
			return list, err
		}
	}
	refs, err := s.personRefs(ctx, s.DB, a, ids)
	if err != nil {
		return list, err
	}
	for _, id := range ids {
		list.Items = append(list.Items, refs[id])
	}
	return list, nil
}

// listPersonsSQL orders the whole tree by the stored NAMES sort keys, which
// persons_tree_sort serves without sorting (#49).
const listPersonsSQL = `
	SELECT id FROM persons WHERE tree_id = ?
	ORDER BY surname = '', surname_key, given_key, id
	LIMIT ? OFFSET ?`

// searchPersonsSQL orders matches by relevance, then like listPersonsSQL;
// the id keeps pages stable for names that collate equal.
const searchPersonsSQL = `
	SELECT p.id FROM persons_fts f JOIN persons p ON p.id = f.rowid
	WHERE persons_fts MATCH ? AND p.tree_id = ?
	ORDER BY f.rank, p.surname_key, p.given_key, p.id
	LIMIT ? OFFSET ?`

// personIDs returns the ids of the tree's people in list order (see
// ListPersons); limit < 0 means all of them.
func personIDs(ctx context.Context, q queryer, a Actor, match string, limit, offset int) ([]int64, error) {
	if match == "" {
		return queryIDs(ctx, q, listPersonsSQL, a.TreeID, limit, offset)
	}
	return queryIDs(ctx, q, searchPersonsSQL, match, a.TreeID, limit, offset)
}

// PersonFilter narrows and orders the people list; the zero value is the
// plain list of ListPersons.
type PersonFilter struct {
	Sort   string // "", "name", "birth", "-birth", "updated"
	Sex    string // "", "M", "F", "X", "U"
	Living *bool
}

// zero reports whether the filter leaves the plain list: alphabetical, or
// by relevance when searching.
func (f PersonFilter) zero(match string) bool {
	return (f.Sort == "" || (f.Sort == "name" && match == "")) && f.Sex == "" && f.Living == nil
}

// birthSortSQL is the birth date_sort of person p, by the same rule as
// personRefs (keep in sync).
const birthSortSQL = `(SELECT e.date_sort FROM events e WHERE e.person_id = p.id AND e.type IN ('BIRT','CHR','BAPM')
	AND e.status <> 'disproven' AND e.date_sort IS NOT NULL ORDER BY e.type <> 'BIRT', e.date_sort LIMIT 1)`

// livingSQL mirrors the Go rule in personRefs; its one placeholder is the
// earliest birth year still presumed living (now minus livingCutoffYears).
// If date_sort changes, both must change together.
const livingSQL = `CASE WHEN p.is_living IS NOT NULL THEN p.is_living
	WHEN EXISTS (SELECT 1 FROM events e WHERE e.person_id = p.id AND e.type IN ('DEAT','BURI','CREM') AND e.status <> 'disproven') THEN 0
	WHEN ` + birthSortSQL + ` IS NOT NULL THEN (` + birthSortSQL + ` / 10000) > ?
	ELSE 1 END`

// filteredPersonsQuery builds the FROM and WHERE clauses of a filtered list.
func (s *Store) filteredPersonsQuery(a Actor, match string, f PersonFilter) (from, where string, args []any) {
	from = `FROM persons p `
	where = `WHERE p.tree_id = ?`
	if match != "" {
		from = `FROM persons_fts f JOIN persons p ON p.id = f.rowid `
		where = `WHERE persons_fts MATCH ? AND p.tree_id = ?`
		args = append(args, match)
	}
	args = append(args, a.TreeID)
	if f.Sex != "" {
		where += ` AND p.sex = ?`
		args = append(args, f.Sex)
	}
	if f.Living != nil {
		where += ` AND (` + livingSQL + `) = ?`
		living := 0
		if *f.Living {
			living = 1
		}
		args = append(args, s.Now().Year()-livingCutoffYears, living)
	}
	return from, where, args
}

// personOrderSQL is the ORDER BY of a filtered list. Every order ends in
// the stored name keys and the id, so pages are stable; unknown birth dates
// come last in both birth orders.
func personOrderSQL(match, sort string) string {
	const names = `p.surname_key, p.given_key, p.id`
	switch sort {
	case "birth":
		return birthSortSQL + ` IS NULL, ` + birthSortSQL + `, ` + names
	case "-birth":
		return birthSortSQL + ` IS NULL, ` + birthSortSQL + ` DESC, ` + names
	case "updated":
		return `p.updated_at DESC, p.id DESC`
	case "name":
		return `p.surname = '', ` + names
	}
	if match != "" {
		return `f.rank, ` + names
	}
	return `p.surname = '', ` + names
}

// searchFold is applied to indexed names and to the query, because the FTS
// tokenizer removes accents of Latin letters only: ё and й would otherwise
// not match е and и ("петр" finds "Пётр").
var searchFold = strings.NewReplacer("ё", "е", "Ё", "Е", "й", "и", "Й", "И")

// foldSQL is searchFold as an SQL expression over expr; migration 00010
// spells out the same replacements.
func foldSQL(expr string) string {
	return `replace(replace(replace(replace(` + expr + `, 'ё', 'е'), 'Ё', 'Е'), 'й', 'и'), 'Й', 'И')`
}

// searchWords splits user input into the folded words of a people search,
// at most eight; every word must match.
func searchWords(input string) []string {
	input = searchFold.Replace(input)
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
	})
	words := make([]string, 0, len(fields))
	for _, w := range fields {
		w = strings.ReplaceAll(w, "'", "")
		if w == "" {
			continue
		}
		words = append(words, w)
		if len(words) == 8 {
			break
		}
	}
	return words
}

// ftsQuery turns user input into an FTS5 prefix query; every word must match.
func ftsQuery(input string) string {
	words := searchWords(input)
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = `"` + w + `"*`
	}
	return strings.Join(parts, " ")
}

func queryIDs(ctx context.Context, q queryer, query string, args ...any) ([]int64, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// personRefs loads short person records with their birth and death dates.
// Disproven events are ignored.
func (s *Store) personRefs(ctx context.Context, q queryer, a Actor, ids []int64) (map[int64]PersonRef, error) {
	refs := map[int64]PersonRef{}
	if len(ids) == 0 {
		return refs, nil
	}
	args := append([]any{a.TreeID}, int64Args(ids)...)
	rows, err := q.QueryContext(ctx, `
		SELECT p.id, p.given_names, p.surname, p.sex, p.is_living,
			(SELECT coalesce(nullif(e.date, ''), e.date_raw) FROM events e
				WHERE e.person_id = p.id AND e.type IN ('BIRT', 'CHR', 'BAPM') AND e.status <> 'disproven'
				ORDER BY e.type <> 'BIRT', e.date_sort IS NULL, e.date_sort LIMIT 1),
			(SELECT e.date_sort FROM events e
				WHERE e.person_id = p.id AND e.type IN ('BIRT', 'CHR', 'BAPM') AND e.status <> 'disproven' AND e.date_sort IS NOT NULL
				ORDER BY e.type <> 'BIRT', e.date_sort LIMIT 1),
			(SELECT coalesce(nullif(e.date, ''), e.date_raw) FROM events e
				WHERE e.person_id = p.id AND e.type IN ('DEAT', 'BURI', 'CREM') AND e.status <> 'disproven'
				ORDER BY e.type <> 'DEAT', e.date_sort IS NULL, e.date_sort LIMIT 1),
			EXISTS (SELECT 1 FROM events e
				WHERE e.person_id = p.id AND e.type IN ('DEAT', 'BURI', 'CREM') AND e.status <> 'disproven'),
			p.portrait_media_id, p.portrait_region_id
		FROM persons p
		WHERE p.tree_id = ? AND p.id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	thisYear := s.Now().Year()
	for rows.Next() {
		var r PersonRef
		var stated, birthSort sql.NullInt64
		var birth, death sql.NullString
		var dead bool
		var portrait, portraitRegion sql.NullInt64
		if err := rows.Scan(&r.ID, &r.GivenNames, &r.Surname, &r.Sex, &stated, &birth, &birthSort, &death, &dead,
			&portrait, &portraitRegion); err != nil {
			return nil, err
		}
		if portrait.Valid {
			r.Portrait = &PortraitRef{MediaID: portrait.Int64, RegionID: ptrID(portraitRegion)}
		}
		r.BirthDate, r.DeathDate = birth.String, death.String
		r.BirthSort = birthSort.Int64
		switch {
		case stated.Valid:
			r.Living = stated.Int64 == 1
		case dead:
			r.Living = false
		case birthSort.Valid:
			r.Living = gendate.KeyYear(int(birthSort.Int64)) > thisYear-livingCutoffYears
		default:
			// Nothing known: treat as living, the safe choice for privacy.
			r.Living = true
		}
		refs[r.ID] = r
	}
	return refs, rows.Err()
}

func orEmpty(refs []CitationRef) []CitationRef {
	if refs == nil {
		return []CitationRef{}
	}
	return refs
}
