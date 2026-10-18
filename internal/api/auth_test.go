package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

// client talks to a test server with a cookie jar, like a browser would.
type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, ts *httptest.Server) *client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, headers ...string) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp.StatusCode, out
}

// call expects the given status and decodes the response into out.
func (c *client) call(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	status, raw := c.do(method, path, body)
	if status != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode %s: %v", method, path, raw, err)
		}
	}
}

const testPassword = "correct horse battery"

// loggedIn returns a client that has completed the first-run setup.
func loggedIn(t *testing.T, ts *httptest.Server) *client {
	t.Helper()
	c := newClient(t, ts)
	c.call("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": testPassword}, http.StatusOK, nil)
	return c
}

func TestSetupLoginLogout(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := newClient(t, ts)

	var state authState
	c.call("GET", "/api/auth/state", nil, http.StatusOK, &state)
	if !state.SetupRequired || state.User != nil {
		t.Fatalf("fresh install: %+v", state)
	}
	c.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)

	var errBody errorBody
	c.call("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": "short"}, http.StatusUnprocessableEntity, &errBody)
	if errBody.Fields["password"] == "" {
		t.Errorf("validation errors must name the field: %+v", errBody)
	}

	c.call("POST", "/api/auth/setup", map[string]string{"username": "admin", "password": testPassword, "treeName": "Weber"}, http.StatusOK, &state)
	if state.SetupRequired || state.User == nil || state.Tree == nil || state.Tree.Name != "Weber" || state.Tree.Role != "owner" {
		t.Fatalf("after setup the user must be logged in: %+v", state)
	}
	c.call("GET", "/api/persons", nil, http.StatusOK, nil)

	// Setup cannot be repeated to create another account.
	other := newClient(t, ts)
	other.call("POST", "/api/auth/setup", map[string]string{"username": "evil", "password": testPassword}, http.StatusConflict, nil)

	c.call("POST", "/api/auth/logout", nil, http.StatusNoContent, nil)
	c.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)
	c.call("GET", "/api/auth/state", nil, http.StatusOK, &state)
	if state.User != nil {
		t.Errorf("logged out but state has a user: %+v", state)
	}

	c.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "wrong password"}, http.StatusUnauthorized, nil)
	c.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": testPassword}, http.StatusNoContent, nil)
	c.call("GET", "/api/persons", nil, http.StatusOK, nil)
}

func TestSessionCookieFlags(t *testing.T) {
	ts := newServer(t, builtFrontend)
	resp, err := http.Post(ts.URL+"/api/auth/setup", "application/json",
		strings.NewReader(`{"username":"admin","password":"`+testPassword+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	var c *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookie {
			c = ck
		}
	}
	if c == nil {
		t.Fatal("no session cookie")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge <= 0 {
		t.Errorf("cookie flags: %+v", c)
	}
}

func TestLoginThrottle(t *testing.T) {
	ts := newServer(t, builtFrontend)
	loggedIn(t, ts)
	c := newClient(t, ts)
	for range 10 {
		c.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "nope nope nope"}, http.StatusUnauthorized, nil)
	}
	// Even the right password is refused while throttled.
	c.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": testPassword}, http.StatusTooManyRequests, nil)
}

func TestThrottleWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newLoginThrottle()
	th.now = func() time.Time { return now }
	for i := range th.maxIP {
		th.fail("1.2.3.4", fmt.Sprintf("u%d", i))
	}
	if th.allow("1.2.3.4", "x") {
		t.Error("allowed after max failures from one address")
	}
	if !th.allow("5.6.7.8", "x") {
		t.Error("another address must not be throttled")
	}
	now = now.Add(th.window + time.Second)
	if !th.allow("1.2.3.4", "x") {
		t.Error("failures must expire after the window")
	}
}

func TestThrottlePerUsername(t *testing.T) {
	th := newLoginThrottle()
	for i := range th.maxUser {
		th.fail(fmt.Sprintf("10.0.0.%d", i), "Admin")
	}
	if th.allow("9.9.9.9", "admin") {
		t.Error("usernames are case-insensitive: admin must be throttled")
	}
	if !th.allow("9.9.9.9", "other") {
		t.Error("another username must not be throttled")
	}
	th.succeed("admin")
	if !th.allow("9.9.9.9", "admin") {
		t.Error("a successful login must clear the username's failures")
	}
}

func TestThrottleSucceedKeepsAddressCount(t *testing.T) {
	th := newLoginThrottle()
	for i := range th.maxIP - 1 {
		th.fail("1.2.3.4", fmt.Sprintf("u%d", i))
	}
	th.succeed("me")
	th.fail("1.2.3.4", "last")
	if th.allow("1.2.3.4", "new") {
		t.Error("a success by name must not reset the address count")
	}
}

func TestThrottleIsBounded(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newLoginThrottle()
	th.now = func() time.Time { return now }
	th.maxKeys = 100
	for i := range 1000 {
		th.fail(fmt.Sprintf("10.0.%d.%d", i/250, i%250), "bot")
		if len(th.failures) > th.maxKeys {
			t.Fatalf("map grew to %d keys, limit %d", len(th.failures), th.maxKeys)
		}
	}
	now = now.Add(th.window + time.Minute)
	th.fail("203.0.113.1", "fresh")
	if len(th.failures) != 2 {
		t.Errorf("after the window one fail must sweep the rest: %d keys", len(th.failures))
	}
}

