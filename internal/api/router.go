// Package api wires the HTTP routes: the JSON API under /api and the
// embedded single-page app for everything else.
package api

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
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
	// KeepPhotoLocation serves photos with their GPS position; by default it is
	// removed from files leaving the server (the parsed position stays in the database).
	KeepPhotoLocation bool

	throttle *loginThrottle
	// fileLocks keeps "this file is unused, remove it" and an upload of the same
	// content from interleaving.
	fileLocks media.HashLocks

	// TrustedProxies are the reverse proxies whose X-Forwarded-For names the
	// client (GOTREE_TRUSTED_PROXIES). Empty: the TCP peer is the client.
	TrustedProxies []netip.Prefix

	// verifySlot lets one export check run at a time; each one builds a whole database.
	verifySlot chan struct{}
}

// Handler builds the complete HTTP handler.
func (s *Server) Handler() http.Handler {
	if s.throttle == nil {
		s.throttle = newLoginThrottle()
		s.throttle.onFull = func() {
			s.Log.Warn("login throttle is full of locked keys; refusing logins from unknown addresses")
		}
	}
	if s.verifySlot == nil {
		s.verifySlot = make(chan struct{}, 1)
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.NoCache)
		r.Use(bodyDeadline)
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

		// Account settings need a login but no tree, and viewers may change
		// their own password, so these sit outside requireEditor.
		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)
			r.Get("/auth/sessions", s.listSessions)
			r.Delete("/auth/sessions/{id}", s.endSession)
			r.Post("/auth/logout-all", s.logoutAll)
			r.Post("/auth/password", s.changePassword)
		})

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
