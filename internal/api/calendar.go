package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/ical"
	"github.com/praetorianer777/gotree/internal/store"
)

func (s *Server) calendarFeedRoutes(r chi.Router) {
	r.Get("/calendar-feeds", s.listCalendarFeeds)
	r.Post("/calendar-feeds", s.createCalendarFeed)
	r.Delete("/calendar-feeds/{id}", s.revokeCalendarFeed)
}

func (s *Server) listCalendarFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := s.Store.ListCalendarFeeds(r.Context(), actorFrom(r))
	s.respond(w, http.StatusOK, feeds, err)
}

func (s *Server) createCalendarFeed(w http.ResponseWriter, r *http.Request) {
	var in store.CalendarFeedInput
	if !decode(w, r, &in) {
		return
	}
	feed, err := s.Store.CreateCalendarFeed(r.Context(), actorFrom(r), in)
	s.respond(w, http.StatusCreated, feed, err)
}

func (s *Server) revokeCalendarFeed(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.RevokeCalendarFeed(r.Context(), actorFrom(r), id))
	}
}

// icalFeed serves /ical/{token}.ics. Calendar apps cannot log in, so the
// token in the URL is the credential. The optional lang query parameter
// (en, ru, kk) picks the language of the event texts.
func (s *Server) icalFeed(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutSuffix(chi.URLParam(r, "file"), ".ics")
	if !ok {
		http.NotFound(w, r)
		return
	}
	cal, err := s.Store.CalendarByToken(r.Context(), token)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.Log.Error("calendar feed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	texts := calendarTextsFor(r.URL.Query().Get("lang"))
	name := cal.TreeName
	if name == store.DefaultTreeName {
		name = texts.defaultTree
	}
	out := ical.Calendar{Name: name}
	from := fmt.Sprintf(texts.from, name)
	for _, e := range cal.Entries {
		names := make([]string, len(e.Names))
		for i, n := range e.Names {
			if n == "" {
				n = texts.unknown
			}
			names[i] = n
		}
		summary := texts.summary(e.Kind, strings.Join(names, " & "), e.Year)
		out.Events = append(out.Events, ical.Event{UID: e.UID, Summary: summary, Description: from, Year: e.Year, Month: e.Month, Day: e.Day})
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if err := ical.Write(w, out, s.Store.Now()); err != nil {
		s.Log.Error("calendar feed", "err", err)
	}
}
