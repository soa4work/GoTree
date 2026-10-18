package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/praetorianer777/gotree/internal/db"
	"github.com/praetorianer777/gotree/internal/store"
)

func TestPruneSessionsRunsPeriodically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	st := store.New(conn)
	u, err := st.Setup(ctx, store.SetupInput{Username: "admin", Password: "correct horse battery"})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := conn.ExecContext(ctx, `
		INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at) VALUES (x'01', ?, ?, ?, ?)`,
		u.ID, past, past, past); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		pruneSessions(ctx, st, 10*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sessions`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d expired sessions were not pruned", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pruneSessions did not stop when the context ended")
	}
}
