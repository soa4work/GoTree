package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// savedShareLink is a link to a branch, read before "Replace the tree"
// deletes its root person (share_links.root_person_id cascades).
type savedShareLink struct {
	id                     int64
	tokenHash              []byte
	label, scope, privacy  string
	createdAt              string
	expiresAt, revokedAt   sql.NullString
	lastUsedAt             sql.NullString
	createdBy              sql.NullInt64
	rootID                 int64
	rootXref               string
	rootGiven, rootSurname string
}

// working reports whether the link opens today: not revoked, not expired
// (the rule of ShareByToken).
func (l savedShareLink) working(now time.Time) bool {
	return !l.revokedAt.Valid && (!l.expiresAt.Valid || l.expiresAt.String > now.UTC().Format(time.RFC3339Nano))
}

func saveShareLinks(ctx context.Context, tx *sql.Tx, treeID int64) ([]savedShareLink, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT l.id, l.token_hash, l.label, l.scope, l.privacy, l.expires_at, l.revoked_at, l.last_used_at,
			l.created_at, l.created_by, p.id, p.gedcom_xref, p.given_names, p.surname
		FROM share_links l JOIN persons p ON p.id = l.root_person_id
		WHERE l.tree_id = ? AND l.scope = 'descendants' ORDER BY l.id`, treeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []savedShareLink
	for rows.Next() {
		var l savedShareLink
		if err := rows.Scan(&l.id, &l.tokenHash, &l.label, &l.scope, &l.privacy, &l.expiresAt, &l.revokedAt, &l.lastUsedAt,
			&l.createdAt, &l.createdBy, &l.rootID, &l.rootXref, &l.rootGiven, &l.rootSurname); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func sameName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// restoreShareLinks recreates the saved links whose person is in the newly
// imported tree, with the same id and token, so the URL keeps working. A
// person is the same when the file has their GEDCOM ID (the old one, or
// I<old id> when the file is an export of this tree) and the same given
// names and surname: the name check keeps a reused ID in an unrelated file
// from opening somebody else's branch. Links that do not work any more are
// restored or dropped without being reported.
func restoreShareLinks(ctx context.Context, tx *sql.Tx, treeID int64, saved []savedShareLink, now time.Time, report *ImportReport) error {
	for _, l := range saved {
		candidates := []string{}
		if l.rootXref != "" {
			candidates = append(candidates, l.rootXref)
		}
		if x := personXref(l.rootID); x != l.rootXref {
			candidates = append(candidates, x)
		}
		var newRoot int64
		for _, xref := range candidates {
			var id int64
			var given, surname string
			err := tx.QueryRowContext(ctx, `
				SELECT id, given_names, surname FROM persons WHERE tree_id = ? AND gedcom_xref = ? ORDER BY id LIMIT 1`,
				treeID, xref).Scan(&id, &given, &surname)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			if sameName(given, l.rootGiven) && sameName(surname, l.rootSurname) {
				newRoot = id
				break
			}
		}
		if newRoot == 0 {
			if l.working(now) {
				report.ShareLinksLost = append(report.ShareLinksLost, l.label)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO share_links (id, tree_id, token_hash, label, scope, root_person_id, privacy, expires_at, revoked_at, last_used_at, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			l.id, treeID, l.tokenHash, l.label, l.scope, newRoot, l.privacy, l.expiresAt, l.revokedAt, l.lastUsedAt, l.createdAt, l.createdBy); err != nil {
			return err
		}
		if l.working(now) {
			report.ShareLinksKept++
		}
	}
	return nil
}

// clearTranscriptionPeople forgets the people in transcription rows. After
// "Replace the tree" their ids name nobody. A draft row keeps its action, so
// saving it asks for the person again instead of quietly creating a
// duplicate. Applied transcriptions are read-only history; clearing stops
// them from naming people of the new tree. Sources, places and events
// already become NULL through ON DELETE SET NULL.
func clearTranscriptionPeople(ctx context.Context, tx *sql.Tx, treeID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, rows_json FROM transcriptions WHERE tree_id = ?`, treeID)
	if err != nil {
		return err
	}
	type update struct {
		id   int64
		json string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var list []TranscriptionRow
		if err := json.Unmarshal([]byte(raw), &list); err != nil {
			continue // not ours to repair
		}
		changed := false
		for i := range list {
			if list[i].PersonID != nil {
				list[i].PersonID = nil
				changed = true
			}
		}
		if !changed {
			continue
		}
		out, err := json.Marshal(orEmptyRows(list))
		if err != nil {
			rows.Close()
			return err
		}
		updates = append(updates, update{id, string(out)})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// updated_at stays as it was, so the list of drafts keeps its order.
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE transcriptions SET rows_json = ? WHERE id = ?`, u.json, u.id); err != nil {
			return err
		}
	}
	return nil
}
