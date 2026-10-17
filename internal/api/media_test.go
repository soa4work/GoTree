package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/store"
)

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// upload sends one file and returns the status and body.
func (c *client) upload(path, name string, data []byte) (int, []byte) {
	c.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		c.t.Fatal(err)
	}
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func TestMediaUpload(t *testing.T) {
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	srv := testServers[ts]

	var anna store.Person
	c.call("POST", "/api/persons", map[string]any{"givenNames": "Anna"}, http.StatusCreated, &anna)

	photo := jpegBytes(t, 300, 200)
	status, body := c.upload(fmt.Sprintf("/api/media?entityType=person&entityId=%d", anna.ID), "Hochzeit 1950.jpg", photo)
	if status != http.StatusCreated {
		t.Fatalf("upload: %d %s", status, body)
	}
	var m store.Media
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m.Kind != "image" || m.Mime != "image/jpeg" || *m.Width != 300 || *m.Height != 200 || m.Title != "Hochzeit 1950" || len(m.Links) != 1 {
		t.Errorf("media: %+v", m)
	}

	// The same bytes again: same record, 200 instead of 201.
	status, body = c.upload("/api/media", "copy.jpg", photo)
	var again store.Media
	_ = json.Unmarshal(body, &again)
	if status != http.StatusOK || again.ID != m.ID {
		t.Errorf("duplicate upload: %d %+v", status, again)
	}

	if status, _ := c.upload("/api/media", "evil.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)); status != http.StatusUnsupportedMediaType {
		t.Errorf("svg: status %d, want 415", status)
	}
	if status, _ := c.upload("/api/media", "big.jpg", append(jpegBytes(t, 8, 8), make([]byte, 2<<20)...)); status != http.StatusRequestEntityTooLarge {
		t.Errorf("too large: status %d, want 413", status)
	}

	// A tiny PNG declaring 40000x40000 pixels is refused up front.
	ihdr := append([]byte("IHDR"), 0, 0, 0x9c, 0x40, 0, 0, 0x9c, 0x40, 8, 6, 0, 0, 0)
	bomb := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0d"), ihdr...)
	bomb = binary.BigEndian.AppendUint32(bomb, crc32.ChecksumIEEE(ihdr))
	if status, body := c.upload("/api/media", "bomb.png", bomb); status != http.StatusUnprocessableEntity {
		t.Errorf("huge image: status %d %s, want 422", status, body)
	}
	// Nothing of it is kept: no stored file, no leftover upload, no record.
	sum := sha256.Sum256(bomb)
	if p, _ := srv.Files.Path(hex.EncodeToString(sum[:])); p != "" {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("rejected image was stored: %v", err)
		}
	}
	if left, _ := os.ReadDir(filepath.Join(srv.Files.Root, "tmp")); len(left) != 0 {
		t.Errorf("%d temporary files left", len(left))
	}
	lr, err := c.http.Get(c.base + "/api/media")
	if err != nil {
		t.Fatal(err)
	}
	list, _ := io.ReadAll(lr.Body)
	lr.Body.Close()
	if strings.Contains(string(list), "bomb.png") {
		t.Errorf("rejected image has a record: %s", list)
	}

	// The original, with headers that keep it inert, and a range request.
	resp, err := c.http.Get(fmt.Sprintf("%s/api/media/%d/file", c.base, m.ID))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got, photo) || resp.Header.Get("Content-Type") != "image/jpeg" ||
		!strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("file response: %d bytes, headers %v", len(got), resp.Header)
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/media/%d/file", c.base, m.ID), nil)
	req.Header.Set("Range", "bytes=0-9")
	resp, err = c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	part, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || len(part) != 10 {
		t.Errorf("range: %d, %d bytes", resp.StatusCode, len(part))
	}

	resp, err = c.http.Get(fmt.Sprintf("%s/api/media/%d/thumb?size=128", c.base, m.ID))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(resp.Body)
	resp.Body.Close()
	if err != nil || cfg.Width != 128 || cfg.Height != 85 {
		t.Errorf("thumb: %+v %v", cfg, err)
	}

	// Face tag and portrait.
	c.call("POST", fmt.Sprintf("/api/media/%d/regions", m.ID), map[string]any{"personId": anna.ID, "x": 0.1, "y": 0.1, "w": 0.3, "h": 0.4}, http.StatusCreated, &m)
	rid := m.Regions[0].ID
	var p store.Person
	c.call("PUT", fmt.Sprintf("/api/persons/%d/portrait", anna.ID), map[string]any{"mediaId": m.ID, "regionId": rid}, http.StatusOK, &p)
	if p.Portrait == nil || *p.Portrait.RegionID != rid {
		t.Errorf("portrait: %+v", p.Portrait)
	}
	resp, err = c.http.Get(fmt.Sprintf("%s/api/media/%d/thumb?size=128&region=%d", c.base, m.ID, rid))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("cropped thumb: %d", resp.StatusCode)
	}

	var gallery []store.MediaRef
	c.call("GET", fmt.Sprintf("/api/media?entityType=person&entityId=%d", anna.ID), nil, http.StatusOK, &gallery)
	if len(gallery) != 1 {
		t.Errorf("gallery: %+v", gallery)
	}

	// Deleting the only use removes the file from disk.
	var sha string
	if err := srv.Store.DB.QueryRow(`SELECT sha256 FROM media WHERE id = ?`, m.ID).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	path, _ := srv.Files.Path(sha)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist before delete: %v", err)
	}
	c.call("DELETE", fmt.Sprintf("/api/media/%d", m.ID), nil, http.StatusNoContent, nil)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still on disk after delete: %v", err)
	}
	c.call("GET", fmt.Sprintf("/api/media/%d/file", m.ID), nil, http.StatusNotFound, nil)
}
