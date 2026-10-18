package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestShareEndpoints(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)

	var anna store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, &anna)
	c.call("POST", "/api/events", map[string]any{"personId": anna.ID, "type": "DEAT", "date": "1 MAR 1950"}, http.StatusCreated, nil)
	var link store.ShareLink
	c.call("POST", "/api/share-links", map[string]any{"label": "Cousins", "scope": "tree", "privacy": "deceased", "expiresOn": ""}, http.StatusCreated, &link)
	if link.Token == "" {
		t.Fatal("no token")
	}

	// A visitor has no session; the token is enough.
	visitor := newClient(t, ts)
	base := "/api/share/" + link.Token
	var info store.ShareInfo
	visitor.call("GET", base, nil, http.StatusOK, &info)
	if info.Label != "Cousins" || info.StartID == nil || *info.StartID != anna.ID {
		t.Errorf("info: %+v", info)
	}
	var list store.PersonList
	visitor.call("GET", base+"/persons?q=ann", nil, http.StatusOK, &list)
	if list.Total != 1 {
		t.Errorf("persons: %+v", list)
	}
	visitor.call("GET", fmt.Sprintf("%s/persons/%d", base, anna.ID), nil, http.StatusOK, nil)
	visitor.call("GET", fmt.Sprintf("%s/tree/%d?up=2", base, anna.ID), nil, http.StatusOK, nil)
	var day store.DayReport
	visitor.call("GET", base+"/onthisday?month=3&day=1", nil, http.StatusOK, &day)
	if len(day.Events) != 1 {
		t.Errorf("on this day: %+v", day)
	}
	visitor.call("POST", base+"/persons", map[string]any{"givenNames": "X"}, http.StatusMethodNotAllowed, nil)
	visitor.call("GET", "/api/share/wrong-token", nil, http.StatusNotFound, nil)
	visitor.call("GET", "/api/persons", nil, http.StatusUnauthorized, nil)
	resp, _ := get(t, ts.URL+base)
	if resp.Header.Get("X-Robots-Tag") == "" {
		t.Error("share responses are not to be indexed")
	}

	var links []store.ShareLink
	c.call("GET", "/api/share-links", nil, http.StatusOK, &links)
	if len(links) != 1 || links[0].Token != "" {
		t.Errorf("links: %+v", links)
	}
	c.call("GET", "/api/onthisday?month=3&day=1", nil, http.StatusOK, &day)
	var rotated store.ShareLink
	c.call("POST", fmt.Sprintf("/api/share-links/%d/token", link.ID), nil, http.StatusOK, &rotated)
	if rotated.Token == "" || rotated.Token == link.Token {
		t.Fatalf("rotated: %+v", rotated)
	}
	var gone errorBody
	visitor.call("GET", base, nil, http.StatusNotFound, &gone)
	if gone.Code != CodeShareGone {
		t.Errorf("old address: %+v", gone)
	}
	visitor.call("GET", "/api/share/"+rotated.Token, nil, http.StatusOK, nil)
	c.call("DELETE", fmt.Sprintf("/api/share-links/%d", link.ID), nil, http.StatusNoContent, nil)
	visitor.call("GET", "/api/share/"+rotated.Token, nil, http.StatusNotFound, nil)
	var missing errorBody
	c.call("POST", fmt.Sprintf("/api/share-links/%d/token", link.ID), nil, http.StatusNotFound, &missing)
	if missing.Code != CodeNotFound {
		t.Errorf("revoked rotate: %+v", missing)
	}

	srv := testServers[ts]
	if _, err := srv.Store.DB.Exec(`UPDATE tree_members SET role = 'editor'`); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/share-links", nil, http.StatusForbidden, nil)
	c.call("POST", "/api/share-links", map[string]any{"label": "x", "scope": "tree", "privacy": "deceased", "expiresOn": ""}, http.StatusForbidden, nil)
	c.call("POST", fmt.Sprintf("/api/share-links/%d/token", link.ID), nil, http.StatusForbidden, nil)
}
