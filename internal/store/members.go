package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"
)

// TreeSummary is a tree the user belongs to.
type TreeSummary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Current bool   `json:"current"`
}

// Member is a user's membership in a tree.
type Member struct {
	UserID      int64  `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

// MemberInput adds an existing account to a tree.
type MemberInput struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// treeRole returns the user's role in a tree. A user who is not a member
// gets ErrNotFound, which hides whether the tree exists.
func treeRole(ctx context.Context, q queryer, treeID, userID int64) (string, error) {
	var role string
	err := q.QueryRowContext(ctx, `SELECT role FROM tree_members WHERE tree_id = ? AND user_id = ?`, treeID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return role, err
}

// requireOwner checks that the user owns the tree: ErrNotFound for a
// non-member, ErrOwnerOnly for editors and viewers.
func requireOwner(ctx context.Context, q queryer, treeID, userID int64) (Actor, error) {
	role, err := treeRole(ctx, q, treeID, userID)
	if err != nil {
		return Actor{}, err
	}
	if role != RoleOwner {
		return Actor{}, ErrOwnerOnly
	}
	return Actor{UserID: userID, TreeID: treeID, Role: role}, nil
}

func ownerCount(ctx context.Context, q queryer, treeID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM tree_members WHERE tree_id = ? AND role = 'owner'`, treeID).Scan(&n)
	return n, err
}

func validTreeName(name string) error {
	var v validator
	v.check(name != "", "name", "is required")
	v.check(utf8.RuneCountInString(name) <= 200, "name", "is too long")
	return v.err()
}

// ListTrees returns the trees the user belongs to; currentTreeID marks the
// one their session is in.
func (s *Store) ListTrees(ctx context.Context, userID, currentTreeID int64) ([]TreeSummary, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT t.id, t.name, m.role FROM tree_members m JOIN trees t ON t.id = m.tree_id
		WHERE m.user_id = ? ORDER BY t.name COLLATE NOCASE, t.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TreeSummary{}
	for rows.Next() {
		var t TreeSummary
		if err := rows.Scan(&t.ID, &t.Name, &t.Role); err != nil {
			return nil, err
		}
		t.Current = t.ID == currentTreeID
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateTree makes a new tree with the user as its only owner. It does not
// switch the session; see SwitchTree.
func (s *Store) CreateTree(ctx context.Context, userID int64, name string) (TreeSummary, error) {
	name = strings.TrimSpace(name)
	if err := validTreeName(name); err != nil {
		return TreeSummary{}, err
	}
	var out TreeSummary
	err := s.tx(ctx, func(tx *sql.Tx) error {
		now := s.now()
		res, err := tx.ExecContext(ctx, `INSERT INTO trees (name, created_at, updated_at) VALUES (?, ?, ?)`, name, now, now)
		if err != nil {
			return err
		}
		tid, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx, `INSERT INTO tree_members (tree_id, user_id, role) VALUES (?, ?, 'owner')`, tid, userID); err != nil {
			return err
		}
		out = TreeSummary{ID: tid, Name: name, Role: RoleOwner}
		return s.logChange(ctx, tx, Actor{UserID: userID, TreeID: tid, Role: RoleOwner}, "tree", tid, "create", nil, out)
	})
	return out, err
}

// RenameTree renames a tree. Only its owners may.
func (s *Store) RenameTree(ctx context.Context, userID, treeID int64, name string) (TreeSummary, error) {
	name = strings.TrimSpace(name)
	if err := validTreeName(name); err != nil {
		return TreeSummary{}, err
	}
	var out TreeSummary
	err := s.tx(ctx, func(tx *sql.Tx) error {
		a, err := requireOwner(ctx, tx, treeID, userID)
		if err != nil {
			return err
		}
		var before string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM trees WHERE id = ?`, treeID).Scan(&before); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE trees SET name = ?, updated_at = ? WHERE id = ?`, name, s.now(), treeID); err != nil {
			return err
		}
		out = TreeSummary{ID: treeID, Name: name, Role: RoleOwner}
		return s.logChange(ctx, tx, a, "tree", treeID, "update",
			TreeSummary{ID: treeID, Name: before, Role: RoleOwner}, out)
	})
	return out, err
}

