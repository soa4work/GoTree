package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestCalendarFeed(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	var anna store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna", "surname": "Weber"}, http.StatusCreated, &anna)
	c.call("POST", "/api/events", map[string]any{"personId": anna.ID, "type": "BIRT", "date": "12 MAR 1850"}, http.StatusCreated, nil)
	c.call("POST", "/api/events", map[string]any{"personId": anna.ID, "type": "DEAT", "date": "1 MAY 1920"}, http.StatusCreated, nil)

	var feed store.CalendarFeed
	c.call("POST", "/api/calendar-feeds", map[string]any{"label": "Phone", "includeLiving": false}, http.StatusCreated, &feed)

	// The calendar app has no session.
	resp, body := get(t, ts.URL+"/ical/"+feed.Token+".ics")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/calendar") {
		t.Fatalf("status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, want := range []string{"SUMMARY:Birthday: Anna Weber (born 1850)", "SUMMARY:Remembrance: Anna Weber (died 1920)", "DTSTART;VALUE=DATE:18500312"} {
		if !strings.Contains(body, want) {
			t.Errorf("feed lacks %q:\n%s", want, body)
		}
	}
	if resp, _ := get(t, ts.URL+"/ical/"+feed.Token); resp.StatusCode != http.StatusNotFound {
		t.Errorf("without .ics: %d", resp.StatusCode)
	}

	var feeds []store.CalendarFeed
	c.call("GET", "/api/calendar-feeds", nil, http.StatusOK, &feeds)
	if len(feeds) != 1 {
		t.Errorf("feeds: %+v", feeds)
	}
	c.call("DELETE", fmt.Sprintf("/api/calendar-feeds/%d", feed.ID), nil, http.StatusNoContent, nil)
	if resp, _ := get(t, ts.URL+"/ical/"+feed.Token+".ics"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("revoked feed: %d", resp.StatusCode)
	}
}

func TestCalendarFeedLanguage(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	var anna store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna", "surname": "Weber"}, http.StatusCreated, &anna)
	c.call("POST", "/api/events", map[string]any{"personId": anna.ID, "type": "BIRT", "date": "12 MAR 1850"}, http.StatusCreated, nil)
	c.call("POST", "/api/events", map[string]any{"personId": anna.ID, "type": "DEAT", "date": "1 MAY 1920"}, http.StatusCreated, nil)
	var feed store.CalendarFeed
	c.call("POST", "/api/calendar-feeds", map[string]any{"label": "Phone", "includeLiving": false}, http.StatusCreated, &feed)

	en := []string{"SUMMARY:Birthday: Anna Weber (born 1850)", "SUMMARY:Remembrance: Anna Weber (died 1920)", "DESCRIPTION:From the family tree"}
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", en},
		{"?lang=en", en},
		{"?lang=xx", en},
		{"?lang=", en},
		{"?lang=ru", []string{"SUMMARY:День рождения: Anna Weber (год рождения 1850)", "SUMMARY:День памяти: Anna Weber (год смерти 1920)", "DESCRIPTION:Из семейного дерева «"}},
		{"?lang=KK-kz", []string{"SUMMARY:Туған күн: Anna Weber (туған жылы 1850)", "SUMMARY:Еске алу күні: Anna Weber (қайтыс болған жылы 1920)", `DESCRIPTION:GoTree-дегі «`}},
	} {
		resp, body := get(t, ts.URL+"/ical/"+feed.Token+".ics"+tc.query)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%q: status %d", tc.query, resp.StatusCode)
		}
		// Long lines are folded; compare the unfolded feed.
		body = strings.ReplaceAll(body, "\r\n ", "")
		for _, want := range tc.want {
			if !strings.Contains(body, want) {
				t.Errorf("%q: feed lacks %q:\n%s", tc.query, want, body)
			}
		}
	}

	// The secret URL works the same whatever the language.
	if resp, _ := get(t, ts.URL+"/ical/wrong.ics?lang=ru"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("wrong token: %d", resp.StatusCode)
	}
}

func TestCalendarTexts(t *testing.T) {
	for _, tc := range []struct {
		lang, want string
	}{
		{"en", "Wedding anniversary: A & B (1900)"},
		{"ru", "Годовщина свадьбы: A & B (1900)"},
		{"kk", "Үйлену мерейтойы: A & B (1900)"},
		{"", "Wedding anniversary: A & B (1900)"},
	} {
		if got := calendarTextsFor(tc.lang).summary("MARR", "A & B", 1900); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.lang, got, tc.want)
		}
	}
}

func TestCalendarLanguagesComplete(t *testing.T) {
	for _, lang := range []string{"en", "ru", "kk"} {
		if _, ok := calendarLanguages[lang]; !ok {
			t.Errorf("language %s is missing", lang)
		}
	}
	for lang, texts := range calendarLanguages {
		for name, s := range map[string]string{"birthday": texts.birthday, "anniversary": texts.anniversary, "remembrance": texts.remembrance, "from": texts.from} {
			if strings.TrimSpace(s) == "" {
				t.Errorf("%s: %s is empty", lang, name)
			}
			want := 2
			if name == "from" {
				want = 1
			}
			if n := strings.Count(s, "%"); n != want {
				t.Errorf("%s: %s %q has %d verbs, want %d", lang, name, s, n, want)
			}
		}
	}
}
