package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/store"
)

func (s *Server) qualityRoutes(r chi.Router) {
	r.Get("/checks", s.checks)
	r.Get("/dates/proposals", s.dateProposals)
	r.Post("/dates/normalize", s.normalizeDates)
	r.Get("/relationship", s.relationship)
}

func (s *Server) checks(w http.ResponseWriter, r *http.Request) {
	rep, err := s.Store.Checks(r.Context(), actorFrom(r), int64(queryInt(r, "person", 0)))
	s.respond(w, http.StatusOK, rep, err)
}

func (s *Server) dateProposals(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.ProposeDates(r.Context(), actorFrom(r))
	s.respond(w, http.StatusOK, p, err)
}

func (s *Server) normalizeDates(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Changes []store.DateChange `json:"changes"`
	}
	if !decode(w, r, &in) {
		return
	}
	res, err := s.Store.ApplyDates(r.Context(), actorFrom(r), in.Changes)
	s.respond(w, http.StatusOK, res, err)
}

func (s *Server) relationship(w http.ResponseWriter, r *http.Request) {
	a, errA := strconv.ParseInt(r.URL.Query().Get("a"), 10, 64)
	b, errB := strconv.ParseInt(r.URL.Query().Get("b"), 10, 64)
	if errA != nil || errB != nil {
		writeError(w, http.StatusBadRequest, CodeIDsRequired, "a and b must be person ids")
		return
	}
	rep, err := s.Store.Relationship(r.Context(), actorFrom(r), a, b)
	s.respond(w, http.StatusOK, rep, err)
}
