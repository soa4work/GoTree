package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"unicode/utf8"
)

// Place is a node in the place hierarchy, e.g. Berlin → Brandenburg → Germany.
type Place struct {
	ID        int64    `json:"id"`
	ParentID  *int64   `json:"parentId"`
	Name      string   `json:"name"`
	PlaceType string   `json:"placeType"`
	Lat       *float64 `json:"lat"`
	Lng       *float64 `json:"lng"`
	Notes     string   `json:"notes"`
	// FullName joins the names from this place up to the root, as GEDCOM
	// writes it: "Berlin, Brandenburg, Germany".
	FullName  string `json:"fullName"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// PlaceRef is the short form of a place embedded in other records.
type PlaceRef struct {
	ID       int64  `json:"id"`
	FullName string `json:"fullName"`
}

// PlaceInput is the editable part of a Place.
type PlaceInput struct {
	ParentID  *int64   `json:"parentId"`
	Name      string   `json:"name"`
	PlaceType string   `json:"placeType"`
	Lat       *float64 `json:"lat"`
	Lng       *float64 `json:"lng"`
	Notes     string   `json:"notes"`
}

// maxPlaceDepth guards the hierarchy walk against data that slipped past
// the cycle check, e.g. from a hand-edited database.
const maxPlaceDepth = 32

const placeFullNameCTE = `
	WITH RECURSIVE chain(start_id, id, parent_id, name, depth) AS (
		SELECT id, id, parent_id, name, 0 FROM places WHERE tree_id = ?
		UNION ALL
		SELECT c.start_id, p.id, p.parent_id, p.name, c.depth + 1
		FROM chain c JOIN places p ON p.id = c.parent_id
		WHERE c.depth < 32
	),
	full_names(id, full_name) AS (
		SELECT start_id, group_concat(name, ', ' ORDER BY depth) FROM chain GROUP BY start_id
	)`

func (in *PlaceInput) normalize() {
	in.Name = strings.TrimSpace(in.Name)
	in.PlaceType = strings.TrimSpace(strings.ToLower(in.PlaceType))
	in.Notes = strings.TrimSpace(in.Notes)
}

func (s *Store) validatePlace(ctx context.Context, q queryer, a Actor, id int64, in PlaceInput) error {
	var v validator
	v.check(in.Name != "", "name", "is required")
	v.check(!strings.Contains(in.Name, ","), "name", "must not contain a comma; use the parent place for the larger region")
	v.check(utf8.RuneCountInString(in.Name) <= 200, "name", "is too long")
	v.check(utf8.RuneCountInString(in.PlaceType) <= 50, "placeType", "is too long")
	v.check((in.Lat == nil) == (in.Lng == nil), "lat", "latitude and longitude must be given together")
	if in.Lat != nil {
		v.check(!math.IsNaN(*in.Lat) && *in.Lat >= -90 && *in.Lat <= 90, "lat", "must be between -90 and 90")
	}
	if in.Lng != nil {
		v.check(!math.IsNaN(*in.Lng) && *in.Lng >= -180 && *in.Lng <= 180, "lng", "must be between -180 and 180")
	}
	if in.ParentID != nil {
		if err := requireInTree(ctx, q, "places", *in.ParentID, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add("parentId", "does not exist")
		} else if err != nil {
			return err
		} else if id != 0 {
			cyclic, err := placeIsAncestorOf(ctx, q, id, *in.ParentID)
			if err != nil {
				return err
			}
			v.check(!cyclic, "parentId", "a place cannot be inside itself")
		}
	}
	return v.err()
}

// placeIsAncestorOf reports whether ancestor is candidate or one of its parents.
func placeIsAncestorOf(ctx context.Context, q queryer, ancestor, candidate int64) (bool, error) {
	var found int
	err := q.QueryRowContext(ctx, `
		WITH RECURSIVE up(id, parent_id, depth) AS (
			SELECT id, parent_id, 0 FROM places WHERE id = ?
			UNION ALL
			SELECT p.id, p.parent_id, up.depth + 1 FROM places p JOIN up ON p.id = up.parent_id WHERE up.depth < ?
		)
		SELECT count(*) FROM up WHERE id = ?`, candidate, maxPlaceDepth, ancestor).Scan(&found)
	return found > 0, err
}

// ListPlaces returns the tree's places, filtered by a case-insensitive
// substring of the full name when q is not empty.
func (s *Store) ListPlaces(ctx context.Context, a Actor, q string, limit int) ([]Place, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{a.TreeID, a.TreeID}
	filter := ""
	if q = strings.TrimSpace(q); q != "" {
		filter = ` AND f.full_name LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(q)+"%")
	}
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, placeFullNameCTE+`
		SELECT p.id, p.parent_id, p.name, p.place_type, p.lat, p.lng, p.notes, f.full_name, p.created_at, p.updated_at
		FROM places p JOIN full_names f ON f.id = p.id
		WHERE p.tree_id = ?`+filter+`
		ORDER BY f.full_name COLLATE NAMES, f.id
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	places := []Place{}
	for rows.Next() {
		p, err := scanPlace(rows)
		if err != nil {
			return nil, err
		}
		places = append(places, p)
	}
	return places, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

type scanner interface{ Scan(dest ...any) error }

func scanPlace(sc scanner) (Place, error) {
	var p Place
	var parent sql.NullInt64
	var lat, lng sql.NullFloat64
	if err := sc.Scan(&p.ID, &parent, &p.Name, &p.PlaceType, &lat, &lng, &p.Notes, &p.FullName, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return Place{}, err
	}
	p.ParentID = ptrID(parent)
	if lat.Valid && lng.Valid {
		p.Lat, p.Lng = &lat.Float64, &lng.Float64
	}
	return p, nil
}

// GetPlace returns one place.
func (s *Store) GetPlace(ctx context.Context, a Actor, id int64) (Place, error) {
	return getPlace(ctx, s.DB, a, id)
}

func getPlace(ctx context.Context, q queryer, a Actor, id int64) (Place, error) {
	p, err := scanPlace(q.QueryRowContext(ctx, placeFullNameCTE+`
		SELECT p.id, p.parent_id, p.name, p.place_type, p.lat, p.lng, p.notes, f.full_name, p.created_at, p.updated_at
		FROM places p JOIN full_names f ON f.id = p.id
		WHERE p.tree_id = ? AND p.id = ?`, a.TreeID, a.TreeID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Place{}, ErrNotFound
	}
	return p, err
}

// placeRefs resolves full names for a set of place ids.
func placeRefs(ctx context.Context, q queryer, a Actor, ids []int64) (map[int64]PlaceRef, error) {
	refs := map[int64]PlaceRef{}
	if len(ids) == 0 {
		return refs, nil
	}
	args := append([]any{a.TreeID}, int64Args(ids)...)
	rows, err := q.QueryContext(ctx, placeFullNameCTE+`
		SELECT id, full_name FROM full_names WHERE id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r PlaceRef
		if err := rows.Scan(&r.ID, &r.FullName); err != nil {
			return nil, err
		}
		refs[r.ID] = r
	}
	return refs, rows.Err()
}

