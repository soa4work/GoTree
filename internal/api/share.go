package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/store"
)

const shareKey ctxKey = 1

func shareFrom(ctx context.Context) store.Share {
	sh, _ := ctx.Value(shareKey).(store.Share)
	return sh
}

// shareRoutes are the public, read-only routes of share links. They need
// no session; the token in the path is the credential.
func (s *Server) shareRoutes(r chi.Router) {
	r.Route("/share/{token}", func(r chi.Router) {
		r.Use(s.requireShare)
		r.Get("/", s.shareInfo)
		r.Get("/persons", s.sharePersons)
		r.Get("/persons/{id}", s.sharePerson)
		r.Get("/tree/{id}", s.shareTree)
		r.Get("/onthisday", s.shareOnThisDay)
	})
}

// shareLinkRoutes manage share links and need a session.
func (s *Server) shareLinkRoutes(r chi.Router) {
	r.Get("/share-links", s.listShareLinks)
	r.Post("/share-links", s.createShareLink)
	r.Delete("/share-links/{id}", s.revokeShareLink)
	r.Post("/share-links/{id}/token", s.rotateShareLink)
	r.Get("/onthisday", s.onThisDay)
}

func (s *Server) requireShare(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, CodeShareReadOnly, "share links are read-only")
			return
		}
		sh, err := s.Store.ShareByToken(r.Context(), chi.URLParam(r, "token"))
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, CodeShareGone, "this link does not exist, has expired or was withdrawn")
			return
		}
		if err != nil {
			writeStoreError(w, s.Log, err)
			return
		}
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), shareKey, sh)))
	})
}

func (s *Server) shareInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.Store.ShareInfo(r.Context(), shareFrom(r.Context()))
	s.respond(w, http.StatusOK, info, err)
}

func (s *Server) sharePersons(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.SharePersons(r.Context(), shareFrom(r.Context()), r.URL.Query().Get("q"))
	s.respond(w, http.StatusOK, list, err)
}

func (s *Server) sharePerson(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		d, err := s.Store.SharePerson(r.Context(), shareFrom(r.Context()), id)
		s.respond(w, http.StatusOK, d, err)
	}
}

func (s *Server) shareTree(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		g, err := s.Store.ShareTree(r.Context(), shareFrom(r.Context()), id, treeOptions(r))
		s.respond(w, http.StatusOK, g, err)
	}
}

func (s *Server) shareOnThisDay(w http.ResponseWriter, r *http.Request) {
	rep, err := s.Store.ShareOnThisDay(r.Context(), shareFrom(r.Context()), queryInt(r, "month", 0), queryInt(r, "day", 0))
	s.respond(w, http.StatusOK, rep, err)
}

func (s *Server) onThisDay(w http.ResponseWriter, r *http.Request) {
	rep, err := s.Store.OnThisDay(r.Context(), actorFrom(r), queryInt(r, "month", 0), queryInt(r, "day", 0))
	s.respond(w, http.StatusOK, rep, err)
}

func (s *Server) listShareLinks(w http.ResponseWriter, r *http.Request) {
	links, err := s.Store.ListShareLinks(r.Context(), actorFrom(r))
	s.respond(w, http.StatusOK, links, err)
}

func (s *Server) createShareLink(w http.ResponseWriter, r *http.Request) {
	var in store.ShareLinkInput
	if !decode(w, r, &in) {
		return
	}
	link, err := s.Store.CreateShareLink(r.Context(), actorFrom(r), in)
	s.respond(w, http.StatusCreated, link, err)
}

func (s *Server) revokeShareLink(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.noContent(w, s.Store.RevokeShareLink(r.Context(), actorFrom(r), id))
	}
}

func (s *Server) rotateShareLink(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		link, err := s.Store.RotateShareLink(r.Context(), actorFrom(r), id)
		s.respond(w, http.StatusOK, link, err)
	}
}
