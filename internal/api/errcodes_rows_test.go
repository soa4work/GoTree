package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The transcription apply step reports a row problem as "<column>: <message>"
// (store.rowError). Every message pinned in errcodes_store_test.go must keep
// its code behind that prefix, with the column as a param, so a reworded
// message still breaks a test.
func TestRowMessagesAreClassified(t *testing.T) {
	t.Parallel()
	for m, want := range pinned {
		msg := strings.ReplaceAll(m, "{}", "7")
		got := classifyField("givenNames: " + msg)
		if got.Code != want {
			t.Errorf("row message %q: code %q, want %q", m, got.Code, want)
			continue
		}
		if want != FieldInvalid && got.Params["field"] != "givenNames" {
			t.Errorf("row message %q: params %+v lack the column", m, got.Params)
		}
	}
	if got := classifyField("givenNames: something nobody anticipated"); got.Code != FieldInvalid || got.Params != nil {
		t.Errorf("unknown row message: %+v", got)
	}
}

func TestRowErrorFromApplyIsCoded(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	var src struct {
		ID int64 `json:"id"`
	}
	c.call("POST", "/api/sources", map[string]any{"title": "Kirchenbuch", "author": "", "publication": "", "callNumber": "", "repositoryId": nil, "notes": ""}, http.StatusCreated, &src)
	rows := []map[string]any{
		{"line": "1", "role": "deceased", "values": map[string]string{"given": strings.Repeat("x", 300), "surname": "Weber"}, "action": "", "personId": nil},
	}
	body := map[string]any{"templateKey": "burial", "title": "", "sourceId": src.ID, "page": "", "date": "5 JAN 1900", "placeId": nil, "notes": "", "rows": rows}
	var tr struct {
		ID int64 `json:"id"`
	}
	c.call("POST", "/api/transcriptions", body, http.StatusCreated, &tr)
	e := callError(t, c, "POST", fmt.Sprintf("/api/transcriptions/%d/apply", tr.ID), nil, http.StatusUnprocessableEntity)
	fc, ok := e.FieldCodes["rows.0"]
	if !ok {
		t.Fatalf("no code for rows.0: %+v", e)
	}
	if fc.Code != FieldTooLong || fc.Params["field"] != "givenNames" {
		t.Errorf("rows.0: %+v (%q)", fc, e.Fields["rows.0"])
	}
}

func zipOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		f, _ := zw.Create(n)
		_, _ = f.Write([]byte("0 HEAD\n"))
	}
	zw.Close()
	return buf.Bytes()
}

func TestGedcomErrorsHaveSubCodes(t *testing.T) {
	t.Parallel()
	ts := newServer(t, builtFrontend)
	c := loggedIn(t, ts)
	for _, tc := range []struct {
		name string
		data []byte
		code string
		line any
	}{
		{"junk.ged", []byte("hello world\nnot gedcom\n"), CodeGedcomNotGedcom, float64(1)},
		{"empty.ged", []byte("\n\n"), CodeGedcomEmpty, nil},
		{"none.zip", zipOf(t, "readme.txt"), CodeGedcomZipNoFile, nil},
		{"two.zip", zipOf(t, "a.ged", "b.ged"), CodeGedcomZipSeveral, nil},
		{"broken.zip", []byte("PK\x03\x04 broken"), CodeGedcomZipBad, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := c.upload("/api/import/gedcom", tc.name, tc.data)
			if status != http.StatusUnprocessableEntity {
				t.Fatalf("status %d: %s", status, raw)
			}
			var e codedError
			if err := json.Unmarshal(raw, &e); err != nil {
				t.Fatal(err)
			}
			if e.Code != tc.code || e.Params["line"] != tc.line || e.Error == "" {
				t.Errorf("got %+v, want code %q line %v", e, tc.code, tc.line)
			}
		})
	}
}
