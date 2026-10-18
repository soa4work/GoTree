package store

import (
	"bytes"
	"strings"
	"testing"

	"github.com/praetorianer777/gotree/internal/gedcom"
)

func TestExportMediaRoundTrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	p := mustPerson(t, s, a, "Anna", "")
	m, _, err := s.CreateMedia(ctx, a, newMedia(0), &MediaLinkInput{EntityType: "person", EntityID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateMedia(ctx, a, m.ID, MediaUpdate{Title: "Family", Description: "Taken at the farm"}); err != nil {
		t.Fatal(err)
	}
	opt := ExportOptions{Version7: true, WithMedia: true}
	ex, err := s.ExportGEDCOM(ctx, a, opt)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := gedcom.Write(&buf, ex.Records, gedcom.WriteOptions{Version7: true}); err != nil {
		t.Fatal(err)
	}
	doc, err := gedcom.Parse(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	files := ImportFiles{Media: map[string]NewMedia{}}
	for _, f := range ex.Media {
		files.Media[f.Path] = NewMedia{SHA256: f.SHA256, Mime: f.Mime, Kind: "image", Size: 10, OriginalName: "x.jpg", Orientation: 1}
	}
	if got := MediaPaths(doc); len(got) != 1 || got[0] != ex.Media[0].Path {
		t.Fatalf("paths: %v", got)
	}

	s2 := newStore(t)
	a2 := setup(t, s2)
	report, err := s2.ImportGEDCOMFiles(ctx, a2, doc, ImportIntoEmpty, files)
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts["media"] != 1 {
		t.Fatalf("counts: %v", report.Counts)
	}
	list, err := s2.ListMedia(ctx, a2, "", 0, 10, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("media: %+v %v", list, err)
	}
	got, err := s2.GetMedia(ctx, a2, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Family" || got.Description != "Taken at the farm" || len(got.Links) != 1 || got.Links[0].EntityType != "person" {
		t.Errorf("round trip: %+v", got)
	}
	// Exporting again gives the same media objects.
	if out := exportText(t, s2, a2, opt); !strings.Contains(out, "2 TITL Family") || !strings.Contains(out, "1 NOTE Taken at the farm") {
		t.Errorf("second export:\n%s", out)
	}
}

func TestExportDropsDanglingMediaPointer(t *testing.T) {
	t.Parallel()
	// A pointer to a media object, kept under a citation, is only valid in an
	// export that contains the object.
	src := strings.Join([]string{
		"0 HEAD", "1 GEDC", "2 VERS 5.5.1", "1 CHAR UTF-8",
		"0 @I1@ INDI", "1 NAME Anna /Weber/", "1 SOUR @S1@", "2 OBJE @O1@",
		"0 @S1@ SOUR", "1 TITL Book",
		"0 @O1@ OBJE", "1 FILE a.png", "2 FORM png",
		"0 TRLR",
	}, "\n")
	doc, err := gedcom.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	a := setup(t, s)
	files := ImportFiles{Media: map[string]NewMedia{"a.png": pngMedia("a", "a.png")}}
	if _, err := s.ImportGEDCOMFiles(ctx, a, doc, ImportIntoEmpty, files); err != nil {
		t.Fatal(err)
	}
	if out := exportText(t, s, a, ExportOptions{}); strings.Contains(out, "OBJE") {
		t.Errorf("a plain export points to a media object:\n%s", out)
	}
	out := exportText(t, s, a, ExportOptions{WithMedia: true})
	if !strings.Contains(out, "2 OBJE @O") || !strings.Contains(out, "0 @O") {
		t.Errorf("the export with media lost the pointer:\n%s", out)
	}
}
