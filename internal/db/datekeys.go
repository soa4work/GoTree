package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/praetorianer777/gotree/internal/gendate"
)

const dateKeyVersionKey = "date_key_version"

// refreshDateKeys recomputes the derived date columns of events and
// heirlooms when the stored gendate.KeyVersion is missing or differs, so a
// change of how dates are read or sorted cannot leave stale keys behind.
// date_raw and made_date_raw stay as typed. The update and the new version
// are one transaction.
func refreshDateKeys(ctx context.Context, conn *sql.DB) error {
	want := gendate.KeyVersion
	var have string
	err := conn.QueryRowContext(ctx, `SELECT value FROM app_meta WHERE key = ?`, dateKeyVersionKey).Scan(&have)
	if err == nil && have == want {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("date key version: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := refreshEventDates(ctx, tx); err != nil {
		return fmt.Errorf("recompute event dates: %w", err)
	}
	if err := refreshHeirloomDates(ctx, tx); err != nil {
		return fmt.Errorf("recompute heirloom dates: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, dateKeyVersionKey, want); err != nil {
		return err
	}
	return tx.Commit()
}

// refreshEventDates computes what store.parseDate computes; keep in sync
// with store.parseDate. The db package cannot import store.
func refreshEventDates(ctx context.Context, tx *sql.Tx) error {
	type row struct {
		id             int64
		raw, date      string
		sort, sortEnd  sql.NullInt64
		qualifier      string
		newDate        string
		newSort, newTo sql.NullInt64
		newQualifier   string
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, date_raw, date, date_sort, date_sort_end, date_qualifier
		FROM events WHERE date_raw <> ''`)
	if err != nil {
		return err
	}
	var changed []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.raw, &r.date, &r.sort, &r.sortEnd, &r.qualifier); err != nil {
			rows.Close()
			return err
		}
		if d, err := gendate.Parse(r.raw); err == nil {
			r.newDate, r.newQualifier = d.String(), string(d.Qualifier)
			if k, ok := d.SortKey(); ok {
				r.newSort = sql.NullInt64{Int64: int64(k), Valid: true}
			}
			if k, ok := d.SortKeyEnd(); ok {
				r.newTo = sql.NullInt64{Int64: int64(k), Valid: true}
			}
		}
		if r.newDate != r.date || r.newSort != r.sort || r.newTo != r.sortEnd || r.newQualifier != r.qualifier {
			changed = append(changed, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	// updated_at stays: this repairs derived columns, it is not an edit.
	for _, r := range changed {
		if _, err := tx.ExecContext(ctx,
			`UPDATE events SET date = ?, date_sort = ?, date_sort_end = ?, date_qualifier = ? WHERE id = ?`,
			r.newDate, r.newSort, r.newTo, r.newQualifier, r.id); err != nil {
			return err
		}
	}
	return nil
}

// refreshHeirloomDates computes what store.madeSort computes; keep in sync
// with store.madeSort.
func refreshHeirloomDates(ctx context.Context, tx *sql.Tx) error {
	type row struct {
		id        int64
		sort, new sql.NullInt64
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, made_date_raw, made_date_sort FROM heirlooms WHERE made_date_raw <> ''`)
	if err != nil {
		return err
	}
	var changed []row
	for rows.Next() {
		var r row
		var raw string
		if err := rows.Scan(&r.id, &raw, &r.sort); err != nil {
			rows.Close()
			return err
		}
		if d, err := gendate.Parse(raw); err == nil {
			if k, ok := d.SortKey(); ok {
				r.new = sql.NullInt64{Int64: int64(k), Valid: true}
			}
		}
		if r.new != r.sort {
			changed = append(changed, r)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, r := range changed {
		if _, err := tx.ExecContext(ctx, `UPDATE heirlooms SET made_date_sort = ? WHERE id = ?`, r.new, r.id); err != nil {
			return err
		}
	}
	return nil
}
