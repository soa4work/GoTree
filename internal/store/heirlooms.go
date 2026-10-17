package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/praetorianer777/gotree/internal/gendate"
)

var heirloomKinds = []string{"jewellery", "furniture", "document", "photo_album", "tool", "textile", "other"}

var custodyWays = []string{"inherited", "gift", "purchased", "made", "found", "other"}

// Custody is one holder of an heirloom.
type Custody struct {
	ID int64 `json:"id"`
	// Person is nil when the holder is not in the tree; Notes then says who.
	Person   *PersonRef `json:"person"`
	FromDate string     `json:"fromDate"`
	ToDate   string     `json:"toDate"`
	How      string     `json:"how"`
	Notes    string     `json:"notes"`
}

// CustodyInput is the editable part of a Custody.
type CustodyInput struct {
	PersonID *int64 `json:"personId"`
	FromDate string `json:"fromDate"`
	ToDate   string `json:"toDate"`
	How      string `json:"how"`
	Notes    string `json:"notes"`
}

// Heirloom is an object handed down in the family.
type Heirloom struct {
	ID              int64         `json:"id"`
	Name            string        `json:"name"`
	Kind            string        `json:"kind"`
	Description     string        `json:"description"`
	MadeDate        string        `json:"madeDate"`
	OriginPlace     *PlaceRef     `json:"originPlace"`
	CurrentLocation string        `json:"currentLocation"`
	Notes           string        `json:"notes"`
	Custody         []Custody     `json:"custody"`
	Citations       []CitationRef `json:"citations"`
	CreatedAt       string        `json:"createdAt"`
	UpdatedAt       string        `json:"updatedAt"`
}

// HeirloomRef is an heirloom in a list.
type HeirloomRef struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	MadeDate string `json:"madeDate"`
	// Holder is the last holder in the custody list.
	Holder *PersonRef `json:"holder"`
	// PhotoID is the first photo linked to the heirloom.
	PhotoID *int64 `json:"photoId"`
}

// HeirloomInput is the editable part of an Heirloom. Custody replaces the
// list, in order.
type HeirloomInput struct {
	Name            string         `json:"name"`
	Kind            string         `json:"kind"`
	Description     string         `json:"description"`
	MadeDate        string         `json:"madeDate"`
	OriginPlaceID   *int64         `json:"originPlaceId"`
	CurrentLocation string         `json:"currentLocation"`
	Notes           string         `json:"notes"`
	Custody         []CustodyInput `json:"custody"`
	AddCitations    []NewCitation  `json:"addCitations,omitempty"`
	RemoveCitations []int64        `json:"removeCitations,omitempty"`
}

func (in *HeirloomInput) normalize() {
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	in.CurrentLocation, in.Notes = strings.TrimSpace(in.CurrentLocation), strings.TrimSpace(in.Notes)
	in.MadeDate = storedDate(in.MadeDate)
	if in.Kind == "" {
		in.Kind = "other"
	}
	for i := range in.Custody {
		c := &in.Custody[i]
		c.FromDate, c.ToDate, c.Notes = storedDate(c.FromDate), storedDate(c.ToDate), strings.TrimSpace(c.Notes)
		if c.How == "" {
			c.How = "inherited"
		}
	}
}

func (s *Store) validateHeirloom(ctx context.Context, q queryer, a Actor, in HeirloomInput) error {
	var v validator
	v.check(in.Name != "" && utf8.RuneCountInString(in.Name) <= 200, "name", "give the object a name of at most 200 characters")
	v.check(oneOf(in.Kind, heirloomKinds...), "kind", "unknown kind")
	v.check(utf8.RuneCountInString(in.Description) <= 10000, "description", "is too long")
	v.check(utf8.RuneCountInString(in.MadeDate) <= 120, "madeDate", "is too long")
	v.check(utf8.RuneCountInString(in.CurrentLocation) <= 300, "currentLocation", "is too long")
	v.check(utf8.RuneCountInString(in.Notes) <= 10000, "notes", "is too long")
	v.check(len(in.Custody) <= 100, "custody", "too many holders")
	if in.OriginPlaceID != nil {
		if err := requireInTree(ctx, q, "places", *in.OriginPlaceID, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add("originPlaceId", "does not exist")
		} else if err != nil {
			return err
		}
	}
	for _, c := range in.Custody {
		v.check(oneOf(c.How, custodyWays...), "custody", "unknown way of getting it: "+c.How)
		v.check(c.PersonID != nil || c.Notes != "", "custody", "name the holder: pick a person or say who in the notes")
		v.check(utf8.RuneCountInString(c.FromDate) <= 120 && utf8.RuneCountInString(c.ToDate) <= 120, "custody", "a date is too long")
		if c.PersonID != nil {
			if err := requireInTree(ctx, q, "persons", *c.PersonID, a.TreeID); errors.Is(err, ErrNotFound) {
				v.add("custody", "a holder does not exist")
			} else if err != nil {
				return err
			}
		}
	}
	return v.err()
}

