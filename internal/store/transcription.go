package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/praetorianer777/gotree/internal/gendate"
	"github.com/praetorianer777/gotree/internal/match"
)

// Row actions: what applying does with the person of a row.
const (
	RowActionNew    = "new"
	RowActionPerson = "person"
	RowActionSkip   = "skip"
)

// TranscriptionRow is one person in a record.
type TranscriptionRow struct {
	// Line is where the person appears in the record, e.g. "12".
	Line   string            `json:"line"`
	Role   string            `json:"role"`
	Values map[string]string `json:"values"`
	// Action says whether to create a person, use PersonID, or skip.
	Action   string `json:"action"`
	PersonID *int64 `json:"personId"`
}

// Transcription is a record transcribed row by row.
type Transcription struct {
	ID          int64              `json:"id"`
	TemplateKey string             `json:"templateKey"`
	Title       string             `json:"title"`
	Source      *SourceRef         `json:"source"`
	Page        string             `json:"page"`
	Date        string             `json:"date"`
	Place       *PlaceRef          `json:"place"`
	Notes       string             `json:"notes"`
	Rows        []TranscriptionRow `json:"rows"`
	Status      string             `json:"status"`
	EventID     *int64             `json:"eventId"`
	AppliedAt   *string            `json:"appliedAt"`
	// Persons are the people rows refer to.
	Persons   map[int64]PersonRef `json:"persons"`
	CreatedAt string              `json:"createdAt"`
	UpdatedAt string              `json:"updatedAt"`
}

// SourceRef is the short form of a source.
type SourceRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// TranscriptionInput is the editable part of a transcription.
type TranscriptionInput struct {
	TemplateKey string             `json:"templateKey"`
	Title       string             `json:"title"`
	SourceID    *int64             `json:"sourceId"`
	Page        string             `json:"page"`
	Date        string             `json:"date"`
	PlaceID     *int64             `json:"placeId"`
	Notes       string             `json:"notes"`
	Rows        []TranscriptionRow `json:"rows"`
}

// maxRows bounds a record; a census page has about 50 lines.
const maxRows = 200

func (in *TranscriptionInput) normalize() {
	in.Title, in.Page, in.Notes = strings.TrimSpace(in.Title), strings.TrimSpace(in.Page), strings.TrimSpace(in.Notes)
	in.Date = storedDate(in.Date)
	for i := range in.Rows {
		r := &in.Rows[i]
		r.Line = strings.TrimSpace(r.Line)
		if r.Values == nil {
			r.Values = map[string]string{}
		}
		for k, v := range r.Values {
			if v = strings.TrimSpace(v); v == "" {
				delete(r.Values, k)
			} else {
				r.Values[k] = v
			}
		}
		if r.Action == "" {
			r.Action = RowActionNew
		}
		if r.Action != RowActionPerson {
			r.PersonID = nil
		}
	}
}

