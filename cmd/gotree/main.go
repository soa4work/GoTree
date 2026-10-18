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

	// baseCtx is the parent of every request context; it is cancelled only
	// when the graceful shutdown ran out of time, to abort what is still running.
	baseCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	var inflight sync.WaitGroup
	srv := newHTTPServer(cfg.ListenAddr, trackRequests(&inflight, (&api.Server{
		Store:     st,
		Files:     media.Files{Root: cfg.MediaDir()},
		MaxUpload: int64(cfg.MaxUploadMB) << 20,
		Version:   version,
		Frontend:  web.Dist(),
		Log:       log,

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

// newHTTPServer builds the server with the timeouts that are safe for every
// route. There is deliberately no global ReadTimeout or WriteTimeout: they
// would cut off legitimate large uploads, imports, backups and GEDZIP
// downloads. Slow request bodies are bounded per handler instead, with a
// read deadline sized to the upload limit (api.Server.limitUpload). Request
// contexts derive from base.
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
