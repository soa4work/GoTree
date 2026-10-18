package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

const memberPassword = "another good password"

// newAccount has the admin create an account and returns a client logged in
// as it.
func newAccount(t *testing.T, ts *httptest.Server, admin *client, username, role string) *client {
	t.Helper()
	admin.call("POST", "/api/admin/users", map[string]string{
		"username": username, "password": memberPassword, "role": role,
	}, http.StatusCreated, nil)
	c := newClient(t, ts)
	c.call("POST", "/api/auth/login", map[string]string{"username": username, "password": memberPassword}, http.StatusNoContent, nil)
	return c
}

func stateOf(t *testing.T, c *client) authState {
	t.Helper()
	var st authState
	c.call("GET", "/api/auth/state", nil, http.StatusOK, &st)
	return st
}

func accountID(t *testing.T, admin *client, username string) int64 {
	t.Helper()
	var list []store.Account
	admin.call("GET", "/api/admin/users", nil, http.StatusOK, &list)
	for _, a := range list {
		if a.Username == username {
			return a.ID
		}
	}
	t.Fatalf("no account %s", username)
	return 0
}

func TestAdminAccountsAPI(t *testing.T) {
	ts := newServer(t, builtFrontend)
	admin := loggedIn(t, ts)

	var created store.Account
	admin.call("POST", "/api/admin/users", map[string]string{
		"username": "bob", "displayName": "Bob", "password": memberPassword,
	}, http.StatusCreated, &created)
	if created.Role != "user" || created.Trees == nil {
		t.Errorf("created: %+v", created)
	}
	var list []store.Account
	admin.call("GET", "/api/admin/users", nil, http.StatusOK, &list)
	if len(list) != 2 {
		t.Fatalf("accounts: %+v", list)
	}
	if status, raw := admin.do("GET", "/api/admin/users", nil); status != 200 || containsHash(raw) {
		t.Errorf("response must never carry password hashes: %s", raw)
	}

	bob := newClient(t, ts)
	bob.call("POST", "/api/auth/login", map[string]string{"username": "bob", "password": memberPassword}, http.StatusNoContent, nil)
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/admin/users", nil},
		{"POST", "/api/admin/users", map[string]string{"username": "eve", "password": memberPassword}},
		{"PUT", "/api/admin/users/1", map[string]string{"role": "user"}},
		{"POST", "/api/admin/users/1/password", map[string]string{"password": memberPassword}},
		{"DELETE", "/api/admin/users/1", nil},
	} {
		if e := callError(t, bob, r.method, r.path, r.body, http.StatusForbidden); e.Code != CodeAdminOnly {
			t.Errorf("%s %s: code %q", r.method, r.path, e.Code)
		}
	}

	e := callError(t, admin, "POST", "/api/admin/users", map[string]string{"username": "BOB", "password": memberPassword}, http.StatusUnprocessableEntity)
	if e.FieldCodes["username"].Code != FieldTaken {
		t.Errorf("duplicate: %+v", e.FieldCodes)
	}

	// Reset ends bob's sessions; the new password logs in.
	admin.call("POST", fmt.Sprintf("/api/admin/users/%d/password", created.ID), map[string]string{"password": "a fresh password!"}, http.StatusNoContent, nil)
	bob.call("GET", "/api/trees", nil, http.StatusUnauthorized, nil)
	bob.call("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "a fresh password!"}, http.StatusNoContent, nil)

	adminID := stateOf(t, admin).User.ID
	e = callError(t, admin, "POST", fmt.Sprintf("/api/admin/users/%d/password", adminID), map[string]string{"password": memberPassword}, http.StatusConflict)
	if e.Code != CodeResetOwnPassword {
		t.Errorf("reset own: %q", e.Code)
	}
	e = callError(t, admin, "DELETE", fmt.Sprintf("/api/admin/users/%d", adminID), nil, http.StatusConflict)
	if e.Code != CodeDeleteSelf {
		t.Errorf("delete self: %q", e.Code)
	}
	e = callError(t, admin, "PUT", fmt.Sprintf("/api/admin/users/%d", adminID), map[string]string{"role": "user"}, http.StatusConflict)
	if e.Code != CodeLastAdmin {
		t.Errorf("demote self: %q", e.Code)
	}

	// Updating and deleting another account.
	var upd store.Account
	admin.call("PUT", fmt.Sprintf("/api/admin/users/%d", created.ID), map[string]string{"displayName": "Robert", "role": "admin"}, http.StatusOK, &upd)
	if upd.DisplayName != "Robert" || upd.Role != "admin" {
		t.Errorf("updated: %+v", upd)
	}
	admin.call("DELETE", fmt.Sprintf("/api/admin/users/%d", created.ID), nil, http.StatusNoContent, nil)
	admin.call("DELETE", fmt.Sprintf("/api/admin/users/%d", created.ID), nil, http.StatusNotFound, nil)

	anon := newClient(t, ts)
	if e := callError(t, anon, "GET", "/api/admin/users", nil, http.StatusUnauthorized); e.Code != CodeNotLoggedIn {
		t.Errorf("logged out: %q", e.Code)
	}
}