func (s *Store) validateTranscription(ctx context.Context, q queryer, a Actor, in TranscriptionInput) (RecordTemplate, error) {
	t, err := s.template(ctx, q, a, in.TemplateKey)
	if errors.Is(err, ErrNotFound) {
		return t, &ValidationError{Fields: map[string]string{"templateKey": "unknown template"}}
	}
	if err != nil {
		return t, err
	}
	var v validator
	v.check(utf8.RuneCountInString(in.Title) <= 200, "title", "is too long")
	v.check(utf8.RuneCountInString(in.Page) <= 200, "page", "is too long")
	v.check(utf8.RuneCountInString(in.Date) <= 120, "date", "is too long")
	v.check(utf8.RuneCountInString(in.Notes) <= 20000, "notes", "is too long")
	v.check(len(in.Rows) <= maxRows, "rows", fmt.Sprintf("a record can have at most %d rows", maxRows))
	for _, ref := range []struct {
		field, table string
		id           *int64
	}{{"sourceId", "sources", in.SourceID}, {"placeId", "places", in.PlaceID}} {
		if ref.id == nil {
			continue
		}
		if err := requireInTree(ctx, q, ref.table, *ref.id, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add(ref.field, "does not exist")
		} else if err != nil {
			return t, err
		}
	}
	roles := map[string]bool{}
	for _, r := range t.Roles {
		roles[r.Key] = true
	}
	cols := map[string]bool{}
	for _, c := range t.Columns {
		cols[c] = true
	}
	for i, r := range in.Rows {
		field := fmt.Sprintf("rows.%d", i)
		v.check(r.Role == "" || roles[r.Role], field, "unknown role "+r.Role)
		v.check(oneOf(r.Action, RowActionNew, RowActionPerson, RowActionSkip), field, "unknown action "+r.Action)
		v.check(utf8.RuneCountInString(r.Line) <= 20, field, "the line is too long")
		for k, val := range r.Values {
			v.check(cols[k], field, "unknown column "+k)
			v.check(utf8.RuneCountInString(val) <= 300, field, "a value is too long")
		}
		if r.Action == RowActionPerson {
			if r.PersonID == nil {
				v.add(field, "pick the person")
			} else if err := requireInTree(ctx, q, "persons", *r.PersonID, a.TreeID); errors.Is(err, ErrNotFound) {
				v.add(field, "the person does not exist")
			} else if err != nil {
				return t, err
			}
		}
	}
	return t, v.err()
}

// CreateTranscription starts a draft.
func (s *Store) CreateTranscription(ctx context.Context, a Actor, in TranscriptionInput) (Transcription, error) {
	in.normalize()
	var tr Transcription
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := s.validateTranscription(ctx, tx, a, in); err != nil {
			return err
		}
		rows, err := json.Marshal(orEmptyRows(in.Rows))
		if err != nil {
			return err
		}
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO transcriptions (tree_id, template_key, title, source_id, page, date_raw, place_id, notes, rows_json,
				created_at, updated_at, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, in.TemplateKey, in.Title, nullIDPtr(in.SourceID), in.Page, in.Date, nullIDPtr(in.PlaceID), in.Notes, string(rows),
			now, now, nullID(a.UserID), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		tr, err = s.getTranscription(ctx, tx, a, id)
		return err
	})
	return tr, err
}

// ErrApplied is returned for changes to a transcription that was applied.
var ErrApplied = errors.New("the transcription was applied and can no longer change")

// UpdateTranscription replaces a draft.
func (s *Store) UpdateTranscription(ctx context.Context, a Actor, id int64, in TranscriptionInput) (Transcription, error) {
	in.normalize()
	var tr Transcription
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getTranscription(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if before.Status != "draft" {
			return ErrApplied
		}
		if _, err := s.validateTranscription(ctx, tx, a, in); err != nil {
			return err
		}
		rows, err := json.Marshal(orEmptyRows(in.Rows))
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE transcriptions SET template_key = ?, title = ?, source_id = ?, page = ?, date_raw = ?, place_id = ?, notes = ?,
				rows_json = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			in.TemplateKey, in.Title, nullIDPtr(in.SourceID), in.Page, in.Date, nullIDPtr(in.PlaceID), in.Notes, string(rows),
			s.now(), nullID(a.UserID), id, a.TreeID); err != nil {
			return err
		}
		tr, err = s.getTranscription(ctx, tx, a, id)
		return err
	})
	return tr, err
}

