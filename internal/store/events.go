package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/praetorianer777/gotree/internal/gendate"
)

// DateValue is a genealogical date as stored: the text as entered plus, when
// it parses, its normalized GEDCOM form and sort key.
type DateValue struct {
	Raw        string `json:"raw"`
	Normalized string `json:"normalized"`
	Qualifier  string `json:"qualifier"`
	// Valid is false for text that is not a GEDCOM date; it is kept as
	// entered and listed by the data-quality checks.
	Valid   bool `json:"valid"`
	SortKey *int `json:"sortKey"`
}

// Participant is a person who takes part in an event besides its principal.
type Participant struct {
	Person     PersonRef `json:"person"`
	Role       string    `json:"role"`
	CustomRole string    `json:"customRole"`
	Notes      string    `json:"notes"`
}

// Event is something that happened to a person or a family.
type Event struct {
	ID           int64         `json:"id"`
	PersonID     *int64        `json:"personId"`
	FamilyID     *int64        `json:"familyId"`
	Type         string        `json:"type"`
	CustomLabel  string        `json:"customLabel"`
	Date         DateValue     `json:"date"`
	Place        *PlaceRef     `json:"place"`
	Description  string        `json:"description"`
	Notes        string        `json:"notes"`
	Status       string        `json:"status"`
	StatusReason string        `json:"statusReason"`
	SortOrder    int           `json:"sortOrder"`
	Participants []Participant `json:"participants"`
	Citations    []CitationRef `json:"citations"`
	// Role is set when the event is listed for one of its participants
	// rather than for its principal.
	Role      string `json:"role,omitempty"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ParticipantInput adds a person to an event.
type ParticipantInput struct {
	PersonID   int64  `json:"personId"`
	Role       string `json:"role"`
	CustomRole string `json:"customRole"`
	Notes      string `json:"notes"`
}

// EventInput is the editable part of an Event. Exactly one of PersonID and
// FamilyID must be set.
type EventInput struct {
	PersonID     *int64             `json:"personId"`
	FamilyID     *int64             `json:"familyId"`
	Type         string             `json:"type"`
	CustomLabel  string             `json:"customLabel"`
	Date         string             `json:"date"`
	PlaceID      *int64             `json:"placeId"`
	Description  string             `json:"description"`
	Notes        string             `json:"notes"`
	Status       string             `json:"status"`
	StatusReason string             `json:"statusReason"`
	SortOrder    int                `json:"sortOrder"`
	Participants []ParticipantInput `json:"participants"`
	// AddCitations cites sources for the event.
	AddCitations []NewCitation `json:"addCitations,omitempty"`
	// RemoveCitations detaches citations from the event.
	RemoveCitations []int64 `json:"removeCitations,omitempty"`
}

// Participant roles. RoleOther takes a CustomRole.
var participantRoles = []string{
	"head", "spouse", "child", "parent", "sibling", "relative", "witness", "godparent",
	"informant", "officiant", "clergy", "friend", "neighbor", "other",
}

var eventTypePattern = regexp.MustCompile(`^_?[A-Z][A-Z0-9_]{1,31}$`)

// familyEventTypes can only be attached to a family.
var familyEventTypes = map[string]bool{
	"MARR": true, "DIV": true, "DIVF": true, "ENGA": true, "MARB": true,
	"MARC": true, "MARL": true, "MARS": true, "ANUL": true,
}

func parseDate(raw string) (date string, sortKey, sortEnd sql.NullInt64, qualifier string) {
	d, err := gendate.Parse(raw)
	if err != nil {
		return "", sql.NullInt64{}, sql.NullInt64{}, ""
	}
	if k, ok := d.SortKey(); ok {
		sortKey = sql.NullInt64{Int64: int64(k), Valid: true}
	}
	if k, ok := d.SortKeyEnd(); ok {
		sortEnd = sql.NullInt64{Int64: int64(k), Valid: true}
	}
	return d.String(), sortKey, sortEnd, string(d.Qualifier)
}

func (in *EventInput) normalize() {
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.CustomLabel = strings.TrimSpace(in.CustomLabel)
	in.Date = strings.Join(strings.Fields(in.Date), " ")
	// Plain-language dates ("about 1850", "between 1850 and 1860", "12 March
	// 1850") are stored as GEDCOM, so sorting, checks and export see them.
	if _, err := gendate.Parse(in.Date); err != nil {
		if d, err := gendate.Normalize(in.Date); err == nil {
			in.Date = d.String()
		}
	}
	in.Description = strings.TrimSpace(in.Description)
	in.Notes = strings.TrimSpace(in.Notes)
	in.Status = defaultStatus(in.Status)
	in.StatusReason = strings.TrimSpace(in.StatusReason)
	for i := range in.Participants {
		p := &in.Participants[i]
		p.Role = strings.ToLower(strings.TrimSpace(p.Role))
		p.CustomRole = strings.TrimSpace(p.CustomRole)
		p.Notes = strings.TrimSpace(p.Notes)
	}
}

func (s *Store) validateEvent(ctx context.Context, q queryer, a Actor, in EventInput) error {
	var v validator
	v.check((in.PersonID == nil) != (in.FamilyID == nil), "personId", "an event belongs to exactly one person or one family")
	v.check(eventTypePattern.MatchString(in.Type), "type", "must be a GEDCOM tag such as BIRT, or a custom tag starting with _")
	if in.Type == "EVEN" || in.Type == "FACT" {
		v.check(in.CustomLabel != "", "customLabel", "is required for custom events")
	}
	if familyEventTypes[in.Type] {
		v.check(in.FamilyID != nil, "type", "this event belongs to a family")
	}
	v.check(utf8.RuneCountInString(in.Date) <= 120, "date", "is too long")
	v.check(utf8.RuneCountInString(in.CustomLabel) <= 120, "customLabel", "is too long")
	v.check(utf8.RuneCountInString(in.Description) <= 1000, "description", "is too long")
	v.check(validStatus(in.Status), "status", "must be accepted, disputed or disproven")
	v.check(in.Status == StatusAccepted || in.StatusReason != "", "statusReason", "say why the fact is disputed or disproven")

	refs := []struct {
		field, table string
		id           *int64
	}{
		{"personId", "persons", in.PersonID},
		{"familyId", "families", in.FamilyID},
		{"placeId", "places", in.PlaceID},
	}
	for _, r := range refs {
		if r.id == nil {
			continue
		}
		if err := requireInTree(ctx, q, r.table, *r.id, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add(r.field, "does not exist")
		} else if err != nil {
			return err
		}
	}

	type participantKey struct {
		personID int64
		role     string
	}
	seen := map[participantKey]bool{}
	for _, p := range in.Participants {
		field := "participants"
		v.check(oneOf(p.Role, participantRoles...), field, "unknown role "+p.Role)
		v.check(p.Role != "other" || p.CustomRole != "", field, "a custom role needs a name")
		if in.PersonID != nil {
			v.check(p.PersonID != *in.PersonID, field, "the principal cannot also be a participant")
		}
		key := participantKey{p.PersonID, p.Role}
		v.check(!seen[key], field, "the same person is listed twice with the same role")
		seen[key] = true
		if err := requireInTree(ctx, q, "persons", p.PersonID, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add(field, "a participant does not exist")
		} else if err != nil {
			return err
		}
	}
	return v.err()
}

// CreateEvent adds an event.
func (s *Store) CreateEvent(ctx context.Context, a Actor, in EventInput) (Event, error) {
	var e Event
	err := s.tx(ctx, func(tx *sql.Tx) (err error) {
		e, err = s.createEvent(ctx, tx, a, in)
		return err
	})
	return e, err
}

func (s *Store) createEvent(ctx context.Context, tx *sql.Tx, a Actor, in EventInput) (Event, error) {
	in.normalize()
	var e Event
	err := func() error {
		if err := s.validateEvent(ctx, tx, a, in); err != nil {
			return err
		}
		date, key, keyEnd, qual := parseDate(in.Date)
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO events (tree_id, person_id, family_id, type, custom_label, date_raw, date, date_sort, date_sort_end,
				date_qualifier, place_id, description, notes, status, status_reason, sort_order,
				created_at, updated_at, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, nullIDPtr(in.PersonID), nullIDPtr(in.FamilyID), in.Type, in.CustomLabel, in.Date, date, key, keyEnd,
			qual, nullIDPtr(in.PlaceID), in.Description, in.Notes, in.Status, in.StatusReason, in.SortOrder,
			now, now, nullID(a.UserID), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if err := writeParticipants(ctx, tx, id, in.Participants); err != nil {
			return err
		}
		if err := s.addCitations(ctx, tx, a, "event", id, in.AddCitations); err != nil {
			return err
		}
		if e, err = s.getEvent(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "event", id, "create", nil, e)
	}()
	return e, err
}

// UpdateEvent replaces an event, including its participants.
func (s *Store) UpdateEvent(ctx context.Context, a Actor, id int64, in EventInput) (Event, error) {
	in.normalize()
	var e Event
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getEvent(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if err := s.validateEvent(ctx, tx, a, in); err != nil {
			return err
		}
		date, key, keyEnd, qual := parseDate(in.Date)
		_, err = tx.ExecContext(ctx, `
			UPDATE events SET person_id = ?, family_id = ?, type = ?, custom_label = ?, date_raw = ?, date = ?,
				date_sort = ?, date_sort_end = ?, date_qualifier = ?, place_id = ?, description = ?, notes = ?,
				status = ?, status_reason = ?, sort_order = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			nullIDPtr(in.PersonID), nullIDPtr(in.FamilyID), in.Type, in.CustomLabel, in.Date, date,
			key, keyEnd, qual, nullIDPtr(in.PlaceID), in.Description, in.Notes,
			in.Status, in.StatusReason, in.SortOrder, s.now(), nullID(a.UserID), id, a.TreeID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM event_participants WHERE event_id = ?`, id); err != nil {
			return err
		}
		if err := writeParticipants(ctx, tx, id, in.Participants); err != nil {
			return err
		}
		if err := s.addCitations(ctx, tx, a, "event", id, in.AddCitations); err != nil {
			return err
		}
		if err := s.removeCitations(ctx, tx, a, "event", id, in.RemoveCitations); err != nil {
			return err
		}
		if e, err = s.getEvent(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "event", id, "update", before, e)
	})
	return e, err
}

// DeleteEvent removes an event.
func (s *Store) DeleteEvent(ctx context.Context, a Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getEvent(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "event", id, "delete", before, nil)
	})
}

// GetEvent returns one event.
func (s *Store) GetEvent(ctx context.Context, a Actor, id int64) (Event, error) {
	return s.getEvent(ctx, s.DB, a, id)
}

func writeParticipants(ctx context.Context, tx *sql.Tx, eventID int64, ps []ParticipantInput) error {
	for _, p := range ps {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO event_participants (event_id, person_id, role, custom_role, notes) VALUES (?, ?, ?, ?, ?)`,
			eventID, p.PersonID, p.Role, p.CustomRole, p.Notes); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) getEvent(ctx context.Context, q queryer, a Actor, id int64) (Event, error) {
	events, err := s.loadEvents(ctx, q, a, `e.id = ?`, id)
	if err != nil {
		return Event{}, err
	}
	if len(events) == 0 {
		return Event{}, ErrNotFound
	}
	return events[0], nil
}