func containsHash(raw []byte) bool {
	s := string(raw)
	return strings.Contains(s, "$2a$") || strings.Contains(s, "passwordHash") || strings.Contains(s, "password_hash")
}

func TestNoTreeUserCanCreateAndSwitch(t *testing.T) {
	ts := newServer(t, builtFrontend)
	admin := loggedIn(t, ts)
	bob := newAccount(t, ts, admin, "bob", "user")

	if e := callError(t, bob, "GET", "/api/persons", nil, http.StatusForbidden); e.Code != CodeNotMember {
		t.Errorf("persons without a tree: %q", e.Code)
	}
	var trees []store.TreeSummary
	bob.call("GET", "/api/trees", nil, http.StatusOK, &trees)
	if trees == nil || len(trees) != 0 {
		t.Fatalf("trees: %v", trees)
	}
	var made store.TreeSummary
	bob.call("POST", "/api/trees", map[string]string{"name": "Bob"}, http.StatusCreated, &made)
	if made.Role != "owner" || made.ID == 0 {
		t.Fatalf("created: %+v", made)
	}
	var st authState
	bob.call("PUT", "/api/auth/tree", map[string]int64{"treeId": made.ID}, http.StatusOK, &st)
	if st.Tree == nil || st.Tree.ID != made.ID || st.Tree.Role != "owner" {
		t.Fatalf("state after switch: %+v", st)
	}
	var persons struct {
		Total int `json:"total"`
	}
	bob.call("GET", "/api/persons", nil, http.StatusOK, &persons)
	admin.call("POST", "/api/persons", map[string]string{"givenNames": "Adam"}, http.StatusCreated, nil)
	bob.call("GET", "/api/persons", nil, http.StatusOK, &persons)
	if persons.Total != 0 {
		t.Errorf("bob sees %d persons of the admin's tree", persons.Total)
	}
	adminState := stateOf(t, admin)
	if adminState.Tree == nil || adminState.Tree.ID == made.ID {
		t.Errorf("the admin must stay in their own tree: %+v", adminState.Tree)
	}

	// Renaming and a tree id the user is not in.
	bob.call("PUT", fmt.Sprintf("/api/trees/%d", made.ID), map[string]string{"name": "Bob's tree"}, http.StatusOK, nil)
	bob.call("PUT", "/api/auth/tree", map[string]int64{"treeId": adminState.Tree.ID}, http.StatusNotFound, nil)
	bob.call("PUT", fmt.Sprintf("/api/trees/%d", adminState.Tree.ID), map[string]string{"name": "mine now"}, http.StatusNotFound, nil)
	bob.call("POST", "/api/trees", map[string]string{"name": ""}, http.StatusUnprocessableEntity, nil)
}

