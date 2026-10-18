package api

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praetorianer777/gotree/internal/media"
	"github.com/praetorianer777/gotree/internal/store"
)

// gpsJPEG is a small JPEG whose EXIF holds 51°20'24"N 12°22'12"E.
func gpsJPEG(t *testing.T) []byte {
	t.Helper()
	le := binary.LittleEndian
	u16 := func(v int) []byte { return le.AppendUint16(nil, uint16(v)) }
	u32 := func(v int) []byte { return le.AppendUint32(nil, uint32(v)) }
	rationals := func(a, b, c int) []byte {
		var out []byte
		for _, v := range []int{a, b, c} {
			out = append(out, u32(v)...)
			out = append(out, u32(1)...)
		}
		return out
	}
	entry := func(tag, typ, count int, value []byte) []byte {
		out := append(append(u16(tag), u16(typ)...), u32(count)...)
		return append(out, append(value, make([]byte, 4-len(value))...)...)
	}
	tiff := append([]byte("II*\x00"), u32(8)...)
	// IFD0 at 8 (one entry: the GPS IFD at 26), GPS IFD at 26, values at 80 and 104.
	tiff = append(tiff, u16(1)...)
	tiff = append(tiff, entry(0x8825, 4, 1, u32(26))...)
	tiff = append(tiff, u32(0)...)
	tiff = append(tiff, u16(4)...)
	tiff = append(tiff, entry(1, 2, 2, []byte("N\x00"))...)
	tiff = append(tiff, entry(2, 5, 3, u32(80))...)
	tiff = append(tiff, entry(3, 2, 2, []byte("E\x00"))...)
	tiff = append(tiff, entry(4, 5, 3, u32(104))...)
	tiff = append(tiff, u32(0)...)
	tiff = append(tiff, rationals(51, 20, 24)...)
	tiff = append(tiff, rationals(12, 22, 12)...)

	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := append([]byte{0xFF, 0xE1}, binary.BigEndian.AppendUint16(nil, uint16(len(payload)+2))...)
	seg = append(seg, payload...)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 30)), nil); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), seg...), b[2:]...)
}

func hasPosition(t *testing.T, data []byte) bool {
	t.Helper()
	m := media.ReadMeta(bytes.NewReader(data))
	return m.Lat != nil || m.Lng != nil
}

