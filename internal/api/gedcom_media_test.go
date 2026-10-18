package api

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func buildZip(t *testing.T, entries map[string][]byte, order ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entries[name]); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	return buf.Bytes()
}

// storedFiles counts the files below the media root, apart from the
// temporary ones.
func storedFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if rel, _ := filepath.Rel(root, path); err == nil && !d.IsDir() && !strings.HasPrefix(rel, "tmp"+string(filepath.Separator)) {
			n++
		}
		return nil
	})
	return n
}

func tagReason(r store.ImportReport, path string) string {
	for _, ts := range r.Tags {
		if ts.Path == path && ts.Outcome == store.TagDropped {
			return ts.Reason
		}
	}
	return ""
}

func TestImportGedzipMedia(t *testing.T) {
	// A tree with a photo goes out as a GEDZIP ...
	src := loggedIn(t, newServer(t, builtFrontend))
	var anna store.Person
	src.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, &anna)
	photo := jpegBytes(t, 300, 200)
	if status, body := src.upload(fmt.Sprintf("/api/media?entityType=person&entityId=%d", anna.ID), "Hochzeit 1950.jpg", photo); status != http.StatusCreated {
		t.Fatalf("upload: %d %s", status, body)
	}
	resp, archive := src.get("/api/export/gedcom?version=7.0&format=gedzip")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export: %d", resp.StatusCode)
	}

	// ... and comes back in another installation with its file.
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	status, body := c.upload("/api/import/gedcom", "tree.gdz", archive)
	if status != http.StatusOK {
		t.Fatalf("import: %d %s", status, body)
	}
	var report store.ImportReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if report.Counts["media"] != 1 {
		t.Fatalf("counts: %v", report.Counts)
	}
	var list []store.MediaRef
	c.call("GET", "/api/media", nil, http.StatusOK, &list)
	if len(list) != 1 {
		t.Fatalf("media: %+v", list)
	}
	var m store.Media
	c.call("GET", fmt.Sprintf("/api/media/%d", list[0].ID), nil, http.StatusOK, &m)
	if len(m.Links) != 1 || m.Links[0].EntityType != "person" || m.Title != "Hochzeit 1950" || *m.Width != 300 {
		t.Errorf("media: %+v", m)
	}
	if _, file := c.get(fmt.Sprintf("/api/media/%d/file", m.ID)); !bytes.Equal(file, photo) {
		t.Error("the file differs from the original")
	}
}

