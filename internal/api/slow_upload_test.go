package api

import (
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"
)

// A client that stalls mid-body is cut off by the read deadline and gets 408.
func TestSlowUploadTimesOut(t *testing.T) {
	// Not parallel: it changes uploadGrace and minUploadRate.
	oldGrace, oldRate := uploadGrace, minUploadRate
	uploadGrace, minUploadRate = 300*time.Millisecond, 1<<40
	t.Cleanup(func() { uploadGrace, minUploadRate = oldGrace, oldRate })

	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		fw, _ := mw.CreateFormFile("file", "slow.jpg")
		_, _ = fw.Write(jpegBytes(t, 20, 20)[:100])
		time.Sleep(time.Second) // stall past the deadline
		_, _ = fw.Write([]byte("more"))
		mw.Close()
		pw.Close()
	}()
	req, _ := http.NewRequest("POST", c.base+"/api/media", pr)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestTimeout {
		t.Errorf("status = %d, want 408", resp.StatusCode)
	}
}
