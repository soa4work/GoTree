package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// User is an account.
type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

// UserRoleAdmin is the role of users who manage the whole instance, such
// as downloading backups.
const UserRoleAdmin = "admin"

// Session is an authenticated browser session.
type Session struct {
	// ID names the session without revealing its token.
	ID int64
	// CookieMaxAge is set when this use extended the session: the cookie
	// should be sent again with this lifetime, which never reaches past
	// SessionMaxAge after login.
	CookieMaxAge time.Duration
	User         User
	TreeID       int64
	// TreeRole is the user's role in TreeID.
	TreeRole string
}

// Actor returns the actor for operations performed in this session.
func (s Session) Actor() Actor {
	return Actor{UserID: s.User.ID, TreeID: s.TreeID, Role: s.TreeRole}
}

// SessionTTL is how long a session stays valid without being used.
const SessionTTL = 30 * 24 * time.Hour

// SessionMaxAge is how long a session lasts at most after logging in,
// however often it is used, so a stolen cookie does not work forever.
const SessionMaxAge = 90 * 24 * time.Hour

// MinPasswordLength is the shortest accepted password.
const MinPasswordLength = 10

// ErrSetupDone is returned by Setup once any user exists.
var ErrSetupDone = errors.New("setup has already been completed")

// ErrBadCredentials is returned for an unknown user or a wrong password.
var ErrBadCredentials = errors.New("wrong username or password")

// SetupInput creates the first administrator and their tree.
type SetupInput struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
	TreeName    string `json:"treeName"`
}

// NeedsSetup reports whether no account exists yet.
func (s *Store) NeedsSetup(ctx context.Context) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n == 0, err
}

// Setup creates the first admin and the first tree. It fails with
// ErrSetupDone once any user exists, so it cannot be used to add accounts.
func (s *Store) Setup(ctx context.Context, in SetupInput) (User, error) {
	in.Username = strings.TrimSpace(in.Username)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.TreeName = strings.TrimSpace(in.TreeName)
	if in.TreeName == "" {
		in.TreeName = "Family tree"
	}
	var v validator
	v.check(validUsername(in.Username), "username", "use 2–64 letters, digits, dots, dashes or underscores")
	checkPassword(&v, "password", in.Password)
	v.check(utf8.RuneCountInString(in.TreeName) <= 200, "treeName", "is too long")
	if err := v.err(); err != nil {
		return User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	var u User
	err = s.tx(ctx, func(tx *sql.Tx) error {
		// The transaction takes the write lock up front (_txlock=immediate),
		// so two concurrent setups cannot both see an empty table.
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrSetupDone
		}
		now := s.now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO users (username, display_name, password_hash, role, created_at, updated_at)
			VALUES (?, ?, ?, 'admin', ?, ?)`, in.Username, in.DisplayName, string(hash), now, now)
		if err != nil {
			return err
		}
		uid, _ := res.LastInsertId()
		res, err = tx.ExecContext(ctx, `INSERT INTO trees (name, created_at, updated_at) VALUES (?, ?, ?)`, in.TreeName, now, now)
		if err != nil {
			return err
		}
		tid, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx, `INSERT INTO tree_members (tree_id, user_id, role) VALUES (?, ?, 'owner')`, tid, uid); err != nil {
			return err
		}
		u = User{ID: uid, Username: in.Username, DisplayName: in.DisplayName, Role: UserRoleAdmin}
		return nil
	})
	return u, err
}

// checkPassword applies the password rules to field.
func checkPassword(v *validator, field, pw string) {
	v.check(utf8.RuneCountInString(pw) >= MinPasswordLength, field, "must be at least 10 characters")
	v.check(len(pw) <= 72, field, "must be at most 72 bytes")
}

func validUsername(s string) bool {
	if len(s) < 2 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		ok := r == '.' || r == '-' || r == '_' || r == '@' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// dummyHash makes a login for an unknown user cost as much as one with a
// wrong password, so response times do not reveal which usernames exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("gotree-dummy-password"), bcrypt.DefaultCost)

// Authenticate checks a username and password.
func (s *Store) Authenticate(ctx context.Context, username, password string) (User, error) {
	var u User
	var hash string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, username, display_name, role, password_hash FROM users WHERE username = ?`,
		strings.TrimSpace(username)).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return User{}, ErrBadCredentials
	}
	if err != nil {
		return User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, ErrBadCredentials
	}
	return u, nil
}

// CreateSession starts a session for the user in their first tree and
// returns the token for the cookie.
func (s *Store) CreateSession(ctx context.Context, userID int64, userAgent string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}

	var treeID sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `SELECT tree_id FROM tree_members WHERE user_id = ? ORDER BY tree_id LIMIT 1`, userID).Scan(&treeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	now := s.Now().UTC()
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, tree_id, created_at, last_seen_at, expires_at, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		hashToken(token), userID, treeID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano),
		now.Add(SessionTTL).Format(time.RFC3339Nano), userAgent)
	if err != nil {
		return "", err
	}
	return token, nil
}