func (c *client) get(path string, headers ...string) (*http.Response, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", c.base+path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

func uploadPhoto(t *testing.T, c *client, name string, data []byte) store.Media {
	t.Helper()
	status, body := c.upload("/api/media", name, data)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("upload: %d %s", status, body)
	}
	var m store.Media
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestMediaFileWithoutGPS(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	photo := gpsJPEG(t)
	if !hasPosition(t, photo) {
		t.Fatal("test photo has no position")
	}
	m := uploadPhoto(t, c, "gps.jpg", photo)

	// The position read on upload stays in the record.
	var rec store.Media
	c.call("GET", fmt.Sprintf("/api/media/%d", m.ID), nil, http.StatusOK, &rec)
	if rec.Lat == nil || rec.Lng == nil || *rec.Lat < 51.3 || *rec.Lat > 51.4 {
		t.Errorf("parsed position lost: %v %v", rec.Lat, rec.Lng)
	}

	for _, q := range []string{"", "?download=1"} {
		resp, body := c.get(fmt.Sprintf("/api/media/%d/file%s", m.ID, q))
		if resp.StatusCode != http.StatusOK || hasPosition(t, body) || len(body) != len(photo) {
			t.Errorf("file%s: %d, %d bytes, position %v", q, resp.StatusCode, len(body), hasPosition(t, body))
		}
	}
	resp, body := c.get(fmt.Sprintf("/api/media/%d/file", m.ID), "Range", "bytes=0-9")
	if resp.StatusCode != http.StatusPartialContent || len(body) != 10 {
		t.Errorf("range: %d, %d bytes", resp.StatusCode, len(body))
	}
	// The thumbnail is re-encoded and carries no metadata.
	resp, body = c.get(fmt.Sprintf("/api/media/%d/thumb?size=%d", m.ID, media.ThumbSizes[0]))
	if resp.StatusCode != http.StatusOK || hasPosition(t, body) || bytes.Contains(body, []byte("Exif")) {
		t.Errorf("thumb: %d, position %v", resp.StatusCode, hasPosition(t, body))
	}
	// The stored original is untouched.
	sum := sha256.Sum256(photo)
	stored, _ := serverOf(ts).Files.Path(hex.EncodeToString(sum[:]))
	if got := mustRead(t, stored); !bytes.Equal(got, photo) {
		t.Error("the original changed")
	}
}

func TestMediaFileKeepLocation(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	serverOf(ts).KeepPhotoLocation = true
	c := loggedIn(t, ts)
	photo := gpsJPEG(t)
	m := uploadPhoto(t, c, "gps.jpg", photo)
	if _, body := c.get(fmt.Sprintf("/api/media/%d/file", m.ID)); !bytes.Equal(body, photo) {
		t.Error("the original was not served")
	}
}

func TestGedzipWithoutGPS(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	photo := gpsJPEG(t)
	uploadPhoto(t, c, "gps.jpg", photo)

	exported := func(query string) []byte {
		t.Helper()
		resp, body := c.get("/api/export/gedcom?version=7.0&format=gedzip" + query)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("export: %d", resp.StatusCode)
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range zr.File {
			if strings.HasPrefix(f.Name, "media/") {
				return readZip(t, f)
			}
		}
		t.Fatal("no media in the archive")
		return nil
	}

	if got := exported(""); hasPosition(t, got) || len(got) != len(photo) {
		t.Errorf("privacy=all: position %v", hasPosition(t, got))
	}
	serverOf(ts).KeepPhotoLocation = true
	if got := exported(""); !bytes.Equal(got, photo) {
		t.Error("keep with privacy=all: original expected")
	}
	for _, privacy := range []string{"exclude-living", "name-only"} {
		if got := exported("&privacy=" + privacy); hasPosition(t, got) {
			t.Errorf("keep with privacy=%s: position left", privacy)
		}
	}
}

func TestPlainGedcomHasNoCoordinates(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	uploadPhoto(t, c, "gps.jpg", gpsJPEG(t))
	_, body := c.get("/api/export/gedcom?version=7.0")
	for _, s := range []string{"51.3", "12.3", "LATI", "LONG", "MAP"} {
		if strings.Contains(string(body), s) {
			t.Errorf("%s in the plain export", s)
		}
	}
}

func TestDeleteRacesIdenticalUpload(t *testing.T) {
	// Not parallel: it sets afterMediaDeleteDecision.
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	photo := jpegBytes(t, 33, 22)
	first := uploadPhoto(t, c, "a.jpg", photo)

	var (
		wg      sync.WaitGroup
		second  store.Media
		upload  = make(chan struct{})
		started sync.Once
	)
	afterMediaDeleteDecision = func() {
		started.Do(func() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				close(upload)
				second = uploadPhoto(t, c, "b.jpg", photo)
			}()
			<-upload
			// Long enough for the upload to reach the lock; without it the
			// upload would commit and record now, then lose its file below.
			time.Sleep(100 * time.Millisecond)
		})
	}
	t.Cleanup(func() { afterMediaDeleteDecision = nil })

	c.call("DELETE", fmt.Sprintf("/api/media/%d", first.ID), nil, http.StatusNoContent, nil)
	wg.Wait()
	if second.ID == 0 {
		t.Fatal("second upload failed")
	}
	resp, body := c.get(fmt.Sprintf("/api/media/%d/file", second.ID))
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, photo) {
		t.Errorf("file of the new upload: %d %s", resp.StatusCode, firstN(body, 80))
	}
}

func firstN(b []byte, n int) []byte { return b[:min(n, len(b))] }

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMediaFileMissingOriginal(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	photo := gpsJPEG(t)
	m := uploadPhoto(t, c, "gps.jpg", photo)
	sum := sha256.Sum256(photo)
	stored, _ := serverOf(ts).Files.Path(hex.EncodeToString(sum[:]))
	if err := os.Remove(stored); err != nil {
		t.Fatal(err)
	}
	if resp, body := c.get(fmt.Sprintf("/api/media/%d/file", m.ID)); resp.StatusCode != http.StatusNotFound || !bytes.Contains(body, []byte(CodeFileMissing)) {
		t.Errorf("missing original: %d %s", resp.StatusCode, firstN(body, 80))
	}
}