func TestTreeIsolationAfterSwitch(t *testing.T) {
	ts := newServer(t, builtFrontend)
	admin := loggedIn(t, ts)
	bob := newAccount(t, ts, admin, "bob", "user")
	tree1 := stateOf(t, admin).Tree.ID
	admin.call("POST", fmt.Sprintf("/api/trees/%d/members", tree1), map[string]string{"username": "bob", "role": "editor"}, http.StatusCreated, nil)

	var own store.TreeSummary
	bob.call("POST", "/api/trees", map[string]string{"name": "Bob"}, http.StatusCreated, &own)
	bob.call("PUT", "/api/auth/tree", map[string]int64{"treeId": own.ID}, http.StatusOK, nil)
	var person struct {
		ID int64 `json:"id"`
	}
	bob.call("POST", "/api/persons", map[string]string{"givenNames": "Bo"}, http.StatusCreated, &person)

	bob.call("PUT", "/api/auth/tree", map[string]int64{"treeId": tree1}, http.StatusOK, nil)
	bob.call("GET", fmt.Sprintf("/api/persons/%d", person.ID), nil, http.StatusNotFound, nil)
}

func TestMembersAPIPermissions(t *testing.T) {
	ts := newServer(t, builtFrontend)
	admin := loggedIn(t, ts)
	bob := newAccount(t, ts, admin, "bob", "user")
	carol := newAccount(t, ts, admin, "carol", "user")
	tree1 := stateOf(t, admin).Tree.ID
	members := fmt.Sprintf("/api/trees/%d/members", tree1)
	adminID := stateOf(t, admin).User.ID
	bobID := accountID(t, admin, "bob")

	admin.call("POST", members, map[string]string{"username": "bob", "role": "viewer"}, http.StatusCreated, nil)
	e := callError(t, admin, "POST", members, map[string]string{"username": "bob", "role": "viewer"}, http.StatusUnprocessableEntity)
	if e.FieldCodes["username"].Code != FieldNotAddable {
		t.Errorf("second add: %+v", e.FieldCodes)
	}
	e = callError(t, admin, "POST", members, map[string]string{"username": "ghost", "role": "viewer"}, http.StatusUnprocessableEntity)
	if e.FieldCodes["username"].Code != FieldNotAddable {
		t.Errorf("unknown username: %+v", e.FieldCodes)
	}

	// Bob's session predates the membership; he picks the tree now.
	bob.call("PUT", "/api/auth/tree", map[string]int64{"treeId": tree1}, http.StatusOK, nil)
	if e := callError(t, bob, "POST", "/api/persons", map[string]string{"givenNames": "X"}, http.StatusForbidden); e.Code != CodeViewOnly {
		t.Errorf("viewer write: %q", e.Code)
	}
	if e := callError(t, bob, "GET", members, nil, http.StatusForbidden); e.Code != CodeOwnerOnly {
		t.Errorf("viewer lists members: %q", e.Code)
	}
	if e := callError(t, bob, "PUT", fmt.Sprintf("%s/%d", members, adminID), map[string]string{"role": "viewer"}, http.StatusForbidden); e.Code != CodeOwnerOnly {
		t.Errorf("viewer changes a role: %q", e.Code)
	}
	if e := callError(t, bob, "DELETE", fmt.Sprintf("%s/%d", members, adminID), nil, http.StatusForbidden); e.Code != CodeOwnerOnly {
		t.Errorf("viewer removes the owner: %q", e.Code)
	}
	// Someone outside the tree learns nothing about it.
	carol.call("GET", members, nil, http.StatusNotFound, nil)

	// The only owner cannot demote themselves.
	if e := callError(t, admin, "PUT", fmt.Sprintf("%s/%d", members, adminID), map[string]string{"role": "editor"}, http.StatusConflict); e.Code != CodeLastOwner {
		t.Errorf("last owner: %q", e.Code)
	}
	if e := callError(t, admin, "PUT", fmt.Sprintf("%s/%d", members, bobID), map[string]string{"role": "admin"}, http.StatusUnprocessableEntity); e.FieldCodes["role"].Code != FieldInvalidChoice {
		t.Errorf("bad role: %+v", e.FieldCodes)
	}

	// A role change applies to the next request, without logging in again.
	var m store.Member
	admin.call("PUT", fmt.Sprintf("%s/%d", members, bobID), map[string]string{"role": "editor"}, http.StatusOK, &m)
	if m.Role != "editor" {
		t.Errorf("member: %+v", m)
	}
	bob.call("POST", "/api/persons", map[string]string{"givenNames": "X"}, http.StatusCreated, nil)
	var list []store.Member
	admin.call("GET", members, nil, http.StatusOK, &list)
	if len(list) != 2 || list[0].Username != "admin" {
		t.Errorf("members: %+v", list)
	}

	// Leaving; the session is then in no tree.
	bob.call("DELETE", fmt.Sprintf("%s/%d", members, bobID), nil, http.StatusNoContent, nil)
	if e := callError(t, bob, "GET", "/api/persons", nil, http.StatusForbidden); e.Code != CodeNotMember {
		t.Errorf("after leaving: %q", e.Code)
	}
	st := stateOf(t, bob)
	if st.User == nil || st.Tree != nil {
		t.Errorf("state after leaving: %+v", st)
	}
}

