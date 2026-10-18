package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/gendate"
	"github.com/praetorianer777/gotree/internal/store"
)

func (s *Server) resourceRoutes(r chi.Router) {
	r.Route("/persons", func(r chi.Router) {
		r.Get("/", s.listPersons)
		r.Post("/", s.createPerson)
		r.Get("/{id}", s.getPerson)
		r.Put("/{id}", s.updatePerson)
		r.Delete("/{id}", s.deletePerson)
		r.Post("/{id}/relatives", s.addRelative)
	})
	r.Get("/dates/parse", parseDate)
	r.Get("/tree/{id}", s.tree)
	r.Route("/families", func(r chi.Router) {
		r.Post("/", s.createFamily)
		r.Get("/{id}", s.getFamily)
		r.Put("/{id}", s.updateFamily)
		r.Delete("/{id}", s.deleteFamily)
		r.Put("/{id}/children/{childID}", s.setChild)
		r.Delete("/{id}/children/{childID}", s.removeChild)
	})
	r.Route("/events", func(r chi.Router) {
		r.Post("/", s.createEvent)
		r.Get("/{id}", s.getEvent)
		r.Put("/{id}", s.updateEvent)
		r.Delete("/{id}", s.deleteEvent)
	})
	r.Route("/places", func(r chi.Router) {
		r.Get("/", s.listPlaces)
		r.Post("/", s.createPlace)
		r.Get("/{id}", s.getPlace)
		r.Put("/{id}", s.updatePlace)
		r.Delete("/{id}", s.deletePlace)
	})
}

// respond writes v, or maps err.
func (s *Server) respond(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	writeJSON(w, status, v)
}

func (s *Server) noContent(w http.ResponseWriter, err error) {
	if err != nil {
		writeStoreError(w, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listPersons serves GET /api/persons?q&limit&offset&sort&sex&living. Unknown
// sort, sex and living values are ignored, as if unset, so they need no
// error code.
func (s *Server) listPersons(w http.ResponseWriter, r *http.Request) {
	qv := r.URL.Query()
	var f store.PersonFilter
	if v := qv.Get("sort"); v == "name" || v == "birth" || v == "-birth" || v == "updated" {
		f.Sort = v
	}
	if v := qv.Get("sex"); v == "M" || v == "F" || v == "X" || v == "U" {
		f.Sex = v
	}
	switch qv.Get("living") {
	case "1":
		t := true
		f.Living = &t
	case "0":
		t := false
		f.Living = &t
	}
	list, err := s.Store.ListPersonsFiltered(r.Context(), actorFrom(r), qv.Get("q"), f, queryInt(r, "limit", 50), queryInt(r, "offset", 0))
	s.respond(w, http.StatusOK, list, err)
}

func (s *Server) createPerson(w http.ResponseWriter, r *http.Request) {
	var in store.PersonInput
	if !decode(w, r, &in) {
		return
	}
	p, err := s.Store.CreatePerson(r.Context(), actorFrom(r), in)
	s.respond(w, http.StatusCreated, p, err)
}

func (s *Server) getPerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := s.Store.GetPersonDetail(r.Context(), actorFrom(r), id)
	s.respond(w, http.StatusOK, d, err)
}

func (s *Server) updatePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in store.PersonInput
	if !decode(w, r, &in) {
		return
	}
	p, err := s.Store.UpdatePerson(r.Context(), actorFrom(r), id, in)
	s.respond(w, http.StatusOK, p, err)
}

func (s *Server) deletePerson(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.DeletePerson(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) createFamily(w http.ResponseWriter, r *http.Request) {
	var in store.FamilyInput
	if !decode(w, r, &in) {
		return
	}
	f, err := s.Store.CreateFamily(r.Context(), actorFrom(r), in)
	s.respond(w, http.StatusCreated, f, err)
}

func (s *Server) getFamily(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		f, err := s.Store.GetFamily(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, f, err)
	}
}

func (s *Server) updateFamily(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in store.FamilyInput
	if !decode(w, r, &in) {
		return
	}
	f, err := s.Store.UpdateFamily(r.Context(), actorFrom(r), id, in)
	s.respond(w, http.StatusOK, f, err)
}

func (s *Server) deleteFamily(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.DeleteFamily(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) setChild(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	childID, ok := pathID(w, r, "childID")
	if !ok {
		return
	}
	var in store.ChildInput
	if !decode(w, r, &in) {
		return
	}
	f, err := s.Store.SetChild(r.Context(), actorFrom(r), id, childID, in)
	s.respond(w, http.StatusOK, f, err)
}

func (s *Server) removeChild(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	childID, ok := pathID(w, r, "childID")
	if !ok {
		return
	}
	f, err := s.Store.RemoveChild(r.Context(), actorFrom(r), id, childID)
	s.respond(w, http.StatusOK, f, err)
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var in store.EventInput
	if !decode(w, r, &in) {
		return
	}
	e, err := s.Store.CreateEvent(r.Context(), actorFrom(r), in)
	s.respond(w, http.StatusCreated, e, err)
}

func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		e, err := s.Store.GetEvent(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, e, err)
	}
}