// SessionByToken resolves a cookie token. Expired and unknown tokens give
// ErrNotFound. Using a session extends it, at most once a minute and never
// past SessionMaxAge after it was created.
func (s *Store) SessionByToken(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrNotFound
	}
	h := hashToken(token)
	now := s.Now().UTC()

	var sess Session
	var treeID sql.NullInt64
	var treeRole sql.NullString
	var lastSeen, expires, createdAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT s.id, s.created_at, u.id, u.username, u.display_name, u.role, s.tree_id, m.role, s.last_seen_at, s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN tree_members m ON m.tree_id = s.tree_id AND m.user_id = s.user_id
		WHERE s.token_hash = ?`, h).Scan(
		&sess.ID, &createdAt, &sess.User.ID, &sess.User.Username, &sess.User.DisplayName, &sess.User.Role,
		&treeID, &treeRole, &lastSeen, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	exp, err := time.Parse(time.RFC3339Nano, expires)
	created, cerr := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil || cerr != nil || !now.Before(exp) || !now.Before(created.Add(SessionMaxAge)) {
		_, _ = s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, h)
		return Session{}, ErrNotFound
	}
	// A membership that was removed leaves the session without a tree.
	if treeRole.Valid {
		sess.TreeID, sess.TreeRole = treeID.Int64, treeRole.String
	}

	if seen, err := time.Parse(time.RFC3339Nano, lastSeen); err == nil && now.Sub(seen) > time.Minute {
		next := now.Add(SessionTTL)
		if limit := created.Add(SessionMaxAge); next.After(limit) {
			next = limit
		}
		_, _ = s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`,
			now.Format(time.RFC3339Nano), next.Format(time.RFC3339Nano), h)
		sess.CookieMaxAge = next.Sub(now)
	}
	return sess, nil
}

// DeleteSession ends a session.
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// PruneSessions removes sessions that expired or outlived SessionMaxAge.
// julianday compares the times as times: RFC 3339 strings with and without
// fractional seconds do not sort correctly as text.
func (s *Store) PruneSessions(ctx context.Context) error {
	now := s.Now().UTC()
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM sessions WHERE julianday(expires_at) <= julianday(?) OR julianday(created_at) <= julianday(?)`,
		now.Format(time.RFC3339Nano), now.Add(-SessionMaxAge).Format(time.RFC3339Nano))
	return err
}

// SessionInfo describes one of a user's sessions for the account page.
type SessionInfo struct {
	ID         int64  `json:"id"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	ExpiresAt  string `json:"expiresAt"`
	UserAgent  string `json:"userAgent"`
	Current    bool   `json:"current"`
}

// ListSessions returns the user's unexpired sessions, the most recently used
// first; currentID marks the one making the request.
func (s *Store) ListSessions(ctx context.Context, userID, currentID int64) ([]SessionInfo, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, created_at, last_seen_at, expires_at, user_agent FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := s.Now().UTC()
	type listed struct {
		info SessionInfo
		seen time.Time
	}
	var found []listed
	for rows.Next() {
		var si SessionInfo
		if err := rows.Scan(&si.ID, &si.CreatedAt, &si.LastSeenAt, &si.ExpiresAt, &si.UserAgent); err != nil {
			return nil, err
		}
		created, err1 := time.Parse(time.RFC3339Nano, si.CreatedAt)
		exp, err2 := time.Parse(time.RFC3339Nano, si.ExpiresAt)
		seen, _ := time.Parse(time.RFC3339Nano, si.LastSeenAt)
		if err1 != nil || err2 != nil {
			continue
		}
		limit := created.Add(SessionMaxAge)
		if !now.Before(exp) || !now.Before(limit) {
			continue
		}
		if limit.Before(exp) {
			exp = limit
		}
		si.ExpiresAt = exp.UTC().Format(time.RFC3339Nano)
		si.Current = si.ID == currentID
		found = append(found, listed{si, seen})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].seen.After(found[j].seen) })
	out := []SessionInfo{}
	for _, f := range found {
		out = append(out, f.info)
	}
	return out, nil
}

// DeleteUserSession ends one of the user's sessions; a session of another
// user or an unknown id gives ErrNotFound.
func (s *Store) DeleteUserSession(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUserSessions ends all of the user's sessions except exceptID (0: all).
func (s *Store) DeleteUserSessions(ctx context.Context, userID, exceptID int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, exceptID)
	return err
}

// ChangePassword sets a new password after checking the current one and
// ends the user's other sessions (all but keepSessionID), so a stolen
// cookie stops working once the password is changed.
func (s *Store) ChangePassword(ctx context.Context, userID, keepSessionID int64, current, next string) error {
	var v validator
	checkPassword(&v, "newPassword", next)
	if err := v.err(); err != nil {
		return err
	}
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return &ValidationError{Fields: map[string]string{"currentPassword": "is not your current password"}}
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, string(newHash), s.now(), userID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keepSessionID)
		return err
	})
}

// TreeName returns the name of a tree.
func (s *Store) TreeName(ctx context.Context, treeID int64) (string, error) {
	var name string
	err := s.DB.QueryRowContext(ctx, `SELECT name FROM trees WHERE id = ?`, treeID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

// newToken returns 256 random bits, URL-safe.
func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
