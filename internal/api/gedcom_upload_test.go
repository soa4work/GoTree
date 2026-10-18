package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// A multipart body that ends before its closing boundary is a broken
// upload, not one that is too large.
func TestGedcomBrokenUploadIsNotTooLarge(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	body := "--X\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.ged\"\r\nContent-Type: text/plain\r\n\r\n0 HEAD\r\n"
	req, err := http.NewRequest("POST", c.base+"/api/import/gedcom", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary=X")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), `"code":"invalid_upload"`) {
		t.Errorf("status %d: %s, want 400 invalid_upload", resp.StatusCode, raw)
	}
}
