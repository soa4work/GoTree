package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

// PortraitRef is the photo shown for a person, optionally cropped to a
// face tag.
type PortraitRef struct {
	MediaID  int64  `json:"mediaId"`
	RegionID *int64 `json:"regionId"`
}

// MediaRegion is a face tag on a photo.
type MediaRegion struct {
	ID     int64      `json:"id"`
	Person *PersonRef `json:"person"`
	// Name is what an imported tag called the face; it stays when the
	// region is matched to a person.
	Name   string  `json:"name"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Source string  `json:"source"`
}

// MediaLink is a record a medium belongs to.
type MediaLink struct {
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
	// Label is the person's name, "TYPE|name" for an event (as for
	// citations) or the source title.
	Label    string `json:"label"`
	PersonID *int64 `json:"personId"`
}

// Media is an uploaded file with what is known about it.
type Media struct {
	ID           int64         `json:"id"`
	SHA256       string        `json:"-"`
	Orientation  int           `json:"-"`
	Mime         string        `json:"mime"`
	Kind         string        `json:"kind"`
	Size         int64         `json:"size"`
	OriginalName string        `json:"originalName"`
	Width        *int          `json:"width"`
	Height       *int          `json:"height"`
	Title        string        `json:"title"`
	DateRaw      string        `json:"date"`
	Description  string        `json:"description"`
	Transcript   string        `json:"transcript"`
	TakenAt      string        `json:"takenAt"`
	Lat          *float64      `json:"lat"`
	Lng          *float64      `json:"lng"`
	Links        []MediaLink   `json:"links"`
	Regions      []MediaRegion `json:"regions"`
	CreatedAt    string        `json:"createdAt"`
	UpdatedAt    string        `json:"updatedAt"`
}

// MediaRef is the short form used in galleries and lists.
type MediaRef struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Mime   string `json:"mime"`
	Title  string `json:"title"`
	Width  *int   `json:"width"`
	Height *int   `json:"height"`
}

// RegionInput places a face tag; coordinates are relative 0-1.
type RegionInput struct {
	PersonID *int64  `json:"personId"`
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
}

// NewMedia describes a file that has just been stored.
type NewMedia struct {
	SHA256       string
	Mime         string
	Kind         string
	Size         int64
	OriginalName string
	Width        *int
	Height       *int
	Orientation  int
	TakenAt      string
	Lat          *float64
	Lng          *float64
	// Regions are face tags read from the file; they are imported only
	// when the file is new to the tree.
	Regions []RegionInput
}

// MediaUpdate is the editable part of a Media.
type MediaUpdate struct {
	Title       string `json:"title"`
	Date        string `json:"date"`
	Description string `json:"description"`
	Transcript  string `json:"transcript"`
}

// MediaLinkInput attaches a medium to a record.
type MediaLinkInput struct {
	EntityType string `json:"entityType"`
	EntityID   int64  `json:"entityId"`
}

// PortraitInput sets or, with a nil MediaID, clears a portrait.
type PortraitInput struct {
	MediaID  *int64 `json:"mediaId"`
	RegionID *int64 `json:"regionId"`
}

var mediaEntityTables = map[string]string{"person": "persons", "event": "events", "family": "families", "source": "sources", "heirloom": "heirlooms"}

func (s *Store) checkMediaLink(ctx context.Context, q queryer, a Actor, l MediaLinkInput) error {
	table, ok := mediaEntityTables[l.EntityType]
	if !ok {
		return &ValidationError{Fields: map[string]string{"entityType": "must be person, event, family or source"}}
	}
	if err := requireInTree(ctx, q, table, l.EntityID, a.TreeID); errors.Is(err, ErrNotFound) {
		return &ValidationError{Fields: map[string]string{"entityId": "does not exist"}}
	} else if err != nil {
		return err
	}
	return nil
}

// CreateMedia records a stored file, or returns the existing record when
// the tree already has the same file. link, when given, attaches it.
func (s *Store) CreateMedia(ctx context.Context, a Actor, in NewMedia, link *MediaLinkInput) (m Media, created bool, err error) {
	var id int64
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if link != nil {
			if err := s.checkMediaLink(ctx, tx, a, *link); err != nil {
				return err
			}
		}
		var err error
		if id, created, err = s.insertMedia(ctx, tx, a, in, s.now()); err != nil {
			return err
		}
		if link != nil {
			if err := insertMediaLink(ctx, tx, id, *link); err != nil {
				return err
			}
		}
		if m, err = s.getMedia(ctx, tx, a, id); err != nil {
			return err
		}
		if created {
			return s.logChange(ctx, tx, a, "media", id, "create", nil, m)
		}
		return nil
	})
	return m, created, err
}

// insertMedia adds the record of a stored file to the tree, or finds the
// one the tree already has for the same content. Face tags are imported
// only for a new record.
func (s *Store) insertMedia(ctx context.Context, tx *sql.Tx, a Actor, in NewMedia, now string) (id int64, created bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id FROM media WHERE tree_id = ? AND sha256 = ?`, a.TreeID, in.SHA256).Scan(&id)
	if !errors.Is(err, sql.ErrNoRows) {
		return id, false, err
	}
	title := strings.TrimSuffix(in.OriginalName, extOf(in.OriginalName))
	res, err := tx.ExecContext(ctx, `
		INSERT INTO media (tree_id, sha256, mime, kind, size, original_name, width, height, orientation, title,
			taken_at, gps_lat, gps_lng, created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TreeID, in.SHA256, in.Mime, in.Kind, in.Size, in.OriginalName, in.Width, in.Height, max(in.Orientation, 1),
		title, in.TakenAt, in.Lat, in.Lng, now, now, nullID(a.UserID), nullID(a.UserID))
	if err != nil {
		return 0, false, err
	}
	id, _ = res.LastInsertId()
	for _, r := range in.Regions {
		if validRegion(r) {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO media_regions (media_id, name, x, y, w, h, source) VALUES (?, ?, ?, ?, ?, ?, 'xmp')`,
				id, truncate(r.Name, 200), r.X, r.Y, r.W, r.H); err != nil {
				return 0, false, err
			}
		}
	}
	return id, true, nil
}

