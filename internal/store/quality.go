package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/praetorianer777/gotree/internal/check"
	"github.com/praetorianer777/gotree/internal/gendate"
	"github.com/praetorianer777/gotree/internal/graph"
)

// CheckReport lists the consistency findings with the people they name.
type CheckReport struct {
	Findings []FindingView       `json:"findings"`
	Persons  map[int64]PersonRef `json:"persons"`
	// TaskIDs maps the origin of findings that became research tasks to
	// the task.
	TaskIDs map[string]int64 `json:"taskIds"`
}

// FindingView is a finding with the origin a research task made from it
// carries.
type FindingView struct {
	check.Finding
	Origin string `json:"origin"`
}

func findingOrigin(f check.Finding) string {
	return fmt.Sprintf("check:%s:%d:%d:%d", f.Rule, f.PersonID, f.EventID, f.OtherPersonID)
}

// bloodRelations are child relations that count as descent.
var bloodRelations = map[string]bool{"birth": true, "unknown": true}

type familyRow struct {
	id, p1, p2 int64
	children   []childRow
}

type childRow struct {
	id         int64
	rel1, rel2 string
}

func (s *Store) familyLinks(ctx context.Context, a Actor) ([]familyRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT f.id, coalesce(f.partner1_id, 0), coalesce(f.partner2_id, 0), c.child_id, c.relation_partner1, c.relation_partner2
		FROM families f LEFT JOIN family_children c ON c.family_id = f.id
		WHERE f.tree_id = ? ORDER BY f.id, c.sort_order, c.child_id`, a.TreeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []familyRow
	for rows.Next() {
		var f familyRow
		var child sql.NullInt64
		var rel1, rel2 sql.NullString
		if err := rows.Scan(&f.id, &f.p1, &f.p2, &child, &rel1, &rel2); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].id != f.id {
			out = append(out, f)
		}
		if child.Valid {
			last := &out[len(out)-1]
			last.children = append(last.children, childRow{child.Int64, rel1.String, rel2.String})
		}
	}
	return out, rows.Err()
}

// Checks runs the consistency rules over the tree. With personID set, only
// findings naming that person are returned.
func (s *Store) Checks(ctx context.Context, a Actor, personID int64) (CheckReport, error) {
	if personID != 0 {
		if err := requireInTree(ctx, s.DB, "persons", personID, a.TreeID); err != nil {
			return CheckReport{}, err
		}
	}
	var d check.Data
	rows, err := s.DB.QueryContext(ctx, `SELECT id, sex, is_living FROM persons WHERE tree_id = ?`, a.TreeID)
	if err != nil {
		return CheckReport{}, err
	}
	for rows.Next() {
		var p check.Person
		var living sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Sex, &living); err != nil {
			rows.Close()
			return CheckReport{}, err
		}
		if living.Valid {
			v := living.Int64 == 1
			p.Living = &v
		}
		d.Persons = append(d.Persons, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return CheckReport{}, err
	}

	rows, err = s.DB.QueryContext(ctx, `
		SELECT id, coalesce(person_id, 0), coalesce(family_id, 0), type, date_raw, date, date_sort, date_sort_end, date_qualifier
		FROM events WHERE tree_id = ? AND status <> 'disproven'`, a.TreeID)
	if err != nil {
		return CheckReport{}, err
	}
	for rows.Next() {
		var e check.Event
		var raw, norm, qual string
		var start, end sql.NullInt64
		if err := rows.Scan(&e.ID, &e.PersonID, &e.FamilyID, &e.Type, &raw, &norm, &start, &end, &qual); err != nil {
			rows.Close()
			return CheckReport{}, err
		}
		e.Invalid = raw != "" && norm == ""
		if start.Valid && end.Valid {
			r := check.FromSortKeys(int(start.Int64), int(end.Int64), qual)
			e.Date = &r
		}
		d.Events = append(d.Events, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return CheckReport{}, err
	}

	fams, err := s.familyLinks(ctx, a)
	if err != nil {
		return CheckReport{}, err
	}
	for _, f := range fams {
		cf := check.Family{ID: f.id, Partner1: f.p1, Partner2: f.p2}
		for _, c := range f.children {
			cf.Children = append(cf.Children, check.Child{PersonID: c.id, Birth1: bloodRelations[c.rel1], Birth2: bloodRelations[c.rel2]})
		}
		d.Families = append(d.Families, cf)
	}

	report := CheckReport{Findings: []FindingView{}}
	var ids []int64
	for _, f := range check.Run(d, s.Now()) {
		if personID != 0 && f.PersonID != personID && f.OtherPersonID != personID {
			continue
		}
		report.Findings = append(report.Findings, FindingView{Finding: f, Origin: findingOrigin(f)})
		ids = append(ids, f.PersonID)
		if f.OtherPersonID != 0 {
			ids = append(ids, f.OtherPersonID)
		}
	}
	if report.TaskIDs, err = s.taskOrigins(ctx, a, "check:"); err != nil {
		return CheckReport{}, err
	}
	report.Persons, err = s.personRefsChunked(ctx, a, ids)
	return report, err
}

// personRefsChunked stays below SQLite's limit on bound parameters.
func (s *Store) personRefsChunked(ctx context.Context, a Actor, ids []int64) (map[int64]PersonRef, error) {
	slices.Sort(ids)
	ids = slices.Compact(ids)
	out := map[int64]PersonRef{}
	for chunk := range slices.Chunk(ids, 500) {
		refs, err := s.personRefs(ctx, s.DB, a, chunk)
		if err != nil {
			return nil, err
		}
		for id, r := range refs {
			out[id] = r
		}
	}
	return out, nil
}

// DateProposal suggests a GEDCOM form for an event date.
type DateProposal struct {
	EventID     int64  `json:"eventId"`
	Type        string `json:"type"`
	CustomLabel string `json:"customLabel"`
	PersonID    *int64 `json:"personId"`
	FamilyID    *int64 `json:"familyId"`
	// PersonIDs are the people to name for the event: the person, or the
	// partners of the family.
	PersonIDs []int64 `json:"personIds"`
	Raw       string  `json:"raw"`
	Proposed  string  `json:"proposed"`
	// WasValid is false for every proposal now: dates Parse reads are not
	// offered. It stays so older clients keep reading the response.
	WasValid bool `json:"wasValid"`
}

// DateProposals is the preview of bulk date normalization.
type DateProposals struct {
	Items []DateProposal `json:"items"`
	// Unreadable counts dates that cannot be read even leniently.
	Unreadable int                 `json:"unreadable"`
	Persons    map[int64]PersonRef `json:"persons"`
}

// ProposeDates lists the event dates that gendate.Parse cannot read but
// gendate.Normalize can (older or imported text such as "March 12, 1850"),
// with their canonical GEDCOM form. Dates Parse reads are stored, sorted and
// exported correctly as typed, so they are not offered.
func (s *Store) ProposeDates(ctx context.Context, a Actor) (DateProposals, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.id, e.type, e.custom_label, e.person_id, e.family_id, coalesce(f.partner1_id, 0), coalesce(f.partner2_id, 0),
			e.date_raw, e.date
		FROM events e LEFT JOIN families f ON f.id = e.family_id
		WHERE e.tree_id = ? AND e.date_raw <> '' AND e.date_raw <> e.date
		ORDER BY e.date_sort IS NULL, e.date_sort, e.id`, a.TreeID)
	if err != nil {
		return DateProposals{}, err
	}
	defer rows.Close()
	out := DateProposals{Items: []DateProposal{}}
	var ids []int64
	for rows.Next() {
		var p DateProposal
		var person, family sql.NullInt64
		var p1, p2 int64
		var norm string
		if err := rows.Scan(&p.EventID, &p.Type, &p.CustomLabel, &person, &family, &p1, &p2, &p.Raw, &norm); err != nil {
			return DateProposals{}, err
		}
		d, err := gendate.Normalize(p.Raw)
		if err != nil {
			out.Unreadable++
			continue
		}
		if norm != "" || d.String() == p.Raw {
			continue
		}
		p.Proposed = d.String()
		p.PersonID, p.FamilyID = ptrID(person), ptrID(family)
		if person.Valid {
			p.PersonIDs = []int64{person.Int64}
		} else {
			for _, id := range []int64{p1, p2} {
				if id != 0 {
					p.PersonIDs = append(p.PersonIDs, id)
				}
			}
		}
		if p.PersonIDs == nil {
			p.PersonIDs = []int64{}
		}
		ids = append(ids, p.PersonIDs...)
		out.Items = append(out.Items, p)
	}
	if err := rows.Err(); err != nil {
		return DateProposals{}, err
	}
	rows.Close()
	out.Persons, err = s.personRefsChunked(ctx, a, ids)
	return out, err
}

