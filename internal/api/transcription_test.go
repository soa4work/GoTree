package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestTranscriptionEndpoints(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	var src store.Source
	c.call("POST", "/api/sources", map[string]any{"title": "Kirchenbuch", "author": "", "publication": "", "callNumber": "", "repositoryId": nil, "notes": ""}, http.StatusCreated, &src)

	var templates []store.RecordTemplate
	c.call("GET", "/api/templates", nil, http.StatusOK, &templates)
	if len(templates) != 6 || templates[0].Key != "census" {
		t.Errorf("templates: %+v", templates)
	}

	rows := []map[string]any{
		{"line": "1", "role": "deceased", "values": map[string]string{"given": "Hans", "surname": "Weber", "age": "70"}, "action": "", "personId": nil},
	}
	body := map[string]any{"templateKey": "burial", "title": "", "sourceId": src.ID, "page": "f. 3", "date": "5 JAN 1900", "placeId": nil, "notes": "", "rows": rows}
	var tr store.Transcription
	c.call("POST", "/api/transcriptions", body, http.StatusCreated, &tr)

	var cands [][]store.MatchCandidate
	c.call("POST", "/api/transcriptions/match", map[string]any{"date": tr.Date, "rows": tr.Rows}, http.StatusOK, &cands)
	if len(cands) != 1 || len(cands[0]) != 0 {
		t.Errorf("candidates in an empty tree: %+v", cands)
	}

	var res store.ApplyResult
	c.call("POST", fmt.Sprintf("/api/transcriptions/%d/apply", tr.ID), nil, http.StatusOK, &res)
	if len(res.Created) != 1 || res.Facts != 1 {
		t.Errorf("result: %+v", res)
	}
	c.call("PUT", fmt.Sprintf("/api/transcriptions/%d", tr.ID), body, http.StatusConflict, nil)
	var list []store.Transcription
	c.call("GET", "/api/transcriptions", nil, http.StatusOK, &list)
	if len(list) != 1 || list[0].Status != "applied" || list[0].Persons[res.Created[0]].GivenNames != "Hans" {
		t.Errorf("list: %+v", list)
	}
	c.call("DELETE", fmt.Sprintf("/api/transcriptions/%d", tr.ID), nil, http.StatusNoContent, nil)

	tpl := map[string]any{
		"key": "", "name": "School register", "builtin": false, "eventType": "EDUC", "columns": []string{"given", "surname"},
		"roles": []map[string]any{{"key": "pupil", "label": "Pupil", "kind": "principal", "participant": ""}},
	}
	var saved store.RecordTemplate
	c.call("POST", "/api/templates", tpl, http.StatusCreated, &saved)
	tpl["name"] = "School list"
	var id int64
	fmt.Sscanf(saved.Key, "custom:%d", &id)
	c.call("PUT", fmt.Sprintf("/api/templates/%d", id), tpl, http.StatusOK, &saved)
	if saved.Name != "School list" {
		t.Errorf("renamed: %+v", saved)
	}
	c.call("DELETE", fmt.Sprintf("/api/templates/%d", id), nil, http.StatusNoContent, nil)
}
