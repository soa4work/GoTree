package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sort"
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
			s.refreshCookie(w, r, c.Value, sess)
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
	// No account has a longer name; refuse before it reaches the throttle.
	if len(in.Username) > maxUsernameLen {
		writeError(w, http.StatusUnauthorized, CodeBadCredentials, store.ErrBadCredentials.Error())
		return
	}
	if !s.throttle.allow(ip, in.Username) {
		writeTooManyLogins(w)
		return
	}
	u, err := s.Store.Authenticate(r.Context(), in.Username, in.Password)
	if errors.Is(err, store.ErrBadCredentials) {
		// allow already counted this attempt as a failure.
		s.Log.Warn("failed login", "user", truncate(in.Username, maxUsernameLen), "ip", ip)
		writeError(w, http.StatusUnauthorized, CodeBadCredentials, err.Error())
		return
	}
	if err != nil {
		s.throttle.release(ip, in.Username)
		writeStoreError(w, s.Log, err)
		return
	}
	s.throttle.succeed(ip, in.Username)
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
	s.refreshCookie(w, r, c.Value, sess)
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
	// allow counted this attempt as a failure; give it back unless the
	// current password was really tested and found wrong.
	err := s.Store.ChangePassword(r.Context(), sess.User.ID, sess.ID, in.CurrentPassword, in.NewPassword)
	var ve *store.ValidationError
	wrong := errors.As(err, &ve) && ve.Fields["currentPassword"] != ""
	if err != nil {
		if !wrong {
			s.throttle.release(ip, username)
		}
		writeStoreError(w, s.Log, err)
		return
	}
	s.throttle.succeed(ip, username)
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
		addr, ok := parseHop(hops[i])
		if !ok {
			break
		}
		peer = addr.Unmap()
		if !s.trusted(peer) {
			break
		}
	}
	return peer.String()
}

// parseHop reads an X-Forwarded-For entry: an address, optionally in
// brackets and/or followed by a port.
func parseHop(v string) (netip.Addr, bool) {
	v = strings.TrimSpace(v)
	if ap, err := netip.ParseAddrPort(v); err == nil {
		return ap.Addr(), true
	}
	a, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(v, "["), "]"))
	return a, err == nil
}