func TestBackupStaysAdminOnlyWithSeveralTrees(t *testing.T) {
	ts := newServer(t, builtFrontend)
	admin := loggedIn(t, ts)
	bob := newAccount(t, ts, admin, "bob", "user")
	var own store.TreeSummary
	bob.call("POST", "/api/trees", map[string]string{"name": "Bob"}, http.StatusCreated, &own)
	bob.call("PUT", "/api/auth/tree", map[string]int64{"treeId": own.ID}, http.StatusOK, nil)

	if e := callError(t, bob, "GET", "/api/backup", nil, http.StatusForbidden); e.Code != CodeAdminOnlyBackup {
		t.Errorf("bob's backup: %q", e.Code)
	}
	admin.call("GET", "/api/backup", nil, http.StatusOK, nil)
}

func TestAdminWithoutTreeCanDownloadBackup(t *testing.T) {
	ts := newServer(t, builtFrontend)
	admin := loggedIn(t, ts)
	bob := newAccount(t, ts, admin, "bob", "user")
	st := stateOf(t, admin)
	members := fmt.Sprintf("/api/trees/%d/members", st.Tree.ID)
	admin.call("POST", members, map[string]string{"username": "bob", "role": "owner"}, http.StatusCreated, nil)
	admin.call("DELETE", fmt.Sprintf("%s/%d", members, st.User.ID), nil, http.StatusNoContent, nil)
	callError(t, admin, "GET", "/api/persons", nil, http.StatusForbidden)

	admin.call("GET", "/api/backup", nil, http.StatusOK, nil)
	if e := callError(t, bob, "GET", "/api/backup", nil, http.StatusForbidden); e.Code != CodeAdminOnlyBackup {
		t.Errorf("regular user: %q", e.Code)
	}
	anon := newClient(t, ts)
	anon.call("GET", "/api/backup", nil, http.StatusUnauthorized, nil)
}

func TestViewerExportWithoutLiving(t *testing.T) {
	ts := newServer(t, builtFrontend)
	admin := loggedIn(t, ts)
	viewer := newAccount(t, ts, admin, "vera", "user")
	editor := newAccount(t, ts, admin, "eddie", "user")
	tree1 := stateOf(t, admin).Tree.ID
	for name, role := range map[string]string{"vera": "viewer", "eddie": "editor"} {
		admin.call("POST", fmt.Sprintf("/api/trees/%d/members", tree1), map[string]string{"username": name, "role": role}, http.StatusCreated, nil)
	}
	viewer.call("PUT", "/api/auth/tree", map[string]int64{"treeId": tree1}, http.StatusOK, nil)
	editor.call("PUT", "/api/auth/tree", map[string]int64{"treeId": tree1}, http.StatusOK, nil)

	for _, path := range []string{"/api/export/gedcom?privacy=all", "/api/export/gedcom"} {
		if e := callError(t, viewer, "GET", path, nil, http.StatusForbidden); e.Code != CodeExportLivingEditors {
			t.Errorf("viewer %s: %q", path, e.Code)
		}
	}
	viewer.call("GET", "/api/export/gedcom?privacy=exclude-living", nil, http.StatusOK, nil)
	viewer.call("GET", "/api/export/gedcom?privacy=name-only", nil, http.StatusOK, nil)
	editor.call("GET", "/api/export/gedcom?privacy=all", nil, http.StatusOK, nil)
	admin.call("GET", "/api/export/gedcom", nil, http.StatusOK, nil)
}
