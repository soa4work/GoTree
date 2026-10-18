package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/store"
)

func (s *Server) sourceRoutes(r chi.Router) {
	r.Route("/repositories", func(r chi.Router) {
		r.Get("/", s.listRepositories)
		r.Post("/", s.createRepository)
		r.Put("/{id}", s.updateRepository)
		r.Delete("/{id}", s.deleteRepository)
	})
	r.Route("/sources", func(r chi.Router) {
		r.Get("/", s.listSources)
		r.Post("/", s.createSource)
		r.Get("/{id}", s.getSource)
		r.Put("/{id}", s.updateSource)
		r.Delete("/{id}", s.deleteSource)
	})
	r.Route("/citations", func(r chi.Router) {
		r.Post("/", s.createCitation)
		r.Get("/{id}", s.getCitation)
		r.Put("/{id}", s.updateCitation)
		r.Delete("/{id}", s.deleteCitation)
		r.Delete("/{id}/links/{type}/{entityID}", s.unlinkCitation)
	})
}

func (s *Server) listRepositories(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListRepositories(r.Context(), actorFrom(r), r.URL.Query().Get("q"))
	s.respond(w, http.StatusOK, list, err)
}

func (s *Server) createRepository(w http.ResponseWriter, r *http.Request) {
	var in store.RepositoryInput
	if decode(w, r, &in) {
		repo, err := s.Store.CreateRepository(r.Context(), actorFrom(r), in)
		s.respond(w, http.StatusCreated, repo, err)
	}
}

func (s *Server) updateRepository(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.RepositoryInput
	if ok && decode(w, r, &in) {
		repo, err := s.Store.UpdateRepository(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusOK, repo, err)
	}
}

func (s *Server) deleteRepository(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.DeleteRepository(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) listSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSources(r.Context(), actorFrom(r), r.URL.Query().Get("q"), queryInt(r, "limit", 100), queryInt(r, "offset", 0))
	s.respond(w, http.StatusOK, list, err)
}

func (s *Server) createSource(w http.ResponseWriter, r *http.Request) {
	var in store.SourceInput
	if decode(w, r, &in) {
		src, err := s.Store.CreateSource(r.Context(), actorFrom(r), in)
		s.respond(w, http.StatusCreated, src, err)
	}
}

func (s *Server) getSource(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		src, err := s.Store.GetSource(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, src, err)
	}
}

func (s *Server) updateSource(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.SourceInput
	if ok && decode(w, r, &in) {
		src, err := s.Store.UpdateSource(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusOK, src, err)
	}
}

func (s *Server) deleteSource(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.DeleteSource(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) createCitation(w http.ResponseWriter, r *http.Request) {
	var in store.CitationInput
	if decode(w, r, &in) {
		c, err := s.Store.CreateCitation(r.Context(), actorFrom(r), in)
		s.respond(w, http.StatusCreated, c, err)
	}
}

func (s *Server) getCitation(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		c, err := s.Store.GetCitation(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, c, err)
	}
}

func (s *Server) updateCitation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	var in store.CitationInput
	if ok && decode(w, r, &in) {
		c, err := s.Store.UpdateCitation(r.Context(), actorFrom(r), id, in)
		s.respond(w, http.StatusOK, c, err)
	}
}

func (s *Server) deleteCitation(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.DeleteCitation(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) unlinkCitation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	entityID, ok := pathID(w, r, "entityID")
	if !ok {
		return
	}
	s.noContent(w, s.Store.Unlink(r.Context(), actorFrom(r), id, chi.URLParam(r, "type"), entityID))
}
