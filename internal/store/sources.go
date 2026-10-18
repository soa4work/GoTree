package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

// Repository is where sources are kept: an archive, library or website.
type Repository struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// RepositoryInput is the editable part of a Repository.
type RepositoryInput struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

// Source is a document, register, book or website that facts come from.
type Source struct {
	ID          int64       `json:"id"`
	Title       string      `json:"title"`
	Author      string      `json:"author"`
	Publication string      `json:"publication"`
	CallNumber  string      `json:"callNumber"`
	Repository  *Repository `json:"repository"`
	Notes       string      `json:"notes"`
	// CitationCount is how many citations point into this source.
	CitationCount int    `json:"citationCount"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// SourceInput is the editable part of a Source.
type SourceInput struct {
	Title        string `json:"title"`
	Author       string `json:"author"`
	Publication  string `json:"publication"`
	CallNumber   string `json:"callNumber"`
	RepositoryID *int64 `json:"repositoryId"`
	Notes        string `json:"notes"`
}

// CitationLink is one fact a citation supports.
type CitationLink struct {
	EntityType   string `json:"entityType"`
	EntityID     int64  `json:"entityId"`
	Field        string `json:"field"`
	Status       string `json:"status"`
	StatusReason string `json:"statusReason"`
	// Label describes the target for display, e.g. "Birth of Anna Müller".
	Label string `json:"label"`
	// PersonID is the person to link to when showing the target.
	PersonID *int64 `json:"personId"`
}

// Citation points into a source and lists what it supports.
type Citation struct {
	ID        int64          `json:"id"`
	SourceID  int64          `json:"sourceId"`
	Page      string         `json:"page"`
	Quality   *int           `json:"quality"`
	Text      string         `json:"text"`
	Notes     string         `json:"notes"`
	Links     []CitationLink `json:"links"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

// CitationRef is a citation as shown next to the fact it supports.
type CitationRef struct {
	CitationID  int64  `json:"citationId"`
	SourceID    int64  `json:"sourceId"`
	SourceTitle string `json:"sourceTitle"`
	Page        string `json:"page"`
	Quality     *int   `json:"quality"`
	Field       string `json:"field"`
	Status      string `json:"status"`
}

// NewCitation cites a source for the record it is sent with, created in
// the same transaction.
type NewCitation struct {
	SourceID int64  `json:"sourceId"`
	Page     string `json:"page"`
	Quality  *int   `json:"quality"`
	Text     string `json:"text"`
}

// LinkInput says what a citation supports.
type LinkInput struct {
	EntityType   string `json:"entityType"`
	EntityID     int64  `json:"entityId"`
	Field        string `json:"field"`
	Status       string `json:"status"`
	StatusReason string `json:"statusReason"`
}

// CitationInput creates or replaces a citation. Links replaces the list
// of supported facts.
type CitationInput struct {
	SourceID int64       `json:"sourceId"`
	Page     string      `json:"page"`
	Quality  *int        `json:"quality"`
	Text     string      `json:"text"`
	Notes    string      `json:"notes"`
	Links    []LinkInput `json:"links"`
}

// SourceDetail is a source with its citations.
type SourceDetail struct {
	Source
	Citations []Citation `json:"citations"`
}

var entityTables = map[string]string{"person": "persons", "event": "events", "family": "families", "heirloom": "heirlooms"}

func (in *RepositoryInput) normalize() {
	in.Name, in.Address, in.URL, in.Notes = strings.TrimSpace(in.Name), strings.TrimSpace(in.Address), strings.TrimSpace(in.URL), strings.TrimSpace(in.Notes)
}

func (in RepositoryInput) validate() error {
	var v validator
	v.check(in.Name != "", "name", "is required")
	v.check(utf8.RuneCountInString(in.Name) <= 300, "name", "is too long")
	v.check(in.URL == "" || strings.HasPrefix(in.URL, "http://") || strings.HasPrefix(in.URL, "https://"), "url", "must start with http:// or https://")
	return v.err()
}

// ListRepositories returns the tree's repositories by name, filtered by q.
func (s *Store) ListRepositories(ctx context.Context, a Actor, q string) ([]Repository, error) {
	args := []any{a.TreeID}
	filter := ""
	if q = strings.TrimSpace(q); q != "" {
		filter = ` AND name LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(q)+"%")
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, name, address, url, notes, created_at, updated_at FROM repositories
		WHERE tree_id = ?`+filter+` ORDER BY name COLLATE NAMES, id LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Repository{}
	for rows.Next() {
		var r Repository
		if err := rows.Scan(&r.ID, &r.Name, &r.Address, &r.URL, &r.Notes, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func getRepository(ctx context.Context, q queryer, a Actor, id int64) (Repository, error) {
	var r Repository
	err := q.QueryRowContext(ctx, `
		SELECT id, name, address, url, notes, created_at, updated_at FROM repositories WHERE id = ? AND tree_id = ?`,
		id, a.TreeID).Scan(&r.ID, &r.Name, &r.Address, &r.URL, &r.Notes, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Repository{}, ErrNotFound
	}
	return r, err
}

// CreateRepository adds a repository.
func (s *Store) CreateRepository(ctx context.Context, a Actor, in RepositoryInput) (Repository, error) {
	in.normalize()
	if err := in.validate(); err != nil {
		return Repository{}, err
	}
	var r Repository
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO repositories (tree_id, name, address, url, notes, created_at, updated_at, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, in.Name, in.Address, in.URL, in.Notes, now, now, nullID(a.UserID), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if r, err = getRepository(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "repository", id, "create", nil, r)
	})
	return r, err
}

// UpdateRepository replaces a repository's fields.
func (s *Store) UpdateRepository(ctx context.Context, a Actor, id int64, in RepositoryInput) (Repository, error) {
	in.normalize()
	if err := in.validate(); err != nil {
		return Repository{}, err
	}
	var r Repository
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := getRepository(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE repositories SET name = ?, address = ?, url = ?, notes = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			in.Name, in.Address, in.URL, in.Notes, s.now(), nullID(a.UserID), id, a.TreeID); err != nil {
			return err
		}
		if r, err = getRepository(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "repository", id, "update", before, r)
	})
	return r, err
}

// DeleteRepository removes a repository; its sources stay, without it.
func (s *Store) DeleteRepository(ctx context.Context, a Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := getRepository(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM repositories WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "repository", id, "delete", before, nil)
	})
}

func (in *SourceInput) normalize() {
	in.Title = collapse(in.Title)
	in.Author, in.Publication = strings.TrimSpace(in.Author), strings.TrimSpace(in.Publication)
	in.CallNumber, in.Notes = strings.TrimSpace(in.CallNumber), strings.TrimSpace(in.Notes)
}

func (s *Store) validateSource(ctx context.Context, q queryer, a Actor, in SourceInput) error {
	var v validator
	v.check(in.Title != "", "title", "is required")
	v.check(utf8.RuneCountInString(in.Title) <= 500, "title", "is too long")
	v.check(utf8.RuneCountInString(in.Author) <= 300, "author", "is too long")
	if in.RepositoryID != nil {
		if err := requireInTree(ctx, q, "repositories", *in.RepositoryID, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add("repositoryId", "does not exist")
		} else if err != nil {
			return err
		}
	}
	return v.err()
}

const sourceColumns = `
	s.id, s.title, s.author, s.publication, s.call_number, s.notes, s.created_at, s.updated_at,
	(SELECT count(*) FROM citations c WHERE c.source_id = s.id),
	r.id, r.name, r.address, r.url, r.notes, r.created_at, r.updated_at`

func scanSource(sc scanner) (Source, error) {
	var src Source
	var rid sql.NullInt64
	var rname, raddr, rurl, rnotes, rcreated, rupdated sql.NullString
	if err := sc.Scan(&src.ID, &src.Title, &src.Author, &src.Publication, &src.CallNumber, &src.Notes, &src.CreatedAt,
		&src.UpdatedAt, &src.CitationCount, &rid, &rname, &raddr, &rurl, &rnotes, &rcreated, &rupdated); err != nil {
		return Source{}, err
	}
	if rid.Valid {
		src.Repository = &Repository{ID: rid.Int64, Name: rname.String, Address: raddr.String, URL: rurl.String,
			Notes: rnotes.String, CreatedAt: rcreated.String, UpdatedAt: rupdated.String}
	}
	return src, nil
}

// ListSources returns sources by title, filtered by a substring of the
// title or author.
func (s *Store) ListSources(ctx context.Context, a Actor, q string, limit int) ([]Source, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args := []any{a.TreeID}
	filter := ""
	if q = strings.TrimSpace(q); q != "" {
		filter = ` AND (s.title LIKE ? ESCAPE '\' OR s.author LIKE ? ESCAPE '\')`
		like := "%" + escapeLike(q) + "%"
		args = append(args, like, like)
	}
	args = append(args, limit)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+sourceColumns+`
		FROM sources s LEFT JOIN repositories r ON r.id = s.repository_id
		WHERE s.tree_id = ?`+filter+` ORDER BY s.title COLLATE NAMES, s.id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Source{}
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, src)
	}
	return list, rows.Err()
}

func getSource(ctx context.Context, q queryer, a Actor, id int64) (Source, error) {
	src, err := scanSource(q.QueryRowContext(ctx, `SELECT `+sourceColumns+`
		FROM sources s LEFT JOIN repositories r ON r.id = s.repository_id
		WHERE s.id = ? AND s.tree_id = ?`, id, a.TreeID))
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, ErrNotFound
	}
	return src, err
}

// GetSource returns a source with all its citations and what they support.
func (s *Store) GetSource(ctx context.Context, a Actor, id int64) (SourceDetail, error) {
	src, err := getSource(ctx, s.DB, a, id)
	if err != nil {
		return SourceDetail{}, err
	}
	cits, err := s.loadCitations(ctx, s.DB, a, `c.source_id = ?`, id)
	if err != nil {
		return SourceDetail{}, err
	}
	return SourceDetail{Source: src, Citations: cits}, nil
}

// CreateSource adds a source.
func (s *Store) CreateSource(ctx context.Context, a Actor, in SourceInput) (Source, error) {
	in.normalize()
	var src Source
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := s.validateSource(ctx, tx, a, in); err != nil {
			return err
		}
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO sources (tree_id, repository_id, call_number, title, author, publication, notes, created_at, updated_at, created_by, updated_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, nullIDPtr(in.RepositoryID), in.CallNumber, in.Title, in.Author, in.Publication, in.Notes, now, now,
			nullID(a.UserID), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		if src, err = getSource(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "source", id, "create", nil, src)
	})
	return src, err
}

// UpdateSource replaces a source's fields.
func (s *Store) UpdateSource(ctx context.Context, a Actor, id int64, in SourceInput) (Source, error) {
	in.normalize()
	var src Source
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := getSource(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if err := s.validateSource(ctx, tx, a, in); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE sources SET repository_id = ?, call_number = ?, title = ?, author = ?, publication = ?, notes = ?,
				updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			nullIDPtr(in.RepositoryID), in.CallNumber, in.Title, in.Author, in.Publication, in.Notes, s.now(), nullID(a.UserID),
			id, a.TreeID); err != nil {
			return err
		}
		if src, err = getSource(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "source", id, "update", before, src)
	})
	return src, err
}

// DeleteSource removes a source with all its citations.
func (s *Store) DeleteSource(ctx context.Context, a Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := getSource(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sources WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "source", id, "delete", before, nil)
	})
}

func (in *CitationInput) normalize() {
	in.Page, in.Text, in.Notes = strings.TrimSpace(in.Page), strings.TrimSpace(in.Text), strings.TrimSpace(in.Notes)
	for i := range in.Links {
		l := &in.Links[i]
		l.EntityType = strings.ToLower(strings.TrimSpace(l.EntityType))
		l.Field = strings.TrimSpace(l.Field)
		l.Status = defaultStatus(l.Status)
		l.StatusReason = strings.TrimSpace(l.StatusReason)
	}
}

func (s *Store) validateCitation(ctx context.Context, q queryer, a Actor, in CitationInput) error {
	var v validator
	if err := requireInTree(ctx, q, "sources", in.SourceID, a.TreeID); errors.Is(err, ErrNotFound) {
		v.add("sourceId", "does not exist")
	} else if err != nil {
		return err
	}
	v.check(in.Quality == nil || (*in.Quality >= 0 && *in.Quality <= 3), "quality", "must be between 0 and 3")
	v.check(utf8.RuneCountInString(in.Page) <= 500, "page", "is too long")
	v.check(utf8.RuneCountInString(in.Text) <= 100_000, "text", "is too long")
	v.check(len(in.Links) <= 200, "links", "too many links")
	seen := map[LinkInput]bool{}
	for _, l := range in.Links {
		key := LinkInput{EntityType: l.EntityType, EntityID: l.EntityID, Field: l.Field}
		v.check(!seen[key], "links", "the same fact is listed twice")
		seen[key] = true
		v.check(validStatus(l.Status), "links", "unknown status "+l.Status)
		v.check(utf8.RuneCountInString(l.Field) <= 50, "links", "field name is too long")
		if err := requireEntity(ctx, q, a, l.EntityType, l.EntityID); errors.Is(err, ErrNotFound) {
			v.add("links", "a cited "+l.EntityType+" does not exist")
		} else if err != nil {
			return err
		}
	}
	return v.err()
}

// requireEntity checks that a citation target exists in the actor's tree.
func requireEntity(ctx context.Context, q queryer, a Actor, entityType string, id int64) error {
	if entityType == "name" {
		var one int
		err := q.QueryRowContext(ctx, `
			SELECT 1 FROM person_names n JOIN persons p ON p.id = n.person_id WHERE n.id = ? AND p.tree_id = ?`,
			id, a.TreeID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	table, ok := entityTables[entityType]
	if !ok {
		return ErrNotFound
	}
	return requireInTree(ctx, q, table, id, a.TreeID)
}

// CreateCitation adds a citation and links it to the facts it supports.
func (s *Store) CreateCitation(ctx context.Context, a Actor, in CitationInput) (Citation, error) {
	var c Citation
	err := s.tx(ctx, func(tx *sql.Tx) (err error) {
		c, err = s.createCitation(ctx, tx, a, in)
		return err
	})
	return c, err
}

func (s *Store) createCitation(ctx context.Context, tx *sql.Tx, a Actor, in CitationInput) (Citation, error) {
	in.normalize()
	if err := s.validateCitation(ctx, tx, a, in); err != nil {
		return Citation{}, err
	}
	now := s.now()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO citations (tree_id, source_id, page, quality, text, notes, created_at, updated_at, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TreeID, in.SourceID, in.Page, in.Quality, in.Text, in.Notes, now, now, nullID(a.UserID), nullID(a.UserID))
	if err != nil {
		return Citation{}, err
	}
	id, _ := res.LastInsertId()
	if err := writeLinks(ctx, tx, id, in.Links); err != nil {
		return Citation{}, err
	}
	c, err := s.getCitation(ctx, tx, a, id)
	if err != nil {
		return Citation{}, err
	}
	return c, s.logChange(ctx, tx, a, "citation", id, "create", nil, c)
}

// UpdateCitation replaces a citation and its links.
func (s *Store) UpdateCitation(ctx context.Context, a Actor, id int64, in CitationInput) (Citation, error) {
	in.normalize()
	var c Citation
	err := s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getCitation(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if err := s.validateCitation(ctx, tx, a, in); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE citations SET source_id = ?, page = ?, quality = ?, text = ?, notes = ?, updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ?`,
			in.SourceID, in.Page, in.Quality, in.Text, in.Notes, s.now(), nullID(a.UserID), id, a.TreeID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM citation_links WHERE citation_id = ?`, id); err != nil {
			return err
		}
		if err := writeLinks(ctx, tx, id, in.Links); err != nil {
			return err
		}
		if c, err = s.getCitation(ctx, tx, a, id); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "citation", id, "update", before, c)
	})
	return c, err
}

// DeleteCitation removes a citation and its links.
func (s *Store) DeleteCitation(ctx context.Context, a Actor, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.getCitation(ctx, tx, a, id)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM citations WHERE id = ? AND tree_id = ?`, id, a.TreeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "citation", id, "delete", before, nil)
	})
}

