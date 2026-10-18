package store

import (
	"errors"
	"strings"
	"testing"
)

func newMedia(sha byte) NewMedia {
	w, h := 400, 300
	return NewMedia{
		SHA256: strings.Repeat(string(rune('a'+sha%6)), 64), Mime: "image/jpeg", Kind: "image", Size: 1234,
		OriginalName: "Familie 1950.jpg", Width: &w, Height: &h, Orientation: 1,
		Regions: []RegionInput{{Name: "Anna Müller", X: 0.1, Y: 0.1, W: 0.2, H: 0.2}, {Name: "bad", X: 0.9, Y: 0.9, W: 0.5, H: 0.5}},
	}
}

func TestMedia(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := setup(t, s)
	anna := mustPerson(t, s, a, "Anna", "Müller")

	m, created, err := s.CreateMedia(ctx, a, newMedia(0), &MediaLinkInput{EntityType: "person", EntityID: anna.ID})
	if err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	if m.Title != "Familie 1950" || len(m.Links) != 1 || m.Links[0].Label != "Anna Müller" {
		t.Errorf("media: %+v", m)
	}
	// Imported face tags: the valid one is kept, unmatched to a person.
	if len(m.Regions) != 1 || m.Regions[0].Name != "Anna Müller" || m.Regions[0].Person != nil || m.Regions[0].Source != "xmp" {
		t.Fatalf("regions: %+v", m.Regions)
	}

	// The same file again is the same record, and regions are not duplicated.
	again, created, err := s.CreateMedia(ctx, a, newMedia(0), nil)
	if err != nil || created || again.ID != m.ID || len(again.Regions) != 1 {
		t.Errorf("dedupe: %+v %v %v", again, created, err)
	}
	// Another tree keeps its own record of the same file.
	b := otherTree(t, s)
	other, created, err := s.CreateMedia(ctx, b, newMedia(0), nil)
	if err != nil || !created || other.ID == m.ID {
		t.Errorf("other tree: %+v %v %v", other, created, err)
	}
	if _, err := s.GetMedia(ctx, b, m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("media across trees: %v", err)
	}

	_, _, err = s.CreateMedia(ctx, a, newMedia(1), &MediaLinkInput{EntityType: "person", EntityID: 999})
	validationField(t, err, "entityId")
	_, _, err = s.CreateMedia(ctx, a, newMedia(1), &MediaLinkInput{EntityType: "planet", EntityID: 1})
	validationField(t, err, "entityType")

	// Match the imported face to Anna, and add a second face for Hans,
	// which puts the photo into Hans' gallery.
	hans := mustPerson(t, s, a, "Hans", "Müller")
	rid := m.Regions[0].ID
	m, err = s.UpdateRegion(ctx, a, rid, RegionInput{PersonID: &anna.ID, Name: "Anna Müller", X: 0.1, Y: 0.1, W: 0.2, H: 0.2})
	if err != nil || m.Regions[0].Person == nil || m.Regions[0].Person.ID != anna.ID {
		t.Fatalf("match region: %+v %v", m.Regions, err)
	}
	m, err = s.AddRegion(ctx, a, m.ID, RegionInput{PersonID: &hans.ID, X: 0.5, Y: 0.2, W: 0.2, H: 0.3})
	if err != nil || len(m.Regions) != 2 {
		t.Fatalf("add region: %+v %v", m.Regions, err)
	}
	gallery, err := s.ListMedia(ctx, a, "person", hans.ID, 10, 0)
	if err != nil || len(gallery) != 1 {
		t.Errorf("Hans' gallery: %+v %v", gallery, err)
	}
	_, err = s.AddRegion(ctx, a, m.ID, RegionInput{X: 0.9, Y: 0, W: 0.5, H: 0.1})
	validationField(t, err, "x")

	// Portrait: the face tag on the photo.
	p, err := s.SetPortrait(ctx, a, anna.ID, PortraitInput{MediaID: &m.ID, RegionID: &rid})
	if err != nil || p.Portrait == nil || p.Portrait.MediaID != m.ID || *p.Portrait.RegionID != rid {
		t.Fatalf("portrait: %+v %v", p.Portrait, err)
	}
	list, err := s.ListPersons(ctx, a, "anna", 10, 0)
	if err != nil || list.Items[0].Portrait == nil {
		t.Errorf("portrait in lists: %+v %v", list, err)
	}
	_, err = s.SetPortrait(ctx, a, anna.ID, PortraitInput{MediaID: &m.ID, RegionID: ptr(int64(9999))})
	validationField(t, err, "regionId")

	doc, _, err := s.CreateMedia(ctx, a, NewMedia{SHA256: strings.Repeat("d", 64), Mime: "application/pdf", Kind: "document", Size: 10}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetPortrait(ctx, a, anna.ID, PortraitInput{MediaID: &doc.ID})
	validationField(t, err, "mediaId")
	_, err = s.AddRegion(ctx, a, doc.ID, RegionInput{X: 0, Y: 0, W: 1, H: 1})
	validationField(t, err, "mediaId")

	// Deleting the region clears the portrait crop but keeps the photo.
	if _, err := s.DeleteRegion(ctx, a, rid); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetPerson(ctx, a, anna.ID)
	if p.Portrait == nil || p.Portrait.RegionID != nil {
		t.Errorf("portrait after region delete: %+v", p.Portrait)
	}

	// Delete: the other tree still uses the file, so it is not orphaned.
	sha, orphaned, err := s.DeleteMedia(ctx, a, m.ID)
	if err != nil || orphaned || sha != m.SHA256 {
		t.Errorf("delete shared: %q %v %v", sha, orphaned, err)
	}
	p, _ = s.GetPerson(ctx, a, anna.ID)
	if p.Portrait != nil {
		t.Errorf("portrait must go with the photo: %+v", p.Portrait)
	}
	if _, orphaned, err = s.DeleteMedia(ctx, b, other.ID); err != nil || !orphaned {
		t.Errorf("delete last use: %v %v", orphaned, err)
	}

	// Links go when the record goes.
	ev, err := s.CreateEvent(ctx, a, EventInput{PersonID: &hans.ID, Type: "BIRT"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkMedia(ctx, a, doc.ID, MediaLinkInput{EntityType: "event", EntityID: ev.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePerson(ctx, a, hans.ID); err != nil {
		t.Fatal(err)
	}
	var links int
	if err := s.DB.QueryRow(`SELECT count(*) FROM media_links`).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Errorf("%d media links survived their targets", links)
	}
}
