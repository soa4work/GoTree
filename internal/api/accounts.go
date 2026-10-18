package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/praetorianer777/gotree/internal/store"
)

func (s *Server) accountRoutes(r chi.Router) {
	r.Route("/admin/users", func(r chi.Router) {
		r.Use(requireAdmin)
		r.Get("/", s.listAccounts)
		r.Post("/", s.createAccount)
		r.Put("/{id}", s.updateAccount)
		r.Post("/{id}/password", s.resetPassword)
		r.Delete("/{id}", s.deleteAccount)
	})
}

// requireAdmin lets only administrators through. The store checks again.
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sessionFrom(r.Context()).User.Role != store.UserRoleAdmin {
			writeError(w, http.StatusForbidden, CodeAdminOnly, "only administrators can do this")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAccounts(r.Context(), sessionFrom(r.Context()).User)
	s.respondAccount(w, http.StatusOK, list, err)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in store.AccountInput
	if !decode(w, r, &in) {
		return
	}
	by := sessionFrom(r.Context()).User
	acc, err := s.Store.CreateAccount(r.Context(), by, in)
	if err == nil {
		s.Log.Info("account created", "by", by.Username, "user", acc.Username, "role", acc.Role)
	}
	s.respondAccount(w, http.StatusCreated, acc, err)
}

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in store.AccountUpdate
	if !decode(w, r, &in) {
		return
	}
	by := sessionFrom(r.Context()).User
	acc, err := s.Store.UpdateAccount(r.Context(), by, id, in)
	if err == nil {
		s.Log.Info("account updated", "by", by.Username, "user", acc.Username, "role", acc.Role)
	}
	s.respondAccount(w, http.StatusOK, acc, err)
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	by := sessionFrom(r.Context()).User
	err := s.Store.ResetPassword(r.Context(), by, id, in.Password)
	if err == nil {
		s.Log.Info("password reset", "by", by.Username, "userId", id)
	}
	s.respondAccount(w, http.StatusNoContent, nil, err)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	by := sessionFrom(r.Context()).User
	err := s.Store.DeleteAccount(r.Context(), by, id)
	if err == nil {
		s.Log.Info("account deleted", "by", by.Username, "userId", id)
	}
	s.respondAccount(w, http.StatusNoContent, nil, err)
}
