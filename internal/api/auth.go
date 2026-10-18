package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/praetorianer777/gotree/internal/store"
)

const sessionCookie = "gotree_session"

type ctxKey int

const sessionKey ctxKey = 0

func sessionFrom(ctx context.Context) store.Session {
	s, _ := ctx.Value(sessionKey).(store.Session)
	return s
}

func actorFrom(r *http.Request) store.Actor {
	return sessionFrom(r.Context()).Actor()
}

type treeInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type authState struct {
	SetupRequired bool        `json:"setupRequired"`
	User          *store.User `json:"user"`
	Tree          *treeInfo   `json:"tree"`
}

func (s *Server) authState(w http.ResponseWriter, r *http.Request) {
	need, err := s.Store.NeedsSetup(r.Context())
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	state := authState{SetupRequired: need}
	if c, err := r.Cookie(sessionCookie); err == nil {
		if sess, err := s.Store.SessionByToken(r.Context(), c.Value); err == nil {
			state.User = &sess.User
			if sess.TreeID != 0 {
				name, err := s.Store.TreeName(r.Context(), sess.TreeID)
				if err != nil {
					writeStoreError(w, s.Log, err)
					return
				}
				state.Tree = &treeInfo{ID: sess.TreeID, Name: name, Role: sess.TreeRole}
			}
		}
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var in store.SetupInput
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Store.Setup(r.Context(), in)
	if errors.Is(err, store.ErrSetupDone) {
		writeError(w, http.StatusConflict, CodeSetupDone, err.Error())
		return
	}
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	if !s.startSession(w, r, u.ID) {
		return
	}
	s.Log.Info("setup completed", "user", u.Username)
	s.authState(w, r)
}

type loginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	var in loginInput
	if !decode(w, r, &in) {
		return
	}
	if !s.throttle.allow(ip, in.Username) {
		writeTooManyLogins(w)
		return
	}
	u, err := s.Store.Authenticate(r.Context(), in.Username, in.Password)
	if errors.Is(err, store.ErrBadCredentials) {
		s.throttle.fail(ip, in.Username)
		s.Log.Warn("failed login", "user", in.Username, "ip", ip)
		writeError(w, http.StatusUnauthorized, CodeBadCredentials, err.Error())
		return
	}
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	s.throttle.succeed(in.Username)
	if s.startSession(w, r, u.ID) {
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeTooManyLogins(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "900")
	writeError(w, http.StatusTooManyRequests, CodeTooManyLogins, "too many failed logins, try again later")
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.Store.DeleteSession(r.Context(), c.Value); err != nil {
			writeStoreError(w, s.Log, err)
			return
		}
	}
	http.SetCookie(w, s.cookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

// startSession creates a session and sets its cookie. The new cookie is
// also put on the request, so a handler can answer with the logged-in state
// right away.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) bool {
	token, err := s.Store.CreateSession(r.Context(), userID, r.UserAgent())
	if err != nil {
		writeStoreError(w, s.Log, err)
		return false
	}
	c := s.cookie(r, token, int(store.SessionTTL/time.Second))
	http.SetCookie(w, c)
	r.Header.Del("Cookie")
	r.AddCookie(c)
	return true
}

func (s *Server) cookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Behind a TLS-terminating proxy the request itself is plain HTTP;
		// the header only ever makes the cookie stricter.
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	}
}

// sessionOf resolves the request's session; it answers 401 (or 500) itself
// and reports false when there is none.
func (s *Server) sessionOf(w http.ResponseWriter, r *http.Request) (store.Session, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, CodeNotLoggedIn, "not logged in")
		return store.Session{}, false
	}
	sess, err := s.Store.SessionByToken(r.Context(), c.Value)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, CodeNotLoggedIn, "not logged in")
		return store.Session{}, false
	}
	if err != nil {
		writeStoreError(w, s.Log, err)
		return store.Session{}, false
	}
	return sess, true
}

// requireSession rejects requests without a valid session in a tree.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.sessionOf(w, r)
		if !ok {
			return
		}
		if sess.TreeID == 0 {
			writeError(w, http.StatusForbidden, CodeNotMember, "you are not a member of any tree")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}

// requireUser rejects requests without a valid session; unlike
// requireSession it does not need a tree, so account settings work for
// everyone who can log in.
func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, ok := s.sessionOf(w, r)
		if !ok {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}

// listSessions lists the signed-in user's own sessions.
func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	list, err := s.Store.ListSessions(r.Context(), sess.User.ID, sess.ID)
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// endSession signs one of the user's devices out.
func (s *Server) endSession(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	sess := sessionFrom(r.Context())
	if err := s.Store.DeleteUserSession(r.Context(), sess.User.ID, id); err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	if id == sess.ID {
		http.SetCookie(w, s.cookie(r, "", -1))
	}
	w.WriteHeader(http.StatusNoContent)
}

