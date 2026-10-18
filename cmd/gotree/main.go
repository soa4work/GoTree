// Command gotree runs the GoTree server: the JSON API and the embedded web UI.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/praetorianer777/gotree/internal/api"
	"github.com/praetorianer777/gotree/internal/config"
	"github.com/praetorianer777/gotree/internal/db"
	"github.com/praetorianer777/gotree/internal/media"
	"github.com/praetorianer777/gotree/internal/store"
	"github.com/praetorianer777/gotree/web"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.FromOS()
	if err != nil {
		return err
	}
	if cfg.HealthCheck {
		return healthcheck(context.Background(), cfg.ListenAddr)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := db.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer conn.Close()

	st := store.New(conn)
	if err := bootstrapAdmin(ctx, st, cfg, log); err != nil {
		return err
	}
	if err := st.PruneSessions(ctx); err != nil {
		return err
	}
	go pruneSessions(ctx, st, sessionPruneInterval, log)

	// baseCtx is the parent of every request context; it is cancelled only
	// when the graceful shutdown ran out of time, to abort what is still running.
	baseCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	var inflight sync.WaitGroup
	srv := newHTTPServer(cfg.ListenAddr, trackRequests(&inflight, (&api.Server{
		Store:             st,
		Files:             media.Files{Root: cfg.MediaDir()},
		MaxUpload:         int64(cfg.MaxUploadMB) << 20,
		KeepPhotoLocation: cfg.KeepPhotoLocation,
		Version:           version,
		Frontend:          web.Dist(),
		Log:               log,

		TrustedProxies: cfg.TrustedProxies,
	}).Handler()), baseCtx)

	errc := make(chan error, 1)
	go func() {
		log.Info("gotree listening", "addr", cfg.ListenAddr, "version", version, "data", cfg.DataDir, "trustedProxies", len(cfg.TrustedProxies))
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		// Restore the default signal behavior: a second SIGINT or SIGTERM
		// kills the process at once.
		stop()
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		// conn.Close is deferred, so it runs after this returns. Shutdown
		// waits for in-flight requests (backup, export, import); when it gives
		// up, the connections are closed, the requests' contexts are cancelled
		// (which rolls back a running import) and handlers get a few more
		// seconds to return before the database is closed under them.
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			cancelRequests()
			if !waitTimeout(&inflight, handlerWait) {
				log.Error("requests still running, closing the database anyway")
			}
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}

const (
	// shutdownTimeout is how long in-flight requests get to finish on SIGINT
	// or SIGTERM.
	shutdownTimeout = 30 * time.Second
	// handlerWait is how long handlers get to return after their contexts
	// were cancelled because shutdownTimeout expired.
	handlerWait = 5 * time.Second
)

// sessionPruneInterval is how often expired sessions are removed.
const sessionPruneInterval = time.Hour

// pruneSessions removes expired sessions every interval until ctx ends.
func pruneSessions(ctx context.Context, st *store.Store, every time.Duration, log *slog.Logger) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := st.PruneSessions(ctx); err != nil && ctx.Err() == nil {
				log.Warn("prune sessions", "err", err)
			}
		}
	}
}

// newHTTPServer builds the server with the timeouts that are safe for every
// route. There is deliberately no global ReadTimeout or WriteTimeout: they
// would cut off legitimate large uploads, imports, backups and GEDZIP
// downloads. Slow request bodies are bounded per request instead: JSON bodies
// by a short deadline (api bodyDeadline), uploads by one sized to the upload
// limit (api.Server.limitUpload). Request contexts derive from base.
func newHTTPServer(addr string, h http.Handler, base context.Context) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return base },
	}
}

// trackRequests counts the requests being handled in wg.
func trackRequests(wg *sync.WaitGroup, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wg.Add(1)
		defer wg.Done()
		h.ServeHTTP(w, r)
	})
}

// waitTimeout waits for wg and reports whether it finished within d.
func waitTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// bootstrapAdmin creates the first account from the environment, for
// unattended installs. Without GOTREE_ADMIN_PASSWORD the web UI asks for it.
func bootstrapAdmin(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) error {
	if cfg.AdminPassword == "" {
		return nil
	}
	need, err := st.NeedsSetup(ctx)
	if err != nil || !need {
		return err
	}
	if _, err := st.Setup(ctx, store.SetupInput{Username: cfg.AdminUser, Password: cfg.AdminPassword}); err != nil {
		return fmt.Errorf("create admin from GOTREE_ADMIN_PASSWORD: %w", err)
	}
	log.Info("created admin account from the environment", "user", cfg.AdminUser)
	return nil
}

// healthcheck asks the server listening on addr for /api/health and fails
// unless it answers 200. The distroless image has no shell or curl, so the
// Docker HEALTHCHECK runs the binary itself.
func healthcheck(ctx context.Context, addr string) error {
	url, err := healthURL(addr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// No proxy: a proxy variable in the container must not redirect the check.
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: %s answered %s", url, resp.Status)
	}
	return nil
}

// healthURL turns a listen address into the URL of the health endpoint. A
// wildcard host (":8080", "0.0.0.0:8080", "[::]:8080") is reached on loopback.
func healthURL(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("healthcheck: listen address %q: %w", addr, err)
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/api/health", nil
}
