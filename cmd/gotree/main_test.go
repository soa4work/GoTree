package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerTimeouts(t *testing.T) {
	srv := newHTTPServer(":0", http.NotFoundHandler(), context.Background())
	if srv.ReadHeaderTimeout <= 0 || srv.IdleTimeout < time.Minute {
		t.Errorf("ReadHeaderTimeout=%v IdleTimeout=%v, want both set", srv.ReadHeaderTimeout, srv.IdleTimeout)
	}
	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 {
		t.Errorf("ReadTimeout=%v WriteTimeout=%v, want none: they would cut off large transfers", srv.ReadTimeout, srv.WriteTimeout)
	}
}