// logoutAll ends every session of the user, this one included.
func (s *Server) logoutAll(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	if err := s.Store.DeleteUserSessions(r.Context(), sess.User.ID, 0); err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	s.Log.Info("signed out everywhere", "user", sess.User.Username)
	http.SetCookie(w, s.cookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

type passwordInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// changePassword sets a new password and signs the other devices out. A
// wrong current password answers 422, not 401: the client treats every 401
// as a logout.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	var in passwordInput
	if !decode(w, r, &in) {
		return
	}
	// Guessing the current password with a stolen cookie counts like a failed login.
	ip, username := s.clientIP(r), sess.User.Username
	if !s.throttle.allow(ip, username) {
		writeTooManyLogins(w)
		return
	}
	err := s.Store.ChangePassword(r.Context(), sess.User.ID, sess.ID, in.CurrentPassword, in.NewPassword)
	var ve *store.ValidationError
	if errors.As(err, &ve) {
		if _, wrong := ve.Fields["currentPassword"]; wrong {
			s.throttle.fail(ip, username)
		}
	}
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	s.throttle.succeed(username)
	s.Log.Info("password changed", "user", username)
	w.WriteHeader(http.StatusNoContent)
}

// requireEditor lets viewers read but not change anything.
func requireEditor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !actorFrom(r).CanEdit() {
				writeError(w, http.StatusForbidden, CodeViewOnly, "you can only view this tree")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the address of the client. It is the TCP peer unless the peer
// is a trusted proxy (GOTREE_TRUSTED_PROXIES): then X-Forwarded-For is read
// from the right, skipping trusted hops, and the first untrusted address is
// the client. Without trusted proxies X-Forwarded-For is ignored, since any
// client can send it.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return r.RemoteAddr
	}
	peer = peer.Unmap()
	if !s.trusted(peer) {
		return peer.String()
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		peer = addr.Unmap()
		if !s.trusted(peer) {
			break
		}
	}
	return peer.String()
}

func (s *Server) trusted(a netip.Addr) bool {
	for _, p := range s.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// loginThrottle limits failed logins per client address and per username,
// so neither one guessing client nor a botnet guessing one account gets far.
// It sweeps expired entries at most once a minute, on the calls themselves
// rather than in a goroutine nothing would own, and holds at most maxKeys
// keys, so the map cannot grow without bound.
type loginThrottle struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	maxIP     int
	maxUser   int
	window    time.Duration
	maxKeys   int
	lastSweep time.Time
	now       func() time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{failures: map[string][]time.Time{}, maxIP: 20, maxUser: 10,
		window: 15 * time.Minute, maxKeys: 10_000, now: time.Now}
}

// addrKey names a client address; an IPv6 client usually holds a whole /64,
// so its addresses share one key.
func addrKey(addr string) string {
	a, err := netip.ParseAddr(addr)
	if err != nil {
		return addr
	}
	a = a.Unmap()
	if a.Is6() {
		return netip.PrefixFrom(a, 64).Masked().String()
	}
	return a.String()
}

func ipKey(addr string) string   { return "ip|" + addrKey(addr) }
func userKey(name string) string { return "user|" + strings.ToLower(strings.TrimSpace(name)) }

func (t *loginThrottle) recent(key string) []time.Time {
	cutoff := t.now().Add(-t.window)
	kept := t.failures[key][:0]
	for _, at := range t.failures[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(t.failures, key)
		return nil
	}
	t.failures[key] = kept
	return kept
}

func (t *loginThrottle) allow(ip, user string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweep()
	return len(t.recent(ipKey(ip))) < t.maxIP && len(t.recent(userKey(user))) < t.maxUser
}

func (t *loginThrottle) fail(ip, user string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweep()
	t.add(ipKey(ip), t.maxIP)
	t.add(userKey(user), t.maxUser)
}

// add records a failure under key, keeping only the newest max of them.
func (t *loginThrottle) add(key string, max int) {
	if _, ok := t.failures[key]; !ok && len(t.failures) >= t.maxKeys {
		t.evict()
	}
	list := append(t.recent(key), t.now())
	if len(list) > max {
		list = list[len(list)-max:]
	}
	t.failures[key] = list
}

// succeed forgets the username's failures. The address count stays, or an
// attacker could log into their own account between guesses to reset it.
func (t *loginThrottle) succeed(user string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, userKey(user))
}

// sweep drops expired entries, at most once a minute.
func (t *loginThrottle) sweep() {
	now := t.now()
	if now.Sub(t.lastSweep) < time.Minute {
		return
	}
	for k := range t.failures {
		t.recent(k)
	}
	t.lastSweep = now
}

// evict makes room for a new key: it sweeps, and if the map is still full it
// drops arbitrary keys (Go's random map order) down to 90% of maxKeys. Under
// a flood of distinct keys, keeping the server's memory bounded matters more
// than keeping every counter.
func (t *loginThrottle) evict() {
	t.lastSweep = time.Time{}
	t.sweep()
	if len(t.failures) < t.maxKeys {
		return
	}
	for k := range t.failures {
		if len(t.failures) < t.maxKeys*9/10 {
			return
		}
		delete(t.failures, k)
	}
}
