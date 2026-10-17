// Command gotree runs the GoTree server: the JSON API and the embedded web UI.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

	srv := newHTTPServer(cfg.ListenAddr, (&api.Server{
		Store:     st,
		Files:     media.Files{Root: cfg.MediaDir()},
		MaxUpload: int64(cfg.MaxUploadMB) << 20,
		Version:   version,
		Frontend:  web.Dist(),
		Log:       log,
	}).Handler())

	errc := make(chan error, 1)
	go func() {
		log.Info("gotree listening", "addr", cfg.ListenAddr, "version", version, "data", cfg.DataDir)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		// conn.Close is deferred, so it runs only after Shutdown has returned:
		// in-flight handlers (backup, export, import) are done by then, or the
		// grace period expired and the connections are cut below.
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}

// shutdownTimeout is how long in-flight requests get to finish on SIGINT or
// SIGTERM, before the remaining connections are closed.
const shutdownTimeout = 30 * time.Second

// newHTTPServer builds the server with the timeouts that are safe for every
// route. There is deliberately no global ReadTimeout or WriteTimeout: they
// would cut off legitimate large uploads, imports, backups and GEDZIP
// downloads. Slow request bodies are bounded per handler instead, with a
// read deadline sized to the upload limit (api.Server.limitUpload).
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
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
