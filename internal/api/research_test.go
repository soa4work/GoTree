package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestResearchEndpoints(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	var anna store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, &anna)

	var sugg store.SuggestionReport
	c.call("GET", fmt.Sprintf("/api/research/suggestions?person=%d", anna.ID), nil, http.StatusOK, &sugg)
	if len(sugg.Suggestions) != 1 || sugg.Suggestions[0].Kind != "missing_birth" {
		t.Fatalf("suggestions: %+v", sugg)
	}
	origin := sugg.Suggestions[0].Origin
	task := map[string]any{
		"title": "Find Anna's birth", "status": "", "priority": "", "dueOn": "", "notes": "", "origin": origin,
		"links": []map[string]any{{"entityType": "person", "entityId": anna.ID}},
	}
	var created store.ResearchTask
	c.call("POST", "/api/research/tasks", task, http.StatusCreated, &created)
	c.call("POST", "/api/research/tasks", task, http.StatusConflict, nil)

	var entry store.LogEntry
	c.call("POST", "/api/research/log", map[string]any{
		"taskId": created.ID, "searchedOn": "2026-10-06", "query": "Births 1850", "location": "", "result": "not_found", "notes": "",
		"links": []map[string]any{},
	}, http.StatusCreated, &entry)

	var tasks []store.ResearchTask
	c.call("GET", fmt.Sprintf("/api/research/tasks?status=active&person=%d", anna.ID), nil, http.StatusOK, &tasks)
	if len(tasks) != 1 || tasks[0].LogCount != 1 {
		t.Errorf("tasks: %+v", tasks)
	}
	var log []store.LogEntry
	c.call("GET", fmt.Sprintf("/api/research/log?task=%d", created.ID), nil, http.StatusOK, &log)
	if len(log) != 1 {
		t.Errorf("log: %+v", log)
	}
	task["status"] = "done"
	c.call("PUT", fmt.Sprintf("/api/research/tasks/%d", created.ID), task, http.StatusOK, &created)
	if created.DoneAt == nil {
		t.Errorf("done: %+v", created)
	}
	c.call("DELETE", fmt.Sprintf("/api/research/log/%d", entry.ID), nil, http.StatusNoContent, nil)
	c.call("DELETE", fmt.Sprintf("/api/research/tasks/%d", created.ID), nil, http.StatusNoContent, nil)
	c.call("GET", "/api/research/suggestions", nil, http.StatusNotFound, nil)
}