// truncate cuts s to at most n bytes, for log lines.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// refreshCookie sends the session cookie again after the store extended the
// session, so the browser's copy lasts as long as the session does.
func (s *Server) refreshCookie(w http.ResponseWriter, r *http.Request, token string, sess store.Session) {
	if sess.CookieMaxAge > 0 {
		http.SetCookie(w, s.cookie(r, token, int(sess.CookieMaxAge/time.Second)))
	}
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
// allow reserves the attempt as a pending failure under the same lock that
// checks the limits, so concurrent guesses cannot all slip past; succeed and
// release give the reservation back. It sweeps expired entries at most once a
// minute, on the calls themselves rather than in a goroutine nothing would
// own, and holds at most maxKeys keys, so the map cannot grow without bound.
//
// A client address that logged in as a user within trustTTL is not locked out
// by that user's counter, so an anonymous client guessing the owner's name
// cannot lock the owner out of their usual device. The per-address limit still
// applies to it. That memory is in-process and bounded.
type loginThrottle struct {
	mu        sync.Mutex
	failures  map[string][]time.Time
	known     map[string]map[string]time.Time // user key -> address key -> last login
	maxIP     int
	maxUser   int
	window    time.Duration
	maxKeys   int
	trustTTL  time.Duration
	lastSweep time.Time
	now       func() time.Time
}

const (
	maxKnownUsers   = 1000
	maxKnownPerUser = 8
	// maxUsernameLen is validUsername's maximum; longer names cannot exist.
	maxUsernameLen = 64
	// knownAddressTTL outlasts a session, so the owner is still recognised
	// when the session from their last login reaches SessionMaxAge and they
	// have to log in again.
	knownAddressTTL = store.SessionMaxAge + store.SessionTTL
)

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{failures: map[string][]time.Time{}, known: map[string]map[string]time.Time{},
		maxIP: 20, maxUser: 10, window: 15 * time.Minute, maxKeys: 10_000,
		trustTTL: knownAddressTTL, now: time.Now}
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

func ipKey(addr string) string { return "ip|" + addrKey(addr) }

// userKey is a fixed-size digest of the lowercased name, so a long username
// costs no more memory than a short one.
func userKey(name string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return "user|" + string(sum[:])
}

func (t *loginThrottle) limit(key string) int {
	if strings.HasPrefix(key, "ip|") {
		return t.maxIP
	}
	return t.maxUser
}

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

// allow reports whether the attempt may go on and, if so, records it as a
// pending failure for both the address and the username.
func (t *loginThrottle) allow(ip, user string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sweep()
	ik, uk := ipKey(ip), userKey(user)
	if len(t.recent(ik)) >= t.maxIP {
		return false
	}
	if len(t.recent(uk)) >= t.maxUser && !t.isKnown(uk, ik) {
		return false
	}
	t.add(ik)
	t.add(uk)
	return true
}

func (t *loginThrottle) isKnown(uk, ik string) bool {
	at, ok := t.known[uk][ik]
	return ok && t.now().Sub(at) < t.trustTTL
}

// add records a failure under key, keeping only the newest `limit` of them.
// When the map is full of keys that cannot be evicted, the new key is dropped.
func (t *loginThrottle) add(key string) {
	if _, ok := t.failures[key]; !ok && len(t.failures) >= t.maxKeys {
		t.evict()
		if len(t.failures) >= t.maxKeys {
			return
		}
	}
	list := append(t.recent(key), t.now())
	if max := t.limit(key); len(list) > max {
		list = list[len(list)-max:]
	}
	t.failures[key] = list
}

// drop removes the newest failure under key.
func (t *loginThrottle) drop(key string) {
	list := t.recent(key)
	if len(list) <= 1 {
		delete(t.failures, key)
		return
	}
	t.failures[key] = list[:len(list)-1]
}

// release gives back the reservation of an attempt that did not test a password.
func (t *loginThrottle) release(ip, user string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drop(ipKey(ip))
	t.drop(userKey(user))
}

// succeed ends the user's failures and remembers the address as theirs. The
// address's failures stay, except the reservation of this attempt, or an
// attacker could log into their own account between guesses to reset them.
func (t *loginThrottle) succeed(ip, user string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ik, uk := ipKey(ip), userKey(user)
	t.drop(ik)
	delete(t.failures, uk)
	m := t.known[uk]
	if m == nil {
		if len(t.known) >= maxKnownUsers {
			for k := range t.known { // arbitrary victim; the set is only a convenience
				delete(t.known, k)
				break
			}
		}
		m = map[string]time.Time{}
		t.known[uk] = m
	}
	if _, ok := m[ik]; !ok && len(m) >= maxKnownPerUser {
		var oldest string
		var oldestAt time.Time
		for k, at := range m {
			if oldest == "" || at.Before(oldestAt) {
				oldest, oldestAt = k, at
			}
		}
		delete(m, oldest)
	}
	m[ik] = t.now()
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
	for uk, m := range t.known {
		for ik, at := range m {
			if now.Sub(at) >= t.trustTTL {
				delete(m, ik)
			}
		}
		if len(m) == 0 {
			delete(t.known, uk)
		}
	}
	t.lastSweep = now
}

// evict makes room for a new key: it sweeps, and if the map is still full it
// drops the keys with the fewest failures first, down to 90% of maxKeys. A key
// at its limit is never evicted, so a flood of one-off keys cannot unlock a
// locked account; when only such keys are left, the new key is dropped instead.
func (t *loginThrottle) evict() {
	t.lastSweep = time.Time{}
	t.sweep()
	if len(t.failures) < t.maxKeys {
		return
	}
	var cands []string
	for k, list := range t.failures {
		if len(list) < t.limit(k) {
			cands = append(cands, k)
		}
	}
	sort.Slice(cands, func(i, j int) bool { return len(t.failures[cands[i]]) < len(t.failures[cands[j]]) })
	for _, k := range cands {
		if len(t.failures) < t.maxKeys*9/10 {
			return
		}
		delete(t.failures, k)
	}
}
