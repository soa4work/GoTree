package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/store"
)

func (s *Server) treeRoutes(r chi.Router) {
	r.Get("/trees", s.listTrees)
	r.Post("/trees", s.createTree)
	r.Put("/trees/{id}", s.renameTree)
	r.Put("/auth/tree", s.switchTree)
	r.Get("/trees/{id}/members", s.listMembers)
	r.Post("/trees/{id}/members", s.addMember)
	r.Put("/trees/{id}/members/{userId}", s.setMemberRole)
	r.Delete("/trees/{id}/members/{userId}", s.removeMember)
}

// respondAccount is respond with the account and membership errors of the
// store mapped to their own codes.
func (s *Server) respondAccount(w http.ResponseWriter, status int, v any, err error) {
	switch {
	case errors.Is(err, store.ErrAdminOnly):
		writeError(w, http.StatusForbidden, CodeAdminOnly, err.Error())
	case errors.Is(err, store.ErrLastAdmin):
		writeError(w, http.StatusConflict, CodeLastAdmin, err.Error())
	case errors.Is(err, store.ErrLastOwner):
		writeError(w, http.StatusConflict, CodeLastOwner, err.Error())
	case errors.Is(err, store.ErrDeleteSelf):
		writeError(w, http.StatusConflict, CodeDeleteSelf, err.Error())
	case errors.Is(err, store.ErrResetOwnPassword):
		writeError(w, http.StatusConflict, CodeResetOwnPassword, err.Error())
	case err == nil && v == nil:
		w.WriteHeader(status)
	default:
		s.respond(w, status, v, err) // ErrOwnerOnly → 403 owner_only, ErrNotFound → 404, 422 …
	}
}

type treeNameInput struct {
	Name string `json:"name"`
}

func (s *Server) listTrees(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	list, err := s.Store.ListTrees(r.Context(), sess.User.ID, sess.TreeID)
	s.respondAccount(w, http.StatusOK, list, err)
}

func (s *Server) createTree(w http.ResponseWriter, r *http.Request) {
	var in treeNameInput
	if !decode(w, r, &in) {
		return
	}
	sess := sessionFrom(r.Context())
	t, err := s.Store.CreateTree(r.Context(), sess.User.ID, in.Name)
	if err == nil {
		s.Log.Info("tree created", "user", sess.User.Username, "tree", t.ID)
	}
	s.respondAccount(w, http.StatusCreated, t, err)
}

func (s *Server) renameTree(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in treeNameInput
	if !decode(w, r, &in) {
		return
	}
	t, err := s.Store.RenameTree(r.Context(), sessionFrom(r.Context()).User.ID, id, in.Name)
	s.respondAccount(w, http.StatusOK, t, err)
}

// switchTree moves this session to another tree of the user and answers with
// the new auth state.
func (s *Server) switchTree(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TreeID int64 `json:"treeId"`
	}
	if !decode(w, r, &in) {
		return
	}
	// requireUser has already checked the cookie.
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, CodeNotLoggedIn, "not logged in")
		return
	}
	sess := sessionFrom(r.Context())
	if err := s.Store.SwitchTree(r.Context(), c.Value, sess.User.ID, in.TreeID); err != nil {
		s.respondAccount(w, http.StatusOK, nil, err)
		return
	}
	s.authState(w, r)
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.Store.ListMembers(r.Context(), sessionFrom(r.Context()).User.ID, id)
	s.respondAccount(w, http.StatusOK, list, err)
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in store.MemberInput
	if !decode(w, r, &in) {
		return
	}
	sess := sessionFrom(r.Context())
	m, err := s.Store.AddMember(r.Context(), sess.User.ID, id, in)
	if err == nil {
		s.Log.Info("member added", "by", sess.User.Username, "tree", id, "user", m.Username, "role", m.Role)
	}
	s.respondAccount(w, http.StatusCreated, m, err)
}

func (s *Server) setMemberRole(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	memberID, ok := pathID(w, r, "userId")
	if !ok {
		return
	}
	var in struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	sess := sessionFrom(r.Context())
	m, err := s.Store.SetMemberRole(r.Context(), sess.User.ID, id, memberID, in.Role)
	if err == nil {
		s.Log.Info("member role changed", "by", sess.User.Username, "tree", id, "user", m.Username, "role", m.Role)
	}
	s.respondAccount(w, http.StatusOK, m, err)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	memberID, ok := pathID(w, r, "userId")
	if !ok {
		return
	}
	sess := sessionFrom(r.Context())
	err := s.Store.RemoveMember(r.Context(), sess.User.ID, id, memberID)
	if err == nil {
		s.Log.Info("member removed", "by", sess.User.Username, "tree", id, "user", memberID)
	}
	s.respondAccount(w, http.StatusNoContent, nil, err)
}
