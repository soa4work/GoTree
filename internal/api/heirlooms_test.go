package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestHeirloomEndpoints(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	var anna store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, &anna)

	body := map[string]any{
		"name": "Ring", "kind": "jewellery", "description": "", "madeDate": "", "originPlaceId": nil, "currentLocation": "", "notes": "",
		"custody": []map[string]any{{"personId": anna.ID, "fromDate": "1950", "toDate": "", "how": "gift", "notes": ""}},
	}
	var h store.Heirloom
	c.call("POST", "/api/heirlooms", body, http.StatusCreated, &h)
	var list []store.HeirloomRef
	c.call("GET", fmt.Sprintf("/api/heirlooms?person=%d", anna.ID), nil, http.StatusOK, &list)
	if len(list) != 1 || list[0].Holder.ID != anna.ID {
		t.Errorf("list: %+v", list)
	}

	// Photos attach to heirlooms like to anyone else.
	if status, out := c.upload(fmt.Sprintf("/api/media?entityType=heirloom&entityId=%d", h.ID), "ring.jpg", jpegBytes(t, 20, 20)); status != http.StatusCreated {
		t.Fatalf("upload: %d %s", status, out)
	}
	c.call("GET", "/api/heirlooms", nil, http.StatusOK, &list)
	if list[0].PhotoID == nil {
		t.Errorf("photo: %+v", list[0])
	}

	body["name"] = "Wedding ring"
	c.call("PUT", fmt.Sprintf("/api/heirlooms/%d", h.ID), body, http.StatusOK, &h)
	if h.Name != "Wedding ring" {
		t.Errorf("renamed: %+v", h)
	}
	c.call("DELETE", fmt.Sprintf("/api/heirlooms/%d", h.ID), nil, http.StatusNoContent, nil)
	c.call("GET", fmt.Sprintf("/api/heirlooms/%d", h.ID), nil, http.StatusNotFound, nil)
}
