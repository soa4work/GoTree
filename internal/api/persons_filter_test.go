package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestPersonsFilterAPI(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)

	var old, young, woman store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Old", "surname": "Zed", "sex": "M", "birth": map[string]any{"date": "1850"}}, http.StatusCreated, &old)
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Young", "surname": "Abe", "sex": "M", "birth": map[string]any{"date": "1990"}}, http.StatusCreated, &young)
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Eve", "surname": "Mid", "sex": "F", "birth": map[string]any{"date": "1995"}}, http.StatusCreated, &woman)

	var list store.PersonList
	c.call("GET", "/api/persons?sort=birth", nil, http.StatusOK, &list)
	if len(list.Items) != 3 || list.Items[0].ID != old.ID {
		t.Errorf("sort=birth: %+v", list.Items)
	}
	c.call("GET", "/api/persons?sort=birth&sex=F&living=1", nil, http.StatusOK, &list)
	if len(list.Items) != 1 || list.Items[0].ID != woman.ID || list.Total != 1 {
		t.Errorf("filtered: %+v", list)
	}
	c.call("GET", "/api/persons?sort=bogus&sex=Q&living=maybe", nil, http.StatusOK, &list)
	if len(list.Items) != 3 || list.Items[0].ID != young.ID {
		t.Errorf("bogus values should give the default order: %+v", list.Items)
	}
}

func TestSourcesPagingAPI(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	for i := 1; i <= 5; i++ {
		c.call("POST", "/api/sources", map[string]any{"title": fmt.Sprintf("Book %d", i)}, http.StatusCreated, nil)
	}
	var list []store.Source
	c.call("GET", "/api/sources?limit=2&offset=2", nil, http.StatusOK, &list)
	if len(list) != 2 || list[0].Title != "Book 3" || list[1].Title != "Book 4" {
		t.Errorf("page 2: %+v", list)
	}
}
