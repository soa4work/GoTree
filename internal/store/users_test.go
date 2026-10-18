package store

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

const adminPassword = "correct horse battery"

// clockAt moves the store's clock to t0 plus d.
func clockAt(s *Store, t0 time.Time, d time.Duration) {
	s.Now = func() time.Time { return t0.Add(d) }
}

func TestSessionAbsoluteLifetime(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	t0 := s.Now()
	token, err := s.CreateSession(ctx, a.UserID, "ua")
	if err != nil {
		t.Fatal(err)
	}
	day := 24 * time.Hour
	for _, d := range []time.Duration{20 * day, 40 * day, 60 * day, 80 * day} {
		clockAt(s, t0, d)
		if _, err := s.SessionByToken(ctx, token); err != nil {
			t.Fatalf("after %v: %v", d, err)
		}
	}
	var expires string
	if err := s.DB.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE token_hash = ?`, hashToken(token)).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	exp, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		t.Fatal(err)
	}
	if exp.After(t0.Add(SessionMaxAge)) {
		t.Errorf("expires_at %v is past the absolute limit %v", exp, t0.Add(SessionMaxAge))
	}
	clockAt(s, t0, SessionMaxAge-time.Second)
	if _, err := s.SessionByToken(ctx, token); err != nil {
		t.Errorf("just before the limit: %v", err)
	}
	clockAt(s, t0, SessionMaxAge)
	if _, err := s.SessionByToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("at the limit: %v, want ErrNotFound", err)
	}
}

func TestSessionByTokenGivesID(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	var ids []int64
	for range 2 {
		token, err := s.CreateSession(ctx, a.UserID, "ua")
		if err != nil {
			t.Fatal(err)
		}
		sess, err := s.SessionByToken(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if sess.ID == 0 {
			t.Error("session id is zero")
		}
		ids = append(ids, sess.ID)
	}
	if ids[0] == ids[1] {
		t.Errorf("sessions share id %d", ids[0])
	}
}

func TestListAndDeleteSessions(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	b := otherTree(t, s)
	t0 := s.Now()

	mk := func(user int64, ua string) (string, int64) {
		t.Helper()
		token, err := s.CreateSession(ctx, user, ua)
		if err != nil {
			t.Fatal(err)
		}
		sess, err := s.SessionByToken(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		return token, sess.ID
	}
	tokA1, idA1 := mk(a.UserID, "Firefox")
	clockAt(s, t0, 5*time.Minute)
	tokA2, idA2 := mk(a.UserID, "Safari")
	tokB, idB := mk(b.UserID, "ua")

	list, err := s.ListSessions(ctx, a.UserID, idA1)
	if err != nil {
		t.Fatal(err)
	}
	// setup() made one more session for the admin, so there are three.
	if len(list) != 3 {
		t.Fatalf("got %d sessions, want 3: %+v", len(list), list)
	}
	current := 0
	for _, si := range list {
		if si.Current {
			current++
			if si.ID != idA1 || si.UserAgent != "Firefox" {
				t.Errorf("wrong current session: %+v", si)
			}
		}
		if si.UserAgent == "" {
			t.Errorf("user agent missing: %+v", si)
		}
	}
	if current != 1 {
		t.Errorf("%d current sessions, want 1", current)
	}
	if list[0].ID != idA2 {
		t.Errorf("newest session must come first: %+v", list)
	}

	if err := s.DeleteUserSession(ctx, a.UserID, idB); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting another user's session: %v, want ErrNotFound", err)
	}
	if _, err := s.SessionByToken(ctx, tokB); err != nil {
		t.Errorf("B's session must survive: %v", err)
	}
	if err := s.DeleteUserSession(ctx, a.UserID, idA2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByToken(ctx, tokA2); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted session still resolves: %v", err)
	}

	if err := s.DeleteUserSessions(ctx, a.UserID, idA1); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListSessions(ctx, a.UserID, idA1)
	if len(list) != 1 || list[0].ID != idA1 {
		t.Errorf("only the kept session must remain: %+v", list)
	}
	if _, err := s.SessionByToken(ctx, tokA1); err != nil {
		t.Errorf("kept session: %v", err)
	}
	if err := s.DeleteUserSessions(ctx, a.UserID, 0); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListSessions(ctx, a.UserID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("all of A's sessions must be gone: %+v", list)
	}
	if raw, _ := json.Marshal(list); string(raw) != "[]" {
		t.Errorf("an empty list must be [], got %s", raw)
	}
	if _, err := s.SessionByToken(ctx, tokB); err != nil {
		t.Errorf("B's session must stay: %v", err)
	}

	// Expired sessions are not listed.
	clockAt(s, t0, SessionTTL+time.Hour)
	list, err = s.ListSessions(ctx, b.UserID, idB)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("expired session listed: %+v", list)
	}
}

func TestChangePassword(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	keep, err := s.CreateSession(ctx, a.UserID, "keep")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateSession(ctx, a.UserID, "other")
	if err != nil {
		t.Fatal(err)
	}
	kept, err := s.SessionByToken(ctx, keep)
	if err != nil {
		t.Fatal(err)
	}

	err = s.ChangePassword(ctx, a.UserID, kept.ID, adminPassword, "short")
	validationField(t, err, "newPassword")

	err = s.ChangePassword(ctx, a.UserID, kept.ID, "not the password", "a brand new password")
	validationField(t, err, "currentPassword")
	var ve *ValidationError
	if errors.As(err, &ve) && ve.Fields["currentPassword"] != "is not your current password" {
		t.Errorf("message %q", ve.Fields["currentPassword"])
	}

	if err := s.ChangePassword(ctx, a.UserID, kept.ID, adminPassword, "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, "admin", "a brand new password"); err != nil {
		t.Errorf("new password: %v", err)
	}
	if _, err := s.Authenticate(ctx, "admin", adminPassword); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("old password: %v, want ErrBadCredentials", err)
	}
	if _, err := s.SessionByToken(ctx, keep); err != nil {
		t.Errorf("kept session: %v", err)
	}
	if _, err := s.SessionByToken(ctx, other); !errors.Is(err, ErrNotFound) {
		t.Errorf("other session: %v, want ErrNotFound", err)
	}
}

func TestPruneSessions(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	now := s.Now()
	f := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339Nano) }
	insert := func(tok byte, created, expires string) {
		t.Helper()
		if _, err := s.DB.ExecContext(ctx, `
			INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
			[]byte{tok}, a.UserID, created, created, expires); err != nil {
			t.Fatal(err)
		}
	}
	insert(1, f(-48*time.Hour), f(-24*time.Hour))       // expired yesterday
	insert(2, f(-91*24*time.Hour), f(24*time.Hour))     // older than the absolute limit
	insert(3, f(-time.Hour), f(24*time.Hour))           // fresh
	insert(4, f(-48*time.Hour), "2026-10-05T12:00:00Z") // expired half a second ago; sorts after as text
	s.Now = func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 500_000_000, time.UTC) }
	if err := s.PruneSessions(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT token_hash FROM sessions WHERE token_hash IN (x'01', x'02', x'03', x'04')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var left []byte
	for rows.Next() {
		var h []byte
		if err := rows.Scan(&h); err != nil {
			t.Fatal(err)
		}
		left = append(left, h...)
	}
	if string(left) != "\x03" {
		t.Errorf("sessions left: %v, want only #3", left)
	}
}

func TestSessionExtensionReportsCookieLifetime(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	t0 := s.Now()
	token, err := s.CreateSession(ctx, a.UserID, "ua")
	if err != nil {
		t.Fatal(err)
	}
	day := 24 * time.Hour
	clockAt(s, t0, 30*time.Second)
	if sess, _ := s.SessionByToken(ctx, token); sess.CookieMaxAge != 0 {
		t.Errorf("no extension within a minute, got CookieMaxAge %v", sess.CookieMaxAge)
	}
	for _, d := range []time.Duration{10 * day, 35 * day, 60 * day} {
		clockAt(s, t0, d)
		sess, err := s.SessionByToken(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		want := SessionTTL
		if limit := SessionMaxAge - d; limit < want {
			want = limit
		}
		if sess.CookieMaxAge != want {
			t.Errorf("after %v: CookieMaxAge %v, want %v", d, sess.CookieMaxAge, want)
		}
	}
}