// MediaFileUsed reports whether any tree has a medium with this content,
// that is, whether the stored file is still needed.
func (s *Store) MediaFileUsed(ctx context.Context, sha string) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM media WHERE sha256 = ?`, sha).Scan(&n)
	return n > 0, err
}

func extOf(name string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		return name[i:]
	}
	return ""
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func validRegion(r RegionInput) bool {
	return r.X >= 0 && r.Y >= 0 && r.W > 0 && r.H > 0 && r.X+r.W <= 1.0001 && r.Y+r.H <= 1.0001
}

func insertMediaLink(ctx context.Context, tx *sql.Tx, mediaID int64, l MediaLinkInput) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO media_links (media_id, entity_type, entity_id, sort_order)
		VALUES (?, ?, ?, (SELECT coalesce(max(sort_order), -1) + 1 FROM media_links WHERE entity_type = ? AND entity_id = ?))
		ON CONFLICT DO NOTHING`, mediaID, l.EntityType, l.EntityID, l.EntityType, l.EntityID)
	return err
}

// GetMedia returns a medium with its links and face tags.
func (s *Store) GetMedia(ctx context.Context, a Actor, id int64) (Media, error) {
	return s.getMedia(ctx, s.DB, a, id)
}

func (s *Store) getMedia(ctx context.Context, q queryer, a Actor, id int64) (Media, error) {
	var m Media
	var w, h sql.NullInt64
	var lat, lng sql.NullFloat64
	err := q.QueryRowContext(ctx, `
		SELECT id, sha256, orientation, mime, kind, size, original_name, width, height, title, date_raw, description,
			transcript, taken_at, gps_lat, gps_lng, created_at, updated_at
		FROM media WHERE id = ? AND tree_id = ?`, id, a.TreeID).Scan(
		&m.ID, &m.SHA256, &m.Orientation, &m.Mime, &m.Kind, &m.Size, &m.OriginalName, &w, &h, &m.Title, &m.DateRaw,
		&m.Description, &m.Transcript, &m.TakenAt, &lat, &lng, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Media{}, ErrNotFound
	}
	if err != nil {
		return Media{}, err
	}
	m.Width, m.Height = intPtr(w), intPtr(h)
	if lat.Valid && lng.Valid {
		m.Lat, m.Lng = &lat.Float64, &lng.Float64
	}

	if m.Links, err = s.mediaLinks(ctx, q, a, id); err != nil {
		return Media{}, err
	}
	rows, err := q.QueryContext(ctx, `
		SELECT id, person_id, name, x, y, w, h, source FROM media_regions WHERE media_id = ? ORDER BY x, id`, id)
	if err != nil {
		return Media{}, err
	}
	m.Regions = []MediaRegion{}
	var personIDs []int64
	regionPerson := map[int]int64{}
	for rows.Next() {
		var r MediaRegion
		var pid sql.NullInt64
		if err := rows.Scan(&r.ID, &pid, &r.Name, &r.X, &r.Y, &r.W, &r.H, &r.Source); err != nil {
			rows.Close()
			return Media{}, err
		}
		if pid.Valid {
			regionPerson[len(m.Regions)] = pid.Int64
			personIDs = append(personIDs, pid.Int64)
		}
		m.Regions = append(m.Regions, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Media{}, err
	}
	refs, err := s.personRefs(ctx, q, a, personIDs)
	if err != nil {
		return Media{}, err
	}
	for i, pid := range regionPerson {
		ref := refs[pid]
		m.Regions[i].Person = &ref
	}
	return m, nil
}

func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func (s *Store) mediaLinks(ctx context.Context, q queryer, a Actor, mediaID int64) ([]MediaLink, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT l.entity_type, l.entity_id,
			CASE l.entity_type
				WHEN 'person' THEN l.entity_id
				WHEN 'event' THEN coalesce(
					(SELECT person_id FROM events WHERE id = l.entity_id),
					(SELECT coalesce(f.partner1_id, f.partner2_id) FROM events e JOIN families f ON f.id = e.family_id WHERE e.id = l.entity_id))
				WHEN 'family' THEN (SELECT coalesce(partner1_id, partner2_id) FROM families WHERE id = l.entity_id)
			END,
			CASE l.entity_type
				WHEN 'event' THEN (SELECT type || CASE WHEN custom_label <> '' THEN ':' || custom_label ELSE '' END FROM events WHERE id = l.entity_id)
				WHEN 'source' THEN (SELECT title FROM sources WHERE id = l.entity_id)
				WHEN 'heirloom' THEN (SELECT name FROM heirlooms WHERE id = l.entity_id)
			END
		FROM media_links l WHERE l.media_id = ? ORDER BY l.entity_type, l.entity_id`, mediaID)
	if err != nil {
		return nil, err
	}
	links := []MediaLink{}
	var personIDs []int64
	extra := map[int]string{}
	for rows.Next() {
		var l MediaLink
		var pid sql.NullInt64
		var label sql.NullString
		if err := rows.Scan(&l.EntityType, &l.EntityID, &pid, &label); err != nil {
			rows.Close()
			return nil, err
		}
		l.PersonID = ptrID(pid)
		if pid.Valid {
			personIDs = append(personIDs, pid.Int64)
		}
		extra[len(links)] = label.String
		links = append(links, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	refs, err := s.personRefs(ctx, q, a, personIDs)
	if err != nil {
		return nil, err
	}
	for i := range links {
		l := &links[i]
		name := ""
		if l.PersonID != nil {
			p := refs[*l.PersonID]
			name = strings.TrimSpace(p.GivenNames + " " + p.Surname)
		}
		switch l.EntityType {
		case "event":
			l.Label = extra[i] + "|" + name
		case "source", "heirloom":
			l.Label = extra[i]
		default:
			l.Label = name
		}
	}
	return links, nil
}

// ListMedia lists the media of one record in their order, or all media of
// the tree (newest first) when entityType is empty.
func (s *Store) ListMedia(ctx context.Context, a Actor, entityType string, entityID int64, limit, offset int) ([]MediaRef, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows *sql.Rows
	var err error
	if entityType == "" {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT id, kind, mime, title, width, height FROM media WHERE tree_id = ?
			ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, a.TreeID, limit, max(offset, 0))
	} else {
		rows, err = s.DB.QueryContext(ctx, `
			SELECT m.id, m.kind, m.mime, m.title, m.width, m.height
			FROM media_links l JOIN media m ON m.id = l.media_id
			WHERE m.tree_id = ? AND l.entity_type = ? AND l.entity_id = ?
			ORDER BY l.sort_order, m.id LIMIT ? OFFSET ?`, a.TreeID, entityType, entityID, limit, max(offset, 0))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []MediaRef{}
	for rows.Next() {
		var r MediaRef
		var w, h sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Kind, &r.Mime, &r.Title, &w, &h); err != nil {
			return nil, err
		}
		r.Width, r.Height = intPtr(w), intPtr(h)
		list = append(list, r)
	}
	return list, rows.Err()
}

// UpdateMedia replaces the descriptive fields of a medium.
func (s *Store) UpdateMedia(ctx context.Context, a Actor, id int64, in MediaUpdate) (Media, error) {
	in.Title, in.Date = collapse(in.Title), storedDate(in.Date)
	in.Description, in.Transcript = strings.TrimSpace(in.Description), strings.TrimSpace(in.Transcript)
	var v validator
	v.check(utf8.RuneCountInString(in.Title) <= 300, "title", "is too long")
	v.check(utf8.RuneCountInString(in.Date) <= 120, "date", "is too long")
	v.check(utf8.RuneCountInString(in.Transcript) <= 1_000_000, "transcript", "is too long")
	if err := v.err(); err != nil {
		return Media{}, err
	}
	var m Media
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getMedia(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE media SET title = ?, date_raw = ?, description = ?, transcript = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			in.Title, in.Date, in.Description, in.Transcript, s.now(), nullID(a.UserID), id, a.TreeID); err != nil {
			return err
		}
		if m, err = s.getMedia(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "media", id, "update", before, m)
	})
	return m, err
}

// DeleteMedia removes a medium. orphaned reports that no tree uses the
// file any more, so the caller may delete it from disk.
func (s *Store) DeleteMedia(ctx context.Context, a Actor, id int64) (sha string, orphaned bool, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getMedia(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		var others int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM media WHERE sha256 = ?`, before.SHA256).Scan(&others); err != nil {
			return err
		}
		sha, orphaned = before.SHA256, others == 0
		return s.logChange(ctx, tx, a, "media", id, "delete", before, nil)
	})
	return sha, orphaned, err
}