func madeSort(date string) sql.NullInt64 {
	if d, err := gendate.Parse(date); err == nil {
		if k, ok := d.SortKey(); ok {
			return sql.NullInt64{Int64: int64(k), Valid: true}
		}
	}
	return sql.NullInt64{}
}

func writeCustody(ctx context.Context, tx *sql.Tx, id int64, custody []CustodyInput) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM heirloom_custody WHERE heirloom_id = ?`, id); err != nil {
		return err
	}
	for i, c := range custody {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO heirloom_custody (heirloom_id, person_id, from_date_raw, to_date_raw, how, notes, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, id, nullIDPtr(c.PersonID), c.FromDate, c.ToDate, c.How, c.Notes, i); err != nil {
			return err
		}
	}
	return nil
}

// CreateHeirloom adds an heirloom.
func (s *Store) CreateHeirloom(ctx context.Context, a Actor, in HeirloomInput) (Heirloom, error) {
	in.normalize()
	var h Heirloom
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.validateHeirloom(ctx, tx, a, in); err != nil {
			return err
		}
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO heirlooms (tree_id, name, kind, description, made_date_raw, made_date_sort, origin_place_id, current_location, notes,
				created_at, updated_at, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, in.Name, in.Kind, in.Description, in.MadeDate, madeSort(in.MadeDate), nullIDPtr(in.OriginPlaceID),
			in.CurrentLocation, in.Notes, now, now, nullID(a.UserID), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if err := writeCustody(ctx, tx, id, in.Custody); err != nil {
			return err
		}
		if err := s.addCitations(ctx, tx, a, "heirloom", id, in.AddCitations); err != nil {
			return err
		}
		if h, err = s.getHeirloom(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "heirloom", id, "create", nil, h)
	})
	return h, err
}

// UpdateHeirloom replaces an heirloom and its custody list.
func (s *Store) UpdateHeirloom(ctx context.Context, a Actor, id int64, in HeirloomInput) (Heirloom, error) {
	in.normalize()
	var h Heirloom
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getHeirloom(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if err := s.validateHeirloom(ctx, tx, a, in); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE heirlooms SET name = ?, kind = ?, description = ?, made_date_raw = ?, made_date_sort = ?, origin_place_id = ?,
				current_location = ?, notes = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			in.Name, in.Kind, in.Description, in.MadeDate, madeSort(in.MadeDate), nullIDPtr(in.OriginPlaceID),
			in.CurrentLocation, in.Notes, s.now(), nullID(a.UserID), id, a.TreeID); err != nil {
			return err
		}
		if err := writeCustody(ctx, tx, id, in.Custody); err != nil {
			return err
		}
		if err := s.addCitations(ctx, tx, a, "heirloom", id, in.AddCitations); err != nil {
			return err
		}
		if err := s.removeCitations(ctx, tx, a, "heirloom", id, in.RemoveCitations); err != nil {
			return err
		}
		if h, err = s.getHeirloom(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "heirloom", id, "update", before, h)
	})
	return h, err
}

// DeleteHeirloom removes an heirloom; its photos stay in the media list.
func (s *Store) DeleteHeirloom(ctx context.Context, a Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getHeirloom(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM heirlooms WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "heirloom", id, "delete", before, nil)
	})
}

// GetHeirloom returns one heirloom with its custody and citations.
func (s *Store) GetHeirloom(ctx context.Context, a Actor, id int64) (Heirloom, error) {
	return s.getHeirloom(ctx, s.DB, a, id)
}

func (s *Store) getHeirloom(ctx context.Context, q queryer, a Actor, id int64) (Heirloom, error) {
	var h Heirloom
	var place sql.NullInt64
	err := q.QueryRowContext(ctx, `
		SELECT id, name, kind, description, made_date_raw, origin_place_id, current_location, notes, created_at, updated_at
		FROM heirlooms WHERE id = ? AND tree_id = ?`, id, a.TreeID).Scan(
		&h.ID, &h.Name, &h.Kind, &h.Description, &h.MadeDate, &place, &h.CurrentLocation, &h.Notes, &h.CreatedAt, &h.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Heirloom{}, ErrNotFound
	}
	if err != nil {
		return Heirloom{}, err
	}
	if place.Valid {
		refs, err := placeRefs(ctx, q, a, []int64{place.Int64})
		if err != nil {
			return Heirloom{}, err
		}
		if r, ok := refs[place.Int64]; ok {
			h.OriginPlace = &r
		}
	}
	rows, err := q.QueryContext(ctx, `
		SELECT id, person_id, from_date_raw, to_date_raw, how, notes FROM heirloom_custody WHERE heirloom_id = ? ORDER BY sort_order, id`, id)
	if err != nil {
		return Heirloom{}, err
	}
	h.Custody = []Custody{}
	var personIDs []int64
	var persons []sql.NullInt64
	for rows.Next() {
		var c Custody
		var pid sql.NullInt64
		if err := rows.Scan(&c.ID, &pid, &c.FromDate, &c.ToDate, &c.How, &c.Notes); err != nil {
			rows.Close()
			return Heirloom{}, err
		}
		h.Custody = append(h.Custody, c)
		persons = append(persons, pid)
		if pid.Valid {
			personIDs = append(personIDs, pid.Int64)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Heirloom{}, err
	}
	refs, err := s.personRefs(ctx, q, a, personIDs)
	if err != nil {
		return Heirloom{}, err
	}
	for i, pid := range persons {
		if pid.Valid {
			if r, ok := refs[pid.Int64]; ok {
				h.Custody[i].Person = &r
			}
		}
	}
	cits, err := citationRefs(ctx, q, a, "heirloom", []int64{id})
	if err != nil {
		return Heirloom{}, err
	}
	h.Citations = orEmpty(cits[id])
	return h, nil
}

// ListHeirlooms lists heirlooms by name, optionally those held by one
// person or matching a search.
func (s *Store) ListHeirlooms(ctx context.Context, a Actor, query string, personID int64) ([]HeirloomRef, error) {
	where, args := []string{"h.tree_id = ?"}, []any{a.TreeID}
	if q := strings.TrimSpace(query); q != "" {
		where = append(where, `(h.name LIKE ? ESCAPE '\' OR h.description LIKE ? ESCAPE '\')`)
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		args = append(args, like, like)
	}
	if personID != 0 {
		where = append(where, `EXISTS (SELECT 1 FROM heirloom_custody c WHERE c.heirloom_id = h.id AND c.person_id = ?)`)
		args = append(args, personID)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT h.id, h.name, h.kind, h.made_date_raw,
			(SELECT c.person_id FROM heirloom_custody c WHERE c.heirloom_id = h.id ORDER BY c.sort_order DESC, c.id DESC LIMIT 1),
			(SELECT m.id FROM media_links l JOIN media m ON m.id = l.media_id
				WHERE l.entity_type = 'heirloom' AND l.entity_id = h.id AND m.kind = 'image' ORDER BY l.sort_order LIMIT 1)
		FROM heirlooms h WHERE `+strings.Join(where, " AND ")+` ORDER BY h.name COLLATE NOCASE, h.id LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	list := []HeirloomRef{}
	var holders []sql.NullInt64
	var ids []int64
	for rows.Next() {
		var h HeirloomRef
		var holder, photo sql.NullInt64
		if err := rows.Scan(&h.ID, &h.Name, &h.Kind, &h.MadeDate, &holder, &photo); err != nil {
			rows.Close()
			return nil, err
		}
		h.PhotoID = ptrID(photo)
		list = append(list, h)
		holders = append(holders, holder)
		if holder.Valid {
			ids = append(ids, holder.Int64)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	refs, err := s.personRefsChunked(ctx, a, ids)
	if err != nil {
		return nil, err
	}
	for i, h := range holders {
		if h.Valid {
			if r, ok := refs[h.Int64]; ok {
				list[i].Holder = &r
			}
		}
	}
	return list, nil
}
