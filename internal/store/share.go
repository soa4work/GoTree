package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Share link scopes and privacy modes.
const (
	ShareScopeTree        = "tree"
	ShareScopeDescendants = "descendants"
	// SharePrivacyDeceased shows deceased people only.
	SharePrivacyDeceased = "deceased"
	// SharePrivacyLivingNames also shows living people, by name only.
	SharePrivacyLivingNames = "living_names"
)

// ShareLink is a read-only link to (part of) a tree.
type ShareLink struct {
	ID         int64      `json:"id"`
	Label      string     `json:"label"`
	Scope      string     `json:"scope"`
	Root       *PersonRef `json:"root"`
	Privacy    string     `json:"privacy"`
	ExpiresAt  *string    `json:"expiresAt"`
	RevokedAt  *string    `json:"revokedAt"`
	LastUsedAt *string    `json:"lastUsedAt"`
	CreatedAt  string     `json:"createdAt"`
	// Token is only returned when the link is created; afterwards only its
	// hash is known.
	Token string `json:"token,omitempty"`
}

// ShareLinkInput creates a share link. ExpiresOn is a day (YYYY-MM-DD) the
// link works through, or empty for no expiry.
type ShareLinkInput struct {
	Label        string `json:"label"`
	Scope        string `json:"scope"`
	RootPersonID *int64 `json:"rootPersonId"`
	Privacy      string `json:"privacy"`
	ExpiresOn    string `json:"expiresOn"`
}

// ErrOwnerOnly is returned when someone other than the tree owner manages
// share links.
var ErrOwnerOnly = errors.New("only the owner of the tree can do this")

// CreateShareLink makes a new link. Only the owner may share the tree.
func (s *Store) CreateShareLink(ctx context.Context, a Actor, in ShareLinkInput) (ShareLink, error) {
	if a.Role != RoleOwner {
		return ShareLink{}, ErrOwnerOnly
	}
	in.Label = strings.TrimSpace(in.Label)
	var v validator
	v.check(in.Label != "" && utf8.RuneCountInString(in.Label) <= 120, "label", "give the link a name of at most 120 characters")
	v.check(oneOf(in.Scope, ShareScopeTree, ShareScopeDescendants), "scope", "must be tree or descendants")
	v.check(oneOf(in.Privacy, SharePrivacyDeceased, SharePrivacyLivingNames), "privacy", "must be deceased or living_names")
	if in.Scope == ShareScopeDescendants {
		if in.RootPersonID == nil {
			v.add("rootPersonId", "pick the person whose descendants to share")
		} else if err := requireInTree(ctx, s.DB, "persons", *in.RootPersonID, a.TreeID); errors.Is(err, ErrNotFound) {
			v.add("rootPersonId", "does not exist")
		} else if err != nil {
			return ShareLink{}, err
		}
	} else {
		in.RootPersonID = nil
	}
	var expires sql.NullString
	if in.ExpiresOn != "" {
		day, err := time.Parse(time.DateOnly, in.ExpiresOn)
		switch {
		case err != nil:
			v.add("expiresOn", "must be a date")
		case !day.After(s.Now().UTC().Add(-24 * time.Hour)):
			v.add("expiresOn", "must not be in the past")
		default:
			// Through the end of that day, UTC.
			expires = sql.NullString{String: day.Add(24 * time.Hour).Format(time.RFC3339Nano), Valid: true}
		}
	}
	if err := v.err(); err != nil {
		return ShareLink{}, err
	}

	token, err := newToken()
	if err != nil {
		return ShareLink{}, err
	}
	var link ShareLink
	err = s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO share_links (tree_id, token_hash, label, scope, root_person_id, privacy, expires_at, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.TreeID, hashToken(token), in.Label, in.Scope, nullIDPtr(in.RootPersonID), in.Privacy, expires, s.now(), nullID(a.UserID))
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		links, err := s.shareLinks(ctx, tx, a, `l.id = ?`, id)
		if err != nil {
			return err
		}
		link = links[0]
		return s.logChange(ctx, tx, a, "share_link", id, "create", nil, link)
	})
	link.Token = token
	return link, err
}

// ListShareLinks lists the tree's links, newest first, revoked ones too.
func (s *Store) ListShareLinks(ctx context.Context, a Actor) ([]ShareLink, error) {
	if a.Role != RoleOwner {
		return nil, ErrOwnerOnly
	}
	return s.shareLinks(ctx, s.DB, a, `1 = 1`)
}