// LinkMedia attaches a medium to a record.
func (s *Store) LinkMedia(ctx context.Context, a Actor, id int64, l MediaLinkInput) (Media, error) {
	var m Media
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := requireInTree(ctx, tx, "media", id, a.TreeID); err != nil {
			return err
		}
		if err := s.checkMediaLink(ctx, tx, a, l); err != nil {
			return err
		}
		if err := insertMediaLink(ctx, tx, id, l); err != nil {
			return err
		}
		var err error
		m, err = s.getMedia(ctx, tx, a, id)
		return err
	})
	return m, err
}

// UnlinkMedia detaches a medium from a record. The file stays in the
// media list.
func (s *Store) UnlinkMedia(ctx context.Context, a Actor, id int64, entityType string, entityID int64) error {
	if err := requireInTree(ctx, s.DB, "media", id, a.TreeID); err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM media_links WHERE media_id = ? AND entity_type = ? AND entity_id = ?`, id, entityType, entityID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) validateRegion(ctx context.Context, q queryer, a Actor, in RegionInput) error {
	var v validator
	v.check(validRegion(in), "x", "the box must lie within the image")
	v.check(utf8.RuneCountInString(in.Name) <= 200, "name", "is too long")
	if in.PersonID != nil {
		if err := requireInTree(ctx, q, "persons", *in.PersonID, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add("personId", "does not exist")
		} else if err != nil {
			return err
		}
	}
	return v.err()
}

// AddRegion tags a face on a photo. A tagged person also gets the photo
// in their gallery.
func (s *Store) AddRegion(ctx context.Context, a Actor, mediaID int64, in RegionInput) (Media, error) {
	in.Name = collapse(in.Name)
	var m Media
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getMedia(ctx, tx, a, mediaID)
		if err != nil {
			return err
		}
		if before.Kind != "image" {
			return &ValidationError{Fields: map[string]string{"mediaId": "only photos can have face tags"}}
		}
		if err := s.validateRegion(ctx, tx, a, in); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO media_regions (media_id, person_id, name, x, y, w, h, source) VALUES (?, ?, ?, ?, ?, ?, ?, 'manual')`,
			mediaID, nullIDPtr(in.PersonID), in.Name, in.X, in.Y, in.W, in.H); err != nil {
			return err
		}
		if in.PersonID != nil {
			if err := insertMediaLink(ctx, tx, mediaID, MediaLinkInput{EntityType: "person", EntityID: *in.PersonID}); err != nil {
				return err
			}
		}
		if m, err = s.getMedia(ctx, tx, a, mediaID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "media", mediaID, "update", before, m)
	})
	return m, err
}