// GetCitation returns one citation with its links.
func (s *Store) GetCitation(ctx context.Context, a Actor, id int64) (Citation, error) {
	return s.getCitation(ctx, s.DB, a, id)
}

func writeLinks(ctx context.Context, tx *sql.Tx, citationID int64, links []LinkInput) error {
	for _, l := range links {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO citation_links (citation_id, entity_type, entity_id, field, status, status_reason) VALUES (?, ?, ?, ?, ?, ?)`,
			citationID, l.EntityType, l.EntityID, l.Field, l.Status, l.StatusReason); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) getCitation(ctx context.Context, q queryer, a Actor, id int64) (Citation, error) {
	cits, err := s.loadCitations(ctx, q, a, `c.id = ?`, id)
	if err != nil {
		return Citation{}, err
	}
	if len(cits) == 0 {
		return Citation{}, ErrNotFound
	}
	return cits[0], nil
}

func (s *Store) loadCitations(ctx context.Context, q queryer, a Actor, where string, args ...any) ([]Citation, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT c.id, c.source_id, c.page, c.quality, c.text, c.notes, c.created_at, c.updated_at
		FROM citations c WHERE c.tree_id = ? AND `+where+` ORDER BY c.page COLLATE NAMES, c.id`,
		append([]any{a.TreeID}, args...)...)
	if err != nil {
		return nil, err
	}
	cits := []Citation{}
	index := map[int64]int{}
	for rows.Next() {
		var c Citation
		var quality sql.NullInt64
		if err := rows.Scan(&c.ID, &c.SourceID, &c.Page, &quality, &c.Text, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if quality.Valid {
			qv := int(quality.Int64)
			c.Quality = &qv
		}
		c.Links = []CitationLink{}
		index[c.ID] = len(cits)
		cits = append(cits, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(cits) == 0 {
		return cits, err
	}

	ids := make([]int64, len(cits))
	for i, c := range cits {
		ids[i] = c.ID
	}
	// Labels name the target: the person, the event with its person (or
	// couple), or the family's couple.
	lrows, err := q.QueryContext(ctx, `
		SELECT l.citation_id, l.entity_type, l.entity_id, l.field, l.status, l.status_reason,
			CASE l.entity_type
				WHEN 'person' THEN l.entity_id
				WHEN 'name' THEN (SELECT person_id FROM person_names WHERE id = l.entity_id)
				WHEN 'event' THEN coalesce(
					(SELECT person_id FROM events WHERE id = l.entity_id),
					(SELECT coalesce(f.partner1_id, f.partner2_id) FROM events e JOIN families f ON f.id = e.family_id WHERE e.id = l.entity_id))
				WHEN 'family' THEN (SELECT coalesce(partner1_id, partner2_id) FROM families WHERE id = l.entity_id)
			END,
			CASE WHEN l.entity_type = 'event' THEN (SELECT type || char(31) || custom_label FROM events WHERE id = l.entity_id) END
		FROM citation_links l WHERE l.citation_id IN (`+placeholders(len(ids))+`)
		ORDER BY l.entity_type, l.entity_id`, int64Args(ids)...)
	if err != nil {
		return nil, err
	}
	type row struct {
		citationID int64
		link       CitationLink
		eventType  sql.NullString
	}
	var found []row
	var personIDs []int64
	for lrows.Next() {
		var r row
		var personID sql.NullInt64
		if err := lrows.Scan(&r.citationID, &r.link.EntityType, &r.link.EntityID, &r.link.Field, &r.link.Status,
			&r.link.StatusReason, &personID, &r.eventType); err != nil {
			lrows.Close()
			return nil, err
		}
		r.link.PersonID = ptrID(personID)
		if personID.Valid {
			personIDs = append(personIDs, personID.Int64)
		}
		found = append(found, r)
	}
	lrows.Close()
	if err := lrows.Err(); err != nil {
		return nil, err
	}
	refs, err := s.personRefs(ctx, q, a, personIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range found {
		name := ""
		if r.link.PersonID != nil {
			p := refs[*r.link.PersonID]
			name = strings.TrimSpace(p.GivenNames + " " + p.Surname)
		}
		// The label is data for the UI to compose ("BIRT|Anna Müller");
		// translations happen there.
		label := name
		if r.eventType.Valid {
			typ, custom, _ := strings.Cut(r.eventType.String, "\x1f")
			if custom != "" {
				typ = typ + ":" + custom
			}
			label = typ + "|" + name
		}
		r.link.Label = label
		c := &cits[index[r.citationID]]
		c.Links = append(c.Links, r.link)
	}
	return cits, nil
}

// citationRefs returns, per entity id, the citations supporting entities of
// one type, for showing next to the facts.
func citationRefs(ctx context.Context, q queryer, a Actor, entityType string, ids []int64) (map[int64][]CitationRef, error) {
	refs := map[int64][]CitationRef{}
	if len(ids) == 0 {
		return refs, nil
	}
	args := append([]any{entityType, a.TreeID}, int64Args(ids)...)
	rows, err := q.QueryContext(ctx, `
		SELECT l.entity_id, c.id, s.id, s.title, c.page, c.quality, l.field, l.status
		FROM citation_links l
		JOIN citations c ON c.id = l.citation_id
		JOIN sources s ON s.id = c.source_id
		WHERE l.entity_type = ? AND c.tree_id = ? AND l.entity_id IN (`+placeholders(len(ids))+`)
		ORDER BY c.quality IS NULL, c.quality DESC, s.title COLLATE NAMES, c.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entityID int64
		var r CitationRef
		var quality sql.NullInt64
		if err := rows.Scan(&entityID, &r.CitationID, &r.SourceID, &r.SourceTitle, &r.Page, &quality, &r.Field, &r.Status); err != nil {
			return nil, err
		}
		if quality.Valid {
			qv := int(quality.Int64)
			r.Quality = &qv
		}
		refs[entityID] = append(refs[entityID], r)
	}
	return refs, rows.Err()
}

// addCitations creates one citation per entry, each supporting the given
// entity.
func (s *Store) addCitations(ctx context.Context, tx *sql.Tx, a Actor, entityType string, entityID int64, cits []NewCitation) error {
	for _, c := range cits {
		_, err := s.createCitation(ctx, tx, a, CitationInput{
			SourceID: c.SourceID, Page: c.Page, Quality: c.Quality, Text: c.Text,
			Links: []LinkInput{{EntityType: entityType, EntityID: entityID}},
		})
		var ve *ValidationError
		if errors.As(err, &ve) {
			// Report the problem on the field the client sent.
			fields := map[string]string{}
			for k, v := range ve.Fields {
				fields["citations."+k] = v
			}
			return &ValidationError{Fields: fields}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Unlink removes one fact from a citation, and the citation itself when it
// supports nothing else.
func (s *Store) Unlink(ctx context.Context, a Actor, citationID int64, entityType string, entityID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		return s.unlink(ctx, tx, a, citationID, entityType, entityID)
	})
}

// removeCitations unlinks the given citations from one entity.
func (s *Store) removeCitations(ctx context.Context, tx *sql.Tx, a Actor, entityType string, entityID int64, citationIDs []int64) error {
	for _, id := range citationIDs {
		if err := s.unlink(ctx, tx, a, id, entityType, entityID); errors.Is(err, ErrNotFound) {
			return &ValidationError{Fields: map[string]string{"removeCitations": "a citation is not attached here"}}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) unlink(ctx context.Context, tx *sql.Tx, a Actor, citationID int64, entityType string, entityID int64) error {
	before, err := s.getCitation(ctx, tx, a, citationID)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM citation_links WHERE citation_id = ? AND entity_type = ? AND entity_id = ?`,
		citationID, entityType, entityID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	var left int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM citation_links WHERE citation_id = ?`, citationID).Scan(&left); err != nil {
		return err
	}
	if left == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM citations WHERE id = ?`, citationID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "citation", citationID, "delete", before, nil)
	}
	after, err := s.getCitation(ctx, tx, a, citationID)
	if err != nil {
		return err
	}
	return s.logChange(ctx, tx, a, "citation", citationID, "update", before, after)
}