// SwitchTree moves the session with this token to another tree of the user
// and remembers the choice for their next login.
func (s *Store) SwitchTree(ctx context.Context, token string, userID, treeID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := treeRole(ctx, tx, treeID, userID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE sessions SET tree_id = ? WHERE token_hash = ? AND user_id = ?`,
			treeID, hashToken(token), userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `UPDATE users SET last_tree_id = ? WHERE id = ?`, treeID, userID)
		return err
	})
}

func getMember(ctx context.Context, q queryer, treeID, userID int64) (Member, error) {
	var m Member
	err := q.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.display_name, m.role FROM tree_members m JOIN users u ON u.id = m.user_id
		WHERE m.tree_id = ? AND m.user_id = ?`, treeID, userID).Scan(&m.UserID, &m.Username, &m.DisplayName, &m.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	return m, err
}

// ListMembers lists the members of a tree. Only its owners may.
func (s *Store) ListMembers(ctx context.Context, userID, treeID int64) ([]Member, error) {
	if _, err := requireOwner(ctx, s.DB, treeID, userID); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT u.id, u.username, u.display_name, m.role FROM tree_members m JOIN users u ON u.id = m.user_id
		WHERE m.tree_id = ?
		ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'editor' THEN 1 ELSE 2 END, u.username COLLATE NOCASE`, treeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddMember adds an existing account to the tree by username. Only owners may.
func (s *Store) AddMember(ctx context.Context, userID, treeID int64, in MemberInput) (Member, error) {
	in.Username = strings.TrimSpace(in.Username)
	var v validator
	v.check(in.Username != "", "username", "is required")
	v.check(oneOf(in.Role, RoleOwner, RoleEditor, RoleViewer), "role", "must be owner, editor or viewer")
	if err := v.err(); err != nil {
		return Member{}, err
	}
	var out Member
	err := s.tx(ctx, func(tx *sql.Tx) error {
		a, err := requireOwner(ctx, tx, treeID, userID)
		if err != nil {
			return err
		}
		var target int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, in.Username).Scan(&target)
		if errors.Is(err, sql.ErrNoRows) {
			return &ValidationError{Fields: map[string]string{"username": "cannot be added"}}
		}
		if err != nil {
			return err
		}
		if _, err := treeRole(ctx, tx, treeID, target); err == nil {
			return &ValidationError{Fields: map[string]string{"username": "cannot be added"}}
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tree_members (tree_id, user_id, role) VALUES (?, ?, ?)`, treeID, target, in.Role); err != nil {
			return err
		}
		out, err = getMember(ctx, tx, treeID, target)
		if err != nil {
			return err
		}
		return s.logChange(ctx, tx, a, "member", target, "create", nil, out)
	})
	return out, err
}

// SetMemberRole changes a member's role. Only owners may; the last owner
// cannot be demoted.
func (s *Store) SetMemberRole(ctx context.Context, userID, treeID, memberID int64, role string) (Member, error) {
	var v validator
	v.check(oneOf(role, RoleOwner, RoleEditor, RoleViewer), "role", "must be owner, editor or viewer")
	if err := v.err(); err != nil {
		return Member{}, err
	}
	var out Member
	err := s.tx(ctx, func(tx *sql.Tx) error {
		a, err := requireOwner(ctx, tx, treeID, userID)
		if err != nil {
			return err
		}
		before, err := getMember(ctx, tx, treeID, memberID)
		if err != nil {
			return err
		}
		if before.Role == role {
			out = before
			return nil
		}
		if before.Role == RoleOwner {
			n, err := ownerCount(ctx, tx, treeID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return ErrLastOwner
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tree_members SET role = ? WHERE tree_id = ? AND user_id = ?`, role, treeID, memberID); err != nil {
			return err
		}
		out = before
		out.Role = role
		return s.logChange(ctx, tx, a, "member", memberID, "update", before, out)
	})
	return out, err
}

// RemoveMember removes a member from a tree. Owners may remove anyone; any
// member may remove themselves (leave). The last owner stays. Sessions of
// the removed user in this tree are left without a tree.
func (s *Store) RemoveMember(ctx context.Context, userID, treeID, memberID int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		own, err := treeRole(ctx, tx, treeID, userID)
		if err != nil {
			return err
		}
		if memberID != userID && own != RoleOwner {
			return ErrOwnerOnly
		}
		before, err := getMember(ctx, tx, treeID, memberID)
		if err != nil {
			return err
		}
		if before.Role == RoleOwner {
			n, err := ownerCount(ctx, tx, treeID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return ErrLastOwner
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tree_members WHERE tree_id = ? AND user_id = ?`, treeID, memberID); err != nil {
			return err
		}
		// Not moved to another tree: a form still open for this one could
		// otherwise write into the wrong tree.
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET tree_id = NULL WHERE user_id = ? AND tree_id = ?`, memberID, treeID); err != nil {
			return err
		}
		return s.logChange(ctx, tx, Actor{UserID: userID, TreeID: treeID, Role: own}, "member", memberID, "delete", before, nil)
	})
}