func (s *Store) regionMedia(ctx context.Context, q queryer, a Actor, regionID int64) (int64, error) {
	var mediaID int64
	err := q.QueryRowContext(ctx, `
		SELECT r.media_id FROM media_regions r JOIN media m ON m.id = r.media_id WHERE r.id = ? AND m.tree_id = ?`,
		regionID, a.TreeID).Scan(&mediaID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return mediaID, err
}

// UpdateRegion moves a face tag or changes who it shows.
func (s *Store) UpdateRegion(ctx context.Context, a Actor, regionID int64, in RegionInput) (Media, error) {
	in.Name = collapse(in.Name)
	var m Media
	err := s.tx(ctx, func(tx *sql.Tx) error {
		mediaID, err := s.regionMedia(ctx, tx, a, regionID)
		if err != nil {
			return err
		}
		before, err := s.getMedia(ctx, tx, a, mediaID)
		if err != nil {
			return err
		}
		if err := s.validateRegion(ctx, tx, a, in); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE media_regions SET person_id = ?, name = ?, x = ?, y = ?, w = ?, h = ? WHERE id = ?`,
			nullIDPtr(in.PersonID), in.Name, in.X, in.Y, in.W, in.H, regionID); err != nil {
			return err
		}
		if in.PersonID != nil {
			if err := insertMediaLink(ctx, tx, mediaID, MediaLinkInput{EntityType: "person", EntityID: *in.PersonID}); err != nil {
				return err
			}
		}
		if m, err = s.getMedia(ctx, tx, a, mediaID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "media", mediaID, "update", before, m)
	})
	return m, err
}

// DeleteRegion removes a face tag.
func (s *Store) DeleteRegion(ctx context.Context, a Actor, regionID int64) (Media, error) {
	var m Media
	err := s.tx(ctx, func(tx *sql.Tx) error {
		mediaID, err := s.regionMedia(ctx, tx, a, regionID)
		if err != nil {
			return err
		}
		before, err := s.getMedia(ctx, tx, a, mediaID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_regions WHERE id = ?`, regionID); err != nil {
			return err
		}
		if m, err = s.getMedia(ctx, tx, a, mediaID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "media", mediaID, "update", before, m)
	})
	return m, err
}

