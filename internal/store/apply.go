package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ApplyResult tells what applying a transcription created.
type ApplyResult struct {
	EventID  int64 `json:"eventId"`
	FamilyID int64 `json:"familyId,omitempty"`
	// Created are the people that were new.
	Created []int64 `json:"created"`
	// Facts counts the other events created from the rows (occupations,
	// residences, estimated births).
	Facts int `json:"facts"`
	Transcription
}

// ApplyTranscription turns a draft into data in one transaction: the
// people of the rows (matched or new), the event with its principal and
// participants, family links, facts from the columns, and citations. The
// record as a whole is cited for the event; each row is cited, with its
// line, for the facts taken from it, field by field.
func (s *Store) ApplyTranscription(ctx context.Context, a Actor, id int64) (ApplyResult, error) {
	var res ApplyResult
	err := s.tx(ctx, func(tx *sql.Tx) error {
		tr, err := s.getTranscription(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if tr.Status != "draft" {
			return ErrApplied
		}
		in := TranscriptionInput{TemplateKey: tr.TemplateKey, Title: tr.Title, Page: tr.Page, Date: tr.Date, Notes: tr.Notes, Rows: tr.Rows}
		if tr.Source != nil {
			in.SourceID = &tr.Source.ID
		}
		if tr.Place != nil {
			in.PlaceID = &tr.Place.ID
		}
		t, err := s.validateTranscription(ctx, tx, a, in)
		if err != nil {
			return err
		}
		ap := applier{s: s, tx: tx, a: a, t: t, in: in, year: recordYear(in.Date), places: map[string]int64{}}
		if err := ap.run(ctx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE transcriptions SET status = 'applied', event_id = ?, applied_at = ?, rows_json = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`, ap.eventID, s.now(), ap.rowsJSON, s.now(), nullID(a.UserID), id, a.TreeID); err != nil {
			return err
		}
		res.EventID, res.FamilyID, res.Created, res.Facts = ap.eventID, ap.familyID, ap.created, ap.facts
		res.Transcription, err = s.getTranscription(ctx, tx, a, id)
		return err
	})
	if res.Created == nil {
		res.Created = []int64{}
	}
	return res, err
}

type applier struct {
	s    *Store
	tx   *sql.Tx
	a    Actor
	t    RecordTemplate
	in   TranscriptionInput
	year int

	persons  []int64 // per row, 0 when skipped
	created  []int64
	eventID  int64
	familyID int64
	facts    int
	places   map[string]int64
	rowsJSON string
}

func (ap *applier) roleOf(r TranscriptionRow) TemplateRole {
	for _, role := range ap.t.Roles {
		if role.Key == r.Role {
			return role
		}
	}
	return TemplateRole{Kind: RoleKindParticipant, Participant: "other"}
}

func (ap *applier) run(ctx context.Context) error {
	var v validator
	v.check(ap.in.SourceID != nil, "sourceId", "pick the source the record comes from")
	if ap.t.EventType == "EVEN" {
		v.check(ap.in.Title != "", "title", "name the kind of record, e.g. “School register”")
	}
	var principal, partners, parents []int
	for i, r := range ap.in.Rows {
		if r.Action == RowActionSkip {
			continue
		}
		v.check(r.Action == RowActionPerson || r.Values["given"] != "" || r.Values["surname"] != "", fmt.Sprintf("rows.%d", i), "a new person needs a name")
		switch ap.roleOf(r).Kind {
		case RoleKindPrincipal:
			principal = append(principal, i)
		case RoleKindPartner:
			partners = append(partners, i)
		case RoleKindParent:
			parents = append(parents, i)
		}
	}
	if familyEventTypes[ap.t.EventType] {
		v.check(len(partners) == 2, "rows", "the record needs both partners")
	} else {
		v.check(len(principal) == 1, "rows", "exactly one row needs the main role")
	}
	if err := v.err(); err != nil {
		return err
	}

	ap.persons = make([]int64, len(ap.in.Rows))
	for i, r := range ap.in.Rows {
		switch r.Action {
		case RowActionSkip:
			continue
		case RowActionPerson:
			ap.persons[i] = *r.PersonID
		default:
			p, err := ap.s.createPerson(ctx, ap.tx, ap.a, PersonInput{
				GivenNames: r.Values["given"], Surname: r.Values["surname"], Sex: orU(sexOf(r.Values["sex"])),
				Notes: r.Values["notes"],
			})
			if err != nil {
				return rowError(i, err)
			}
			ap.persons[i] = p.ID
			ap.created = append(ap.created, p.ID)
			// The row now points at the person it made, so the applied
			// transcription shows who is who.
			id := p.ID
			ap.in.Rows[i].Action, ap.in.Rows[i].PersonID = RowActionPerson, &id
		}
	}

	if err := ap.event(ctx, principal, partners, parents); err != nil {
		return err
	}
	if len(principal) == 1 && len(parents) > 0 {
		if err := ap.linkParents(ctx, ap.persons[principal[0]], parents); err != nil {
			return err
		}
	}
	for i := range ap.in.Rows {
		if ap.persons[i] != 0 {
			if err := ap.rowFacts(ctx, i); err != nil {
				return err
			}
		}
	}
	b, err := json.Marshal(ap.in.Rows)
	ap.rowsJSON = string(b)
	return err
}

func orU(sex string) string {
	if sex == "" {
		return "U"
	}
	return sex
}

func rowError(i int, err error) error {
	var ve *ValidationError
	if errors.As(err, &ve) {
		fields := map[string]string{}
		for k, msg := range ve.Fields {
			fields[fmt.Sprintf("rows.%d", i)] = k + ": " + msg
		}
		return &ValidationError{Fields: fields}
	}
	return err
}

func (ap *applier) event(ctx context.Context, principal, partners, parents []int) error {
	in := EventInput{Type: ap.t.EventType, Date: ap.in.Date, PlaceID: ap.in.PlaceID}
	if ap.t.EventType == "EVEN" {
		in.CustomLabel = ap.in.Title
	}
	main := map[int64]bool{}
	if familyEventTypes[ap.t.EventType] {
		p1, p2 := ap.persons[partners[0]], ap.persons[partners[1]]
		if p1 == p2 {
			return &ValidationError{Fields: map[string]string{"rows": "the partners must be two different people"}}
		}
		fam, err := ap.family(ctx, p1, p2, "married")
		if err != nil {
			return err
		}
		ap.familyID = fam
		in.FamilyID = &fam
		main[p1], main[p2] = true, true
	} else {
		p := ap.persons[principal[0]]
		in.PersonID = &p
		main[p] = true
	}
	seen := map[string]bool{}
	for i, r := range ap.in.Rows {
		pid := ap.persons[i]
		role := ap.roleOf(r)
		if pid == 0 || main[pid] || role.Participant == "" {
			continue
		}
		key := fmt.Sprintf("%d/%s", pid, role.Participant)
		if seen[key] {
			continue
		}
		seen[key] = true
		in.Participants = append(in.Participants, ParticipantInput{PersonID: pid, Role: role.Participant})
	}
	e, err := ap.s.createEvent(ctx, ap.tx, ap.a, in)
	if err != nil {
		return err
	}
	ap.eventID = e.ID
	// The record supports the whole event; field-level links are kept for
	// the facts a single row states.
	links := []LinkInput{{EntityType: "event", EntityID: e.ID}}
	if ap.familyID != 0 {
		links = append(links, LinkInput{EntityType: "family", EntityID: ap.familyID})
	}
	_, err = ap.s.createCitation(ctx, ap.tx, ap.a, CitationInput{
		SourceID: *ap.in.SourceID, Page: ap.in.Page, Text: ap.recordText(), Notes: ap.in.Notes, Links: links,
	})
	return err
}

// family finds the couple's family, in either order, or creates it.
func (ap *applier) family(ctx context.Context, p1, p2 int64, union string) (int64, error) {
	var id int64
	err := ap.tx.QueryRowContext(ctx, `
		SELECT id FROM families WHERE tree_id = ? AND ((partner1_id = ? AND partner2_id = ?) OR (partner1_id = ? AND partner2_id = ?))
		ORDER BY id LIMIT 1`, ap.a.TreeID, p1, p2, p2, p1).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	f, err := ap.s.createFamily(ctx, ap.tx, ap.a, FamilyInput{Partner1ID: &p1, Partner2ID: &p2, UnionType: union})
	return f.ID, err
}

// linkParents records the parents of the principal when they have none
// yet; existing parents are never changed from a record.
func (ap *applier) linkParents(ctx context.Context, child int64, parents []int) error {
	var n int
	if err := ap.tx.QueryRowContext(ctx, `SELECT count(*) FROM family_children WHERE child_id = ?`, child).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var ids []int64
	for _, i := range parents {
		if p := ap.persons[i]; p != 0 && p != child && !slices.Contains(ids, p) {
			ids = append(ids, p)
		}
	}
	if len(ids) == 0 || len(ids) > 2 {
		return nil
	}
	var fam int64
	var err error
	if len(ids) == 2 {
		fam, err = ap.family(ctx, ids[0], ids[1], "unknown")
	} else {
		var f Family
		f, err = ap.s.createFamily(ctx, ap.tx, ap.a, FamilyInput{Partner1ID: &ids[0], UnionType: "unknown"})
		fam = f.ID
	}
	if err != nil {
		return err
	}
	_, err = ap.s.setChild(ctx, ap.tx, ap.a, fam, child, ChildInput{RelationPartner1: "birth", RelationPartner2: "birth"})
	return err
}

// rowFacts creates the facts a row's columns state and cites the row for
// them.
func (ap *applier) rowFacts(ctx context.Context, i int) error {
	r := ap.in.Rows[i]
	pid := ap.persons[i]
	var links []LinkInput
	if slices.Contains(ap.created, pid) {
		links = append(links, LinkInput{EntityType: "person", EntityID: pid, Field: "name"})
		if sexOf(r.Values["sex"]) != "" {
			links = append(links, LinkInput{EntityType: "person", EntityID: pid, Field: "sex"})
		}
	}
	add := func(in EventInput, fields ...string) error {
		in.PersonID = &pid
		e, err := ap.s.createEvent(ctx, ap.tx, ap.a, in)
		if err != nil {
			return rowError(i, err)
		}
		ap.facts++
		for _, f := range fields {
			links = append(links, LinkInput{EntityType: "event", EntityID: e.ID, Field: f})
		}
		return nil
	}
	if occ := r.Values["occupation"]; occ != "" {
		if err := add(EventInput{Type: "OCCU", Description: occ, Date: ap.in.Date}, ""); err != nil {
			return err
		}
	}
	if res := r.Values["residence"]; res != "" {
		place, err := ap.place(ctx, res)
		if err != nil {
			return err
		}
		if err := add(EventInput{Type: "RESI", Date: ap.in.Date, PlaceID: &place}, "", "place"); err != nil {
			return err
		}
	}
	var hasBirth bool
	if err := ap.tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM events WHERE person_id = ? AND type = 'BIRT')`, pid).Scan(&hasBirth); err != nil {
		return err
	}
	if !hasBirth && ap.t.EventType != "BIRT" && ap.t.EventType != "BAPM" {
		birth := EventInput{Type: "BIRT"}
		var fields []string
		// Someone aged n on a day of year y was born in y-n-1 or y-n.
		if n, ok := age(r.Values["age"]); ok && ap.year != 0 {
			birth.Date = fmt.Sprintf("BET %d AND %d", ap.year-n-1, ap.year-n)
			fields = append(fields, "date")
		}
		if bp := r.Values["birthplace"]; bp != "" {
			place, err := ap.place(ctx, bp)
			if err != nil {
				return err
			}
			birth.PlaceID = &place
			fields = append(fields, "place")
		}
		if len(fields) > 0 {
			if err := add(birth, fields...); err != nil {
				return err
			}
		}
	}
	if len(links) == 0 {
		return nil
	}
	page := ap.in.Page
	switch {
	case r.Line != "" && page != "":
		page += ", line " + r.Line
	case r.Line != "":
		page = "line " + r.Line
	}
	_, err := ap.s.createCitation(ctx, ap.tx, ap.a, CitationInput{SourceID: *ap.in.SourceID, Page: page, Text: ap.rowText(r), Links: links})
	return err
}

// place finds or creates a place from "Leipzig, Sachsen, Deutschland",
// largest part last, reusing existing places level by level.
func (ap *applier) place(ctx context.Context, full string) (int64, error) {
	parts := strings.Split(full, ",")
	var parent sql.NullInt64
	key := ""
	for i := len(parts) - 1; i >= 0; i-- {
		name := strings.TrimSpace(parts[i])
		if name == "" {
			continue
		}
		key = name + "|" + key
		if id, ok := ap.places[key]; ok {
			parent = sql.NullInt64{Int64: id, Valid: true}
			continue
		}
		id, err := ap.findPlace(ctx, name, parent)
		if errors.Is(err, sql.ErrNoRows) {
			p, cerr := ap.s.createPlaceTx(ctx, ap.tx, ap.a, PlaceInput{Name: name, ParentID: ptrID(parent)})
			id, err = p.ID, cerr
		}
		if err != nil {
			return 0, err
		}
		ap.places[key] = id
		parent = sql.NullInt64{Int64: id, Valid: true}
	}
	if !parent.Valid {
		return 0, &ValidationError{Fields: map[string]string{"rows": "a place is empty"}}
	}
	return parent.Int64, nil
}

// findPlace returns the oldest place under parent whose name equals name
// ignoring case. SQLite's NOCASE and lower() fold ASCII only, so the
// comparison is done with Unicode folding here; accents still differ
// (Müller is not Muller). It returns sql.ErrNoRows when there is none.
func (ap *applier) findPlace(ctx context.Context, name string, parent sql.NullInt64) (int64, error) {
	rows, err := ap.tx.QueryContext(ctx, `
		SELECT id, name FROM places WHERE tree_id = ? AND parent_id IS ? ORDER BY id`,
		ap.a.TreeID, parent)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			return 0, err
		}
		if strings.EqualFold(n, name) {
			return id, nil
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return 0, sql.ErrNoRows
}

func (ap *applier) rowText(r TranscriptionRow) string {
	var parts []string
	if r.Role != "" {
		parts = append(parts, r.Role)
	}
	for _, c := range ap.t.Columns {
		if v := r.Values[c]; v != "" {
			parts = append(parts, c+": "+v)
		}
	}
	return strings.Join(parts, "; ")
}

func (ap *applier) recordText() string {
	var lines []string
	for _, r := range ap.in.Rows {
		line := ap.rowText(r)
		if r.Line != "" {
			line = r.Line + ". " + line
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
