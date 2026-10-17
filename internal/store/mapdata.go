package store

import (
	"context"
	"database/sql"
	"sort"
)

// MapPlace is a place with coordinates.
type MapPlace struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	// Approximate is set when the coordinates are those of an enclosing
	// place, e.g. the county of a village without its own.
	Approximate bool `json:"approximate"`
}

// MapPoint is a person somewhere at a time.
type MapPoint struct {
	// Key is the YYYYMMDD sort key of the event.
	Key     int    `json:"key"`
	PlaceID int64  `json:"placeId"`
	Type    string `json:"type"`
}

// MapTrack is where one person was over their life.
type MapTrack struct {
	PersonID int64      `json:"personId"`
	Points   []MapPoint `json:"points"`
	// Death is the sort key of the death or burial, 0 when unknown.
	Death int `json:"death"`
}

// MapData feeds the migration map.
type MapData struct {
	Tracks  []MapTrack          `json:"tracks"`
	Places  map[int64]MapPlace  `json:"places"`
	Persons map[int64]PersonRef `json:"persons"`
	// Unmapped counts dated events at places without coordinates, which
	// the map cannot show.
	Unmapped int `json:"unmapped"`
}

// Map scopes.
const (
	MapScopeAll         = "all"
	MapScopeAncestors   = "ancestors"
	MapScopeDescendants = "descendants"
)

// mapEventTypes place a person somewhere at a time.
var mapEventTypes = []string{"BIRT", "CHR", "BAPM", "CONF", "EDUC", "GRAD", "OCCU", "RESI", "CENS", "EMIG", "IMMI", "NATU", "RETI", "MARR", "DEAT", "BURI", "CREM"}

// MapData returns the dated, placed events of the tree, or of the
// ancestors or descendants of root.
func (s *Store) MapData(ctx context.Context, a Actor, scope string, root int64) (MapData, error) {
	out := MapData{Tracks: []MapTrack{}, Places: map[int64]MapPlace{}, Persons: map[int64]PersonRef{}}
	var include map[int64]bool
	if scope != MapScopeAll {
		if err := requireInTree(ctx, s.DB, "persons", root, a.TreeID); err != nil {
			return out, err
		}
		var err error
		if scope == MapScopeDescendants {
			include, err = s.descendantsWithPartners(ctx, a, root)
		} else {
			include, err = s.ancestorsOf(ctx, a, root)
		}
		if err != nil {
			return out, err
		}
	}

	coords, err := s.placeCoordinates(ctx, a)
	if err != nil {
		return out, err
	}

	args := []any{a.TreeID}
	for _, t := range mapEventTypes {
		args = append(args, t)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.person_id, f.partner1_id, f.partner2_id, e.type, e.date_sort, e.place_id
		FROM events e LEFT JOIN families f ON f.id = e.family_id
		WHERE e.tree_id = ? AND e.type IN (`+placeholders(len(mapEventTypes))+`) AND e.status <> 'disproven'
			AND e.date_sort > 0 AND e.place_id IS NOT NULL AND e.date_qualifier NOT IN ('BEF', 'AFT')
		ORDER BY e.date_sort, e.id`, args...)
	if err != nil {
		return out, err
	}
	tracks := map[int64]*MapTrack{}
	for rows.Next() {
		var person, p1, p2 sql.NullInt64
		var typ string
		var key int
		var place int64
		if err := rows.Scan(&person, &p1, &p2, &typ, &key, &place); err != nil {
			rows.Close()
			return out, err
		}
		pl, ok := coords[place]
		if !ok {
			out.Unmapped++
			continue
		}
		out.Places[place] = pl
		for _, p := range []sql.NullInt64{person, p1, p2} {
			if !p.Valid || (include != nil && !include[p.Int64]) {
				continue
			}
			t := tracks[p.Int64]
			if t == nil {
				t = &MapTrack{PersonID: p.Int64}
				tracks[p.Int64] = t
			}
			t.Points = append(t.Points, MapPoint{Key: key, PlaceID: place, Type: typ})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	// The death ends a track even when it has no place to show.
	rows, err = s.DB.QueryContext(ctx, `
		SELECT person_id, min(date_sort) FROM events
		WHERE tree_id = ? AND type IN ('DEAT', 'BURI', 'CREM') AND status <> 'disproven' AND date_sort > 0 AND person_id IS NOT NULL
		GROUP BY person_id`, a.TreeID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id int64
		var key int
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return out, err
		}
		if t := tracks[id]; t != nil {
			t.Death = key
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	ids := make([]int64, 0, len(tracks))
	for id, t := range tracks {
		out.Tracks = append(out.Tracks, *t)
		ids = append(ids, id)
	}
	sort.Slice(out.Tracks, func(i, j int) bool { return out.Tracks[i].Points[0].Key < out.Tracks[j].Points[0].Key })
	out.Persons, err = s.personRefsChunked(ctx, a, ids)
	return out, err
}

// placeCoordinates gives every place coordinates: its own, or those of
// the nearest enclosing place that has them.
func (s *Store) placeCoordinates(ctx context.Context, a Actor) (map[int64]MapPlace, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, parent_id, lat, lng FROM places WHERE tree_id = ?`, a.TreeID)
	if err != nil {
		return nil, err
	}
	type place struct {
		parent   sql.NullInt64
		lat, lng sql.NullFloat64
	}
	all := map[int64]place{}
	var ids []int64
	for rows.Next() {
		var id int64
		var p place
		if err := rows.Scan(&id, &p.parent, &p.lat, &p.lng); err != nil {
			rows.Close()
			return nil, err
		}
		all[id] = p
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	names := map[int64]PlaceRef{}
	for start := 0; start < len(ids); start += 500 {
		refs, err := placeRefs(ctx, s.DB, a, ids[start:min(start+500, len(ids))])
		if err != nil {
			return nil, err
		}
		for id, r := range refs {
			names[id] = r
		}
	}
	out := map[int64]MapPlace{}
	for id := range all {
		approx := false
		// Place hierarchies cannot loop (the store refuses it), but the
		// depth bound keeps a damaged database from hanging the request.
		for cur, depth := id, 0; depth < 20; depth++ {
			p, ok := all[cur]
			if !ok {
				break
			}
			if p.lat.Valid && p.lng.Valid {
				out[id] = MapPlace{Name: names[id].FullName, Lat: p.lat.Float64, Lng: p.lng.Float64, Approximate: approx}
				break
			}
			if !p.parent.Valid {
				break
			}
			cur, approx = p.parent.Int64, true
		}
	}
	return out, nil
}

// ancestorsOf is root and everyone root descends from, through any kind
// of parent link.
func (s *Store) ancestorsOf(ctx context.Context, a Actor, root int64) (map[int64]bool, error) {
	fams, err := s.familyLinks(ctx, a)
	if err != nil {
		return nil, err
	}
	parentsOf := map[int64][]int64{}
	for _, f := range fams {
		for _, c := range f.children {
			for _, p := range []int64{f.p1, f.p2} {
				if p != 0 {
					parentsOf[c.id] = append(parentsOf[c.id], p)
				}
			}
		}
	}
	out := map[int64]bool{root: true}
	queue := []int64{root}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, p := range parentsOf[id] {
			if !out[p] {
				out[p] = true
				queue = append(queue, p)
			}
		}
	}
	return out, nil
}

