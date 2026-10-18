package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestHealthURL(t *testing.T) {
	for addr, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/api/health",
		"0.0.0.0:8080":   "http://127.0.0.1:8080/api/health",
		"[::]:8080":      "http://[::1]:8080/api/health",
		"127.0.0.1:9000": "http://127.0.0.1:9000/api/health",
		"tree.lan:80":    "http://tree.lan:80/api/health",
	} {
		if got, err := healthURL(addr); err != nil || got != want {
			t.Errorf("healthURL(%q) = %q, %v; want %q", addr, got, err, want)
		}
	}
	if _, err := healthURL("8080"); err == nil {
		t.Error("healthURL(\"8080\"): want error for an address without a port")
	}
}

func TestHealthcheck(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusOK)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(int(status.Load()))
	}))
	addr := strings.TrimPrefix(srv.URL, "http://")
	if err := healthcheck(context.Background(), addr); err != nil {
		t.Errorf("healthy server: %v", err)
	}
	status.Store(http.StatusServiceUnavailable)
	if err := healthcheck(context.Background(), addr); err == nil {
		t.Error("degraded server: want error")
	}
	srv.Close()
	if err := healthcheck(context.Background(), addr); err == nil {
		t.Error("no server: want error")
	}
}