const gedWithPhoto = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Anna /Weber/
1 OBJE
2 FILE %s
3 FORM jpg
2 TITL Anna
0 TRLR
`

func TestImportGedzipFileResolution(t *testing.T) {
	photo := jpegBytes(t, 40, 30)
	for _, tc := range []struct{ name, value, entry string }{
		{"windows path, by name", `C:\Pics\photo.jpg`, "photo.jpg"},
		{"relative to the GEDCOM file", "photos/a.jpg", "dir/photos/a.jpg"},
		{"escaped, other case", "My%20Photos/B.JPG", "my photos/b.jpg"},
		{"file URL", "file:///x/photo.jpg", "photo.jpg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := loggedIn(t, newServer(t, builtFrontend))
			ged := []byte(fmt.Sprintf(gedWithPhoto, tc.value))
			archive := buildZip(t, map[string][]byte{"dir/gedcom.ged": ged, tc.entry: photo}, "dir/gedcom.ged", tc.entry)
			status, body := c.upload("/api/import/gedcom", "x.zip", archive)
			if status != http.StatusOK {
				t.Fatalf("import: %d %s", status, body)
			}
			var report store.ImportReport
			_ = json.Unmarshal(body, &report)
			if report.Counts["media"] != 1 {
				t.Errorf("counts: %v, tags %+v", report.Counts, report.Tags)
			}
		})
	}
}

func TestImportGedzipSkipsFilesThatFailTheUploadChecks(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	srv := testServers[ts]
	ihdr := append([]byte("IHDR"), 0, 0, 0x9c, 0x40, 0, 0, 0x9c, 0x40, 8, 6, 0, 0, 0)
	bomb := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0d"), ihdr...)
	bomb = binary.BigEndian.AppendUint32(bomb, crc32.ChecksumIEEE(ihdr))
	oversized := append(jpegBytes(t, 8, 8), make([]byte, 2<<20)...) // zips small, unpacks over MaxUpload

	files := []struct{ value, reason string }{
		{"https://example.com/a.jpg", "links to files on the web are not imported"},
		{"gone.jpg", "the file is not in the zip"},
		{"notes.txt", "this kind of file is not supported"},
		{"script.svg", "this kind of file is not supported"},
		{"bomb.png", "the image is larger than the pixel limit"},
		{"big.jpg", "the file is larger than the upload limit"},
		{"ok.jpg", ""},
	}
	var ged strings.Builder
	ged.WriteString("0 HEAD\n1 GEDC\n2 VERS 5.5.1\n1 CHAR UTF-8\n0 @I1@ INDI\n1 NAME Anna /Weber/\n")
	for _, f := range files {
		fmt.Fprintf(&ged, "1 OBJE\n2 FILE %s\n", f.value)
	}
	ged.WriteString("0 TRLR\n")
	archive := buildZip(t, map[string][]byte{
		"gedcom.ged": []byte(ged.String()), "notes.txt": []byte("hello"), "script.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"bomb.png": bomb, "big.jpg": oversized, "ok.jpg": jpegBytes(t, 10, 10),
	}, "gedcom.ged", "notes.txt", "script.svg", "bomb.png", "big.jpg", "ok.jpg")

	status, body := c.upload("/api/import/gedcom", "x.zip", archive)
	if status != http.StatusOK {
		t.Fatalf("import: %d %s", status, body)
	}
	var report store.ImportReport
	_ = json.Unmarshal(body, &report)
	if report.Counts["media"] != 1 || report.Counts["persons"] != 1 {
		t.Errorf("counts: %v", report.Counts)
	}
	if got := tagReason(report, "INDI.OBJE.FILE"); got == "" {
		t.Errorf("no reasons reported: %+v", report.Tags)
	}
	seen := map[string]bool{}
	for _, ts := range report.Tags {
		if ts.Path == "INDI.OBJE.FILE" && ts.Outcome == store.TagDropped {
			seen[ts.Reason] = true
		}
	}
	for _, f := range files {
		if f.reason != "" && !seen[f.reason] {
			t.Errorf("missing reason %q in %+v", f.reason, report.Tags)
		}
	}
	// Only the good file was stored.
	if n := storedFiles(t, srv.Files.Root); n != 1 {
		t.Errorf("stored files: %d, want 1", n)
	}
}

func TestImportGedzipIntoNonEmptyTreeStoresNothing(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	srv := testServers[ts]
	var anna store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, &anna)

	archive := buildZip(t, map[string][]byte{
		"gedcom.ged": []byte(fmt.Sprintf(gedWithPhoto, "a.jpg")), "a.jpg": jpegBytes(t, 20, 20),
	}, "gedcom.ged", "a.jpg")
	if status, body := c.upload("/api/import/gedcom", "x.zip", archive); status != http.StatusConflict {
		t.Fatalf("import: %d %s", status, body)
	}
	if n := storedFiles(t, srv.Files.Root); n != 0 {
		t.Errorf("a refused import left %d files", n)
	}
}

func TestImportGedzipReplaceKeepsFileOfExistingMedia(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	srv := testServers[ts]
	archive := buildZip(t, map[string][]byte{
		"gedcom.ged": []byte(fmt.Sprintf(gedWithPhoto, "a.jpg")), "a.jpg": jpegBytes(t, 20, 20),
	}, "gedcom.ged", "a.jpg")
	for _, q := range []string{"", "?mode=replace"} {
		if status, body := c.upload("/api/import/gedcom"+q, "x.zip", archive); status != http.StatusOK {
			t.Fatalf("import%s: %d %s", q, status, body)
		}
	}
	var list []store.MediaRef
	c.call("GET", "/api/media", nil, http.StatusOK, &list)
	if len(list) != 1 || storedFiles(t, srv.Files.Root) != 1 {
		t.Errorf("media %+v, files %d", list, storedFiles(t, srv.Files.Root))
	}
}
