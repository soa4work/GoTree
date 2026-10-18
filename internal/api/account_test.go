package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

const newTestPassword = "a different horse staple"

func TestSessionsListAndEnd(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c1 := loggedIn(t, ts)
	c2 := newClient(t, ts)
	c2.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": testPassword}, http.StatusNoContent, nil)

	var list []store.SessionInfo
	c1.call("GET", "/api/auth/sessions", nil, http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("got %d sessions, want 2: %+v", len(list), list)
	}
	var mine, theirs int64
	current := 0
	for _, si := range list {
		if si.Current {
			current++
			mine = si.ID
		} else {
			theirs = si.ID
		}
	}
	if current != 1 {
		t.Fatalf("%d current sessions, want 1", current)
	}

	c1.call("DELETE", fmt.Sprintf("/api/auth/sessions/%d", theirs), nil, http.StatusNoContent, nil)
	c2.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)
	c1.call("GET", "/api/persons", nil, http.StatusOK, nil)

	e := callError(t, c1, "DELETE", "/api/auth/sessions/99999", nil, http.StatusNotFound)
	if e.Code != CodeNotFound {
		t.Errorf("code %q", e.Code)
	}

	req, _ := http.NewRequest("DELETE", c1.base+fmt.Sprintf("/api/auth/sessions/%d", mine), nil)
	resp, err := c1.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ending the current session: status %d", resp.StatusCode)
	}
	cleared := false
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookie && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("ending the current session must clear the cookie")
	}
	c1.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)
}

func TestLogoutEverywhere(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c1 := loggedIn(t, ts)
	c2 := newClient(t, ts)
	c2.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": testPassword}, http.StatusNoContent, nil)

	c1.call("POST", "/api/auth/logout-all", nil, http.StatusNoContent, nil)
	c1.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)
	c2.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)
}

func TestChangePasswordAPI(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c1 := loggedIn(t, ts)
	c2 := newClient(t, ts)
	c2.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": testPassword}, http.StatusNoContent, nil)

	e := callError(t, c1, "POST", "/api/auth/password",
		map[string]string{"currentPassword": "not the password", "newPassword": newTestPassword}, http.StatusUnprocessableEntity)
	if e.FieldCodes["currentPassword"].Code != FieldWrongPassword || e.Fields["currentPassword"] == "" {
		t.Errorf("wrong current password: %+v", e)
	}
	e = callError(t, c1, "POST", "/api/auth/password",
		map[string]string{"currentPassword": testPassword, "newPassword": "short"}, http.StatusUnprocessableEntity)
	if e.FieldCodes["newPassword"].Code != FieldPasswordShort {
		t.Errorf("short new password: %+v", e)
	}
	c2.call("GET", "/api/persons", nil, http.StatusOK, nil)

	c1.call("POST", "/api/auth/password",
		map[string]string{"currentPassword": testPassword, "newPassword": newTestPassword}, http.StatusNoContent, nil)
	c2.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)
	c1.call("GET", "/api/persons", nil, http.StatusOK, nil)

	c3 := newClient(t, ts)
	c3.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": newTestPassword}, http.StatusNoContent, nil)
	c3.call("POST", "/api/auth/login", map[string]string{"username": "admin", "password": testPassword}, http.StatusUnauthorized, nil)
}

func TestChangePasswordThrottled(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	body := map[string]string{"currentPassword": "not the password", "newPassword": newTestPassword}
	for range 10 {
		c.call("POST", "/api/auth/password", body, http.StatusUnprocessableEntity, nil)
	}
	e := callError(t, c, "POST", "/api/auth/password", body, http.StatusTooManyRequests)
	if e.Code != CodeTooManyLogins {
		t.Errorf("code %q", e.Code)
	}
}

func TestAccountRoutesWorkForViewersAndWithoutTree(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	srv := serverOf(ts)
	if _, err := srv.Store.DB.Exec(`UPDATE tree_members SET role = 'viewer'`); err != nil {
		t.Fatal(err)
	}
	c.call("POST", "/api/auth/password",
		map[string]string{"currentPassword": testPassword, "newPassword": newTestPassword}, http.StatusNoContent, nil)

	if _, err := srv.Store.DB.Exec(`DELETE FROM tree_members`); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/auth/sessions", nil, http.StatusOK, nil)
	e := callError(t, c, "GET", "/api/persons", nil, http.StatusForbidden)
	if e.Code != CodeNotMember {
		t.Errorf("code %q", e.Code)
	}
}

func TestAccountRoutesNeedLogin(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	loggedIn(t, ts)
	c := newClient(t, ts)
	for _, rt := range []struct{ method, path string }{
		{"GET", "/api/auth/sessions"},
		{"DELETE", "/api/auth/sessions/1"},
		{"POST", "/api/auth/logout-all"},
		{"POST", "/api/auth/password"},
	} {
		var body any
		if rt.method == "POST" {
			body = map[string]string{}
		}
		e := callError(t, c, rt.method, rt.path, body, http.StatusUnauthorized)
		if e.Code != CodeNotLoggedIn {
			t.Errorf("%s %s: code %q", rt.method, rt.path, e.Code)
		}
	}
}