func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in store.EventInput
	if !decode(w, r, &in) {
		return
	}
	e, err := s.Store.UpdateEvent(r.Context(), actorFrom(r), id, in)
	s.respond(w, http.StatusOK, e, err)
}

func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.DeleteEvent(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) listPlaces(w http.ResponseWriter, r *http.Request) {
	places, err := s.Store.ListPlaces(r.Context(), actorFrom(r), r.URL.Query().Get("q"), queryInt(r, "limit", 100))
	s.respond(w, http.StatusOK, places, err)
}

func (s *Server) createPlace(w http.ResponseWriter, r *http.Request) {
	var in store.PlaceInput
	if !decode(w, r, &in) {
		return
	}
	p, err := s.Store.CreatePlace(r.Context(), actorFrom(r), in)
	s.respond(w, http.StatusCreated, p, err)
}

func (s *Server) getPlace(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		p, err := s.Store.GetPlace(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, p, err)
	}
}

func (s *Server) updatePlace(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in store.PlaceInput
	if !decode(w, r, &in) {
		return
	}
	p, err := s.Store.UpdatePlace(r.Context(), actorFrom(r), id, in)
	s.respond(w, http.StatusOK, p, err)
}

func (s *Server) deletePlace(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.DeletePlace(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) addRelative(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in store.RelativeInput
	if !decode(w, r, &in) {
		return
	}
	res, err := s.Store.AddRelative(r.Context(), actorFrom(r), id, in)
	s.respond(w, http.StatusCreated, res, err)
}

type parsedDate struct {
	Valid      bool   `json:"valid"`
	Empty      bool   `json:"empty"`
	Normalized string `json:"normalized"`
	Qualifier  string `json:"qualifier"`
	Error      string `json:"error,omitempty"`
}

// parseDate lets the UI preview how a date will be stored, using the same
// parser as the server instead of a second implementation in JavaScript.
func parseDate(w http.ResponseWriter, r *http.Request) {
	d, err := gendate.Normalize(r.URL.Query().Get("q"))
	switch {
	case errors.Is(err, gendate.ErrEmpty):
		writeJSON(w, http.StatusOK, parsedDate{Empty: true})
	case err != nil:
		writeJSON(w, http.StatusOK, parsedDate{Error: err.Error()})
	default:
		writeJSON(w, http.StatusOK, parsedDate{Valid: true, Normalized: d.String(), Qualifier: string(d.Qualifier)})
	}
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	g, err := s.Store.Tree(r.Context(), actorFrom(r), id, treeOptions(r))
	s.respond(w, http.StatusOK, g, err)
}

func treeOptions(r *http.Request) store.TreeOptions {
	siblings := r.URL.Query().Get("siblings")
	return store.TreeOptions{
		Up:       queryInt(r, "up", 0),
		Down:     queryInt(r, "down", 0),
		Siblings: siblings == "1" || siblings == "true",
	}
}
