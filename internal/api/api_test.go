package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/praetorianer777/gotree/internal/db/dbtest"
	"github.com/praetorianer777/gotree/internal/media"
	"github.com/praetorianer777/gotree/internal/store"
)

func newServer(t *testing.T, frontend fstest.MapFS) *httptest.Server {
	t.Helper()
	return newServerWith(t, frontend, nil)
}

// newServerWith is newServer with a hook to adjust the Server before it starts.
func newServerWith(t *testing.T, frontend fstest.MapFS, adjust func(*Server)) *httptest.Server {
	t.Helper()
	conn := dbtest.Open(t)
	s := &Server{
		Store:     store.New(conn),
		Files:     media.Files{Root: filepath.Join(t.TempDir(), "media")},
		MaxUpload: 1 << 20,
		Version:   "1.2.3",
		Frontend:  frontend,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if adjust != nil {
		adjust(s)
	}
	ts := httptest.NewServer(s.Handler())
	testServersMu.Lock()
	testServers[ts] = s
	testServersMu.Unlock()
	t.Cleanup(func() {
		ts.Close()
		testServersMu.Lock()
		delete(testServers, ts)
		testServersMu.Unlock()
	})
	return ts
}

// testServers gives tests access to the Server behind a test server, e.g.
// to change data directly in the database.
var testServers = map[*httptest.Server]*Server{}

// testServersMu guards testServers: tests run in parallel.
var testServersMu sync.Mutex

// serverOf returns the Server behind a test server.
func serverOf(ts *httptest.Server) *Server {
	testServersMu.Lock()
	defer testServersMu.Unlock()
	return testServers[ts]
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

var builtFrontend = fstest.MapFS{
	"index.html":           {Data: []byte("<!doctype html><title>GoTree</title>")},
	"assets/index-abc1.js": {Data: []byte("console.log(1)")},
	"manifest.webmanifest": {Data: []byte("{}")},
	"icons/icon-192.png":   {Data: []byte("png")},
}

func TestHealth(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	resp, body := get(t, ts.URL+"/api/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var got healthResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := healthResponse{Status: "ok", Version: "1.2.3", Database: "ok"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-cache") {
		t.Errorf("API responses must not be cached, got Cache-Control %q", cc)
	}
}

func TestUnknownAPIRouteIsJSON404(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	resp, body := get(t, ts.URL+"/api/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q, want application/json", ct)
	}
	if !strings.Contains(body, `"error"`) {
		t.Errorf("body %q has no error field", body)
	}
}

func TestSPA(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	tests := []struct {
		path        string
		wantStatus  int
		wantBody    string
		wantCaching string
	}{
		{"/", http.StatusOK, "<title>GoTree</title>", "no-cache"},
		{"/people/42", http.StatusOK, "<title>GoTree</title>", "no-cache"},
		{"/assets/index-abc1.js", http.StatusOK, "console.log", "immutable"},
		{"/manifest.webmanifest", http.StatusOK, "{}", "no-cache"},
		{"/icons/icon-192.png", http.StatusOK, "png", "no-cache"},
		{"/assets/missing.js", http.StatusNotFound, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, body := get(t, ts.URL+tt.path)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body %q does not contain %q", body, tt.wantBody)
			}
			if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, tt.wantCaching) {
				t.Errorf("Cache-Control %q does not contain %q", cc, tt.wantCaching)
			}
			if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Error("security headers missing")
			}
		})
	}
}

func TestSPAContentSecurityPolicy(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	for _, path := range []string{"/", "/people/42", "/share/abc"} {
		resp, _ := get(t, ts.URL+path)
		csp := resp.Header.Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "frame-ancestors 'none'", "https://tile.openstreetmap.org", "connect-src 'self' https://nominatim.openstreetmap.org;"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s: CSP %q does not contain %q", path, csp, want)
			}
		}
		if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
			t.Errorf("%s: CSP %q allows inline or eval scripts", path, csp)
		}
	}
}

func TestManifestContentType(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	resp, _ := get(t, ts.URL+"/manifest.webmanifest")
	if ct := resp.Header.Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("Content-Type %q, want application/manifest+json", ct)
	}
}

func TestSPANotBuilt(t *testing.T) {
	t.Parallel()
	ts := newServer(t, fstest.MapFS{".gitkeep": {}})
	resp, body := get(t, ts.URL+"/")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", resp.StatusCode)
	}
	if !strings.Contains(body, "make build") {
		t.Errorf("body %q should explain how to build the frontend", body)
	}
}
