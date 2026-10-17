package api

import (
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestStatsAndMapEndpoints(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna", "surname": "Weber"}, http.StatusCreated, nil)
	var st store.Stats
	c.call("GET", "/api/stats", nil, http.StatusOK, &st)
	if st.Persons != 1 || st.Surnames[0].Name != "Weber" {
		t.Errorf("stats: %+v", st)
	}
	var d store.MapData
	c.call("GET", "/api/map", nil, http.StatusOK, &d)
	c.call("GET", "/api/map?scope=sideways", nil, http.StatusBadRequest, nil)
	c.call("GET", "/api/map?scope=ancestors&root=999", nil, http.StatusNotFound, nil)

	var atlantis store.Place
	c.call("POST", "/api/places", map[string]any{"name": "Atlantis"}, http.StatusCreated, &atlantis)
	c.call("POST", "/api/events", map[string]any{"personId": 1, "type": "RESI", "date": "1890", "placeId": atlantis.ID}, http.StatusCreated, nil)
	var missing store.UnmappedPlaces
	c.call("GET", "/api/map/unmapped-places", nil, http.StatusOK, &missing)
	if missing.Total != 1 || len(missing.Places) != 1 || missing.Places[0].FullName != "Atlantis" || missing.Places[0].Events != 1 {
		t.Errorf("unmapped places: %+v", missing)
	}
}