// CreatePlace adds a place.
func (s *Store) CreatePlace(ctx context.Context, a Actor, in PlaceInput) (Place, error) {
	var p Place
	err := s.tx(ctx, func(tx *sql.Tx) (err error) {
		p, err = s.createPlaceTx(ctx, tx, a, in)
		return err
	})
	return p, err
}

func (s *Store) createPlaceTx(ctx context.Context, tx *sql.Tx, a Actor, in PlaceInput) (Place, error) {
	in.normalize()
	var p Place
	err := func() error {
		if err := s.validatePlace(ctx, tx, a, 0, in); err != nil {
			return err
		}
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO places (tree_id, parent_id, name, place_type, lat, lng, notes, created_at, updated_at, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, nullIDPtr(in.ParentID), in.Name, in.PlaceType, in.Lat, in.Lng, in.Notes, now, now, nullID(a.UserID), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if p, err = getPlace(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "place", id, "create", nil, p)
	}()
	return p, err
}

// UpdatePlace replaces the editable fields of a place.
func (s *Store) UpdatePlace(ctx context.Context, a Actor, id int64, in PlaceInput) (Place, error) {
	in.normalize()
	var p Place
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := getPlace(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if err := s.validatePlace(ctx, tx, a, id, in); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE places SET parent_id = ?, name = ?, place_type = ?, lat = ?, lng = ?, notes = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			nullIDPtr(in.ParentID), in.Name, in.PlaceType, in.Lat, in.Lng, in.Notes, s.now(), nullID(a.UserID), id, a.TreeID)
		if err != nil {
			return err
		}
		if p, err = getPlace(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "place", id, "update", before, p)
	})
	return p, err
}

// DeletePlace removes a place. It fails with ErrConflict while events or
// other places still refer to it.
func (s *Store) DeletePlace(ctx context.Context, a Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := getPlace(ctx, tx, a, id)
		if err != nil {
			return err
		}
		var uses int
		if err := tx.QueryRowContext(ctx, `
			SELECT (SELECT count(*) FROM events WHERE place_id = ?) + (SELECT count(*) FROM places WHERE parent_id = ?)`,
			id, id).Scan(&uses); err != nil {
			return err
		}
		if uses > 0 {
			return ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM places WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "place", id, "delete", before, nil)
	})
}
