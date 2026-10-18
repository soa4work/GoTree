package api

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/db"
)

func TestBackup(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, nil)
	// With a GPS position, so a location-free copy exists; the backup keeps the original.
	photo := gpsJPEG(t)
	if status, body := c.upload("/api/media", "photo.jpg", photo); status != http.StatusCreated {
		t.Fatalf("upload: %d %s", status, body)
	}
	// Thumbnails are left out; they can be made again.
	c.do("GET", "/api/media/1/thumb?size=200", nil)
	// Nor are location-free copies.
	if status, _ := c.do("GET", "/api/media/1/file", nil); status != http.StatusOK {
		t.Fatalf("file: %d", status)
	}

	resp, err := c.http.Get(ts.URL + "/api/backup")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), `attachment; filename="gotree-backup-`) {
		t.Fatalf("status %d, headers %v", resp.StatusCode, resp.Header)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	if files["RESTORE.txt"] == nil || files["gotree.db"] == nil || len(files) != 3 {
		t.Fatalf("entries: %v", names(zr))
	}
	var mediaFile *zip.File
	for name, f := range files {
		if strings.HasPrefix(name, "media/") && !strings.Contains(name, "thumbs") && !strings.Contains(name, "clean") {
			mediaFile = f
		}
	}
	if mediaFile == nil {
		t.Fatalf("no media file: %v", names(zr))
	}
	if got := readZip(t, mediaFile); !bytes.Equal(got, photo) {
		t.Error("media file differs")
	}

	// The snapshot opens on its own and holds the data.
	path := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(path, readZip(t, files["gotree.db"]), 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM persons WHERE given_names = 'Anna'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("restored persons: %d %v", n, err)
	}

	srv := serverOf(ts)
	if _, err := srv.Store.DB.Exec(`UPDATE users SET role = 'user'`); err != nil {
		t.Fatal(err)
	}
	c.call("GET", "/api/backup", nil, http.StatusForbidden, nil)
}

func names(zr *zip.Reader) []string {
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

func readZip(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