func TestThrottleIPv6Prefix(t *testing.T) {
	th := newLoginThrottle()
	for i := range th.maxIP {
		th.fail(fmt.Sprintf("2001:db8::%x", i+1), fmt.Sprintf("u%d", i))
	}
	if th.allow("2001:db8::ffff", "x") {
		t.Error("addresses of one /64 must share a counter")
	}
	if !th.allow("2001:db9::1", "x") {
		t.Error("another /64 must not be throttled")
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name    string
		trusted []netip.Prefix
		peer    string
		xff     []string
		want    string
	}{
		{"no trusted proxies ignores the header", nil, "10.0.0.1:5000", []string{"1.1.1.1"}, "10.0.0.1"},
		{"trusted proxy names the client", trusted, "10.0.0.1:5000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"forged first entry is skipped", trusted, "10.0.0.1:5000", []string{"6.6.6.6, 203.0.113.7"}, "203.0.113.7"},
		{"two proxies", trusted, "10.0.0.1:5000", []string{"203.0.113.7, 10.0.0.2"}, "203.0.113.7"},
		{"two header lines", trusted, "10.0.0.1:5000", []string{"6.6.6.6", "203.0.113.7, 10.0.0.2"}, "203.0.113.7"},
		{"garbage falls back to the peer", trusted, "10.0.0.1:5000", []string{"garbage"}, "10.0.0.1"},
		{"untrusted peer", trusted, "198.51.100.1:1", []string{"1.1.1.1"}, "198.51.100.1"},
		{"ipv4-mapped peer", trusted, "[::ffff:10.0.0.1]:5000", []string{"203.0.113.7"}, "203.0.113.7"},
		{"no header", trusted, "10.0.0.1:5000", nil, "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{TrustedProxies: tt.trusted}
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.peer
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := s.clientIP(r); got != tt.want {
				t.Errorf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func behindProxy(s *Server) {
	s.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
}

func TestLoginThrottlePerUsernameBehindProxy(t *testing.T) {
	ts := newServerWith(t, builtFrontend, behindProxy)
	loggedIn(t, ts)
	c := newClient(t, ts)
	login := func(user, pw, ip string, want int) {
		t.Helper()
		status, raw := c.do("POST", "/api/auth/login", map[string]string{"username": user, "password": pw}, "X-Forwarded-For", ip)
		if status != want {
			t.Fatalf("login %s from %s: status %d, want %d: %s", user, ip, status, want, raw)
		}
	}
	for i := range 10 {
		login("admin", "nope nope nope", fmt.Sprintf("203.0.113.%d", i+1), http.StatusUnauthorized)
	}
	login("admin", testPassword, "203.0.113.200", http.StatusTooManyRequests)
	login("someone", "nope nope nope", "203.0.113.200", http.StatusUnauthorized)
}

func TestLoginThrottleOneProxyClientDoesNotLockOthers(t *testing.T) {
	ts := newServerWith(t, builtFrontend, behindProxy)
	loggedIn(t, ts)
	c := newClient(t, ts)
	login := func(user, pw, ip string, want int) {
		t.Helper()
		status, raw := c.do("POST", "/api/auth/login", map[string]string{"username": user, "password": pw}, "X-Forwarded-For", ip)
		if status != want {
			t.Fatalf("login %s from %s: status %d, want %d: %s", user, ip, status, want, raw)
		}
	}
	for i := range 20 {
		login(fmt.Sprintf("u%d", i), "nope nope nope", "203.0.113.1", http.StatusUnauthorized)
	}
	login("admin", testPassword, "203.0.113.1", http.StatusTooManyRequests)
	login("admin", testPassword, "203.0.113.2", http.StatusNoContent)
}

func TestLoginThrottleIgnoresForwardedForWithoutTrustedProxy(t *testing.T) {
	ts := newServer(t, builtFrontend)
	loggedIn(t, ts)
	c := newClient(t, ts)
	for i := range 20 {
		status, raw := c.do("POST", "/api/auth/login", map[string]string{"username": fmt.Sprintf("u%d", i), "password": "nope nope nope"},
			"X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i+1))
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d: %s", i, status, raw)
		}
	}
	status, _ := c.do("POST", "/api/auth/login", map[string]string{"username": "fresh", "password": "nope nope nope"}, "X-Forwarded-For", "198.51.100.77")
	if status != http.StatusTooManyRequests {
		t.Errorf("a spoofed X-Forwarded-For must not dodge the limit: status %d", status)
	}
}

func TestCrossOriginWritesAreRejected(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	status, _ := c.do("POST", "/api/persons", map[string]string{"givenNames": "X"},
		"Origin", "https://evil.example", "Sec-Fetch-Site", "cross-site")
	if status != http.StatusForbidden {
		t.Errorf("cross-site POST: status %d, want 403", status)
	}
	status, _ = c.do("POST", "/api/persons", map[string]string{"givenNames": "X"}, "Sec-Fetch-Site", "same-origin")
	if status != http.StatusCreated {
		t.Errorf("same-origin POST: status %d, want 201", status)
	}
}

func TestViewerCannotWrite(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	c.call("POST", "/api/persons", map[string]string{"givenNames": "Before"}, http.StatusCreated, nil)

	srv := testServers[ts]
	if _, err := srv.Store.DB.Exec(`UPDATE tree_members SET role = 'viewer'`); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/persons", nil, http.StatusOK, nil)
	c.call("POST", "/api/persons", map[string]string{"givenNames": "After"}, http.StatusForbidden, nil)
}
