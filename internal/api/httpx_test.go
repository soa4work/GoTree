package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// rawPost sends body as is, which c.do cannot (it always encodes JSON).
func rawPost(t *testing.T, c *client, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", c.base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestInvalidJSONHidesGoTypes(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)

	status, raw := c.do("POST", "/api/persons", map[string]any{"sex": 5})
	if status != http.StatusBadRequest {
		t.Fatalf("status %d: %s", status, raw)
	}
	for _, leak := range []string{"store.", "PersonInput", "Go value", "string"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("response leaks %q: %s", leak, raw)
		}
	}
	e := callError(t, c, "POST", "/api/persons", map[string]any{"sex": 5}, http.StatusBadRequest)
	if e.Code != CodeInvalidJSON || e.Params["detail"] != `wrong type for field "sex"` {
		t.Errorf("wrong type: %+v", e)
	}
	if !strings.HasPrefix(e.Error, "invalid JSON: ") {
		t.Errorf("English message %q", e.Error)
	}

	e = callError(t, c, "POST", "/api/persons", map[string]any{"nope": 1}, http.StatusBadRequest)
	if e.Params["detail"] != `unknown field "nope"` {
		t.Errorf("unknown field: %+v", e)
	}

	for body, want := range map[string]string{"{": "unexpected end of JSON", "": "empty body"} {
		status, raw := rawPost(t, c, "/api/persons", body)
		if status != http.StatusBadRequest || !strings.Contains(raw, want) {
			t.Errorf("body %q: status %d, %s; want detail %q", body, status, raw, want)
		}
	}
}

func TestJSONBodyTooLargeIs413(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := newClient(t, ts)
	body := `{"username":"` + strings.Repeat("a", 2<<20) + `"}`
	status, raw := rawPost(t, c, "/api/auth/login", body)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d: %.200s", status, raw)
	}
	if !strings.Contains(raw, `"code":"request_too_large"`) || !strings.Contains(raw, `"limitKB":1024`) {
		t.Errorf("response: %s", raw)
	}
}

func TestSlowJSONBodyTimesOut(t *testing.T) {
	// Not parallel: it changes bodyReadTimeout.
	old := bodyReadTimeout
	bodyReadTimeout = 300 * time.Millisecond
	t.Cleanup(func() { bodyReadTimeout = old })

	ts := newServer(t, builtFrontend)
	c := newClient(t, ts)
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte(`{"username":"admin",`))
		time.Sleep(time.Second) // stall past the deadline
		_, _ = pw.Write([]byte(`"password":"x"}`))
		pw.Close()
	}()
	req, _ := http.NewRequest("POST", c.base+"/api/auth/login", pr)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestTimeout || !strings.Contains(string(raw), `"code":"request_timeout"`) {
		t.Errorf("status %d: %s, want 408 request_timeout", resp.StatusCode, raw)
	}
}

// The deadline must not stay armed while a long handler runs, or it would
// cancel the request context.
func TestBodyDeadlineIsClearedAfterRead(t *testing.T) {
	// Not parallel: it changes bodyReadTimeout.
	old := bodyReadTimeout
	bodyReadTimeout = 100 * time.Millisecond
	t.Cleanup(func() { bodyReadTimeout = old })

	ts := httptest.NewServer(bodyDeadline(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(400 * time.Millisecond)
		if err := r.Context().Err(); err != nil {
			_, _ = io.WriteString(w, err.Error())
		}
	})))
	defer ts.Close()

	resp, err := http.Post(ts.URL, "text/plain", strings.NewReader("small body"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) != 0 {
		t.Errorf("the request context was cancelled: %s", raw)
	}
}
