// Package api wires the HTTP routes: the JSON API under /api and the
// embedded single-page app for everything else.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/praetorianer777/gotree/internal/media"
	"github.com/praetorianer777/gotree/internal/store"
)

// Server holds the dependencies of the HTTP handlers.
type Server struct {
	Store *store.Store
	// Files holds uploaded media; MaxUpload is the size limit in bytes.
	Files     media.Files
	MaxUpload int64
	Version   string
	// Frontend is the built SPA (web/dist); see web.Dist.
	Frontend fs.FS
	Log      *slog.Logger

	throttle *loginThrottle
	// verifySlot lets one export check run at a time; each one builds a whole database.
	verifySlot chan struct{}
}

// Handler builds the complete HTTP handler.
func (s *Server) Handler() http.Handler {
	if s.throttle == nil {
		s.throttle = newLoginThrottle()
	}
	if s.verifySlot == nil {
		s.verifySlot = make(chan struct{}, 1)
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.NoCache)
		// Refuses state-changing requests from other origins (CSRF), based on
		// Sec-Fetch-Site and Origin; same-origin requests and non-browser
		// clients pass.
		r.Use(http.NewCrossOriginProtection().Handler)

		r.Get("/health", s.health)
		r.Get("/auth/state", s.authState)
		r.Post("/auth/setup", s.setup)
		r.Post("/auth/login", s.login)
		r.Post("/auth/logout", s.logout)
		s.shareRoutes(r)

		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Use(requireEditor)
			s.resourceRoutes(r)
			s.sourceRoutes(r)
			s.mediaRoutes(r)
			s.qualityRoutes(r)
			r.Post("/import/gedcom", s.importGEDCOM)
			r.Get("/export/gedcom", s.exportGEDCOM)
			r.Post("/export/verify", s.verifyExport)
			r.Get("/backup", s.backup)
			s.shareLinkRoutes(r)
			s.calendarFeedRoutes(r)
			s.researchRoutes(r)
			s.transcriptionRoutes(r)
			s.heirloomRoutes(r)
			s.statsRoutes(r)
		})

		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, CodeNotFound, "not found")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed, "method not allowed")
		})
	})

	r.Get("/ical/{file}", s.icalFeed)
	r.Handle("/*", spaHandler(s.Frontend))
	return r
}

type healthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Database string `json:"database"`
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	resp := healthResponse{Status: "ok", Version: s.Version, Database: "ok"}
	status := http.StatusOK
	if err := s.Store.DB.PingContext(ctx); err != nil {
		s.Log.Error("health: database ping failed", "err", err)
		resp.Status, resp.Database = "degraded", "unreachable"
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, resp)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
