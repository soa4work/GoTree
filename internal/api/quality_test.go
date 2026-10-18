package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestQualityEndpoints(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)

	var parent store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, &parent)
	var rel store.RelativeResult
	c.call("POST", fmt.Sprintf("/api/persons/%d/relatives", parent.ID), map[string]any{
		"relation": "child", "person": map[string]any{"givenNames": "Paul"},
	}, http.StatusCreated, &rel)
	var birth store.Event
	c.call("POST", "/api/events", map[string]any{"personId": parent.ID, "type": "BIRT", "date": "1845"}, http.StatusCreated, nil)
	c.call("POST", "/api/events", map[string]any{"personId": rel.Person.ID, "type": "BIRT", "date": "about 1855"}, http.StatusCreated, &birth)

	var rep store.CheckReport
	c.call("GET", fmt.Sprintf("/api/checks?person=%d", parent.ID), nil, http.StatusOK, &rep)
	if len(rep.Findings) != 0 {
		t.Errorf("ABT 1855 may be 1857, when Anna was 12: %+v", rep.Findings)
	}

	// A date saved from the form is never offered; older loose text is.
	var prop store.DateProposals
	c.call("GET", "/api/dates/proposals", nil, http.StatusOK, &prop)
	if len(prop.Items) != 0 {
		t.Fatalf("a just-saved date was proposed: %+v", prop)
	}
	if _, err := serverOf(ts).Store.DB.Exec(`UPDATE events SET date_raw = 'ca. 1855', date = '', date_sort = NULL WHERE id = ?`, birth.ID); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/dates/proposals", nil, http.StatusOK, &prop)
	if len(prop.Items) != 1 || prop.Items[0].Proposed != "ABT 1855" {
		t.Fatalf("proposals: %+v", prop)
	}
	var res store.DateChangeResult
	c.call("POST", "/api/dates/normalize", map[string]any{"changes": []map[string]any{
		{"eventId": birth.ID, "raw": "ca. 1855", "date": "ABT 1855"},
	}}, http.StatusOK, &res)
	if res.Updated != 1 {
		t.Errorf("result: %+v", res)
	}
	c.call("POST", "/api/dates/normalize", map[string]any{"changes": []map[string]any{
		{"eventId": birth.ID, "raw": "ABT 1855", "date": "about 1855"},
	}}, http.StatusUnprocessableEntity, nil)

	var r store.RelationshipReport
	c.call("GET", fmt.Sprintf("/api/relationship?a=%d&b=%d", rel.Person.ID, parent.ID), nil, http.StatusOK, &r)
	if r.Kind != "blood" || r.Kinship.Up != 1 || r.Kinship.Down != 0 {
		t.Errorf("relationship: %+v", r)
	}
	c.call("GET", "/api/relationship?a=1", nil, http.StatusBadRequest, nil)
	c.call("GET", fmt.Sprintf("/api/relationship?a=%d&b=999", parent.ID), nil, http.StatusNotFound, nil)

	srv := serverOf(ts)
	if _, err := srv.Store.DB.Exec(`UPDATE tree_members SET role = 'viewer'`); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/checks", nil, http.StatusOK, nil)
	c.call("POST", "/api/dates/normalize", map[string]any{"changes": []map[string]any{}}, http.StatusForbidden, nil)
}
