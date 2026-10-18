package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// ErrAdminOnly is returned when someone who is not an administrator manages accounts.
var ErrAdminOnly = errors.New("only administrators can do this")

// ErrLastAdmin keeps at least one administrator.
var ErrLastAdmin = errors.New("the last administrator cannot be removed or made a regular user")

// ErrDeleteSelf stops administrators from deleting their own account.
var ErrDeleteSelf = errors.New("you cannot delete your own account")

// ErrResetOwnPassword sends administrators to the password change, which asks for the current password.
var ErrResetOwnPassword = errors.New("change your own password on your account page")

// ErrLastOwner keeps at least one owner in every tree.
var ErrLastOwner = errors.New("a tree needs at least one owner")

// UserRoleUser is the role of accounts without administration rights.
const UserRoleUser = "user"

// AccountTree is one membership of an account, as administrators see it.
type AccountTree struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// Account is a user as administrators see it.
type Account struct {
	User
	CreatedAt  string        `json:"createdAt"`
	LastSeenAt string        `json:"lastSeenAt"` // "" when the account has no session
	Trees      []AccountTree `json:"trees"`      // never nil
}

// AccountInput creates an account.
type AccountInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	Role        string `json:"role"` // "admin" or "user"; "" means "user"
}

// AccountUpdate changes an account's name and instance role.
type AccountUpdate struct {
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

// ListAccounts lists every account with its tree memberships. Only
// administrators may call it.
func (s *Store) ListAccounts(ctx context.Context, by User) ([]Account, error) {
	if by.Role != UserRoleAdmin {
		return nil, ErrAdminOnly
	}
	return listAccounts(ctx, s.DB, 0)
}

// listAccounts reads one account (id != 0) or all of them.
func listAccounts(ctx context.Context, q queryer, id int64) ([]Account, error) {
	where, args := "", []any{}
	if id != 0 {
		where, args = " WHERE u.id = ?", []any{id}
	}
	rows, err := q.QueryContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.role, u.created_at,
		       coalesce((SELECT max(last_seen_at) FROM sessions WHERE user_id = u.id), '')
		FROM users u`+where+` ORDER BY u.username COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	byID := map[int64]int{}
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Username, &a.DisplayName, &a.Role, &a.CreatedAt, &a.LastSeenAt); err != nil {
			return nil, err
		}
		a.Trees = []AccountTree{}
		byID[a.ID] = len(out)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	mrows, err := q.QueryContext(ctx, `
		SELECT m.user_id, t.id, t.name, m.role FROM tree_members m JOIN trees t ON t.id = m.tree_id
		ORDER BY t.name COLLATE NOCASE, t.id`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var uid int64
		var t AccountTree
		if err := mrows.Scan(&uid, &t.ID, &t.Name, &t.Role); err != nil {
			return nil, err
		}
		if i, ok := byID[uid]; ok {
			out[i].Trees = append(out[i].Trees, t)
		}
	}
	return out, mrows.Err()
}

// account reads one account; ErrNotFound when there is none.
func account(ctx context.Context, q queryer, id int64) (Account, error) {
	list, err := listAccounts(ctx, q, id)
	if err != nil {
		return Account{}, err
	}
	if len(list) == 0 {
		return Account{}, ErrNotFound
	}
	return list[0], nil
}

// stillAdmin checks inside the write transaction that by is still an
// administrator: the session's role was read before the transaction, and
// another administrator may have demoted or deleted by since.
func stillAdmin(ctx context.Context, q queryer, by User) error {
	var role string
	err := q.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, by.ID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && role != UserRoleAdmin) {
		return ErrAdminOnly
	}
	return err
}