const eventOrder = ` ORDER BY e.sort_order, e.date_sort IS NULL, e.date_sort, e.id`

// loadEvents returns the events of the actor's tree matching where, with
// places and participants resolved.
func (s *Store) loadEvents(ctx context.Context, q queryer, a Actor, where string, args ...any) ([]Event, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT e.id, e.person_id, e.family_id, e.type, e.custom_label, e.date_raw, e.date, e.date_sort,
			e.date_qualifier, e.place_id, e.description, e.notes, e.status, e.status_reason, e.sort_order,
			e.created_at, e.updated_at
		FROM events e
		WHERE e.tree_id = ? AND `+where+eventOrder, append([]any{a.TreeID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	var placeIDs []int64
	placeOf := map[int]int64{}
	for rows.Next() {
		var e Event
		var person, family, place, sortKey sql.NullInt64
		if err := rows.Scan(&e.ID, &person, &family, &e.Type, &e.CustomLabel, &e.Date.Raw, &e.Date.Normalized,
			&sortKey, &e.Date.Qualifier, &place, &e.Description, &e.Notes, &e.Status, &e.StatusReason,
			&e.SortOrder, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.PersonID, e.FamilyID = ptrID(person), ptrID(family)
		e.Date.Valid = e.Date.Normalized != ""
		if sortKey.Valid {
			k := int(sortKey.Int64)
			e.Date.SortKey = &k
		}
		if place.Valid {
			placeOf[len(events)] = place.Int64
			placeIDs = append(placeIDs, place.Int64)
		}
		e.Participants = []Participant{}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	refs, err := placeRefs(ctx, q, a, placeIDs)
	if err != nil {
		return nil, err
	}
	for i, pid := range placeOf {
		if r, ok := refs[pid]; ok {
			events[i].Place = &r
		}
	}
	if err := s.attachParticipants(ctx, q, a, events); err != nil {
		return nil, err
	}
	ids := make([]int64, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}
	cits, err := citationRefs(ctx, q, a, "event", ids)
	if err != nil {
		return nil, err
	}
	for i := range events {
		events[i].Citations = orEmpty(cits[events[i].ID])
	}
	return events, nil
}

func (s *Store) attachParticipants(ctx context.Context, q queryer, a Actor, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	index := map[int64]int{}
	ids := make([]int64, len(events))
	for i, e := range events {
		index[e.ID] = i
		ids[i] = e.ID
	}
	rows, err := q.QueryContext(ctx, `
		SELECT event_id, person_id, role, custom_role, notes FROM event_participants
		WHERE event_id IN (`+placeholders(len(ids))+`) ORDER BY rowid`, int64Args(ids)...)
	if err != nil {
		return err
	}
	type row struct {
		eventID, personID int64
		p                 Participant
	}
	var found []row
	var personIDs []int64
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.eventID, &r.personID, &r.p.Role, &r.p.CustomRole, &r.p.Notes); err != nil {
			rows.Close()
			return err
		}
		found = append(found, r)
		personIDs = append(personIDs, r.personID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	refs, err := s.personRefs(ctx, q, a, personIDs)
	if err != nil {
		return err
	}
	for _, r := range found {
		r.p.Person = refs[r.personID]
		e := &events[index[r.eventID]]
		e.Participants = append(e.Participants, r.p)
	}
	return nil
}
