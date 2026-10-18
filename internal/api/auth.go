package api

import (
	"context"
	"errors"
	"net"
	"net/http"
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
	ip := clientIP(r)
	if !s.throttle.allow(ip) {
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, CodeTooManyLogins, "too many failed logins, try again later")
		return
	}
	var in loginInput
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Store.Authenticate(r.Context(), in.Username, in.Password)
	if errors.Is(err, store.ErrBadCredentials) {
		s.throttle.fail(ip)
		s.Log.Warn("failed login", "user", in.Username, "ip", ip)
		writeError(w, http.StatusUnauthorized, CodeBadCredentials, err.Error())
		return
	}
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	s.throttle.reset(ip)
	if s.startSession(w, r, u.ID) {
		w.WriteHeader(http.StatusNoContent)
	}
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

// requireSession rejects requests without a valid session in a tree.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, CodeNotLoggedIn, "not logged in")
			return
		}
		sess, err := s.Store.SessionByToken(r.Context(), c.Value)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, CodeNotLoggedIn, "not logged in")
			return
		}
		if err != nil {
			writeStoreError(w, s.Log, err)
			return
		}
		if sess.TreeID == 0 {
			writeError(w, http.StatusForbidden, CodeNotMember, "you are not a member of any tree")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
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

// clientIP is the TCP peer. X-Forwarded-For is deliberately ignored: it is
// client-controlled unless a trusted proxy is configured, and trusting it
// would let anyone dodge the login throttle.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginThrottle limits failed logins per client address.
type loginThrottle struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	max      int
	window   time.Duration
	now      func() time.Time
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{failures: map[string][]time.Time{}, max: 10, window: 15 * time.Minute, now: time.Now}
}

func (t *loginThrottle) recent(ip string) []time.Time {
	cutoff := t.now().Add(-t.window)
	kept := t.failures[ip][:0]
	for _, at := range t.failures[ip] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(t.failures, ip)
		return nil
	}
	t.failures[ip] = kept
	return kept
}

func (t *loginThrottle) allow(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.recent(ip)) < t.max
}

func (t *loginThrottle) fail(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[ip] = append(t.recent(ip), t.now())
}

func (t *loginThrottle) reset(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, ip)
}