// DateChange rewrites one event date. Raw is the text the change was
// proposed for; the change is skipped if the date was edited since.
type DateChange struct {
	EventID int64  `json:"eventId"`
	Raw     string `json:"raw"`
	Date    string `json:"date"`
}

// DateChangeResult counts what ApplyDates did.
type DateChangeResult struct {
	Updated int `json:"updated"`
	// Skipped are changes whose event was edited or deleted meanwhile.
	Skipped int `json:"skipped"`
}

// ApplyDates writes the chosen date proposals in one transaction, each
// recorded in the change log.
func (s *Store) ApplyDates(ctx context.Context, a Actor, changes []DateChange) (DateChangeResult, error) {
	var res DateChangeResult
	for i, c := range changes {
		d, err := gendate.Parse(c.Date)
		if err != nil || d.String() != c.Date {
			return res, &ValidationError{Fields: map[string]string{"changes": fmt.Sprintf("item %d is not a canonical GEDCOM date", i+1)}}
		}
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		update, err := tx.PrepareContext(ctx, `
			UPDATE events SET date_raw = ?, date = ?, date_sort = ?, date_sort_end = ?, date_qualifier = ?,
				updated_at = ?, updated_by = ?
			WHERE id = ? AND tree_id = ? AND date_raw = ?`)
		if err != nil {
			return err
		}
		defer update.Close()
		now := s.now()
		for _, c := range changes {
			date, key, keyEnd, qual := parseDate(c.Date)
			r, err := update.ExecContext(ctx, c.Date, date, key, keyEnd, qual, now, nullID(a.UserID), c.EventID, a.TreeID, c.Raw)
			if err != nil {
				return err
			}
			if n, _ := r.RowsAffected(); n == 0 {
				res.Skipped++
				continue
			}
			res.Updated++
			if err := s.logChange(ctx, tx, a, "event", c.EventID, "update",
				map[string]string{"date": c.Raw}, map[string]string{"date": c.Date}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return DateChangeResult{}, err
	}
	return res, nil
}

// RelationshipReport says what B is to A, with the people on the way.
type RelationshipReport struct {
	A int64 `json:"a"`
	B int64 `json:"b"`
	graph.Relationship
	Persons map[int64]PersonRef `json:"persons"`
}

// Relationship finds how b is related to a.
func (s *Store) Relationship(ctx context.Context, a Actor, aID, bID int64) (RelationshipReport, error) {
	for _, id := range []int64{aID, bID} {
		if err := requireInTree(ctx, s.DB, "persons", id, a.TreeID); err != nil {
			return RelationshipReport{}, err
		}
	}
	g, err := s.familyGraph(ctx, a)
	if err != nil {
		return RelationshipReport{}, err
	}
	rel := g.Relate(aID, bID)
	ids := []int64{aID, bID}
	if rel.Via != 0 {
		ids = append(ids, rel.Via)
	}
	for _, k := range append([]graph.Kinship{deref(rel.Kinship)}, rel.Others...) {
		ids = append(ids, k.Path...)
		ids = append(ids, k.Ancestors...)
	}
	persons, err := s.personRefsChunked(ctx, a, ids)
	return RelationshipReport{A: aID, B: bID, Relationship: rel, Persons: persons}, err
}

func deref(k *graph.Kinship) graph.Kinship {
	if k == nil {
		return graph.Kinship{}
	}
	return *k
}

// familyGraph loads the tree's links. It is rebuilt per request: two
// indexed scans take a few milliseconds even for large trees, and nothing
// can go stale.
func (s *Store) familyGraph(ctx context.Context, a Actor) (*graph.Graph, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM persons WHERE tree_id = ?`, a.TreeID)
	if err != nil {
		return nil, err
	}
	var persons []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		persons = append(persons, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	fams, err := s.familyLinks(ctx, a)
	if err != nil {
		return nil, err
	}
	gf := make([]graph.Family, len(fams))
	for i, f := range fams {
		gf[i] = graph.Family{ID: f.id, Partner1: f.p1, Partner2: f.p2}
		for _, c := range f.children {
			gf[i].Children = append(gf[i].Children, graph.Child{PersonID: c.id, Blood1: bloodRelations[c.rel1], Blood2: bloodRelations[c.rel2]})
		}
	}
	return graph.New(persons, gf), nil
}
