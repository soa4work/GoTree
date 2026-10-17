package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestImportEndpoint(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	ged, err := os.ReadFile("../store/testdata/ancestry-551.ged")
	if err != nil {
		t.Fatal(err)
	}

	status, body := c.upload("/api/import/gedcom", "tree.ged", ged)
	if status != http.StatusOK {
		t.Fatalf("import: %d %s", status, body)
	}
	var report store.ImportReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.Counts["persons"] != 4 {
		t.Errorf("report: %+v", report.Counts)
	}

	// The tree now has data: a second import needs mode=replace.
	if status, _ := c.upload("/api/import/gedcom", "tree.ged", ged); status != http.StatusConflict {
		t.Errorf("second import: %d", status)
	}

	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	f, _ := zw.Create("gedcom.ged")
	_, _ = f.Write(ged)
	_, _ = zw.Create("media/photo.jpg")
	zw.Close()
	if status, body := c.upload("/api/import/gedcom?mode=replace", "tree.zip", zipped.Bytes()); status != http.StatusOK {
		t.Errorf("zip import: %d %s", status, body)
	}

	if status, _ := c.upload("/api/import/gedcom?mode=replace", "x.ged", []byte("<html>nope</html>")); status != http.StatusUnprocessableEntity {
		t.Errorf("not a gedcom: %d", status)
	}
	var empty bytes.Buffer
	zw = zip.NewWriter(&empty)
	_, _ = zw.Create("readme.txt")
	zw.Close()
	if status, _ := c.upload("/api/import/gedcom?mode=replace", "x.zip", empty.Bytes()); status != http.StatusUnprocessableEntity {
		t.Errorf("zip without gedcom: %d", status)
	}
}

func personCount(t *testing.T, c *client) int {
	t.Helper()
	var list struct {
		Total int `json:"total"`
	}
	c.call("GET", "/api/persons?limit=1", nil, http.StatusOK, &list)
	return list.Total
}

func TestImportRejectsOversizedGedcom(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	ged, _ := os.ReadFile("../store/testdata/ancestry-551.ged")
	if status, body := c.upload("/api/import/gedcom", "tree.ged", ged); status != http.StatusOK {
		t.Fatalf("import: %d %s", status, body)
	}
	before := personCount(t, c)

	// A zip well within the upload limit that unpacks past the GEDCOM limit
	// (4 x MaxUpload): must be refused, not truncated, and the tree stays.
	var zipped bytes.Buffer
	zw := zip.NewWriter(&zipped)
	f, _ := zw.Create("gedcom.ged")
	_, _ = f.Write(ged)
	_, _ = f.Write(bytes.Repeat([]byte("1 NOTE padding\n"), 5<<20/15))
	zw.Close()
	if zipped.Len() > 1<<20 {
		t.Fatalf("test zip too big: %d", zipped.Len())
	}
	status, body := c.upload("/api/import/gedcom?mode=replace", "bomb.zip", zipped.Bytes())
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized zip entry: %d %s", status, body)
	}
	if !strings.Contains(string(body), "too large") {
		t.Errorf("message: %s", body)
	}
	if got := personCount(t, c); got != before || before == 0 {
		t.Errorf("tree changed: %d -> %d", before, got)
	}
}

func TestImportReplaceIsOwnerOnly(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	ged, _ := os.ReadFile("../store/testdata/ancestry-551.ged")
	if status, body := c.upload("/api/import/gedcom", "tree.ged", ged); status != http.StatusOK {
		t.Fatalf("import: %d %s", status, body)
	}
	before := personCount(t, c)

	if _, err := testServers[ts].Store.DB.Exec(`UPDATE tree_members SET role = 'editor'`); err != nil {
		t.Fatal(err)
	}
	status, body := c.upload("/api/import/gedcom?mode=replace", "tree.ged", ged)
	if status != http.StatusForbidden || !strings.Contains(string(body), "owner") {
		t.Fatalf("editor replace: %d %s", status, body)
	}
	if got := personCount(t, c); got != before {
		t.Errorf("tree changed: %d -> %d", before, got)
	}
	// Other modes stay open to editors (the tree is not empty: 409).
	if status, _ := c.upload("/api/import/gedcom", "tree.ged", ged); status != http.StatusConflict {
		t.Errorf("editor import into non-empty: %d", status)
	}

	if _, err := testServers[ts].Store.DB.Exec(`UPDATE tree_members SET role = 'owner'`); err != nil {
		t.Fatal(err)
	}
	if status, body := c.upload("/api/import/gedcom?mode=replace", "tree.ged", ged); status != http.StatusOK {
		t.Errorf("owner replace: %d %s", status, body)
	}
}

func TestExportEndpoints(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	ged, _ := os.ReadFile("../store/testdata/ancestry-551.ged")
	if status, _ := c.upload("/api/import/gedcom", "tree.ged", ged); status != http.StatusOK {
		t.Fatalf("import: %d", status)
	}
	if status, _ := c.upload("/api/media", "p.jpg", jpegBytes(t, 20, 10)); status != http.StatusCreated {
		t.Fatalf("media: %d", status)
	}

	resp, err := c.http.Get(c.base + "/api/export/gedcom?version=5.5.1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.HasPrefix(body, []byte("0 HEAD\n")) || !strings.Contains(resp.Header.Get("Content-Disposition"), ".ged") {
		t.Errorf("ged export: %q %v", body[:min(40, len(body))], resp.Header)
	}

	resp, err = c.http.Get(c.base + "/api/export/gedcom?version=7.0&format=gedzip")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	// All media are packed, linked to someone or not, so the archive is a
	// complete copy.
	if len(names) != 2 || names[0] != "gedcom.ged" || !strings.HasPrefix(names[1], "media/") || !strings.Contains(resp.Header.Get("Content-Disposition"), ".gdz") {
		t.Errorf("gedzip: %v %v", names, resp.Header)
	}

	var report store.VerifyReport
	c.call("GET", "/api/export/verify?version=7.0", nil, http.StatusOK, &report)
	if !report.OK || len(report.Rows) == 0 {
		t.Errorf("verify: %+v", report)
	}
	c.call("GET", "/api/export/gedcom?privacy=nobody", nil, http.StatusUnprocessableEntity, nil)
}
