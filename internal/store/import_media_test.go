package store

import (
	"reflect"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/gedcom"
)

func parseLines(t *testing.T, lines ...string) *gedcom.Document {
	t.Helper()
	doc, err := gedcom.Parse([]byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func pngMedia(sha, name string) NewMedia {
	return NewMedia{SHA256: strings.Repeat(sha, 64), Mime: "image/png", Kind: "image", Size: 10, OriginalName: name, Orientation: 1}
}

func countRows(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.DB.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const mediaDoc = `0 HEAD
1 GEDC
2 VERS 5.5.1
1 CHAR UTF-8
0 @I1@ INDI
1 NAME Anna /Weber/
1 OBJE @O1@
2 _PRIM Y
1 BIRT
2 DATE 1900
2 OBJE @O1@
1 OBJE
2 FILE media/b.png
3 FORM png
0 @F1@ FAM
1 HUSB @I1@
1 OBJE @O1@
0 @S1@ SOUR
1 TITL Church book
1 OBJE @O1@
0 @H1@ _HEIRLOOM
1 NAME Clock
1 OBJE @O1@
0 @O1@ OBJE
1 FILE media/a.png
2 FORM png
2 TITL Wedding
1 NOTE desc
1 REFN 7
0 @O2@ OBJE
1 FILE missing.jpg
0 TRLR`

func TestImportMediaRecordsAndLinks(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	doc := parseLines(t, strings.Split(mediaDoc, "\n")...)
	files := ImportFiles{
		Media:   map[string]NewMedia{"media/a.png": pngMedia("a", "a.png"), "media/b.png": pngMedia("b", "b.png")},
		Skipped: map[string]string{"missing.jpg": "the file is not in the zip"},
	}
	report, err := s.ImportGEDCOMFiles(ctx, a, doc, ImportIntoEmpty, files)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts["media"] != 2 || countRows(t, s, `SELECT count(*) FROM media`) != 2 {
		t.Fatalf("counts: %v", report.Counts)
	}
	var title, desc, extra, xref string
	if err := s.DB.QueryRow(`SELECT title, description, extra_json, gedcom_xref FROM media WHERE sha256 = ?`, strings.Repeat("a", 64)).
		Scan(&title, &desc, &extra, &xref); err != nil {
		t.Fatal(err)
	}
	if title != "Wedding" || desc != "desc" || xref != "O1" || !strings.Contains(extra, "REFN") {
		t.Errorf("medium: %q %q %q %q", title, desc, extra, xref)
	}
	var bTitle string
	_ = s.DB.QueryRow(`SELECT title FROM media WHERE sha256 = ?`, strings.Repeat("b", 64)).Scan(&bTitle)
	if bTitle != "b" {
		t.Errorf("title of the inline medium: %q", bTitle)
	}
	for _, typ := range []string{"person", "event", "family", "source", "heirloom"} {
		want := 1
		if typ == "person" {
			want = 2 // the pointer and the inline object
		}
		if n := countRows(t, s, `SELECT count(*) FROM media_links WHERE entity_type = ?`, typ); n != want {
			t.Errorf("%s links: %d, want %d", typ, n, want)
		}
	}
	var portrait int64
	if err := s.DB.QueryRow(`SELECT p.portrait_media_id FROM persons p JOIN media m ON m.id = p.portrait_media_id WHERE m.sha256 = ?`,
		strings.Repeat("a", 64)).Scan(&portrait); err != nil {
		t.Errorf("portrait: %v", err)
	}
	if got := tagOutcome(report, "OBJE"); got != TagDropped && got != TagMapped {
		t.Errorf("OBJE: %q", got)
	}
	dropped := false
	for _, ts := range report.Tags {
		if ts.Path == "OBJE" && ts.Outcome == TagDropped && ts.Reason == "the file is not in the zip" {
			dropped = true
		}
		if strings.HasSuffix(ts.Path, ".OBJE") && ts.Outcome == TagKept {
			t.Errorf("kept OBJE: %+v", ts)
		}
	}
	if !dropped {
		t.Errorf("O2 is not reported as dropped: %+v", report.Tags)
	}
	var personExtra string
	_ = s.DB.QueryRow(`SELECT extra_json FROM persons`).Scan(&personExtra)
	if strings.Contains(personExtra, "OBJE") {
		t.Errorf("person keeps OBJE: %s", personExtra)
	}
}

func TestImportMediaPlainGed(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	doc := parseLines(t, strings.Split(mediaDoc, "\n")...)
	report, err := s.ImportGEDCOM(ctx, a, doc, ImportIntoEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts["media"] != 0 || countRows(t, s, `SELECT count(*) FROM media`) != 0 {
		t.Errorf("media were created: %v", report.Counts)
	}
	reason := ""
	for _, ts := range report.Tags {
		if ts.Path == "OBJE" && ts.Outcome == TagDropped {
			reason = ts.Reason
		}
	}
	if !strings.Contains(reason, "GEDZIP") {
		t.Errorf("reason: %q", reason)
	}
	// Without files the media objects stay where they were, written out in
	// place of the pointers, which would lead nowhere.
	if n := countRows(t, s, `SELECT count(*) FROM persons WHERE extra_json LIKE '%"p":"O%'`); n != 0 {
		t.Error("an OBJE pointer was kept")
	}
	out := exportText(t, s, a, ExportOptions{Version7: true, WithMedia: true})
	if strings.Contains(out, "OBJE @") || !strings.Contains(out, "1 OBJE\n2 FILE media/a.png") || !strings.Contains(out, "3 TITL Wedding") {
		t.Errorf("export:\n%s", out)
	}
}

// A web address is not imported, in a plain file or in a zip, but the line
// goes back out as it came in.
func TestImportMediaWebFileRoundTrip(t *testing.T) {
	doc := func() *gedcom.Document {
		return parseLines(t, "0 HEAD", "1 GEDC", "2 VERS 5.5.1", "1 CHAR UTF-8",
			"0 @I1@ INDI", "1 NAME Anna /Weber/", "1 OBJE @O1@", "1 OBJE", "2 FILE https://example.com/a.jpg",
			"1 OBJE", "2 FILE media/z.png",
			"0 @O1@ OBJE", "1 FILE http://example.com/b.jpg", "2 FORM jpg",
			"0 TRLR")
	}
	for name, files := range map[string]ImportFiles{
		"plain": {},
		"zip": {
			Media:   map[string]NewMedia{"media/z.png": pngMedia("z", "z.png")},
			Skipped: map[string]string{"https://example.com/a.jpg": webFileReason, "http://example.com/b.jpg": webFileReason},
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			a := setup(t, s)
			report, err := s.ImportGEDCOMFiles(ctx, a, doc(), ImportIntoEmpty, files)
			if err != nil {
				t.Fatal(err)
			}
			keptLines := 0
			for _, ts := range report.Tags {
				if ts.Path == "INDI.OBJE" && ts.Outcome == TagKept {
					keptLines = ts.Count
				}
			}
			if want := map[bool]int{true: 3, false: 2}[name == "plain"]; keptLines != want {
				t.Errorf("kept OBJE lines: %d, tags %+v", keptLines, report.Tags)
			}
			for _, ts := range report.Tags {
				if ts.Outcome == TagDropped && ts.Reason == noFilesInUpload && strings.Contains(ts.Path, "FILE") && name == "zip" {
					t.Errorf("a web address is reported as missing: %+v", ts)
				}
				if name == "plain" && ts.Path == "OBJE" && ts.Outcome == TagDropped && ts.Reason != webFileReason {
					t.Errorf("reason for a web file: %q", ts.Reason)
				}
			}
			out := exportText(t, s, a, ExportOptions{WithMedia: true})
			for _, want := range []string{"2 FILE https://example.com/a.jpg", "2 FILE http://example.com/b.jpg"} {
				if !strings.Contains(out, want) {
					t.Errorf("%q lost:\n%s", want, out)
				}
			}
			if strings.Contains(out, "OBJE @") && name == "plain" {
				t.Errorf("a pointer to nothing:\n%s", out)
			}
			wantMedia := 0
			if name == "zip" {
				wantMedia = 1
			}
			if n := countRows(t, s, `SELECT count(*) FROM media`); n != wantMedia {
				t.Errorf("media rows: %d", n)
			}
		})
	}
}

func TestImportMediaReplaceReusesFile(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	files := ImportFiles{Media: map[string]NewMedia{"media/a.png": pngMedia("a", "a.png"), "media/b.png": pngMedia("b", "b.png")}}
	for i := 0; i < 2; i++ {
		mode := ImportIntoEmpty
		if i == 1 {
			mode = ImportReplace
		}
		if _, err := s.ImportGEDCOMFiles(ctx, a, parseLines(t, strings.Split(mediaDoc, "\n")...), mode, files); err != nil {
			t.Fatal(err)
		}
	}
	if n := countRows(t, s, `SELECT count(*) FROM media`); n != 2 {
		t.Errorf("media rows: %d", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM media_links WHERE entity_type = 'person'`); n != 2 {
		t.Errorf("person links: %d", n)
	}
	if n := countRows(t, s, `SELECT count(*) FROM media_links`); n != 6 {
		t.Errorf("links: %d", n)
	}
}

func TestMediaPaths(t *testing.T) {
	doc := parseLines(t,
		"0 HEAD", "1 GEDC", "2 VERS 5.5.1", "1 CHAR UTF-8",
		"0 @I1@ INDI", "1 NAME A /B/", "1 OBJE", "2 FILE x/one.jpg",
		"1 BIRT", "2 OBJE", "3 FILE two.jpg", "3 FORM jpg", "2 OBJE @O1@",
		"1 SOUR @S1@", "2 OBJE", "3 FILE under-citation.jpg",
		"0 @O1@ OBJE", "1 FILE one.jpg", "1 FILE x/one.jpg",
		"0 TRLR")
	got := MediaPaths(doc)
	want := []string{"one.jpg", "x/one.jpg", "two.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths: %v, want %v", got, want)
	}
}

func TestCheckImportTarget(t *testing.T) {
	s := newStore(t)
	a := setup(t, s)
	if err := s.CheckImportTarget(ctx, a, ImportIntoEmpty); err != nil {
		t.Fatal(err)
	}
	mustPerson(t, s, a, "Anna", "")
	if err := s.CheckImportTarget(ctx, a, ImportIntoEmpty); err != ErrTreeNotEmpty {
		t.Errorf("non-empty tree: %v", err)
	}
	if err := s.CheckImportTarget(ctx, a, ImportReplace); err != nil {
		t.Errorf("replace: %v", err)
	}
	if err := s.CheckImportTarget(ctx, a, "bogus"); err == nil {
		t.Error("an unknown mode is accepted")
	}
}