// UnmappedPlace is a place the map cannot show: neither it nor any enclosing
// place has coordinates, yet dated events happen there.
type UnmappedPlace struct {
	Place
	// Events counts the dated events the map would show there.
	Events int `json:"events"`
}

// maxUnmappedPlaces bounds the list; the busiest places come first.
const maxUnmappedPlaces = 200

// UnmappedPlaces lists the places used by the dated events that the map
// cannot show for lack of coordinates, busiest first.
func (s *Store) UnmappedPlaces(ctx context.Context, a Actor) ([]UnmappedPlace, error) {
	coords, err := s.placeCoordinates(ctx, a)
	if err != nil {
		return nil, err
	}
	args := []any{a.TreeID}
	for _, t := range mapEventTypes {
		args = append(args, t)
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT place_id, count(*) FROM events
		WHERE tree_id = ? AND type IN (`+placeholders(len(mapEventTypes))+`) AND status <> 'disproven'
			AND date_sort > 0 AND place_id IS NOT NULL AND date_qualifier NOT IN ('BEF', 'AFT')
		GROUP BY place_id ORDER BY count(*) DESC, place_id`, args...)
	if err != nil {
		return nil, err
	}
	type used struct {
		id     int64
		events int
	}
	var missing []used
	for rows.Next() {
		var u used
		if err := rows.Scan(&u.id, &u.events); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := coords[u.id]; !ok {
			missing = append(missing, u)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []UnmappedPlace{}
	for _, u := range missing[:min(len(missing), maxUnmappedPlaces)] {
		p, err := getPlace(ctx, s.DB, a, u.id)
		if err != nil {
			return nil, err
		}
		out = append(out, UnmappedPlace{Place: p, Events: u.events})
	}
	return out, nil
}
