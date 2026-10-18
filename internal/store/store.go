// Package store holds the domain logic on top of SQLite: validation,
// tree scoping, change logging and the queries behind the API.
//
// Every method that touches genealogy data takes an Actor and only ever sees
// rows of the Actor's tree; an id from another tree behaves exactly like an
// id that does not exist.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Store is the entry point to all persisted data.
type Store struct {
	DB *sql.DB
	// Now is the clock; tests replace it.
	Now func() time.Time
}

// New returns a Store using the real clock.
func New(db *sql.DB) *Store {
	return &Store{DB: db, Now: time.Now}
}

// Actor is who performs an operation, and in which tree.
type Actor struct {
	UserID int64
	TreeID int64
	Role   string
}

// CanEdit reports whether the actor may change data in the tree.
func (a Actor) CanEdit() bool {
	return a.Role == RoleOwner || a.Role == RoleEditor
}

// Tree member roles.
const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
)

var (
	// ErrNotFound is returned for missing rows and for rows of another tree.
	ErrNotFound = errors.New("not found")
	// ErrViewOnly is returned when a viewer tries to change the tree.
	ErrViewOnly = errors.New("you can only view this tree")
	// ErrConflict is returned when an operation would break a reference,
	// such as deleting a place that events still use.
	ErrConflict = errors.New("conflict")
)

// ValidationError lists invalid input fields with a message each.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ": " + e.Fields[k]
	}
	return "invalid input: " + strings.Join(parts, "; ")
}

type validator struct {
	fields map[string]string
}

func (v *validator) add(field, msg string) {
	if v.fields == nil {
		v.fields = map[string]string{}
	}
	if _, exists := v.fields[field]; !exists {
		v.fields[field] = msg
	}
}

func (v *validator) check(ok bool, field, msg string) {
	if !ok {
		v.add(field, msg)
	}
}

func (v *validator) err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: v.fields}
}

func (s *Store) now() string {
	return s.Now().UTC().Format(time.RFC3339Nano)
}

func (s *Store) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// queryer is satisfied by *sql.DB and *sql.Tx, so read helpers work inside
// and outside transactions.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) logChange(ctx context.Context, q queryer, a Actor, entity string, id int64, action string, before, after any) error {
	enc := func(v any) (sql.NullString, error) {
		if v == nil {
			return sql.NullString{}, nil
		}
		b, err := json.Marshal(v)
		if err != nil {
			return sql.NullString{}, err
		}
		return sql.NullString{String: string(b), Valid: true}, nil
	}
	b, err := enc(before)
	if err != nil {
		return err
	}
	aj, err := enc(after)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO change_log (tree_id, user_id, entity_type, entity_id, action, before_json, after_json, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TreeID, nullID(a.UserID), entity, id, action, b, aj, s.now())
	return err
}

// requireInTree returns ErrNotFound-wrapping validation errors when id does
// not name a row of table in the actor's tree.
func requireInTree(ctx context.Context, q queryer, table string, id, treeID int64) error {
	var one int
	err := q.QueryRowContext(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE id = ? AND tree_id = ?`, table), id, treeID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nullID(id int64) sql.NullInt64 {
	return sql.NullInt64{Int64: id, Valid: id != 0}
}

func ptrID(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func idOrZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func nullIDPtr(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func placeholders(n int) string {
	if n == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// Fact statuses for names, events and citations.
const (
	StatusAccepted  = "accepted"
	StatusDisputed  = "disputed"
	StatusDisproven = "disproven"
)

func validStatus(v string) bool {
	return oneOf(v, StatusAccepted, StatusDisputed, StatusDisproven)
}

func defaultStatus(v string) string {
	if v == "" {
		return StatusAccepted
	}
	return v
}