// RevokeShareLink stops a link from working. The row stays, so the list
// still shows who had access until when.
func (s *Store) RevokeShareLink(ctx context.Context, a Actor, id int64) error {
	if a.Role != RoleOwner {
		return ErrOwnerOnly
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.shareLinks(ctx, tx, a, `l.id = ?`, id)
		if err != nil {
			return err
		}
		if len(before) == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE share_links SET revoked_at = coalesce(revoked_at, ?) WHERE id = ? AND tree_id = ?`,
			s.now(), id, a.TreeID); err != nil {
			return err
		}
		after, err := s.shareLinks(ctx, tx, a, `l.id = ?`, id)
		if err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "share_link", id, "update", before[0], after[0])
	})
}

// RotateShareLink gives an active link a new token; the old address stops
// working. The token is returned once, like from CreateShareLink. Revoked
// links cannot be revived; expired ones get a new address but stay expired.
func (s *Store) RotateShareLink(ctx context.Context, a Actor, id int64) (ShareLink, error) {
	if a.Role != RoleOwner {
		return ShareLink{}, ErrOwnerOnly
	}
	token, err := newToken()
	if err != nil {
		return ShareLink{}, err
	}
	var link ShareLink
	err = s.tx(ctx, func(tx *sql.Tx) error {
		before, err := s.shareLinks(ctx, tx, a, `l.id = ? AND l.revoked_at IS NULL`, id)
		if err != nil {
			return err
		}
		if len(before) == 0 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `UPDATE share_links SET token_hash = ? WHERE id = ? AND tree_id = ?`,
			hashToken(token), id, a.TreeID); err != nil {
			return err
		}
		after, err := s.shareLinks(ctx, tx, a, `l.id = ?`, id)
		if err != nil {
			return err
		}
		link = after[0]
		return s.logChange(ctx, tx, a, "share_link", id, "update", before[0], after[0])
	})
	if err != nil {
		return ShareLink{}, err
	}
	link.Token = token
	return link, nil
}

func (s *Store) shareLinks(ctx context.Context, q queryer, a Actor, where string, args ...any) ([]ShareLink, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT l.id, l.label, l.scope, l.root_person_id, l.privacy, l.expires_at, l.revoked_at, l.last_used_at, l.created_at
		FROM share_links l WHERE l.tree_id = ? AND `+where+` ORDER BY l.created_at DESC, l.id DESC`,
		append([]any{a.TreeID}, args...)...)
	if err != nil {
		return nil, err
	}
	links := []ShareLink{}
	var roots []int64
	for rows.Next() {
		var l ShareLink
		var root sql.NullInt64
		var expires, revoked, used sql.NullString
		if err := rows.Scan(&l.ID, &l.Label, &l.Scope, &root, &l.Privacy, &expires, &revoked, &used, &l.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		l.ExpiresAt, l.RevokedAt, l.LastUsedAt = strPtr(expires), strPtr(revoked), strPtr(used)
		if root.Valid {
			l.Root = &PersonRef{ID: root.Int64}
			roots = append(roots, root.Int64)
		}
		links = append(links, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	refs, err := s.personRefs(ctx, q, a, roots)
	if err != nil {
		return nil, err
	}
	for i := range links {
		if links[i].Root != nil {
			r := refs[links[i].Root.ID]
			links[i].Root = &r
		}
	}
	return links, nil
}

func strPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

// Share is a resolved share link: what a visitor may see.
type Share struct {
	Link     ShareLink
	TreeName string
	// Actor reads the tree as a viewer without a user.
	Actor Actor
}

// ShareByToken resolves a link token. Unknown, revoked and expired links
// give ErrNotFound alike. Use is recorded at most once a minute.
func (s *Store) ShareByToken(ctx context.Context, token string) (Share, error) {
	if token == "" {
		return Share{}, ErrNotFound
	}
	now := s.Now().UTC()
	var sh Share
	var id int64
	var expires sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT l.id, l.tree_id, t.name, l.expires_at FROM share_links l JOIN trees t ON t.id = l.tree_id
		WHERE l.token_hash = ? AND l.revoked_at IS NULL`, hashToken(token)).Scan(&id, &sh.Actor.TreeID, &sh.TreeName, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Share{}, ErrNotFound
	}
	if err != nil {
		return Share{}, err
	}
	if expires.Valid && expires.String <= now.Format(time.RFC3339Nano) {
		return Share{}, ErrNotFound
	}
	sh.Actor.Role = RoleViewer
	links, err := s.shareLinks(ctx, s.DB, sh.Actor, `l.id = ?`, id)
	if err != nil {
		return Share{}, err
	}
	sh.Link = links[0]
	_, err = s.DB.ExecContext(ctx, `UPDATE share_links SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR last_used_at < ?)`,
		now.Format(time.RFC3339Nano), id, now.Add(-time.Minute).Format(time.RFC3339Nano))
	return sh, err
}