// otherAdmins counts the administrators besides user id.
func otherAdmins(ctx context.Context, q queryer, id int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND id <> ?`, id).Scan(&n)
	return n, err
}

// CreateAccount adds an account. Only administrators may call it.
func (s *Store) CreateAccount(ctx context.Context, by User, in AccountInput) (Account, error) {
	if by.Role != UserRoleAdmin {
		return Account{}, ErrAdminOnly
	}
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Role == "" {
		in.Role = UserRoleUser
	}
	var v validator
	v.check(validUsername(in.Username), "username", "use 2–64 letters, digits, dots, dashes or underscores")
	checkPassword(&v, "password", in.Password)
	v.check(utf8.RuneCountInString(in.DisplayName) <= 200, "displayName", "is too long")
	v.check(oneOf(in.Role, UserRoleAdmin, UserRoleUser), "role", "must be admin or user")
	if err := v.err(); err != nil {
		return Account{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), BcryptCost)
	if err != nil {
		return Account{}, err
	}
	var out Account
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if err := stillAdmin(ctx, tx, by); err != nil {
			return err
		}
		// The column is COLLATE NOCASE, so "Admin" and "admin" clash.
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM users WHERE username = ?`, in.Username).Scan(&one)
		if err == nil {
			return &ValidationError{Fields: map[string]string{"username": "is already taken"}}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO users (username, display_name, password_hash, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, in.Username, in.DisplayName, string(hash), in.Role, now, now)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		out, err = account(ctx, tx, id)
		return err
	})
	return out, err
}

// UpdateAccount changes an account's display name and instance role. Only
// administrators may call it; the last administrator cannot be demoted.
func (s *Store) UpdateAccount(ctx context.Context, by User, id int64, in AccountUpdate) (Account, error) {
	if by.Role != UserRoleAdmin {
		return Account{}, ErrAdminOnly
	}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	var v validator
	v.check(utf8.RuneCountInString(in.DisplayName) <= 200, "displayName", "is too long")
	v.check(oneOf(in.Role, UserRoleAdmin, UserRoleUser), "role", "must be admin or user")
	if err := v.err(); err != nil {
		return Account{}, err
	}
	var out Account
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := stillAdmin(ctx, tx, by); err != nil {
			return err
		}
		var role string
		err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&role)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if role == UserRoleAdmin && in.Role == UserRoleUser {
			n, err := otherAdmins(ctx, tx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrLastAdmin
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET display_name = ?, role = ?, updated_at = ? WHERE id = ?`,
			in.DisplayName, in.Role, s.now(), id); err != nil {
			return err
		}
		out, err = account(ctx, tx, id)
		return err
	})
	return out, err
}

// ResetPassword sets a new password for another account and signs it out
// everywhere. Only administrators may call it, and not for themselves: your
// own password changes with the current one (ChangePassword).
func (s *Store) ResetPassword(ctx context.Context, by User, id int64, password string) error {
	if by.Role != UserRoleAdmin {
		return ErrAdminOnly
	}
	if id == by.ID {
		return ErrResetOwnPassword
	}
	var v validator
	checkPassword(&v, "password", password)
	if err := v.err(); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := stillAdmin(ctx, tx, by); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, string(hash), s.now(), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id)
		return err
	})
}

// DeleteAccount removes an account. Only administrators may call it. It
// refuses to delete yourself, the last administrator, or a user who is the
// last owner of a tree: a tree is never left without an owner.
func (s *Store) DeleteAccount(ctx context.Context, by User, id int64) error {
	if by.Role != UserRoleAdmin {
		return ErrAdminOnly
	}
	if id == by.ID {
		return ErrDeleteSelf
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if err := stillAdmin(ctx, tx, by); err != nil {
			return err
		}
		var role string
		err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&role)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if role == UserRoleAdmin {
			n, err := otherAdmins(ctx, tx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrLastAdmin
			}
		}
		var sole int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM tree_members m
			WHERE m.user_id = ? AND m.role = 'owner'
			  AND NOT EXISTS (SELECT 1 FROM tree_members o
			                  WHERE o.tree_id = m.tree_id AND o.role = 'owner' AND o.user_id <> m.user_id)`, id).Scan(&sole); err != nil {
			return err
		}
		if sole > 0 {
			return ErrLastOwner
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
		return err
	})
}