// DeleteTranscription removes a transcription; what applying it created
// stays.
func (s *Store) DeleteTranscription(ctx context.Context, a Actor, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM transcriptions WHERE id = ? AND tree_id = ?`, id, a.TreeID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func orEmptyRows(r []TranscriptionRow) []TranscriptionRow {
	if r == nil {
		return []TranscriptionRow{}
	}
	return r
}

// GetTranscription returns one transcription.
func (s *Store) GetTranscription(ctx context.Context, a Actor, id int64) (Transcription, error) {
	return s.getTranscription(ctx, s.DB, a, id)
}

func (s *Store) getTranscription(ctx context.Context, q queryer, a Actor, id int64) (Transcription, error) {
	list, err := s.transcriptions(ctx, q, a, `t.id = ?`, id)
	if err != nil {
		return Transcription{}, err
	}
	if len(list) == 0 {
		return Transcription{}, ErrNotFound
	}
	return list[0], nil
}

// ListTranscriptions lists transcriptions, most recently changed first.
func (s *Store) ListTranscriptions(ctx context.Context, a Actor) ([]Transcription, error) {
	return s.transcriptions(ctx, s.DB, a, `1 = 1`)
}

func (s *Store) transcriptions(ctx context.Context, q queryer, a Actor, where string, args ...any) ([]Transcription, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT t.id, t.template_key, t.title, t.source_id, coalesce(s.title, ''), t.page, t.date_raw, t.place_id, t.notes,
			t.rows_json, t.status, t.event_id, t.applied_at, t.created_at, t.updated_at
		FROM transcriptions t LEFT JOIN sources s ON s.id = t.source_id
		WHERE t.tree_id = ? AND `+where+` ORDER BY t.updated_at DESC, t.id DESC LIMIT 500`, append([]any{a.TreeID}, args...)...)
	if err != nil {
		return nil, err
	}
	list := []Transcription{}
	var placeIDs, personIDs []int64
	for rows.Next() {
		var tr Transcription
		var source, place, event sql.NullInt64
		var sourceTitle, rowsJSON string
		var applied sql.NullString
		if err := rows.Scan(&tr.ID, &tr.TemplateKey, &tr.Title, &source, &sourceTitle, &tr.Page, &tr.Date, &place, &tr.Notes,
			&rowsJSON, &tr.Status, &event, &applied, &tr.CreatedAt, &tr.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if source.Valid {
			tr.Source = &SourceRef{ID: source.Int64, Title: sourceTitle}
		}
		if place.Valid {
			tr.Place = &PlaceRef{ID: place.Int64}
			placeIDs = append(placeIDs, place.Int64)
		}
		tr.EventID, tr.AppliedAt = ptrID(event), strPtr(applied)
		if err := json.Unmarshal([]byte(rowsJSON), &tr.Rows); err != nil {
			rows.Close()
			return nil, err
		}
		for _, r := range tr.Rows {
			if r.PersonID != nil {
				personIDs = append(personIDs, *r.PersonID)
			}
		}
		list = append(list, tr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	places, err := placeRefs(ctx, q, a, placeIDs)
	if err != nil {
		return nil, err
	}
	persons := map[int64]PersonRef{}
	for chunkStart := 0; chunkStart < len(personIDs); chunkStart += 500 {
		chunk := personIDs[chunkStart:min(chunkStart+500, len(personIDs))]
		refs, err := s.personRefs(ctx, q, a, chunk)
		if err != nil {
			return nil, err
		}
		for id, r := range refs {
			persons[id] = r
		}
	}
	for i := range list {
		if list[i].Place != nil {
			p := places[list[i].Place.ID]
			list[i].Place = &p
		}
		list[i].Persons = map[int64]PersonRef{}
		for _, r := range list[i].Rows {
			if r.PersonID != nil {
				if ref, ok := persons[*r.PersonID]; ok {
					list[i].Persons[ref.ID] = ref
				}
			}
		}
	}
	return list, nil
}

var leadingNumber = regexp.MustCompile(`^\s*(\d{1,3})`)

// age reads "34", "34 J." or "34 years"; ok is false otherwise.
func age(v string) (int, bool) {
	m := leadingNumber.FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil && n <= 120
}

func recordYear(date string) int {
	d, err := gendate.Normalize(date)
	if err != nil {
		return 0
	}
	k, ok := d.SortKey()
	if !ok || k <= 0 {
		return 0
	}
	return k / 10000
}

// sexOf reads M/F from the ways records write it.
func sexOf(v string) string {
	switch strings.ToLower(strings.TrimSuffix(strings.TrimSpace(v), ".")) {
	case "m", "male", "männlich", "mann", "man", "son", "sohn":
		return "M"
	case "f", "w", "female", "weiblich", "frau", "woman", "daughter", "tochter":
		return "F"
	}
	return ""
}

// MatchCandidate is a person who may be the one in a row.
type MatchCandidate struct {
	Person PersonRef `json:"person"`
	Score  float64   `json:"score"`
}

// MatchRequest asks for candidates for the rows of a record.
type MatchRequest struct {
	Date string             `json:"date"`
	Rows []TranscriptionRow `json:"rows"`
}

// maxCandidates per row, and the least score worth showing.
const (
	maxCandidates = 5
	minScore      = 0.45
)

// Match finds, for each row, the people of the tree who may be meant,
// best first. The whole tree is scored, so spelling variants that a
// prefix search would miss are found too.
func (s *Store) Match(ctx context.Context, a Actor, req MatchRequest) ([][]MatchCandidate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT p.id, p.given_names, p.surname, p.sex,
			(SELECT e.date_sort FROM events e WHERE e.person_id = p.id AND e.type IN ('BIRT', 'CHR', 'BAPM')
				AND e.status <> 'disproven' AND e.date_sort IS NOT NULL ORDER BY e.type <> 'BIRT', e.date_sort LIMIT 1),
			(SELECT e.date_sort FROM events e WHERE e.person_id = p.id AND e.type IN ('DEAT', 'BURI', 'CREM')
				AND e.status <> 'disproven' AND e.date_sort IS NOT NULL ORDER BY e.type <> 'DEAT', e.date_sort LIMIT 1)
		FROM persons p WHERE p.tree_id = ?`, a.TreeID)
	if err != nil {
		return nil, err
	}
	type person struct {
		id int64
		c  match.Candidate
	}
	var people []person
	for rows.Next() {
		var p person
		var birth, death sql.NullInt64
		if err := rows.Scan(&p.id, &p.c.Given, &p.c.Surname, &p.c.Sex, &birth, &death); err != nil {
			rows.Close()
			return nil, err
		}
		if birth.Valid && birth.Int64 > 0 {
			p.c.BirthYear = int(birth.Int64 / 10000)
		}
		if death.Valid && death.Int64 > 0 {
			p.c.DeathYear = int(death.Int64 / 10000)
		}
		people = append(people, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	year := recordYear(req.Date)
	out := make([][]MatchCandidate, len(req.Rows))
	var ids []int64
	type scored struct {
		id    int64
		score float64
	}
	found := make([][]scored, len(req.Rows))
	for i, r := range req.Rows {
		q := match.Query{Given: r.Values["given"], Surname: r.Values["surname"], Sex: sexOf(r.Values["sex"]), RecordYear: year}
		if q.Given == "" && q.Surname == "" {
			continue
		}
		if n, ok := age(r.Values["age"]); ok && year != 0 {
			q.BirthYear = year - n
		}
		var best []scored
		for _, p := range people {
			if sc := match.Score(q, p.c); sc >= minScore {
				best = append(best, scored{p.id, sc})
			}
		}
		sort.Slice(best, func(x, y int) bool { return best[x].score > best[y].score })
		if len(best) > maxCandidates {
			best = best[:maxCandidates]
		}
		found[i] = best
		for _, b := range best {
			ids = append(ids, b.id)
		}
	}
	refs, err := s.personRefsChunked(ctx, a, ids)
	if err != nil {
		return nil, err
	}
	for i, best := range found {
		out[i] = []MatchCandidate{}
		for _, b := range best {
			out[i] = append(out[i], MatchCandidate{Person: refs[b.id], Score: float64(int(b.score*100)) / 100})
		}
	}
	return out, nil
}