// SetPortrait chooses the photo (and optionally the face tag on it) shown
// for a person; a nil MediaID removes the portrait.
func (s *Store) SetPortrait(ctx context.Context, a Actor, personID int64, in PortraitInput) (Person, error) {
	var p Person
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getPerson(ctx, tx, a, personID)
		if err != nil {
			return err
		}
		if in.MediaID != nil {
			m, err := s.getMedia(ctx, tx, a, *in.MediaID)
			if errors.Is(err, ErrNotFound) {
				return &ValidationError{Fields: map[string]string{"mediaId": "does not exist"}}
			} else if err != nil {
				return err
			}
			if m.Kind != "image" {
				return &ValidationError{Fields: map[string]string{"mediaId": "a portrait must be a photo"}}
			}
			if in.RegionID != nil {
				found := false
				for _, r := range m.Regions {
					found = found || r.ID == *in.RegionID
				}
				if !found {
					return &ValidationError{Fields: map[string]string{"regionId": "is not a face tag on this photo"}}
				}
			}
			if err := insertMediaLink(ctx, tx, *in.MediaID, MediaLinkInput{EntityType: "person", EntityID: personID}); err != nil {
				return err
			}
		} else {
			in.RegionID = nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE persons SET portrait_media_id = ?, portrait_region_id = ?, updated_at = ?, updated_by = ? WHERE id = ?`,
			nullIDPtr(in.MediaID), nullIDPtr(in.RegionID), s.now(), nullID(a.UserID), personID); err != nil {
			return err
		}
		if p, err = s.getPerson(ctx, tx, a, personID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "person", personID, "update", before, p)
	})
	return p, err
}

// Region returns a face tag of a medium, for cropping thumbnails.
func (s *Store) Region(ctx context.Context, a Actor, mediaID, regionID int64) (MediaRegion, error) {
	var r MediaRegion
	err := s.DB.QueryRowContext(ctx, `
		SELECT r.id, r.x, r.y, r.w, r.h FROM media_regions r JOIN media m ON m.id = r.media_id
		WHERE r.id = ? AND r.media_id = ? AND m.tree_id = ?`, regionID, mediaID, a.TreeID).Scan(&r.ID, &r.X, &r.Y, &r.W, &r.H)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaRegion{}, ErrNotFound
	}
	return r, err
}
